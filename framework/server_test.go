package framework

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

// MockLegacyTool is a test implementation of the old (string, error) interface.
type MockLegacyTool struct {
	name        string
	description string
	schema      mcp.ToolInputSchema
	result      string
}

func (m *MockLegacyTool) Name() string                { return m.name }
func (m *MockLegacyTool) Description() string         { return m.description }
func (m *MockLegacyTool) Schema() mcp.ToolInputSchema { return m.schema }
func (m *MockLegacyTool) Handle(ctx context.Context, args map[string]interface{}) (string, error) {
	return m.result, nil
}
func (m *MockLegacyTool) GetEnforcerProfile() *EnforcerProfile {
	return DefaultEnforcerProfile()
}

// MockToolHandler is a test implementation of ToolHandler

type MockToolHandler struct {
	name        string
	description string
	schema      mcp.ToolInputSchema
	result      ToolResult
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

func (m *MockToolHandler) Handle(ctx CallContext, args map[string]interface{}) (ToolResult, error) {
	if m.err != nil {
		return ToolResult{}, m.err
	}
	return m.result, nil
}

func (m *MockToolHandler) EnforcerProfile(args map[string]interface{}) *EnforcerProfile {
	if m.profile != nil {
		return m.profile
	}
	return DefaultEnforcerProfile()
}

func writeTool(name string) *MockToolHandler {
	return &MockToolHandler{
		name:   name,
		result: TextResult("ok"),
		schema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]interface{}{
				"param": map[string]interface{}{
					"type": "string",
				},
			},
		},
		profile: NewEnforcerProfile(
			WithImpact(ImpactWrite),
		),
	}
}

func readTool(name string) *MockToolHandler {
	return &MockToolHandler{
		name:   name,
		result: TextResult("data"),
		schema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]interface{}{
				"param": map[string]interface{}{
					"type": "string",
				},
			},
		},
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
		schema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]interface{}{
				"param": map[string]interface{}{
					"type":        "string",
					"description": "A parameter",
				},
			},
		},
		result: TextResult("test result"),
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
		schema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]interface{}{
				"param": map[string]interface{}{
					"type":        "string",
					"description": "A parameter",
				},
			},
		},
		result: TextResult("test result"),
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

	AssertTextResult(t, result, "test result")
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
		schema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]interface{}{
				"param": map[string]interface{}{
					"type": "string",
				},
			},
		},
		result: TextResult("ok"),
	}
	handler2 := &MockToolHandler{
		name: "test-tool",
		schema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]interface{}{
				"param": map[string]interface{}{
					"type": "string",
				},
			},
		},
		result: TextResult("ok"),
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
				result: TextResult("ok"),
				schema: mcp.ToolInputSchema{
					Type: "object",
					Properties: map[string]interface{}{
						"param": map[string]interface{}{
							"type": "string",
						},
					},
				},
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
				result: TextResult("ok"),
				schema: mcp.ToolInputSchema{
					Type: "object",
					Properties: map[string]interface{}{
						"param": map[string]interface{}{
							"type": "string",
						},
					},
				},
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
		name:   "no-profile",
		result: TextResult("ok"),
		schema: mcp.ToolInputSchema{
			Type:       "object",
			Properties: map[string]interface{}{},
		},
		profile: DefaultEnforcerProfile(),
	}
	_ = s.RegisterTool(tool)

	result, err := s.ExecuteTool(ctx, "no-profile", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RawText != "ok" {
		t.Errorf("expected 'ok', got %q", result.RawText)
	}
}

func TestServerInitialize(t *testing.T) {
	s := NewServer("test", "1.0.0")
	tool := &MockToolHandler{
		name:        "test-tool",
		description: "A test tool",
		schema: mcp.ToolInputSchema{
			Type:       "object",
			Properties: map[string]interface{}{},
		},
		result:  TextResult("hello"),
		profile: NewEnforcerProfile(WithImpact(ImpactRead)),
	}
	_ = s.RegisterTool(tool)

	s.Initialize()

	if s.GetMCPServer() == nil {
		t.Fatal("expected mcpServer to be set")
	}
}

func TestServerInitializeWithPIO(t *testing.T) {
	s := NewServerWithConfig(&Config{
		Name:           "test",
		Version:        "1.0.0",
		PIIScanEnabled: true,
		PIIConfig:      &PIIPipelineConfig{},
	})
	tool := &MockToolHandler{
		name:        "pii-tool",
		description: "A PII tool",
		schema: mcp.ToolInputSchema{
			Type:       "object",
			Properties: map[string]interface{}{},
		},
		result:  TextResult("data"),
		profile: NewEnforcerProfile(WithImpact(ImpactRead)),
	}
	_ = s.RegisterTool(tool)

	s.Initialize()

	if s.GetMCPServer() == nil {
		t.Fatal("expected mcpServer to be set")
	}
}

func TestServerInitializeWriteDisabled(t *testing.T) {
	s := NewServerWithConfig(&Config{
		Name:         "test",
		Version:      "1.0.0",
		WriteEnabled: false,
	})
	tool := &MockToolHandler{
		name:        "readonly-tool",
		description: "A readonly tool",
		schema: mcp.ToolInputSchema{
			Type:       "object",
			Properties: map[string]interface{}{},
		},
		result:  TextResult("data"),
		profile: NewEnforcerProfile(WithImpact(ImpactRead)),
	}
	_ = s.RegisterTool(tool)

	s.Initialize()

	if s.GetMCPServer() == nil {
		t.Fatal("expected mcpServer to be set")
	}
}

func TestAssertToolCompliant(t *testing.T) {
	tool := &MockToolHandler{
		name:        "test-tool",
		description: "A test tool",
		schema: mcp.ToolInputSchema{
			Type:       "object",
			Properties: map[string]interface{}{},
		},
		result:  TextResult("hello"),
		profile: NewEnforcerProfile(WithRisk(RiskLow), WithImpact(ImpactRead)),
	}
	AssertToolCompliant(t, tool, map[string]interface{}{})
}

func TestFormatDataResultWithRows(t *testing.T) {
	result := ToolResult{
		Data: []map[string]interface{}{
			{"name": "Alice", "age": "30"},
			{"name": "Bob", "age": "25"},
		},
	}
	text, err := formatDataResult(result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(text) == 0 {
		t.Error("expected non-empty text")
	}
}

func TestFormatDataResultEmptyRows(t *testing.T) {
	result := ToolResult{
		Data: []map[string]interface{}{},
	}
	text, err := formatDataResult(result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text == "" {
		t.Error("expected non-empty text")
	}
}

func TestFormatDataResultWithSafetyNote(t *testing.T) {
	result := ToolResult{
		Data: []map[string]interface{}{
			{"email": "test@example.com"},
		},
		Meta: ResultMeta{SafetyNote: "pii detected"},
	}
	text, err := formatDataResult(result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text == "" {
		t.Error("expected non-empty text")
	}
}

func TestFormatDataResultNonSlice(t *testing.T) {
	result := ToolResult{
		Data: map[string]interface{}{"key": "value"},
	}
	text, err := formatDataResult(result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text == "" {
		t.Error("expected non-empty text")
	}
}

func TestInitializeBlocksDeleteWhenDisabled(t *testing.T) {
	s := NewServerWithConfig(&Config{
		Name:         "test",
		Version:      "1.0.0",
		WriteEnabled: false,
	})
	tool := &MockToolHandler{
		name:        "delete-tool",
		description: "A delete tool",
		schema:      mcp.ToolInputSchema{Type: "object"},
		result:      TextResult("deleted"),
		profile:     NewEnforcerProfile(WithImpact(ImpactDelete)),
	}
	_ = s.RegisterTool(tool)

	s.Initialize()

	_, err := s.ExecuteTool(context.Background(), "delete-tool", nil)
	if err == nil {
		t.Error("expected error for delete tool when write disabled")
	}
}

func TestInitializeBlocksAdminWhenDisabled(t *testing.T) {
	s := NewServerWithConfig(&Config{
		Name:         "test",
		Version:      "1.0.0",
		WriteEnabled: false,
	})
	tool := &MockToolHandler{
		name:        "admin-tool",
		description: "An admin tool",
		schema:      mcp.ToolInputSchema{Type: "object"},
		result:      TextResult("admin"),
		profile:     NewEnforcerProfile(WithImpact(ImpactAdmin)),
	}
	_ = s.RegisterTool(tool)

	s.Initialize()

	_, err := s.ExecuteTool(context.Background(), "admin-tool", nil)
	if err == nil {
		t.Error("expected error for admin tool when write disabled")
	}
}

func TestInitializeWithNilProfile(t *testing.T) {
	s := NewServer("test", "1.0.0")
	tool := &MockToolHandler{
		name:        "nil-profile-tool",
		description: "A tool with nil profile",
		schema: mcp.ToolInputSchema{
			Type:       "object",
			Properties: map[string]interface{}{},
		},
		result:  TextResult("ok"),
		profile: nil,
	}
	_ = s.RegisterTool(tool)

	s.Initialize()

	if s.GetMCPServer() == nil {
		t.Error("expected mcpServer to be set")
	}
}

func TestWrapLegacyDescription(t *testing.T) {
	legacy := &MockLegacyTool{
		name:        "legacy",
		description: "Legacy description",
		schema:      mcp.ToolInputSchema{Type: "object"},
		result:      "hello",
	}
	wrapped := WrapLegacy(legacy)
	if wrapped.Description() != "Legacy description" {
		t.Error("expected description to match")
	}
}

func TestWrapLegacySchema(t *testing.T) {
	legacy := &MockLegacyTool{
		name:        "legacy",
		description: "Desc",
		schema:      mcp.ToolInputSchema{Type: "object", Properties: map[string]interface{}{"x": nil}},
		result:      "hello",
	}
	wrapped := WrapLegacy(legacy)
	schema := wrapped.Schema()
	if schema.Type != "object" {
		t.Error("expected object type")
	}
}

func TestWrapLegacyEnforcerProfile(t *testing.T) {
	legacy := &MockLegacyTool{
		name:   "legacy",
		result: "test",
	}
	wrapped := WrapLegacy(legacy)
	profile := wrapped.EnforcerProfile(nil)
	if profile == nil {
		t.Fatal("expected non-nil profile")
	}
}

func TestWrapLegacyHandleError(t *testing.T) {
	legacy := &MockLegacyTool{
		name:   "legacy",
		result: "test",
	}
	wrapped := WrapLegacy(legacy)
	result, err := wrapped.Handle(CallContext{Context: context.Background()}, map[string]interface{}{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RawText != "test" {
		t.Errorf("expected 'test', got %s", result.RawText)
	}
}

func TestServerListTools(t *testing.T) {
	s := NewServer("test", "1.0.0")
	tool := &MockToolHandler{
		name:        "tool1",
		description: "Tool 1",
		schema:      mcp.ToolInputSchema{Type: "object"},
		result:      TextResult("ok"),
		profile:     DefaultEnforcerProfile(),
	}
	_ = s.RegisterTool(tool)
	tools := s.ListTools()
	if len(tools) != 1 || tools[0] != "tool1" {
		t.Error("expected tool1 in list")
	}
}

func TestServerListToolsEmpty(t *testing.T) {
	s := NewServer("test", "1.0.0")
	tools := s.ListTools()
	if len(tools) != 0 {
		t.Error("expected empty list")
	}
}

func TestListToolsWithTools(t *testing.T) {
	s := NewServer("test", "1.0.0")
	tool := &MockToolHandler{
		name:        "tool1",
		description: "Tool 1",
		schema:      mcp.ToolInputSchema{Type: "object"},
		result:      TextResult("ok"),
		profile:     DefaultEnforcerProfile(),
	}
	_ = s.RegisterTool(tool)

	tools := s.ListTools()
	if len(tools) != 1 || tools[0] != "tool1" {
		t.Error("expected tool1 in list")
	}
}

func TestServerSetWriteEnabled(t *testing.T) {
	s := NewServer("test", "1.0.0")
	if !s.IsWriteEnabled() {
		t.Error("write should be enabled by default")
	}
	s.SetWriteEnabled(false)
	if s.IsWriteEnabled() {
		t.Error("write should be disabled after SetWriteEnabled(false)")
	}
	s.SetWriteEnabled(true)
	if !s.IsWriteEnabled() {
		t.Error("write should be enabled after SetWriteEnabled(true)")
	}
}

func TestServerExecuteToolNotFound(t *testing.T) {
	s := NewServer("test", "1.0.0")
	_, err := s.ExecuteTool(context.Background(), "non-existent", nil)
	if err == nil {
		t.Error("expected error for non-existent tool")
	}
}

func TestServerExecuteToolValidationError(t *testing.T) {
	s := NewServer("test", "1.0.0")
	tool := &MockToolHandler{
		name:        "validation-tool",
		description: "A tool",
		schema:      mcp.ToolInputSchema{Type: "object", Properties: map[string]interface{}{"x": map[string]interface{}{"type": "string"}}},
		result:      TextResult("ok"),
		profile:     DefaultEnforcerProfile(),
	}
	_ = s.RegisterTool(tool)
	// Pass invalid args to trigger validation error
	_, err := s.ExecuteTool(context.Background(), "validation-tool", map[string]interface{}{"x": 123})
	if err == nil {
		t.Error("expected validation error")
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
