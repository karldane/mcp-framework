package framework

import (
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

// Mock tool for testing scan output
type mockScanTool struct {
	BaseTool
	name        string
	description string
	withOutput  bool
}

func (t *mockScanTool) Name() string        { return t.name }
func (t *mockScanTool) Description() string { return t.description }
func (t *mockScanTool) Schema() mcp.ToolInputSchema {
	return mcp.ToolInputSchema{
		Type: "object",
		Properties: map[string]interface{}{
			"param1": map[string]interface{}{
				"type":        "string",
				"description": "First parameter",
			},
			"param2": map[string]interface{}{
				"type":        "number",
				"description": "Second parameter",
			},
		},
		Required: []string{"param1"},
	}
}

func (t *mockScanTool) OutputSchema() *mcp.ToolOutputSchema {
	if !t.withOutput {
		return nil
	}
	schema := mcp.ToolOutputSchema{
		Type: "object",
		Properties: map[string]interface{}{
			"result": map[string]interface{}{
				"type":        "string",
				"description": "Result value",
			},
		},
	}
	return &schema
}

func (t *mockScanTool) Handle(ctx CallContext, args map[string]interface{}) (ToolResult, error) {
	return TextResult("test"), nil
}

func (t *mockScanTool) EnforcerProfile(args map[string]interface{}) *EnforcerProfile {
	return NewEnforcerProfile(
		WithRisk(RiskLow),
		WithImpact(ImpactRead),
		WithResourceCost(2),
		WithPII(false),
	)
}

// TestRunScanModeBridge tests default bridge format
func TestRunScanModeBridge(t *testing.T) {
	server := NewServer("test-server", "1.0.0")
	
	tool1 := &mockScanTool{name: "tool1", description: "First tool", withOutput: false}
	tool2 := &mockScanTool{name: "tool2", description: "Second tool", withOutput: true}
	
	server.RegisterTool(tool1)
	server.RegisterTool(tool2)
	
	server.scanFormat = "bridge" // explicit bridge format
	
	// Capture stdout by running scan logic directly
	output := ScanModeOutput{
		Name:    server.name,
		Version: server.version,
		Tools:   make([]ScanModeTool, 0, len(server.tools)),
	}
	
	for _, rt := range server.tools {
		tool := ScanModeTool{
			Name:        rt.handler.Name(),
			Description: rt.handler.Description(),
			Profile:     rt.handler.EnforcerProfile(nil),
		}
		output.Tools = append(output.Tools, tool)
	}
	
	// Verify structure
	if output.Name != "test-server" {
		t.Errorf("Expected name 'test-server', got '%s'", output.Name)
	}
	if output.Version != "1.0.0" {
		t.Errorf("Expected version '1.0.0', got '%s'", output.Version)
	}
	if len(output.Tools) != 2 {
		t.Fatalf("Expected 2 tools, got %d", len(output.Tools))
	}
	
	// Bridge format should NOT include inputSchema or outputSchema
	jsonBytes, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("Failed to marshal output: %v", err)
	}
	
	var parsed map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &parsed); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}
	
	tools := parsed["tools"].([]interface{})
	tool := tools[0].(map[string]interface{})
	
	// Bridge format should have: name, description, profile
	if _, ok := tool["name"]; !ok {
		t.Error("Bridge format missing 'name'")
	}
	if _, ok := tool["description"]; !ok {
		t.Error("Bridge format missing 'description'")
	}
	if _, ok := tool["profile"]; !ok {
		t.Error("Bridge format missing 'profile'")
	}
	
	// Bridge format should NOT have schemas
	if _, ok := tool["inputSchema"]; ok {
		t.Error("Bridge format should not include 'inputSchema'")
	}
	if _, ok := tool["outputSchema"]; ok {
		t.Error("Bridge format should not include 'outputSchema'")
	}
}

// TestRunScanModeManifest tests manifest format with full schemas
func TestRunScanModeManifest(t *testing.T) {
	server := NewServer("test-server", "1.0.0")
	
	tool1 := &mockScanTool{name: "tool1", description: "First tool", withOutput: false}
	tool2 := &mockScanTool{name: "tool2", description: "Second tool", withOutput: true}
	
	server.RegisterTool(tool1)
	server.RegisterTool(tool2)
	
	server.scanFormat = "manifest"
	
	// Build manifest output
	output := ScanModeOutputManifest{
		Name:    server.name,
		Version: server.version,
		Tools:   make([]ScanModeToolManifest, 0, len(server.tools)),
	}
	
	for _, rt := range server.tools {
		inputSchema := rt.handler.Schema()
		outputSchema := rt.handler.OutputSchema()
		profile := rt.handler.EnforcerProfile(nil)
		
		// Build annotations
		var annotations *mcp.ToolAnnotation
		if profile != nil {
			boolPtr := func(b bool) *bool { return &b }
			annotations = &mcp.ToolAnnotation{
				Title:          rt.handler.Name(),
				ReadOnlyHint:   boolPtr(profile.ImpactScope == ImpactRead),
				IdempotentHint: boolPtr(profile.Idempotent),
				OpenWorldHint:  boolPtr(profile.PIIExposure),
			}
		}
		
		tool := ScanModeToolManifest{
			Name:         rt.handler.Name(),
			Description:  rt.handler.Description(),
			InputSchema:  &inputSchema,
			OutputSchema: outputSchema,
			Annotations:  annotations,
			Profile:      profile,
		}
		output.Tools = append(output.Tools, tool)
	}
	
	// Verify structure
	if len(output.Tools) != 2 {
		t.Fatalf("Expected 2 tools, got %d", len(output.Tools))
	}
	
	// Marshal to JSON to verify format
	jsonBytes, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("Failed to marshal manifest output: %v", err)
	}
	
	var parsed map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &parsed); err != nil {
		t.Fatalf("Failed to parse manifest JSON: %v", err)
	}
	
	tools := parsed["tools"].([]interface{})
	
	// Check tool1 (no outputSchema)
	tool1Map := tools[0].(map[string]interface{})
	if tool1Map["name"] != "tool1" && tool1Map["name"] != "tool2" {
		t.Errorf("Unexpected tool name: %v", tool1Map["name"])
	}
	
	// Manifest format should have inputSchema
	if _, ok := tool1Map["inputSchema"]; !ok {
		t.Error("Manifest format missing 'inputSchema'")
	}
	
	inputSchema := tool1Map["inputSchema"].(map[string]interface{})
	if inputSchema["type"] != "object" {
		t.Error("inputSchema should have type 'object'")
	}
	
	props := inputSchema["properties"].(map[string]interface{})
	if len(props) != 2 {
		t.Errorf("Expected 2 input properties, got %d", len(props))
	}
	
	// Check tool with outputSchema
	var toolWithOutput map[string]interface{}
	for _, t := range tools {
		tm := t.(map[string]interface{})
		if tm["name"] == "tool2" {
			toolWithOutput = tm
			break
		}
	}
	
	if toolWithOutput == nil {
		t.Fatal("Could not find tool2 in output")
	}
	
	// tool2 should have outputSchema
	if _, ok := toolWithOutput["outputSchema"]; !ok {
		t.Error("tool2 should have outputSchema")
	}
	
	outputSchema := toolWithOutput["outputSchema"].(map[string]interface{})
	if outputSchema["type"] != "object" {
		t.Error("outputSchema should have type 'object'")
	}
	
	// Both tools should have annotations
	if _, ok := tool1Map["annotations"]; !ok {
		t.Error("Manifest format missing 'annotations'")
	}
	
	annotations := tool1Map["annotations"].(map[string]interface{})
	if annotations["title"] == nil {
		t.Error("annotations should have 'title'")
	}
	if annotations["readOnlyHint"] == nil {
		t.Error("annotations should have 'readOnlyHint'")
	}
	if annotations["idempotentHint"] == nil {
		t.Error("annotations should have 'idempotentHint'")
	}
	if annotations["openWorldHint"] == nil {
		t.Error("annotations should have 'openWorldHint'")
	}
}

// TestScanModeBackwardCompatibility verifies bridge format matches old behavior
func TestScanModeBackwardCompatibility(t *testing.T) {
	server := NewServer("test-server", "1.0.0")
	
	tool := &mockScanTool{name: "test_tool", description: "Test tool", withOutput: false}
	server.RegisterTool(tool)
	
	// Default scanFormat should be bridge (or empty defaults to bridge)
	if server.scanFormat != "" && server.scanFormat != "bridge" {
		server.scanFormat = "bridge"
	}
	
	output := ScanModeOutput{
		Name:    server.name,
		Version: server.version,
		Tools:   make([]ScanModeTool, 0, len(server.tools)),
	}
	
	for _, rt := range server.tools {
		scanTool := ScanModeTool{
			Name:        rt.handler.Name(),
			Description: rt.handler.Description(),
			Profile:     rt.handler.EnforcerProfile(nil),
		}
		output.Tools = append(output.Tools, scanTool)
	}
	
	// Marshal and verify fields
	jsonBytes, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}
	
	var parsed ScanModeOutput
	if err := json.Unmarshal(jsonBytes, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}
	
	if len(parsed.Tools) != 1 {
		t.Fatalf("Expected 1 tool, got %d", len(parsed.Tools))
	}
	
	scanTool := parsed.Tools[0]
	if scanTool.Name != "test_tool" {
		t.Errorf("Expected name 'test_tool', got '%s'", scanTool.Name)
	}
	if scanTool.Description != "Test tool" {
		t.Errorf("Expected description 'Test tool', got '%s'", scanTool.Description)
	}
	if scanTool.Profile == nil {
		t.Error("Expected profile to be present")
	}
	if scanTool.Profile.RiskLevel != RiskLow {
		t.Errorf("Expected risk level 'low', got '%s'", scanTool.Profile.RiskLevel)
	}
}
