package framework

import (
	"os"
	"strings"
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
	os.Setenv("TEST_HMAC_KEY2", "abcdefghijklmnopqrstuvwxyz012345")
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

// --- Resolve method tests ---

func makeTestKey(t *testing.T, length int) []byte {
	// Use printable ASCII characters to form a valid key
	key := make([]byte, length)
	for i := range key {
		key[i] = byte((i % 26) + int('a')) // a-z循环
	}
	return key
}

func TestResolveDecryptsTokens(t *testing.T) {
	key := makeTestKey(t, 32)
	os.Setenv("TEST_PII_KEY", string(key))
	defer os.Unsetenv("TEST_PII_KEY")

	cfg := &PIIPipelineConfig{
		HMACKeyEnv:      "TEST_PII_KEY",
		DefaultOperator: "pseudonymise",
	}
	p := NewPIIPipeline(cfg)

	// First encrypt something via Process
	// Note: current Process implementation encrypts entire text when PII detected
	result := p.Process(ToolResult{RawText: "contact alice@example.com"})
	t.Logf("Process result: %q", result.RawText)

	// Extract the token from the result
	token := ""
	if idx := strings.Index(result.RawText, "pii:"); idx >= 0 {
		token = result.RawText[idx:]
	}
	t.Logf("Extracted token: %q", token)
	if token == "" {
		t.Fatal("could not find pii token in result")
	}

	// Resolve should decrypt it
	args := map[string]interface{}{"email": token}
	resolved, err := p.Resolve(args)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	t.Logf("Resolved args: %v", resolved)
	// Process encrypts the entire text when PII is detected, so we get full original
	if resolved["email"] != "contact alice@example.com" {
		t.Errorf("expected 'contact alice@example.com', got: %v", resolved["email"])
	}
}

func TestResolvePassesThroughNonTokenStrings(t *testing.T) {
	key := makeTestKey(t, 32)
	os.Setenv("TEST_PII_KEY2", string(key))
	defer os.Unsetenv("TEST_PII_KEY2")

	cfg := &PIIPipelineConfig{
		HMACKeyEnv:      "TEST_PII_KEY2",
		DefaultOperator: "pseudonymise",
	}
	p := NewPIIPipeline(cfg)

	args := map[string]interface{}{
		"name": "plain text",
		"code": "12345",
	}
	resolved, err := p.Resolve(args)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if resolved["name"] != "plain text" {
		t.Errorf("name should be unchanged, got: %v", resolved["name"])
	}
	if resolved["code"] != "12345" {
		t.Errorf("code should be unchanged, got: %v", resolved["code"])
	}
}

func TestResolvePassesThroughNonStringValues(t *testing.T) {
	key := makeTestKey(t, 32)
	os.Setenv("TEST_PII_KEY3", string(key))
	defer os.Unsetenv("TEST_PII_KEY3")

	cfg := &PIIPipelineConfig{
		HMACKeyEnv:      "TEST_PII_KEY3",
		DefaultOperator: "pseudonymise",
	}
	p := NewPIIPipeline(cfg)

	args := map[string]interface{}{
		"count":  42,
		"active": true,
		"null":   nil,
		"rate":   3.14,
	}
	resolved, err := p.Resolve(args)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if resolved["count"] != 42 {
		t.Errorf("count should be unchanged")
	}
	if resolved["active"] != true {
		t.Errorf("active should be unchanged")
	}
	if resolved["null"] != nil {
		t.Errorf("null should be unchanged")
	}
	if resolved["rate"] != 3.14 {
		t.Errorf("rate should be unchanged")
	}
}

func TestResolveNonPseudonymisePipeline(t *testing.T) {
	cfg := &PIIPipelineConfig{
		DefaultOperator: "redact",
	}
	p := NewPIIPipeline(cfg)

	args := map[string]interface{}{
		"email": "pii:deadbeef123456",
	}
	resolved, err := p.Resolve(args)
	if err != nil {
		t.Fatalf("Resolve should not fail for non-pseudonymise pipeline: %v", err)
	}
	// Should pass through unchanged
	if resolved["email"] != "pii:deadbeef123456" {
		t.Errorf("non-pseudonymise pipeline should pass through unchanged")
	}
}

func TestResolveAuthFailureRejectsCall(t *testing.T) {
	key := makeTestKey(t, 32)
	os.Setenv("TEST_PII_KEY4", string(key))
	defer os.Unsetenv("TEST_PII_KEY4")

	cfg := &PIIPipelineConfig{
		HMACKeyEnv:      "TEST_PII_KEY4",
		DefaultOperator: "pseudonymise",
	}
	p := NewPIIPipeline(cfg)

	// Tampered token
	args := map[string]interface{}{
		"email": "pii:deadbeef1234567890abcdefTampered",
	}
	_, err := p.Resolve(args)
	if err == nil {
		t.Error("Resolve should fail for tampered token")
	}
}

func TestResolveNilPipeline(t *testing.T) {
	// Test via server tests - see TestServerWithNilPIIPipeline in server_test.go
	// This validates that dispatchTool with nil pipeline passes args through unchanged
}

func TestRoundTrip(t *testing.T) {
	key := makeTestKey(t, 32)
	os.Setenv("TEST_PII_KEY5", string(key))
	defer os.Unsetenv("TEST_PII_KEY5")

	cfg := &PIIPipelineConfig{
		HMACKeyEnv:      "TEST_PII_KEY5",
		DefaultOperator: "pseudonymise",
	}
	p := NewPIIPipeline(cfg)

	// Encrypt via Process - use text with recognizable PII
	result := p.Process(ToolResult{RawText: "email is test@example.com"})
	t.Logf("Process result: %q", result.RawText)

	// Find token in result
	token := ""
	if idx := strings.Index(result.RawText, "pii:"); idx >= 0 {
		token = result.RawText[idx:]
	}
	t.Logf("Token: %q", token)
	if token == "" {
		t.Fatal("could not find pii token")
	}

	// Decrypt via Resolve
	resolved, err := p.Resolve(map[string]interface{}{"data": token})
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	t.Logf("Resolved: %v", resolved)
	// Current Process encrypts entire text when PII detected
	if resolved["data"] != "email is test@example.com" {
		t.Errorf("expected 'email is test@example.com', got: %v", resolved["data"])
	}
}

func TestCrossUserTokenRejected(t *testing.T) {
	// Use two different printable keys
	keyA := []byte("abcdefghijklmnopqrstuvwxyz012345") // 32 bytes
	keyB := []byte("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdef") // different 32 bytes

	// Pipeline with keyA
	os.Setenv("TEST_PII_KEY_A", string(keyA))
	defer os.Unsetenv("TEST_PII_KEY_A")
	cfgA := &PIIPipelineConfig{
		HMACKeyEnv:      "TEST_PII_KEY_A",
		DefaultOperator: "pseudonymise",
	}
	pA := NewPIIPipeline(cfgA)

	// Pipeline with keyB
	os.Setenv("TEST_PII_KEY_B", string(keyB))
	defer os.Unsetenv("TEST_PII_KEY_B")
	cfgB := &PIIPipelineConfig{
		HMACKeyEnv:      "TEST_PII_KEY_B",
		DefaultOperator: "pseudonymise",
	}
	pB := NewPIIPipeline(cfgB)

	// Encrypt with pA (use text with PII for detection)
	result := pA.Process(ToolResult{RawText: "email is test@example.com"})
	token := ""
	if idx := strings.Index(result.RawText, "pii:"); idx >= 0 {
		token = result.RawText[idx:]
	}
	if token == "" {
		t.Fatal("could not find pii token")
	}

	// Try to decrypt with pB - should fail
	_, err := pB.Resolve(map[string]interface{}{"data": token})
	if err == nil {
		t.Error("cross-user token should be rejected")
	}
}

func TestConstructionWithKey(t *testing.T) {
	// Use printable ASCII characters for the key
	keyBytes := []byte("abcdefghijklmnopqrstuvwxyz012345")
	t.Logf("Key length: %d", len(keyBytes))
	os.Setenv("TEST_PII_KEY6", string(keyBytes))
	defer os.Unsetenv("TEST_PII_KEY6")

	cfg := &PIIPipelineConfig{
		HMACKeyEnv:      "TEST_PII_KEY6",
		DefaultOperator: "pseudonymise",
	}
	p := NewPIIPipeline(cfg)

	t.Logf("defaultOperator type: %T", p.defaultOperator)

	// Verify the operator has the key
	op, ok := p.defaultOperator.(*presidio.PseudonymiseOperator)
	if !ok {
		t.Fatalf("defaultOperator should be PseudonymiseOperator, got: %T", p.defaultOperator)
	}
	if op.Key == nil {
		t.Error("PseudonymiseOperator should have Key set")
	}
	if len(op.Key) != 32 {
		t.Errorf("Key should be 32 bytes, got: %d", len(op.Key))
	}

	// Verify it actually works
	result := p.Process(ToolResult{RawText: "test@example.com"})
	if !strings.Contains(result.RawText, "pii:") {
		t.Error("Process should produce pii: token")
	}
}
