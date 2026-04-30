package framework

import (
	"errors"
	"fmt"
	"testing"
)

// ScanPolicy describes how a column should be treated by the PII pipeline.
// Backends set this on ColumnHint; the framework maps it to presidio internally.
// Values must remain numerically stable — they are stored in oracle schema cache.
type ScanPolicy int

const (
	ScanPolicyDefault          ScanPolicy = 0 // use pipeline default
	ScanPolicySafe             ScanPolicy = 1 // skip — known safe column type
	ScanPolicyNameOnly         ScanPolicy = 2 // name heuristic only, no value scan
	ScanPolicyStrip            ScanPolicy = 3 // strip binary / no scan
	ScanPolicyTruncateThenScan ScanPolicy = 4 // truncate to MaxLength then scan
	ScanPolicyFull             ScanPolicy = 5 // force full scan regardless of type
)

// ColumnHint carries per-column PII scanning metadata from a backend tool to
// the pipeline. Backends must use this type; they must not import go-presidio.
type ColumnHint struct {
	ScanPolicy ScanPolicy
	MaxLength  int    // 0 = use pipeline default (DefaultMaxScanLength)
	EntityType string // optional: force entity type (e.g., "PERSON", "EMAIL_ADDRESS")
}

// ColumnReport is a framework-owned summary of PII treatment for one column.
type ColumnReport struct {
	ColumnName     string
	PIIDetected    bool
	EntityTypes    []string
	Treatment      string
	RowsScanned    int
	RowsTreated    int
	OriginalLength int
	TruncatedAt    int
}

type ToolResult struct {
	Data        interface{}           `json:"data,omitempty"`
	RawText     string                `json:"text,omitempty"`
	ColumnHints map[string]ColumnHint `json:"_hints,omitempty"`
	Meta        ResultMeta            `json:"_meta,omitempty"`
	IsError     bool                  `json:"is_error,omitempty"`
	Error       *ToolError            `json:"error,omitempty"`
}

type ResultMeta struct {
	PIIScanApplied bool             `json:"pii_scan_applied,omitempty"`
	ColumnReports  []ColumnReport   `json:"column_reports,omitempty"`
	Truncations    []TruncationNote `json:"truncations,omitempty"`
	SafetyNote     string           `json:"safety_note,omitempty"`
	FrameworkVer   string           `json:"framework_version,omitempty"`
}

type TruncationNote struct {
	Column         string `json:"column"`
	OriginalLength int    `json:"original_length"`
	TruncatedAt    int    `json:"truncated_at"`
}

func TextResult(s string) ToolResult {
	return ToolResult{RawText: s}
}

func DataResult(rows []map[string]interface{}) ToolResult {
	return ToolResult{Data: rows}
}

// ErrorResultLegacy is for backward compatibility with the old string-based error
func ErrorResultLegacy(msg string) ToolResult {
	return ToolResult{RawText: msg, IsError: true}
}

// ErrorResult constructs a ToolResult representing a tool error with structured error info
// Also sets RawText for backward compatibility with MCP error conversion
func ErrorResult(err ToolError) ToolResult {
	return ToolResult{IsError: true, Error: &err, RawText: err.Message}
}

// ErrorResultf constructs a ToolResult with an INTERNAL_ERROR code and formatted message.
// Convenience for the common case of wrapping a Go error.
func ErrorResultf(format string, a ...any) ToolResult {
	return ErrorResult(ToolError{
		Code:    ErrCodeInternalError,
		Message: fmt.Sprintf(format, a...),
	})
}

type ValidationError struct {
	Stage string
	Tool  string
	Err   error
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("tool %q %s validation: %v", e.Tool, e.Stage, e.Err)
}

func (e *ValidationError) Unwrap() error {
	return e.Err
}

func IsValidationError(err error) bool {
	var ve *ValidationError
	return errors.As(err, &ve)
}

func validateResult(r ToolResult) error {
	if r.IsError {
		return nil
	}
	if r.Data == nil && r.RawText == "" {
		return fmt.Errorf("ToolResult must have either Data or RawText set")
	}
	return nil
}

func AssertTextResult(t *testing.T, result ToolResult, expected string) {
	if result.IsError {
		t.Fatalf("expected successful result, got IsError=true with text: %q", result.RawText)
	}
	if result.RawText != expected {
		t.Fatalf("expected RawText=%q, got %q", expected, result.RawText)
	}
}

func AssertErrorResult(t *testing.T, result ToolResult, contains string) {
	if !result.IsError {
		t.Fatalf("expected error result, got IsError=false")
	}
	if result.RawText == "" {
		t.Fatal("expected non-empty error text")
	}
	found := false
	for i := 0; i <= len(result.RawText)-len(contains); i++ {
		if result.RawText[i:i+len(contains)] == contains {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected RawText to contain %q, got %q", contains, result.RawText)
	}
}

func AssertToolCompliant(t *testing.T, tool ToolHandler, args map[string]interface{}) {
	t.Helper()

	profile := tool.EnforcerProfile(nil)
	if profile == nil {
		t.Fatal("EnforcerProfile is nil")
	}
	if profile.RiskLevel == "" {
		t.Error("EnforcerProfile.RiskLevel is zero value")
	}
	if profile.ImpactScope == "" {
		t.Error("EnforcerProfile.ImpactScope is zero value")
	}
	if profile.ResourceCost == 0 {
		t.Error("EnforcerProfile.ResourceCost is zero value")
	}

	ctx := Background()
	result, err := tool.Handle(ctx, args)
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	if result.RawText != "" {
		AssertTextResult(t, result, result.RawText)
	}
}
