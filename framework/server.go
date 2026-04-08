package framework

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// ToolHandler defines the interface for MCP tool implementations
type ToolHandler interface {
	// Name returns the unique name of the tool
	Name() string

	// Description returns the tool description shown to users
	Description() string

	// Schema returns the JSON schema for tool parameters
	Schema() mcp.ToolInputSchema

	// Handle executes the tool with the provided arguments
	Handle(ctx context.Context, args map[string]interface{}) (string, error)

	// GetEnforcerProfile returns the self-reported safety metadata for the tool
	// This profile is transmitted during the tools/list handshake via annotations.
	// Return nil to opt out of safety enforcement.
	GetEnforcerProfile() *EnforcerProfile
}

// Config holds server configuration
type Config struct {
	Name         string
	Version      string
	Instructions string
	// WriteEnabled controls whether mutating tools (ImpactWrite/Delete/Admin) are
	// permitted. Defaults to true — set to false to run in readonly mode.
	// In backend servers this should be set to !cfg.ReadOnly().
	WriteEnabled bool
}

// Server provides the base MCP server functionality
type Server struct {
	name         string
	version      string
	instructions string
	writeEnabled bool
	tools        map[string]ToolHandler
	mcpServer    *server.MCPServer
}

// NewServer creates a new MCP server with the given name and version.
// Writes are enabled by default; use SetWriteEnabled(false) or pass
// WriteEnabled: false in Config to restrict to readonly mode.
func NewServer(name, version string) *Server {
	s := &Server{
		name:         name,
		version:      version,
		writeEnabled: true,
		tools:        make(map[string]ToolHandler),
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
	return s
}

// RegisterTool adds a tool handler to the server
func (s *Server) RegisterTool(handler ToolHandler) error {
	name := handler.Name()
	if _, exists := s.tools[name]; exists {
		return fmt.Errorf("tool '%s' already registered", name)
	}
	s.tools[name] = handler
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
func (s *Server) ExecuteTool(ctx context.Context, name string, args map[string]interface{}) (string, error) {
	handler, exists := s.tools[name]
	if !exists {
		return "", fmt.Errorf("tool '%s' not found", name)
	}

	// Check write-gate (skip enforcement for tools that return no profile)
	profile := handler.GetEnforcerProfile()
	if profile != nil && !s.writeEnabled && (profile.ImpactScope == ImpactWrite || profile.ImpactScope == ImpactDelete || profile.ImpactScope == ImpactAdmin) {
		return "", fmt.Errorf("write tools are disabled in readonly mode; start the server without --readonly to allow mutations")
	}

	return handler.Handle(ctx, args)
}

// Initialize sets up the MCP server with all registered tools
func (s *Server) Initialize() {
	serverOptions := []server.ServerOption{}

	if s.instructions != "" {
		serverOptions = append(serverOptions, server.WithInstructions(s.instructions))
	}

	s.mcpServer = server.NewMCPServer(s.name, s.version, serverOptions...)

	// Register all tools with the MCP server
	for _, handler := range s.tools {
		profile := handler.GetEnforcerProfile()

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
		toolHandler := handler
		toolProfile := profile

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
			result, err := toolHandler.Handle(ctx, args)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(result), nil
		})
	}
}

// Start begins serving MCP requests via stdio (blocking)
func (s *Server) Start() error {
	if s.mcpServer == nil {
		s.Initialize()
	}
	return server.ServeStdio(s.mcpServer)
}

// GetMCPServer returns the underlying MCP server for testing or customization
func (s *Server) GetMCPServer() *server.MCPServer {
	return s.mcpServer
}
