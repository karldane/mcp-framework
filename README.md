# MCP Framework

A reusable Go framework for building MCP (Model Context Protocol) servers with built-in safety self-reporting capabilities.

## Overview

This framework provides a simple, extensible base for creating MCP servers. It handles the MCP protocol details and provides a clean interface for implementing custom tools with integrated safety metadata.

## Installation

```bash
go get github.com/karldane/mcp-framework@v0.1.0
```

## Quick Start

### Creating a Basic MCP Server

```go
package main

import (
    "context"
    "fmt"
    
    "github.com/mark3labs/mcp-go/mcp"
    "github.com/karldane/mcp-framework/framework"
)

// MyTool implements the ToolHandler interface
type MyTool struct{}

func (t *MyTool) Name() string {
    return "my_tool"
}

func (t *MyTool) Description() string {
    return "A sample tool"
}

func (t *MyTool) Schema() mcp.ToolInputSchema {
    return mcp.ToolInputSchema{
        Type: "object",
        Properties: map[string]interface{}{
            "input": map[string]interface{}{
                "type":        "string",
                "description": "Input to process",
            },
        },
        Required: []string{"input"},
    }
}

func (t *MyTool) Handle(ctx context.Context, args map[string]interface{}) (string, error) {
    input := args["input"].(string)
    return fmt.Sprintf("Processed: %s", input), nil
}

func (t *MyTool) GetEnforcerProfile() framework.EnforcerProfile {
    return framework.NewEnforcerProfile(
        framework.WithRisk(framework.RiskLow),
        framework.WithImpact(framework.ImpactRead),
        framework.WithResourceCost(3),
        framework.WithPII(false),
        framework.WithIdempotent(true),
        framework.WithApprovalReq(false),
    )
}

func main() {
    // Create server
    server := framework.NewServer("my-server", "1.0.0")
    
    // Register tool
    server.RegisterTool(&MyTool{})
    
    // Initialize and start
    server.Initialize()
    server.Start()
}
```

## Core Concepts

### ToolHandler Interface

The `ToolHandler` interface is the heart of the framework. Implement this interface to create custom tools:

```go
type ToolHandler interface {
    Name() string                    // Unique tool name
    Description() string             // Tool description for users
    Schema() mcp.ToolInputSchema     // JSON Schema for parameters
    Handle(ctx context.Context, args map[string]interface{}) (string, error)
    GetEnforcerProfile() EnforcerProfile  // Safety metadata
}
```

### Server Configuration

```go
config := &framework.Config{
    Name:         "my-server",
    Version:      "1.0.0",
    Instructions: "Optional usage instructions",
}

server := framework.NewServerWithConfig(config)
```

### Safety Self-Reporting (EnforcerProfile)

The framework implements the MCP Self-Reporting Safety Protocol. Each tool must declare its safety metadata via `GetEnforcerProfile()`:

```go
func (t *MyTool) GetEnforcerProfile() framework.EnforcerProfile {
    return framework.NewEnforcerProfile(
        framework.WithRisk(framework.RiskMed),        // low, med, high, critical
        framework.WithImpact(framework.ImpactRead),   // read, write, delete, admin
        framework.WithResourceCost(5),                // 1-10 scale
        framework.WithPII(true),                      // exposes sensitive data?
        framework.WithIdempotent(true),               // safe to retry?
        framework.WithApprovalReq(false),             // require human approval?
    )
}
```

**Risk Levels:**
- `RiskLow`: Read-only operations, minimal impact
- `RiskMed`: Operations with moderate impact or resource usage
- `RiskHigh`: Write operations, deletions, infrastructure changes
- `RiskCritical`: Administrative operations, irreversible changes

**Impact Scopes:**
- `ImpactRead`: Read-only data access
- `ImpactWrite`: Data modification
- `ImpactDelete`: Data deletion or archival
- `ImpactAdmin`: Administrative operations

**Defaults:**
- Risk: `med`
- Impact: `read`
- Resource Cost: `5`
- PII: `true` (assume sensitive until proven otherwise)
- Idempotent: `false`
- Approval Required: `false`

This metadata is transmitted during the `tools/list` handshake in tool annotations, allowing MCP Bridges to make automated security decisions.

## Example Implementations

The framework is used by several production MCP servers:

- **[Oracle MCP](https://github.com/karldane/oracle-mcp)** - Oracle database integration
- **[New Relic MCP](https://github.com/karldane/newrelic-mcp)** - New Relic observability platform
- **[Slack MCP](https://github.com/karldane/slack-mcp)** - Slack workspace management

## Architecture

### Project Structure

```
mcp-framework/
├── framework/          # Core framework
│   ├── server.go      # Base server implementation
│   ├── safety.go      # EnforcerProfile definitions
│   └── server_test.go # Framework tests
├── example/           # Example implementation
│   └── main.go
├── docs/              # Specifications
│   └── MCP-safety-reporting-spec.md
├── go.mod
├── go.sum
└── README.md
```

### Design Principles

1. **Simple**: Minimal abstractions, clear interfaces
2. **Extensible**: Easy to add new tool types
3. **Testable**: Comprehensive test coverage
4. **Production-Ready**: Error handling, timeouts, retries
5. **Safe**: Built-in safety metadata for automated policy enforcement

### Dependencies

- `github.com/mark3labs/mcp-go`: MCP protocol implementation
- Standard library only (minimal external dependencies)

## Building a New Backend

> **Start here:** [`docs/SPEC_MCP_BACKEND.md`](docs/SPEC_MCP_BACKEND.md) is the
> canonical reference for all new backends. It covers the standard Makefile,
> README template, directory layout, `devdocs/` convention, testing requirements,
> and a pre-release checklist. Read it before writing any code.

The short version:

1. Create a new Go module with `main.go` at the repo root
2. Add `github.com/karldane/mcp-framework` as a dependency (use a `replace` directive during development if the framework is local)
3. Implement `framework.ToolHandler` for each tool — `Name`, `Description`, `Schema`, `Handle`, `GetEnforcerProfile`
4. Register tools via a `tools.Register(server, cfg)` helper in `internal/tools/`
5. Copy the standard `Makefile` from the spec verbatim, substituting `BINARY_NAME`
6. Write tests covering EnforcerProfile accuracy, schema, and `Handle` validation before touching the client
7. Follow the README template in the spec — tools table with Risk/Impact/Approval columns is mandatory

### Example Directory Structure

```
my-mcp/
├── main.go                  # Wiring only — no business logic
├── Makefile                 # Standard (copy from SPEC_MCP_BACKEND.md)
├── README.md                # Standard template (from SPEC_MCP_BACKEND.md)
├── LICENSE                  # Functional Source License 1.1, ALv2 Future
├── go.mod
├── go.sum
├── internal/
│   ├── config/              # Config struct and env var parsing
│   ├── client/              # API client for the target service
│   └── tools/               # Tool implementations
└── devdocs/
    ├── SPEC_<NAME>_BACKEND.md  # Backend-specific requirements and phases
    └── AGENTS.md               # Context for AI agents: decisions, gotchas, status
```

### Best Practices

1. **Always declare EnforcerProfile**: Every tool must report its safety characteristics
2. **Use TDD**: Write tests before implementation
3. **Handle errors gracefully**: Return meaningful error messages
4. **Validate inputs**: Check required parameters in Handle()
5. **Use context**: Respect cancellation and timeouts
6. **Log appropriately**: Don't log sensitive data (API keys, tokens)
7. **Keep tools focused**: Each tool should do one thing well

## Testing

Run the framework tests:

```bash
go test ./framework -v
```

## Cross-Platform Builds

Build for different platforms using Go's cross-compilation:

```bash
# Build for Linux (AMD64)
GOOS=linux GOARCH=amd64 go build -o my-server-linux-amd64 .

# Build for macOS (ARM64 - Apple Silicon)
GOOS=darwin GOARCH=arm64 go build -o my-server-darwin-arm64 .

# Build for Windows
GOOS=windows GOARCH=amd64 go build -o my-server.exe .
```

## Stripping Debug Symbols

For production binaries, strip debugging symbols to reduce size:

```bash
# Linux/macOS
go build -ldflags="-s -w" -o my-server .

# Windows
go build -ldflags="-s -w" -o my-server.exe .
```

## Safety Specification

The framework implements the [MCP Safety Reporting Specification](docs/MCP-safety-reporting-spec.md). This specification defines:

- Standard safety metadata fields
- Risk classification guidelines
- Impact scope definitions
- Resource cost calculation
- Integration with MCP Bridges

## Contributing

When contributing to the framework:

1. Maintain backward compatibility
2. Add tests for new features
3. Update documentation
4. Follow Go best practices
5. Ensure EnforcerProfile support for any new interfaces

## License

This project is licensed under the Functional Source License, Version 1.1, ALv2 Future License.

Copyright 2026 Karl Dane

See LICENSE file for full terms.

## References

- [MCP Protocol](https://modelcontextprotocol.io/) - Model Context Protocol specification
- [mcp-go](https://github.com/mark3labs/mcp-go) - Go implementation of MCP
