package framework

import (
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestTextResult(t *testing.T) {
	result := TextResult("hello world")

	if result.RawText != "hello world" {
		t.Errorf("Expected RawText='hello world', got %q", result.RawText)
	}
	if result.IsError {
		t.Error("Expected IsError=false")
	}
}

func TestDataResult(t *testing.T) {
	rows := []map[string]interface{}{
		{"id": 1, "name": "alice"},
		{"id": 2, "name": "bob"},
	}
	result := DataResult(rows)

	data, ok := result.Data.([]map[string]interface{})
	if !ok {
		t.Fatal("Expected Data to be []map[string]interface{}")
	}
	if len(data) != 2 {
		t.Errorf("Expected 2 rows, got %d", len(data))
	}
	if result.IsError {
		t.Error("Expected IsError=false")
	}
}

func TestValidateResultEmpty(t *testing.T) {
	err := validateResult(ToolResult{})
	if err == nil {
		t.Error("Expected error for empty ToolResult")
	}
}

func TestValidateResultValidText(t *testing.T) {
	result := ToolResult{RawText: "valid"}
	err := validateResult(result)
	if err != nil {
		t.Errorf("Unexpected error for valid text result: %v", err)
	}
}

func TestValidateResultValidData(t *testing.T) {
	result := ToolResult{Data: []map[string]interface{}{{"id": 1}}}
	err := validateResult(result)
	if err != nil {
		t.Errorf("Unexpected error for valid data result: %v", err)
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
func (m *mockToolHandlerWithTypedResult) Handle(ctx CallContext, args map[string]interface{}) (ToolResult, error) {
	if m.err != nil {
		return ToolResult{}, m.err
	}
	return m.result, nil
}
func (m *mockToolHandlerWithTypedResult) EnforcerProfile(args map[string]interface{}) *EnforcerProfile {
	if m.profile != nil {
		return m.profile
	}
	return DefaultEnforcerProfile()
}

func (m *mockToolHandlerWithTypedResult) OutputSchema() *mcp.ToolOutputSchema {
	return nil
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
	result := ErrorResult(ToolError{
		Code:    ErrCodeInternalError,
		Message: "error message",
	})
	AssertErrorResult(t, result, "error message")
}

func TestAssertErrorResultContains(t *testing.T) {
	result := ErrorResult(ToolError{
		Code:    ErrCodeInternalError,
		Message: "full error message here",
	})
	AssertErrorResult(t, result, "error")
}

func TestErrorResult(t *testing.T) {
	result := ErrorResult(ToolError{
		Code:    ErrCodeInvalidArgs,
		Message: "something went wrong",
	})

	if result.RawText != "something went wrong" {
		t.Errorf("Expected RawText='something went wrong', got %q", result.RawText)
	}
	if !result.IsError {
		t.Error("Expected IsError=true")
	}
	if result.Error == nil {
		t.Error("Expected Error to be set")
	}
	if result.Error.Code != ErrCodeInvalidArgs {
		t.Errorf("Error.Code = %q, want %q", result.Error.Code, ErrCodeInvalidArgs)
	}
	if result.Error.Message != "something went wrong" {
		t.Errorf("Error.Message = %q, want %q", result.Error.Message, "something went wrong")
	}
}

func TestValidateResultError(t *testing.T) {
	result := ErrorResultLegacy("error")
	err := validateResult(result)
	if err != nil {
		t.Errorf("Unexpected error for error result: %v", err)
	}
}
