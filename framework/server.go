package framework

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sort"
	"strings"
	"sync/atomic"
	"syscall"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

// registeredTool holds a tool handler with its compiled schema validator
type registeredTool struct {
	handler   ToolHandler
	validator *jsonschema.Schema
}

// schemaCompiler is a shared compiler for all schema compilations
var schemaCompiler = jsonschema.NewCompiler()

// schemaCounter ensures unique schema URLs across all servers and registrations
var schemaCounter int64

// ToolHandler defines the interface for MCP tool implementations
type ToolHandler interface {
	// Name returns the unique name of the tool
	Name() string

	// Description returns the tool description shown to users
	Description() string

	// Schema returns the JSON schema for tool parameters
	Schema() mcp.ToolInputSchema

	// Handle executes the tool with the provided arguments
	Handle(ctx CallContext, args map[string]interface{}) (ToolResult, error)

	// EnforcerProfile returns the self-reported safety metadata for the tool.
	// This profile is transmitted during the tools/list handshake via annotations.
	// Called with nil args at tools/list time (return worst-case profile).
	// Called with real args at tools/call time (may return accurate profile).
	// Implementations that are always static should ignore args.
	EnforcerProfile(args map[string]interface{}) *EnforcerProfile
}

// Config holds server configuration
type Config struct {
	Name           string
	Version        string
	Instructions   string
	WriteEnabled   bool
	PIIScanEnabled bool
	PIIConfig      *PIIPipelineConfig
}

// Server provides the base MCP server functionality
type Server struct {
	name         string
	version      string
	instructions string
	writeEnabled bool
	tools        map[string]registeredTool
	mcpServer    *server.MCPServer
	piiEnabled   bool
	piiPipeline  *PIIPipeline
}

// autoFlushingWriter wraps a bufio.Writer and flushes after every write.
type autoFlushingWriter struct {
	writer *bufio.Writer
}

func newAutoFlushingWriter(w io.Writer) *autoFlushingWriter {
	return &autoFlushingWriter{writer: bufio.NewWriter(w)}
}

func (w *autoFlushingWriter) Write(p []byte) (n int, err error) {
	n, err = w.writer.Write(p)
	if err != nil {
		return n, err
	}
	err = w.writer.Flush()
	return n, err
}

// formatDataResult serialises a ToolResult whose Data field is set.
func formatDataResult(result ToolResult) (string, error) {
	rows, ok := result.Data.([]map[string]interface{})
	if !ok {
		b, err := json.Marshal(result.Data)
		if err != nil {
			return "", err
		}
		return string(b), nil
	}

	if len(rows) == 0 {
		text := "0 rows returned."
		if result.Meta.SafetyNote != "" {
			text += "\n\n[PII: " + result.Meta.SafetyNote + "]"
		}
		return text, nil
	}

	cols := make([]string, 0, len(rows[0]))
	for col := range rows[0] {
		cols = append(cols, col)
	}
	sort.Strings(cols)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Rows: %d\n\n", len(rows)))
	sb.WriteString(strings.Join(cols, " | "))
	sb.WriteString("\n")
	sb.WriteString(strings.Repeat("-", 60))
	sb.WriteString("\n")

	for _, row := range rows {
		vals := make([]string, len(cols))
		for i, col := range cols {
			if v, ok := row[col]; ok && v != nil {
				vals[i] = fmt.Sprintf("%v", v)
			} else {
				vals[i] = "NULL"
			}
		}
		sb.WriteString(strings.Join(vals, " | "))
		sb.WriteString("\n")
	}

	if result.Meta.SafetyNote != "" {
		sb.WriteString("\n[PII: ")
		sb.WriteString(result.Meta.SafetyNote)
		sb.WriteString("]\n")
	}

	return sb.String(), nil
}

// NewServer creates a new MCP server with the given name and version.
// Writes are enabled by default; use SetWriteEnabled(false) or pass
// WriteEnabled: false in Config to restrict to readonly mode.
func NewServer(name, version string) *Server {
	s := &Server{
		name:         name,
		version:      version,
		writeEnabled: true,
		tools:        make(map[string]registeredTool),
	}
	return s
}

// SetWriteEnabled enables or disables mutating tools (ImpactWrite/Delete/Admin).
func (s *Server) SetWriteEnabled(enabled bool) {
	s.writeEnabled = enabled
}

// IsWriteEnabled returns whether mutating tools are permitted.
func (s *Server) IsWriteEnabled() bool {
	return s.writeEnabled
}

// NewServerWithConfig creates a server with full configuration.
// If config.WriteEnabled is false, mutating tools will be blocked.
// The zero value of Config.WriteEnabled is false, so callers must explicitly
// set WriteEnabled: true (or call SetWriteEnabled(true) afterwards) unless
// they intend to run in readonly mode.
func NewServerWithConfig(config *Config) *Server {
	s := NewServer(config.Name, config.Version)
	s.instructions = config.Instructions
	s.writeEnabled = config.WriteEnabled
	s.piiEnabled = config.PIIScanEnabled
	if config.PIIScanEnabled && config.PIIConfig != nil {
		s.piiPipeline = NewPIIPipeline(config.PIIConfig)
	}
	return s
}

// RegisterTool adds a tool handler to the server.
// Panics if the tool's schema is invalid — this is a programming error that
// must be fixed before the server starts.
func (s *Server) RegisterTool(handler ToolHandler) error {
	name := handler.Name()
	if _, exists := s.tools[name]; exists {
		return fmt.Errorf("tool '%s' already registered", name)
	}

	schema := handler.Schema()
	schemaJSON, err := json.Marshal(schema)
	if err != nil {
		panic(fmt.Sprintf("tool %q has invalid schema (marshal error): %v", name, err))
	}
	var schemaDoc any
	if err := json.Unmarshal(schemaJSON, &schemaDoc); err != nil {
		panic(fmt.Sprintf("tool %q has invalid schema (unmarshal error): %v", name, err))
	}
	// Use a global counter to make URL unique for each registration
	// This allows the same tool to be registered on different server instances
	id := atomic.AddInt64(&schemaCounter, 1)
	url := fmt.Sprintf("tool://%s/schema/%d", name, id)
	if err := schemaCompiler.AddResource(url, schemaDoc); err != nil {
		panic(fmt.Sprintf("tool %q failed to add schema resource: %v", name, err))
	}
	validator, err := schemaCompiler.Compile(url)
	if err != nil {
		panic(fmt.Sprintf("tool %q has invalid schema: %v", name, err))
	}

	s.tools[name] = registeredTool{
		handler:   handler,
		validator: validator,
	}
	return nil
}

// ListTools returns a list of registered tool names
func (s *Server) ListTools() []string {
	names := make([]string, 0, len(s.tools))
	for name := range s.tools {
		names = append(names, name)
	}
	return names
}

// ExecuteTool runs a tool by name with the provided arguments
func (s *Server) ExecuteTool(ctx context.Context, name string, args map[string]interface{}) (ToolResult, error) {
	rt, ok := s.tools[name]
	if !ok {
		return ToolResult{}, fmt.Errorf("tool '%s' not found", name)
	}

	// Convert context to CallContext for handler
	callCtx := CallContext{Context: ctx}

	// Check write-gate (skip enforcement for tools that return no profile)
	profile := rt.handler.EnforcerProfile(nil) // tools/list call for static profile
	if profile != nil && !s.writeEnabled && (profile.ImpactScope == ImpactWrite || profile.ImpactScope == ImpactDelete || profile.ImpactScope == ImpactAdmin) {
		return ToolResult{}, fmt.Errorf("write tools are disabled in readonly mode; start the server without --readonly to allow mutations")
	}

	if err := rt.validator.Validate(args); err != nil {
		return ToolResult{}, &ValidationError{Stage: "input", Tool: name, Err: err}
	}

	// Call handler with real args for dynamic profile
	result, err := rt.handler.Handle(callCtx, args)
	if err != nil {
		return ToolResult{}, fmt.Errorf("tool %s: %w", name, err)
	}

	if err := validateResult(result); err != nil {
		return ToolResult{}, &ValidationError{Stage: "output", Tool: name, Err: err}
	}

	if s.piiEnabled && s.piiPipeline != nil {
		result = s.piiPipeline.Process(result)
	}

	result.Meta.FrameworkVer = s.version

	return result, nil
}

// Initialize sets up the MCP server with all registered tools
func (s *Server) Initialize() {
	serverOptions := []server.ServerOption{}

	if s.instructions != "" {
		serverOptions = append(serverOptions, server.WithInstructions(s.instructions))
	}

	s.mcpServer = server.NewMCPServer(s.name, s.version, serverOptions...)

	// Register all tools with the MCP server
	for name, rt := range s.tools {
		handler := rt.handler
		profile := handler.EnforcerProfile(nil)

		// Helper function to convert bool to *bool
		boolPtr := func(b bool) *bool {
			return &b
		}

		// Build annotations — use safe defaults when a tool opts out of profiling
		var annotations mcp.ToolAnnotation
		if profile != nil {
			annotations = mcp.ToolAnnotation{
				Title:          handler.Name(),
				ReadOnlyHint:   boolPtr(profile.ImpactScope == ImpactRead),
				IdempotentHint: boolPtr(profile.Idempotent),
				OpenWorldHint:  boolPtr(profile.PIIExposure),
			}
		} else {
			annotations = mcp.ToolAnnotation{
				Title:          handler.Name(),
				ReadOnlyHint:   boolPtr(true),
				IdempotentHint: boolPtr(true),
				OpenWorldHint:  boolPtr(false),
			}
		}

		tool := mcp.Tool{
			Name:        handler.Name(),
			Description: handler.Description(),
			InputSchema: handler.Schema(),
			Annotations: annotations,
			// Store the full profile in Meta for the Bridge to access (nil if no profile)
			Meta: &mcp.Meta{
				AdditionalFields: map[string]any{
					"enforcer_profile": profile,
				},
			},
		}

		// Store values needed in closure
		toolName := name
		toolHandler := handler
		toolProfile := profile
		toolValidator := rt.validator

		// Register the tool handler
		s.mcpServer.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			// Check write-gate (skip for tools with no profile)
			if toolProfile != nil && !s.writeEnabled && (toolProfile.ImpactScope == ImpactWrite || toolProfile.ImpactScope == ImpactDelete || toolProfile.ImpactScope == ImpactAdmin) {
				return mcp.NewToolResultError("write tools are disabled in readonly mode; start the server without --readonly to allow mutations"), nil
			}

			var args map[string]interface{}
			if request.Params.Arguments != nil {
				if argMap, ok := request.Params.Arguments.(map[string]interface{}); ok {
					args = argMap
				}
			}

			// Convert context to CallContext for handler
			callCtx := CallContext{Context: ctx}

			// Validate inputs
			if err := toolValidator.Validate(args); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("tool %q input validation: %v", toolName, err)), nil
			}

			// Call handler with real args for dynamic profile
			result, err := toolHandler.Handle(callCtx, args)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			// Validate output
			if err := validateResult(result); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("tool %q output validation: %v", toolName, err)), nil
			}

			// Apply PII pipeline if enabled
			if s.piiEnabled && s.piiPipeline != nil {
				result = s.piiPipeline.Process(result)
			}

			// Convert ToolResult to MCP CallToolResult
			return toolResultToMCP(result), nil
		})
	}
}

// toolResultToMCP converts a framework ToolResult to an MCP CallToolResult
func toolResultToMCP(result ToolResult) *mcp.CallToolResult {
	if result.IsError {
		return mcp.NewToolResultError(result.RawText)
	}

	if result.RawText != "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: result.RawText,
				},
			},
		}
	}

	if result.Data != nil {
		text, err := formatDataResult(result)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("result serialisation failed: %v", err))
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{Type: "text", Text: text},
			},
		}
	}

	return mcp.NewToolResultError("empty tool result")
}

// Start begins serving MCP requests via stdio (blocking).
// It wraps stdout in an auto-flushing writer to prevent output buffering.
func (s *Server) Start() error {
	if s.mcpServer == nil {
		s.Initialize()
	}

	stdioServer := server.NewStdioServer(s.mcpServer)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		<-sigChan
		cancel()
	}()

	stdout := newAutoFlushingWriter(os.Stdout)

	return stdioServer.Listen(ctx, os.Stdin, stdout)
}

// GetMCPServer returns the underlying MCP server for testing or customization
func (s *Server) GetMCPServer() *server.MCPServer {
	return s.mcpServer
}
