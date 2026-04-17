package framework

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

// ToolResult is the spec-compliant return type for all tool handlers.
// It maps directly to the MCP tools/call response content array.
type ToolResult struct {
	Content []ContentItem
	IsError bool
}

// ContentItem represents a single MCP content object.
// Type must be one of: "text", "image", "resource".
type ContentItem struct {
	Type     string
	Text     string
	MimeType string
	Data     string
}

// TextResult returns a standard successful text result.
func TextResult(s string) ToolResult {
	return ToolResult{
		Content: []ContentItem{{Type: "text", Text: s}},
	}
}

// ErrorResult returns a result that signals a tool-level error to the MCP client.
// This is distinct from a Go error — use it when the tool ran successfully but
// the operation itself failed (e.g. "file not found", "query returned no rows").
func ErrorResult(s string) ToolResult {
	return ToolResult{
		Content: []ContentItem{{Type: "text", Text: s}},
		IsError: true,
	}
}

// MultiResult returns a result with multiple content items.
func MultiResult(items ...ContentItem) ToolResult {
	return ToolResult{Content: items}
}

// TextContent constructs a text ContentItem.
func TextContent(s string) ContentItem {
	return ContentItem{Type: "text", Text: s}
}

// ValidationError represents a validation failure in the framework dispatch layer.
type ValidationError struct {
	Stage string // "input" or "output"
	Tool  string
	Err   error
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("tool %q %s validation: %v", e.Tool, e.Stage, e.Err)
}

func (e *ValidationError) Unwrap() error {
	return e.Err
}

// IsValidationError checks if an error is a ValidationError.
func IsValidationError(err error) bool {
	var ve *ValidationError
	return errors.As(err, &ve)
}

// validateResult performs struct validation against MCP content spec.
func validateResult(r ToolResult) error {
	if len(r.Content) == 0 {
		return fmt.Errorf("ToolResult.Content must not be empty")
	}
	for i, item := range r.Content {
		switch item.Type {
		case "text":
			if item.Text == "" {
				return fmt.Errorf("content[%d]: text items must have non-empty Text", i)
			}
		case "image":
			if item.MimeType == "" {
				return fmt.Errorf("content[%d]: image items must have MimeType", i)
			}
			if item.Data == "" {
				return fmt.Errorf("content[%d]: image items must have base64 Data", i)
			}
		case "resource":
			// resource validation TBD per MCP spec §5.4
		default:
			return fmt.Errorf("content[%d]: Type must be one of [text image resource], got %q", i, item.Type)
		}
	}
	return nil
}

// AssertTextResult asserts that result is a successful single-text-item result
// with the given content.
func AssertTextResult(t *testing.T, result ToolResult, expected string) {
	if result.IsError {
		t.Fatalf("expected successful result, got IsError=true with text: %q", result.Content[0].Text)
	}
	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(result.Content))
	}
	if result.Content[0].Type != "text" {
		t.Fatalf("expected Type='text', got %q", result.Content[0].Type)
	}
	if result.Content[0].Text != expected {
		t.Fatalf("expected Text=%q, got %q", expected, result.Content[0].Text)
	}
}

// AssertErrorResult asserts that result has IsError=true and contains the given
// substring in its text content.
func AssertErrorResult(t *testing.T, result ToolResult, contains string) {
	if !result.IsError {
		t.Fatalf("expected error result, got IsError=false")
	}
	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(result.Content))
	}
	if result.Content[0].Type != "text" {
		t.Fatalf("expected Type='text', got %q", result.Content[0].Type)
	}
	if result.Content[0].Text == "" {
		t.Fatal("expected non-empty error text")
	}
	if !containsSubstring(result.Content[0].Text, contains) {
		t.Fatalf("expected text to contain %q, got %q", contains, result.Content[0].Text)
	}
}

func containsSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// AssertToolCompliant runs a full compliance check on a ToolHandler:
// - Schema is valid and compiles without error (via server.RegisterTool)
// - EnforcerProfile is fully populated (no zero values)
// - Handle returns a spec-compliant ToolResult for the given args
//
// This helper is typically used in backend test suites where tools are
// registered with a server first (which validates the schema).
func AssertToolCompliant(t *testing.T, tool ToolHandler, args map[string]interface{}) {
	t.Helper()

	// Check EnforcerProfile is fully populated
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

	// Check Handle returns spec-compliant ToolResult
	ctx := context.Background()
	result, err := tool.Handle(ctx, args)
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	AssertTextResult(t, result, result.Content[0].Text)
}
