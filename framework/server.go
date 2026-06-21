package framework

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
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

	// Title returns an optional human-readable display name for the tool.
	// This is used by clients for UI display. If empty, clients fall back to Name().
	Title() string

	// OutputSchema returns the JSON schema for tool output, or nil if not defined.
	// When non-nil, the server will include outputSchema in the tool definition.
	OutputSchema() *mcp.ToolOutputSchema

	// Handle executes the tool with the provided arguments
	Handle(ctx CallContext, args map[string]interface{}) (ToolResult, error)

	// EnforcerProfile returns the self-reported safety metadata for the tool.
	// This profile is transmitted during the tools/list handshake via annotations.
	// Called with nil args at tools/list time (return worst-case profile).
	// Called with real args at tools/call time (may return accurate profile).
	// Implementations that are always static should ignore args.
	EnforcerProfile(args map[string]interface{}) *EnforcerProfile
}

// BaseTool provides default implementations for optional methods.
// Embed this struct in your tool to get default no-op implementations.
type BaseTool struct{}

// Title returns empty string (no custom title).
// Override this method to provide a human-readable display name.
func (BaseTool) Title() string {
	return ""
}

// OutputSchema returns nil (no output schema defined).
// Override this method if your tool returns structured data.
func (BaseTool) OutputSchema() *mcp.ToolOutputSchema {
	return nil
}

// EnforcerProfile returns the default safety profile.
// Override this method to provide tool-specific safety metadata.
func (BaseTool) EnforcerProfile(args map[string]interface{}) *EnforcerProfile {
	return DefaultEnforcerProfile()
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

// ScanModeOutput is the JSON structure output in scan mode
type ScanModeOutput struct {
	Name     string                    `json:"name"`
	Version  string                    `json:"version"`
	Tools    []ScanModeTool            `json:"tools"`
	Error    string                    `json:"error,omitempty"`
}

// ScanModeTool represents a tool in scan mode output (bridge format)
type ScanModeTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Profile     *EnforcerProfile       `json:"profile,omitempty"`
}

// ScanModeToolManifest represents a tool in manifest format with full schemas
type ScanModeToolManifest struct {
	Name         string                  `json:"name"`
	Title        string                  `json:"title,omitempty"`
	Description  string                  `json:"description"`
	InputSchema  *mcp.ToolInputSchema    `json:"inputSchema,omitempty"`
	OutputSchema *mcp.ToolOutputSchema   `json:"outputSchema,omitempty"`
	Annotations  *mcp.ToolAnnotation     `json:"annotations,omitempty"`
	Profile      *EnforcerProfile        `json:"profile,omitempty"`
}

// ScanModeOutputManifest is the JSON structure output in manifest scan mode
type ScanModeOutputManifest struct {
	Name     string                     `json:"name"`
	Version  string                     `json:"version"`
	Tools    []ScanModeToolManifest     `json:"tools"`
	Error    string                     `json:"error,omitempty"`
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
	scanMode     bool
	scanFormat   string // "bridge" (default) or "manifest"
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

	// Inbound: resolve PII tokens in args before handler sees them
	if s.piiEnabled && s.piiPipeline != nil {
		var resolveErr error
		args, resolveErr = s.piiPipeline.Resolve(args)
		if resolveErr != nil {
			return ToolResult{}, fmt.Errorf("pii resolve: %w", resolveErr)
		}
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
				Title:          handler.Title(),
				ReadOnlyHint:   boolPtr(profile.ImpactScope == ImpactRead),
				IdempotentHint: boolPtr(profile.Idempotent),
				OpenWorldHint:  boolPtr(profile.PIIExposure),
			}
		} else {
			annotations = mcp.ToolAnnotation{
				Title:          handler.Title(),
				ReadOnlyHint:   boolPtr(true),
				IdempotentHint: boolPtr(true),
				OpenWorldHint:  boolPtr(false),
			}
		}

		tool := mcp.Tool{
			Name:        handler.Name(),
			Title:       handler.Title(),
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

		if outputSchema := handler.OutputSchema(); outputSchema != nil {
			tool.OutputSchema = *outputSchema
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

			// Inbound: resolve PII tokens in args before handler sees them
			if s.piiEnabled && s.piiPipeline != nil {
				var resolveErr error
				args, resolveErr = s.piiPipeline.Resolve(args)
				if resolveErr != nil {
					return mcp.NewToolResultError(fmt.Sprintf("pii resolve: %v", resolveErr)), nil
				}
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

// structuredQueryResult is the shape of CallToolResult.StructuredContent.
type structuredQueryResult struct {
	Rows    []map[string]interface{} `json:"rows"`
	Columns []columnMeta             `json:"columns"`
	Meta    outputMeta               `json:"meta"`
}

type columnMeta struct {
	Name        string   `json:"name"`
	DataType    string   `json:"data_type"`
	ScanPolicy  string   `json:"scan_policy"`
	PIIDetected bool     `json:"pii_detected"`
	EntityTypes []string `json:"entity_types"`
	Treatment   string   `json:"treatment"`
	RowsScanned int      `json:"rows_scanned"`
	RowsTreated int      `json:"rows_treated"`
}

type outputMeta struct {
	RowCount       int  `json:"row_count"`
	PIIScanApplied bool `json:"pii_scan_applied"`
	ColumnsTreated int  `json:"columns_treated"`
}

// toolResultToMCP converts a framework ToolResult to an MCP CallToolResult
func toolResultToMCP(result ToolResult) *mcp.CallToolResult {
	if result.IsError {
		return mcp.NewToolResultError(result.RawText)
	}

	var dataText string
	var rows []map[string]interface{}

	if result.RawText != "" {
		dataText = result.RawText
	} else if result.Data != nil {
		if r, ok := result.Data.([]map[string]interface{}); ok {
			rows = r
		}
		b, err := json.Marshal(result.Data)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("result serialisation failed: %v", err))
		}
		dataText = string(b)
	}

	if dataText == "" {
		return mcp.NewToolResultError("empty tool result")
	}

	contents := []mcp.Content{
		mcp.TextContent{Type: "text", Text: dataText},
	}

	if result.Meta.PIIScanApplied && len(result.Meta.ColumnReports) > 0 {
		contents = append(contents, mcp.TextContent{
			Type: "text",
			Text: formatPIIAudit(result.Meta),
		})
	}

	var structured *structuredQueryResult
	if rows != nil || len(result.Meta.ColumnReports) > 0 {
		cols := make([]columnMeta, 0, len(result.Meta.ColumnReports))
		treated := 0
		for _, cr := range result.Meta.ColumnReports {
			if cr.PIIDetected {
				treated++
			}
			cols = append(cols, columnMeta{
				Name:        cr.ColumnName,
				DataType:    cr.DataType,
				ScanPolicy:  cr.ScanPolicyName,
				PIIDetected: cr.PIIDetected,
				EntityTypes: cr.EntityTypes,
				Treatment:   cr.Treatment,
				RowsScanned: cr.RowsScanned,
				RowsTreated: cr.RowsTreated,
			})
		}
		rowCount := 0
		if rows != nil {
			rowCount = len(rows)
		}
		structured = &structuredQueryResult{
			Rows:    rows,
			Columns: cols,
			Meta: outputMeta{
				RowCount:       rowCount,
				PIIScanApplied: result.Meta.PIIScanApplied,
				ColumnsTreated: treated,
			},
		}
	}

	res := &mcp.CallToolResult{Content: contents}
	if structured != nil {
		res.StructuredContent = structured
	}
	return res
}

// formatPIIAudit renders the per-column PII report as a readable text block.
func formatPIIAudit(meta ResultMeta) string {
	var sb strings.Builder
	sb.WriteString("PII Scan Report\n")
	sb.WriteString(strings.Repeat("─", 56) + "\n")
	sb.WriteString(fmt.Sprintf("%-20s  %-16s  %-22s  %s\n",
		"Column", "Data Type", "Entity", "Rows"))
	sb.WriteString(strings.Repeat("─", 56) + "\n")

	for _, cr := range meta.ColumnReports {
		if cr.ScanPolicyName == "safe" || cr.ScanPolicyName == "strip" {
			sb.WriteString(fmt.Sprintf("%-20s  %-16s  %-22s  %s\n",
				cr.ColumnName, cr.DataType, "—", "not scanned ["+cr.ScanPolicyName+"]"))
			continue
		}
		if !cr.PIIDetected {
			sb.WriteString(fmt.Sprintf("%-20s  %-16s  %-22s  %s\n",
				cr.ColumnName, cr.DataType, "—", "no pii detected"))
			continue
		}
		entities := strings.Join(cr.EntityTypes, ", ")
		policyNote := ""
		if cr.ScanPolicyName == "name_only" {
			policyNote = "  [name_only]"
		}
		rowNote := fmt.Sprintf("%d/%d rows  %s%s",
			cr.RowsTreated, cr.RowsScanned, cr.Treatment, policyNote)
		sb.WriteString(fmt.Sprintf("%-20s  %-16s  %-22s  %s\n",
			cr.ColumnName, cr.DataType, entities, rowNote))
	}

	if len(meta.Truncations) > 0 {
		sb.WriteString("\nTruncations\n")
		sb.WriteString(strings.Repeat("─", 56) + "\n")
		for _, t := range meta.Truncations {
			sb.WriteString(fmt.Sprintf("%-20s  original: %d chars  truncated at: %d\n",
				t.Column, t.OriginalLength, t.TruncatedAt))
		}
	}

	return sb.String()
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

// SetScanMode enables scan mode which outputs tool definitions as JSON to stdout
// and exits immediately without starting the MCP stdio server.
// This allows mcp-bridge to get tool profiles without needing full MCP handshake.
func (s *Server) SetScanMode(enabled bool) {
	s.scanMode = enabled
}

// IsScanMode returns whether scan mode is enabled
func (s *Server) IsScanMode() bool {
	return s.scanMode
}

// RunScanMode outputs tool definitions as JSON and exits.
// Called by main.go when --scan flag is passed.
// Format depends on s.scanFormat: "bridge" (default) or "manifest".
func (s *Server) RunScanMode() error {
	if s.mcpServer == nil {
		s.Initialize()
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")

	// Use manifest format if requested
	if s.scanFormat == "manifest" {
		output := ScanModeOutputManifest{
			Name:    s.name,
			Version: s.version,
			Tools:   make([]ScanModeToolManifest, 0, len(s.tools)),
		}

		for _, rt := range s.tools {
			inputSchema := rt.handler.Schema()
			outputSchema := rt.handler.OutputSchema()
			profile := rt.handler.EnforcerProfile(nil)
			title := rt.handler.Title()
			
			// Build annotations from profile (same logic as Initialize)
			var annotations *mcp.ToolAnnotation
			if profile != nil {
				boolPtr := func(b bool) *bool { return &b }
				annotations = &mcp.ToolAnnotation{
					Title:          title,
					ReadOnlyHint:   boolPtr(profile.ImpactScope == ImpactRead),
					IdempotentHint: boolPtr(profile.Idempotent),
					OpenWorldHint:  boolPtr(profile.PIIExposure),
				}
			}
			
			tool := ScanModeToolManifest{
				Name:         rt.handler.Name(),
				Title:        title,
				Description:  rt.handler.Description(),
				InputSchema:  &inputSchema,
				OutputSchema: outputSchema,
				Annotations:  annotations,
				Profile:      profile,
			}
			output.Tools = append(output.Tools, tool)
		}

		if err := encoder.Encode(output); err != nil {
			output.Error = fmt.Sprintf("failed to encode scan output: %v", err)
			encoder.Encode(output)
			return err
		}
		return nil
	}

	// Default bridge format
	output := ScanModeOutput{
		Name:    s.name,
		Version: s.version,
		Tools:   make([]ScanModeTool, 0, len(s.tools)),
	}

	for _, rt := range s.tools {
		tool := ScanModeTool{
			Name:        rt.handler.Name(),
			Description: rt.handler.Description(),
			Profile:     rt.handler.EnforcerProfile(nil),
		}
		output.Tools = append(output.Tools, tool)
	}

	if err := encoder.Encode(output); err != nil {
		output.Error = fmt.Sprintf("failed to encode scan output: %v", err)
		encoder.Encode(output)
		return err
	}

	return nil
}

// HandleScanFlag checks for --scan flag and runs scan mode if enabled.
// Supports --scan-format=bridge (default) or --scan-format=manifest.
// Call this at the start of main() after creating the server.
// Returns true if scan mode was triggered (program will exit).
func HandleScanFlag(s *Server) bool {
	// Check both --scan and --scan-mode flags
	scanEnabled := false
	scanFormat := "bridge" // default
	
	for i, arg := range os.Args[1:] {
		if arg == "--scan" || arg == "--scan-mode" {
			scanEnabled = true
		}
		// Check for --scan-format=value
		if strings.HasPrefix(arg, "--scan-format=") {
			scanFormat = strings.TrimPrefix(arg, "--scan-format=")
		}
		// Check for --scan-format value (space-separated)
		if arg == "--scan-format" && i+1 < len(os.Args[1:]) {
			scanFormat = os.Args[i+2] // i+2 because os.Args[1:] is offset
		}
	}

	if scanEnabled {
		s.scanFormat = scanFormat
		s.SetScanMode(true)
		if err := s.RunScanMode(); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}

	return false
}

// GetMCPServer returns the underlying MCP server for testing or customization
func (s *Server) GetMCPServer() *server.MCPServer {
	return s.mcpServer
}
