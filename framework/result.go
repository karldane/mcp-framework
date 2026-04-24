package framework

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/karldane/go-presidio/presidio"
)

type ToolResult struct {
	Data        interface{}                    `json:"data,omitempty"`
	RawText     string                         `json:"text,omitempty"`
	ColumnHints map[string]presidio.ColumnHint `json:"_hints,omitempty"`
	Meta        ResultMeta                     `json:"_meta,omitempty"`
	IsError     bool                           `json:"is_error,omitempty"`
}

type ResultMeta struct {
	PIIScanApplied bool                    `json:"pii_scan_applied,omitempty"`
	ColumnReports  []presidio.ColumnReport `json:"column_reports,omitempty"`
	Truncations    []TruncationNote        `json:"truncations,omitempty"`
	SafetyNote     string                  `json:"safety_note,omitempty"`
	FrameworkVer   string                  `json:"framework_version,omitempty"`
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

func ErrorResult(msg string) ToolResult {
	return ToolResult{RawText: msg, IsError: true}
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

	profile := tool.GetEnforcerProfile()
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

	ctx := context.Background()
	result, err := tool.Handle(ctx, args)
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	if result.RawText != "" {
		AssertTextResult(t, result, result.RawText)
	}
}
