package framework

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

// MockToolHandler is a test implementation of ToolHandler

type MockToolHandler struct {
	name        string
	description string
	schema      mcp.ToolInputSchema
	result      string
	err         error
	profile     *EnforcerProfile
}

func (m *MockToolHandler) Name() string {
	return m.name
}

func (m *MockToolHandler) Description() string {
	return m.description
}

func (m *MockToolHandler) Schema() mcp.ToolInputSchema {
	return m.schema
}

func (m *MockToolHandler) Handle(ctx context.Context, args map[string]interface{}) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.result, nil
}

func (m *MockToolHandler) GetEnforcerProfile() *EnforcerProfile {
	if m.profile != nil {
		return m.profile
	}
	return DefaultEnforcerProfile()
}

func writeTool(name string) *MockToolHandler {
	return &MockToolHandler{
		name:   name,
		result: "ok",
		profile: NewEnforcerProfile(
			WithImpact(ImpactWrite),
		),
	}
}

func readTool(name string) *MockToolHandler {
	return &MockToolHandler{
		name:   name,
		result: "data",
		profile: NewEnforcerProfile(
			WithImpact(ImpactRead),
		),
	}
}

func TestServerCreation(t *testing.T) {
	server := NewServer("test-server", "1.0.0")
	if server == nil {
		t.Fatal("Expected server to be created")
	}

	if server.name != "test-server" {
		t.Errorf("Expected server name 'test-server', got '%s'", server.name)
	}

	if server.version != "1.0.0" {
		t.Errorf("Expected version '1.0.0', got '%s'", server.version)
	}
}

func TestServerWriteEnabledByDefault(t *testing.T) {
	s := NewServer("test", "1.0.0")
	if !s.IsWriteEnabled() {
		t.Fatal("NewServer should default to writes enabled")
	}
}

func TestToolRegistration(t *testing.T) {
	server := NewServer("test", "1.0.0")

	handler := &MockToolHandler{
		name:        "test-tool",
		description: "A test tool",
		schema:      mcp.ToolInputSchema{},
		result:      "test result",
	}

	err := server.RegisterTool(handler)
	if err != nil {
		t.Fatalf("Failed to register tool: %v", err)
	}

	tools := server.ListTools()
	if len(tools) != 1 {
		t.Errorf("Expected 1 tool, got %d", len(tools))
	}

	if tools[0] != "test-tool" {
		t.Errorf("Expected tool 'test-tool', got '%s'", tools[0])
	}
}

func TestToolExecution(t *testing.T) {
	server := NewServer("test", "1.0.0")

	handler := &MockToolHandler{
		name:        "test-tool",
		description: "A test tool",
		schema:      mcp.ToolInputSchema{},
		result:      "test result",
	}

	err := server.RegisterTool(handler)
	if err != nil {
		t.Fatalf("Failed to register tool: %v", err)
	}

	ctx := context.Background()
	result, err := server.ExecuteTool(ctx, "test-tool", map[string]interface{}{})

	if err != nil {
		t.Fatalf("Tool execution failed: %v", err)
	}

	if result != "test result" {
		t.Errorf("Expected result 'test result', got '%s'", result)
	}
}

func TestToolExecutionNotFound(t *testing.T) {
	server := NewServer("test", "1.0.0")

	ctx := context.Background()
	_, err := server.ExecuteTool(ctx, "non-existent", map[string]interface{}{})

	if err == nil {
		t.Fatal("Expected error for non-existent tool")
	}

	if err.Error() != "tool 'non-existent' not found" {
		t.Errorf("Unexpected error message: %v", err)
	}
}

func TestDuplicateToolRegistration(t *testing.T) {
	server := NewServer("test", "1.0.0")

	handler1 := &MockToolHandler{
		name: "test-tool",
	}
	handler2 := &MockToolHandler{
		name: "test-tool",
	}

	err := server.RegisterTool(handler1)
	if err != nil {
		t.Fatalf("Failed to register first tool: %v", err)
	}

	err = server.RegisterTool(handler2)
	if err == nil {
		t.Fatal("Expected error for duplicate tool registration")
	}
}

func TestServerWithConfig(t *testing.T) {
	config := &Config{
		Name:         "configured-server",
		Version:      "2.0.0",
		Instructions: "This is a test server",
		WriteEnabled: true,
	}

	server := NewServerWithConfig(config)
	if server == nil {
		t.Fatal("Expected server to be created with config")
	}

	if server.name != "configured-server" {
		t.Errorf("Expected name 'configured-server', got '%s'", server.name)
	}

	if server.instructions != "This is a test server" {
		t.Errorf("Expected instructions, got '%s'", server.instructions)
	}

	if !server.IsWriteEnabled() {
		t.Error("Expected writes enabled when Config.WriteEnabled=true")
	}
}

func TestServerWithConfigReadonly(t *testing.T) {
	config := &Config{
		Name:         "readonly-server",
		Version:      "1.0.0",
		WriteEnabled: false,
	}

	s := NewServerWithConfig(config)
	if s.IsWriteEnabled() {
		t.Error("Expected writes disabled when Config.WriteEnabled=false")
	}
}

// TestWriteGateBlocksMutatingTools verifies that mutating tools are blocked
// when writeEnabled=false and read tools still pass through.
func TestWriteGateBlocksMutatingTools(t *testing.T) {
	ctx := context.Background()

	for _, tc := range []struct {
		name      string
		impact    ImpactScope
		wantBlock bool
	}{
		{"write blocked", ImpactWrite, true},
		{"delete blocked", ImpactDelete, true},
		{"admin blocked", ImpactAdmin, true},
		{"read allowed", ImpactRead, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := NewServer("test", "1.0.0")
			s.SetWriteEnabled(false)

			tool := &MockToolHandler{
				name:   "tool",
				result: "ok",
				profile: NewEnforcerProfile(
					WithImpact(tc.impact),
				),
			}
			_ = s.RegisterTool(tool)

			_, err := s.ExecuteTool(ctx, "tool", nil)
			if tc.wantBlock && err == nil {
				t.Errorf("impact=%s: expected write-gate error, got nil", tc.impact)
			}
			if !tc.wantBlock && err != nil {
				t.Errorf("impact=%s: expected no error, got %v", tc.impact, err)
			}
		})
	}
}

// TestWriteGateAllowsWhenEnabled verifies all impact scopes pass when writes are on.
func TestWriteGateAllowsWhenEnabled(t *testing.T) {
	ctx := context.Background()

	for _, impact := range []ImpactScope{ImpactRead, ImpactWrite, ImpactDelete, ImpactAdmin} {
		t.Run(string(impact), func(t *testing.T) {
			s := NewServer("test", "1.0.0")
			// writeEnabled=true by default

			tool := &MockToolHandler{
				name:   "tool",
				result: "ok",
				profile: NewEnforcerProfile(
					WithImpact(impact),
				),
			}
			_ = s.RegisterTool(tool)

			_, err := s.ExecuteTool(ctx, "tool", nil)
			if err != nil {
				t.Errorf("impact=%s: unexpected error when writes enabled: %v", impact, err)
			}
		})
	}
}

// TestWriteGateErrorMessage verifies the error text no longer references a
// non-existent --write-enabled flag.
func TestWriteGateErrorMessage(t *testing.T) {
	ctx := context.Background()
	s := NewServer("test", "1.0.0")
	s.SetWriteEnabled(false)

	tool := writeTool("mut")
	_ = s.RegisterTool(tool)

	_, err := s.ExecuteTool(ctx, "mut", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if contains(msg, "--write-enabled") {
		t.Errorf("error message should not reference --write-enabled flag; got: %q", msg)
	}
	if !contains(msg, "readonly") {
		t.Errorf("error message should mention readonly mode; got: %q", msg)
	}
}

// TestNilProfileSkipsWriteGate verifies that tools returning nil from
// GetEnforcerProfile are never blocked by the write-gate.
func TestNilProfileSkipsWriteGate(t *testing.T) {
	ctx := context.Background()
	s := NewServer("test", "1.0.0")
	s.SetWriteEnabled(false)

	tool := &MockToolHandler{
		name:    "no-profile",
		result:  "ok",
		profile: nil,
	}
	_ = s.RegisterTool(tool)

	_, err := s.ExecuteTool(ctx, "no-profile", nil)
	if err != nil {
		t.Errorf("nil-profile tool should bypass write-gate; got error: %v", err)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}
