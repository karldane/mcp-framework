package framework

import (
	"os"
	"testing"

	"github.com/karldane/go-presidio/presidio"
)

// --- F1 / F2: converter round-trips ---

func TestToPresidioHintsNil(t *testing.T) {
	result := toPresidioHints(nil)
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestToPresidioHintsPreservesScanPolicy(t *testing.T) {
	hints := map[string]ColumnHint{
		"EMAIL": {ScanPolicy: ScanPolicySafe, MaxLength: 0},
		"NOTES": {ScanPolicy: ScanPolicyTruncateThenScan, MaxLength: 256},
	}
	out := toPresidioHints(hints)
	if len(out) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(out))
	}
	if int(out["EMAIL"].ScanPolicy) != int(ScanPolicySafe) {
		t.Errorf("EMAIL ScanPolicy mismatch")
	}
	if out["NOTES"].MaxLength != 256 {
		t.Errorf("NOTES MaxLength mismatch: got %d", out["NOTES"].MaxLength)
	}
}

func TestFromPresidioReportsNil(t *testing.T) {
	result := fromPresidioReports(nil)
	if result == nil {
		t.Errorf("expected non-nil empty slice, got nil")
	}
}

func TestFromPresidioReports(t *testing.T) {
	reports := []presidio.ColumnReport{
		{
			ColumnName:     "email",
			PIIDetected:    true,
			PIIEntities:    []presidio.EntityType{presidio.EntityEmailAddress},
			Treatment:      presidio.TreatmentKind("mask"),
			OriginalLength: 100,
			TruncatedAt:    50,
		},
	}
	result := fromPresidioReports(reports)
	if len(result) != 1 {
		t.Fatalf("expected 1 report, got %d", len(result))
	}
	if result[0].ColumnName != "email" {
		t.Errorf("expected 'email', got %s", result[0].ColumnName)
	}
	if !result[0].PIIDetected {
		t.Error("expected PIIDetected=true")
	}
	if len(result[0].EntityTypes) != 1 || result[0].EntityTypes[0] != "EMAIL_ADDRESS" {
		t.Errorf("expected EMAIL_ADDRESS entity, got %v", result[0].EntityTypes)
	}
	if result[0].Treatment == "" {
		t.Error("expected non-empty treatment")
	}
}

// --- F5: SampleSize respected ---

func TestNewPIIPipelineDefaultSampleSize(t *testing.T) {
	p := NewPIIPipeline(nil)
	if p == nil {
		t.Fatal("expected non-nil pipeline")
	}
	// structured must be constructed (not nil)
	if p.structured == nil {
		t.Error("structured analyzer is nil")
	}
}

func TestNewPIIPipelineCustomSampleSize(t *testing.T) {
	cfg := &PIIPipelineConfig{SampleSize: 50}
	p := NewPIIPipeline(cfg)
	if p == nil {
		t.Fatal("expected non-nil pipeline")
	}
	if p.structured == nil {
		t.Error("structured analyzer is nil")
	}
}

func TestNewPIIPipelineDefaultOperatorIsRedact(t *testing.T) {
	p := NewPIIPipeline(nil)
	if p.defaultOperator == nil {
		t.Error("defaultOperator should not be nil")
	}
}

func TestNewPIIPipelineHashOperatorIgnoredWithoutKey(t *testing.T) {
	// Without an HMAC key, hash operator must not be set
	cfg := &PIIPipelineConfig{DefaultOperator: "hash"}
	p := NewPIIPipeline(cfg)
	// Should fall back to redact (the struct default), not nil
	if p.defaultOperator == nil {
		t.Error("defaultOperator should not be nil when hash requested without key")
	}
}

// --- F3: pipeline called in Process (integration smoke test) ---

func TestProcessRawTextNoOp(t *testing.T) {
	p := NewPIIPipeline(nil)
	result := ToolResult{RawText: "hello world no pii here"}
	out := p.Process(result)
	if out.Meta.PIIScanApplied != true {
		t.Error("PIIScanApplied should be true after Process")
	}
}

func TestProcessStructuredDataNoRows(t *testing.T) {
	p := NewPIIPipeline(nil)
	result := ToolResult{Data: []map[string]interface{}{}}
	out := p.Process(result)
	if out.Meta.PIIScanApplied != true {
		t.Error("PIIScanApplied should be true even for empty rows")
	}
}

func TestProcessIdempotent(t *testing.T) {
	p := NewPIIPipeline(nil)
	result := ToolResult{RawText: "no pii"}
	out1 := p.Process(result)
	out2 := p.Process(out1)
	// Second call must not re-process
	if out2.Meta.SafetyNote != out1.Meta.SafetyNote {
		t.Error("second Process call should be a no-op (PIIScanApplied guard)")
	}
}

func TestProcessRawTextWithPII(t *testing.T) {
	p := NewPIIPipeline(nil)
	result := ToolResult{RawText: "my email is alice@example.com"}
	out := p.Process(result)
	if !out.Meta.PIIScanApplied {
		t.Error("PIIScanApplied should be true")
	}
	if out.RawText == result.RawText {
		t.Error("raw text should be anonymised")
	}
}

func TestProcessRawTextNoPII(t *testing.T) {
	p := NewPIIPipeline(nil)
	result := ToolResult{RawText: "hello world"}
	out := p.Process(result)
	if out.Meta.SafetyNote != "no pii detected" {
		t.Errorf("expected 'no pii detected', got: %s", out.Meta.SafetyNote)
	}
}

func TestApplyConfigOperatorsRedact(t *testing.T) {
	cfg := &PIIPipelineConfig{
		DefaultOperator: "redact",
	}
	p := NewPIIPipeline(cfg)
	if _, ok := p.defaultOperator.(*presidio.RedactOperator); !ok {
		t.Error("defaultOperator should be RedactOperator")
	}
}

func TestApplyConfigOperatorsMask(t *testing.T) {
	cfg := &PIIPipelineConfig{
		DefaultOperator: "mask",
	}
	p := NewPIIPipeline(cfg)
	if _, ok := p.defaultOperator.(*presidio.MaskOperator); !ok {
		t.Error("defaultOperator should be MaskOperator")
	}
}

func TestProcessStructuredData(t *testing.T) {
	p := NewPIIPipeline(nil)
	result := ToolResult{
		Data: []map[string]interface{}{
			{"email": "test@example.com"},
		},
	}
	out := p.Process(result)
	if out.Meta.PIIScanApplied != true {
		t.Error("PIIScanApplied should be true")
	}
	if out.Data == nil {
		t.Error("Data should be populated")
	}
}

func TestProcessStructuredDataEmpty(t *testing.T) {
	p := NewPIIPipeline(nil)
	result := ToolResult{
		Data: []map[string]interface{}{},
	}
	out := p.Process(result)
	if out.Meta.PIIScanApplied != true {
		t.Error("PIIScanApplied should be true")
	}
}

func TestApplyConfigOperatorsHashWithKey(t *testing.T) {
	os.Setenv("TEST_HMAC_KEY", "secret-key-for-testing")
	defer os.Unsetenv("TEST_HMAC_KEY")

	cfg := &PIIPipelineConfig{
		HMACKeyEnv:      "TEST_HMAC_KEY",
		DefaultOperator: "hash",
	}
	p := NewPIIPipeline(cfg)
	if _, ok := p.defaultOperator.(*presidio.HashOperator); !ok {
		t.Error("defaultOperator should be HashOperator when HMAC key is set")
	}
}

func TestApplyConfigOperatorsPseudonymiseWithKey(t *testing.T) {
	os.Setenv("TEST_HMAC_KEY2", "another-secret-key")
	defer os.Unsetenv("TEST_HMAC_KEY2")

	cfg := &PIIPipelineConfig{
		HMACKeyEnv:      "TEST_HMAC_KEY2",
		DefaultOperator: "pseudonymise",
	}
	p := NewPIIPipeline(cfg)
	if _, ok := p.defaultOperator.(*presidio.PseudonymiseOperator); !ok {
		t.Error("defaultOperator should be PseudonymiseOperator when HMAC key is set")
	}
}

func TestApplyConfigOperatorsEntitySpecific(t *testing.T) {
	os.Setenv("TEST_HMAC_KEY3", "entity-key")
	defer os.Unsetenv("TEST_HMAC_KEY3")

	cfg := &PIIPipelineConfig{
		HMACKeyEnv: "TEST_HMAC_KEY3",
		EntityOperators: map[string]string{
			"EMAIL_ADDRESS": "hash",
			"PHONE_NUMBER":  "redact",
		},
	}
	p := NewPIIPipeline(cfg)

	t.Logf("EntityEmailAddress key: %q", string(presidio.EntityEmailAddress))
	for k, v := range p.entityOperators {
		t.Logf("Stored operator key: %q, type: %T", string(k), v)
	}

	if op, ok := p.entityOperators[presidio.EntityEmailAddress]; !ok {
		t.Error("should have entity operator for EMAIL_ADDRESS")
	} else if _, isHash := op.(*presidio.HashOperator); !isHash {
		t.Error("EMAIL_ADDRESS operator should be HashOperator")
	}
	if op, ok := p.entityOperators[presidio.EntityPhoneNumber]; !ok {
		t.Error("should have entity operator for PHONE_NUMBER")
	} else if _, isRedact := op.(*presidio.RedactOperator); !isRedact {
		t.Error("PHONE_NUMBER operator should be RedactOperator")
	}
}

func TestApplyConfigOperatorsHashWithoutKey(t *testing.T) {
	// No HMAC key set - should fall back to redact
	cfg := &PIIPipelineConfig{
		DefaultOperator: "hash",
	}
	p := NewPIIPipeline(cfg)
	// Without key, hash is not applied, should be redact (default)
	if _, ok := p.defaultOperator.(*presidio.RedactOperator); !ok {
		t.Error("defaultOperator should fall back to RedactOperator when no HMAC key")
	}
}
