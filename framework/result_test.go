package framework

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestTextResult(t *testing.T) {
	result := TextResult("hello world")

	if len(result.Content) != 1 {
		t.Fatalf("Expected 1 content item, got %d", len(result.Content))
	}
	if result.IsError {
		t.Error("Expected IsError=false")
	}
	if result.Content[0].Type != "text" {
		t.Errorf("Expected Type='text', got %q", result.Content[0].Type)
	}
	if result.Content[0].Text != "hello world" {
		t.Errorf("Expected Text='hello world', got %q", result.Content[0].Text)
	}
}

func TestErrorResult(t *testing.T) {
	result := ErrorResult("something went wrong")

	if len(result.Content) != 1 {
		t.Fatalf("Expected 1 content item, got %d", len(result.Content))
	}
	if !result.IsError {
		t.Error("Expected IsError=true")
	}
	if result.Content[0].Type != "text" {
		t.Errorf("Expected Type='text', got %q", result.Content[0].Type)
	}
	if result.Content[0].Text != "something went wrong" {
		t.Errorf("Expected Text='something went wrong', got %q", result.Content[0].Text)
	}
}

func TestMultiResult(t *testing.T) {
	items := []ContentItem{
		{Type: "text", Text: "first"},
		{Type: "text", Text: "second"},
	}
	result := MultiResult(items...)

	if len(result.Content) != 2 {
		t.Fatalf("Expected 2 content items, got %d", len(result.Content))
	}
	if result.IsError {
		t.Error("Expected IsError=false")
	}
	if result.Content[0].Text != "first" {
		t.Errorf("Expected Content[0].Text='first', got %q", result.Content[0].Text)
	}
	if result.Content[1].Text != "second" {
		t.Errorf("Expected Content[1].Text='second', got %q", result.Content[1].Text)
	}
}

func TestTextContent(t *testing.T) {
	item := TextContent("hello")

	if item.Type != "text" {
		t.Errorf("Expected Type='text', got %q", item.Type)
	}
	if item.Text != "hello" {
		t.Errorf("Expected Text='hello', got %q", item.Text)
	}
}

func TestValidateResultEmpty(t *testing.T) {
	err := validateResult(ToolResult{})
	if err == nil {
		t.Error("Expected error for empty Content")
	}
}

func TestValidateResultTextItemEmpty(t *testing.T) {
	result := ToolResult{
		Content: []ContentItem{
			{Type: "text", Text: ""},
		},
	}
	err := validateResult(result)
	if err == nil {
		t.Error("Expected error for text item with empty Text")
	}
}

func TestValidateResultImageItemMissingMimeType(t *testing.T) {
	result := ToolResult{
		Content: []ContentItem{
			{Type: "image", Data: "abc123"},
		},
	}
	err := validateResult(result)
	if err == nil {
		t.Error("Expected error for image item missing MimeType")
	}
}

func TestValidateResultImageItemMissingData(t *testing.T) {
	result := ToolResult{
		Content: []ContentItem{
			{Type: "image", MimeType: "image/png"},
		},
	}
	err := validateResult(result)
	if err == nil {
		t.Error("Expected error for image item missing Data")
	}
}

func TestValidateResultInvalidType(t *testing.T) {
	result := ToolResult{
		Content: []ContentItem{
			{Type: "invalid"},
		},
	}
	err := validateResult(result)
	if err == nil {
		t.Error("Expected error for invalid Type")
	}
}

func TestValidateResultValidText(t *testing.T) {
	result := ToolResult{
		Content: []ContentItem{
			{Type: "text", Text: "valid"},
		},
	}
	err := validateResult(result)
	if err != nil {
		t.Errorf("Unexpected error for valid text result: %v", err)
	}
}

func TestValidateResultValidImage(t *testing.T) {
	result := ToolResult{
		Content: []ContentItem{
			{Type: "image", MimeType: "image/png", Data: "abc123"},
		},
	}
	err := validateResult(result)
	if err != nil {
		t.Errorf("Unexpected error for valid image result: %v", err)
	}
}

func TestValidationError(t *testing.T) {
	err := &ValidationError{
		Stage: "input",
		Tool:  "test-tool",
		Err:   assertErr,
	}

	expected := `tool "test-tool" input validation: assertErr`
	if err.Error() != expected {
		t.Errorf("Expected error message %q, got %q", expected, err.Error())
	}
}

var assertErr = &assertError{msg: "assertErr"}

type assertError struct {
	msg string
}

func (e *assertError) Error() string { return e.msg }

func TestValidationErrorUnwrap(t *testing.T) {
	err := &ValidationError{
		Stage: "output",
		Tool:  "test-tool",
		Err:   assertErr,
	}

	unwrapped := err.Unwrap()
	if unwrapped != assertErr {
		t.Error("Unwrap did not return the original error")
	}
}

type mockToolHandlerWithTypedResult struct {
	name        string
	description string
	schema      mcp.ToolInputSchema
	result      ToolResult
	err         error
	profile     *EnforcerProfile
}

func (m *mockToolHandlerWithTypedResult) Name() string                { return m.name }
func (m *mockToolHandlerWithTypedResult) Description() string         { return m.description }
func (m *mockToolHandlerWithTypedResult) Schema() mcp.ToolInputSchema { return m.schema }
func (m *mockToolHandlerWithTypedResult) Handle(ctx context.Context, args map[string]interface{}) (ToolResult, error) {
	if m.err != nil {
		return ToolResult{}, m.err
	}
	return m.result, nil
}
func (m *mockToolHandlerWithTypedResult) GetEnforcerProfile() *EnforcerProfile {
	if m.profile != nil {
		return m.profile
	}
	return DefaultEnforcerProfile()
}

func TestToolHandlerInterfaceReturnsToolResult(t *testing.T) {
	handler := &mockToolHandlerWithTypedResult{
		name:        "test-tool",
		description: "A test tool",
		schema:      mcp.ToolInputSchema{},
		result:      TextResult("test result"),
	}

	var _ ToolHandler = handler
}

func TestAssertTextResultSuccess(t *testing.T) {
	result := TextResult("expected text")
	AssertTextResult(t, result, "expected text")
}

func TestAssertErrorResultSuccess(t *testing.T) {
	result := ErrorResult("error message")
	AssertErrorResult(t, result, "error message")
}

func TestAssertErrorResultContains(t *testing.T) {
	result := ErrorResult("full error message here")
	AssertErrorResult(t, result, "error")
}
