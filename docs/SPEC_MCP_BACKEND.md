# MCP Backend Development Specification

> This document lives in `mcp-framework/docs/SPEC_MCP_BACKEND.md`.
> It is the canonical reference for building new MCP backends using the framework.

---

## Overview

An MCP backend is a standalone Go binary that:

- Implements one or more MCP tools via the `framework.ToolHandler` interface
- Self-reports safety metadata via `GetEnforcerProfile()` on every tool
- Is spawned and managed by the MCP Bridge as a subprocess per user session
- Can be run directly from GitHub with `go run` for fast iteration
- Produces a stripped, reproducible binary via `make`

This spec codifies the conventions already established across `slack-mcp`, `newrelic-mcp`, `appscan-asoc-mcp`, and `oracle-mcp`.

---

## Repository Structure

```
my-mcp/
├── main.go                  # Entry point only — flag parsing, server init, Start()
├── Makefile                 # Standard targets (see Makefile section)
├── README.md                # Standard README format (see README section)
├── LICENSE                  # Functional Source License 1.1, ALv2 Future
├── go.mod
├── go.sum
├── internal/
│   ├── config/              # Config struct, env var parsing, flag binding
│   │   ├── config.go
│   │   └── config_test.go
│   ├── client/              # API client for the target service (if applicable)
│   │   ├── client.go
│   │   └── client_test.go
│   ├── tools/               # One file per tool group, plus shared helpers
│   │   ├── tools.go         # Tool registration helper / shared types
│   │   ├── <group>.go       # e.g. scan_tools.go, query_tools.go
│   │   └── <group>_test.go
│   └── normalize/           # Optional: response normalisation / formatting
│       ├── normalize.go
│       └── normalize_test.go
└── devdocs/
    ├── SPEC_<NAME>_BACKEND.md  # Backend-specific requirements, phases, API notes
    └── AGENTS.md               # Context for AI agents: decisions, gotchas, current status
```

**Rules:**
- `main.go` must contain no business logic — it wires config, registers tools, and calls `server.Start()`
- All business logic lives under `internal/`
- No package outside `internal/` may import `internal/` packages (enforced by Go module boundaries)
- Each `internal/` package must have its own `_test.go`

---

## main.go Pattern

```go
package main

import (
    "flag"
    "log"

    "github.com/karldane/mcp-framework/framework"
    "github.com/karldane/my-mcp/internal/config"
    "github.com/karldane/my-mcp/internal/tools"
)

func main() {
    cfg := config.FromEnv()

    writeEnabled := flag.Bool("write-enabled", false, "Enable write/mutating tools")
    readOnly     := flag.Bool("readonly", false, "Disable all mutating tools")
    flag.Parse()

    cfg.WriteEnabled = *writeEnabled
    cfg.ReadOnly     = *readOnly

    server := framework.NewServer("my-mcp", "1.0.0")
    tools.Register(server, cfg)

    if err := server.Initialize(); err != nil {
        log.Fatalf("failed to initialise: %v", err)
    }
    server.Start()
}
```

**Notes:**
- Flags and env vars are both supported; flags override env vars
- `--readonly` disables mutating tools (tools remain listed but return an error on execution)
- `--write-enabled` is the opt-in for destructive or state-changing operations
- Both flags may coexist; `--readonly` takes precedence

---

## Config Pattern

```go
// internal/config/config.go
package config

import "os"

type Config struct {
    BaseURL      string
    APIKey       string
    Timeout      int
    WriteEnabled bool
    ReadOnly     bool
}

func FromEnv() *Config {
    return &Config{
        BaseURL: os.Getenv("MY_SERVICE_BASE_URL"),
        APIKey:  os.Getenv("MY_SERVICE_API_KEY"),
        Timeout: 30,
    }
}
```

- All env vars must be documented in README with Required/Default columns
- Secrets must never be logged, even at debug level
- Composite keys (e.g. `KEYID:SECRET`) should be split in `FromEnv()`, not in tools

---

## Tool Implementation

Every tool must implement the full `framework.ToolHandler` interface:

```go
type MyTool struct {
    client *client.Client
    cfg    *config.Config
}

func (t *MyTool) Name() string        { return "my_tool_name" }
func (t *MyTool) Description() string { return "One clear sentence describing what this tool does." }

func (t *MyTool) Schema() mcp.ToolInputSchema {
    return mcp.ToolInputSchema{
        Type: "object",
        Properties: map[string]interface{}{
            "param_one": map[string]interface{}{
                "type":        "string",
                "description": "What this parameter does.",
            },
        },
        Required: []string{"param_one"},
    }
}

func (t *MyTool) Handle(ctx context.Context, args map[string]interface{}) (string, error) {
    val, ok := args["param_one"].(string)
    if !ok || val == "" {
        return "", fmt.Errorf("param_one is required")
    }
    // ... call client, format response
    return result, nil
}

func (t *MyTool) GetEnforcerProfile() *framework.EnforcerProfile {
	return framework.NewEnforcerProfile(
		framework.WithRisk(framework.RiskLow),
		framework.WithImpact(framework.ImpactRead),
		framework.WithResourceCost(3),
		framework.WithPII(false),
		framework.WithIdempotent(true),
		framework.WithApprovalReq(false),
	)
}
```

**Rules:**
- `Name()` must be snake_case and globally unique within the backend
- `Description()` is shown to the LLM — it must be precise and unambiguous
- All required parameters must be validated at the top of `Handle()` before any I/O
- `GetEnforcerProfile()` is **mandatory** — the framework will not register a tool without it
- Never return raw stack traces from `Handle()` — wrap errors with context

---

## ToolHandler Interface (Current)

The framework uses a `ToolResult` envelope instead of raw strings:

```go
type ToolHandler interface {
	Name() string
	Description() string
	Schema() mcp.ToolInputSchema
	Handle(ctx framework.CallContext, args map[string]interface{}) (framework.ToolResult, error)
	EnforcerProfile(args map[string]interface{}) *framework.EnforcerProfile
}
```

Use the constructors in `framework` to build responses:

```go
func (t *MyTool) Handle(ctx framework.CallContext, args map[string]interface{}) (framework.ToolResult, error) {
	val, ok := args["param_one"].(string)
	if !ok || val == "" {
		return framework.ToolResult{}, fmt.Errorf("param_one is required")
	}
	// ... call client, format response
	
	// For text responses:
	return framework.TextResult("success: " + result), nil
	
	// For structured data (rows):
	return framework.DataResult(rows), nil
	
	// For errors:
	return framework.ErrorResult("failed: " + err.Error()), nil
}
```

### Migration from Legacy Interface

If your tool uses the old `(string, error)` signature, wrap it at registration:

```go
// In main.go or tools registration
type LegacyTool struct{ ... }

func (t *LegacyTool) Handle(ctx context.Context, args map[string]interface{}) (string, error) {
	// old implementation
	return "result", nil
}

// Register wrapped:
server.RegisterTool(framework.WrapLegacy(&LegacyTool{}))
```

The wrapper automatically converts the string to `ToolResult{RawText: ...}` and applies PII scanning when enabled.

### EnforcerProfile Quick Reference

| Field | Values | Default | Notes |
|---|---|---|---|
| `Risk` | Low / Med / High / Critical | Med | Read-only → Low; write → High; destructive → Critical |
| `Impact` | Read / Write / Delete / Admin | Read | Matches the worst-case effect of the tool |
| `ResourceCost` | 1–10 | 5 | Estimate API call weight; bulk/aggregate ops score higher |
| `PII` | bool | true | Default true — assume sensitive until proven otherwise |
| `Idempotent` | bool | false | True only if calling twice has identical effect |
| `ApprovalReq` | bool | false | True for write/delete tools that should require HITL |
| `PIILevel` | none / filtered / partial / raw | (empty) | Set by framework PII pipeline — for bridge policies |

**Note:** `EnforcerProfile` now takes an `args map[string]interface{}` parameter and is called twice:
1. At `tools/list` time with `args = nil` (return worst-case profile)
2. At `tools/call` time with real args (may return accurate profile)

---

## PII Scanning (Optional)

> Requires: `go get github.com/karldane/go-presidio` — imported as `presidio` package

The framework can automatically scan and redact PII from tool responses. Enable per server:

```go
server := framework.NewServerWithConfig(&framework.Config{
	Name:        "my-server",
	Version:     "1.0.0",
	PIIScanEnabled: true,
	PIIConfig: &framework.PIIPipelineConfig{
		HMACKeyEnv:      "PRESIDIO_HMAC_KEY",      // For hash/pseudonymise
		MinConfidence:  0.5,
		DefaultOperator: "redact",                // redact | hash | mask | pseudonymise
		EntityOperators: map[string]string{
			"EMAIL_ADDRESS": "hash",               // Override per entity type
		},
		SampleSize: 20,                         // Rows to sample per column
	},
})
```

### Structured Data Responses

For tabulated data (e.g. query results), return `framework.DataResult(rows)`:

```go
func (t *QueryTool) Handle(ctx framework.CallContext, args map[string]interface{}) (framework.ToolResult, error) {
	rows, err := t.client.Query(args["sql"].(string))
	if err != nil {
		return framework.ErrorResult(err.Error()), nil
	}
	return framework.DataResult(rows), nil  // []map[string]interface{}
}
```

The PII pipeline processes each column, applies configured operators, and populates `ResultMeta.ColumnReports`.

### Column Hints (Oracle Backend)

If your backend has schema knowledge, provide type hints to skip unnecessary scanning:

```go
hints := map[string]presidio.ColumnHint{
	"email":     {ScanPolicy: presidio.ScanPolicyFull},
	"created":  {ScanPolicy: presidio.ScanPolicySafe},  // Skip scan — safe type
	"blob_data": {ScanPolicy: presidio.ScanPolicyStrip}, // Strip binary
}
return framework.ToolResult{
	Data:        rows,
	ColumnHints: hints,
}
```

---

## Write/Mutating Tool Pattern

Tools that modify state must respect the `ReadOnly` flag:

```go
func (t *MyWriteTool) Handle(ctx framework.CallContext, args map[string]interface{}) (framework.ToolResult, error) {
	if t.cfg.ReadOnly {
		return framework.ErrorResultf("this tool is disabled in read-only mode"), nil
	}
	if !t.cfg.WriteEnabled {
		return framework.ErrorResultf("write operations require --write-enabled flag"), nil
	}
	// ... proceed
	return framework.TextResult("success"), nil
}
```

Register write tools unconditionally — they must appear in `tools/list` so the bridge
can enforce policy on them. The guard lives inside `Handle()`, not at registration time.
The exception is if a tool is so dangerous that even listing it is undesirable; in that
case, gate registration on `cfg.WriteEnabled`.

---

## Tool Registration

```go
// internal/tools/tools.go
package tools

import (
    "github.com/karldane/mcp-framework/framework"
    "github.com/karldane/my-mcp/internal/client"
    "github.com/karldane/my-mcp/internal/config"
)

func Register(server *framework.Server, cfg *config.Config) {
    c := client.New(cfg)

    server.RegisterTool(&ReadTool{client: c, cfg: cfg})
    server.RegisterTool(&WriteTool{client: c, cfg: cfg})
    // ...
}
```

---

## Testing Requirements

Every backend must have passing tests covering:

1. **EnforcerProfile accuracy** — assert that each tool returns the correct risk/impact values
2. **Schema validation** — assert required fields are declared in `Schema().Required`
3. **Handle validation** — assert that missing required args return an error without making I/O
4. **Handle success** — assert correct output with a mock client
5. **Handle errors** — assert graceful error wrapping when the client returns an error
6. **ReadOnly guard** — assert write tools return the expected error when `cfg.ReadOnly = true`

Minimum coverage targets (enforced by CI):

| Package | Target |
|---|---|
| `internal/tools` | 80% |
| `internal/config` | 60% |
| `internal/client` | 60% |
| `internal/normalize` | 90% |

Run tests:
```bash
make test
# or
go test ./... -v
```

---

## Makefile (Standard)

All backends use a standard Makefile. Copy this verbatim and substitute `BINARY_NAME` and the `test` target's package path.

```makefile
# <Service Name> MCP Server Makefile

BINARY_NAME=my-mcp
BUILD_DIR=.
LDFLAGS=-ldflags="-s -w" -trimpath

# Default target - downloads dependencies and builds
.PHONY: all
all: deps build

# Download and verify dependencies
.PHONY: deps
deps:
	@echo "Downloading dependencies..."
	@GOPROXY=direct GOSUMDB=off go mod tidy
	@echo "Dependencies ready"

# Build the binary
.PHONY: build
build:
	@echo "Building $(BINARY_NAME)..."
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) .
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)"
	@du -h $(BUILD_DIR)/$(BINARY_NAME) | cut -f1

# Build for multiple platforms
.PHONY: build-all
build-all: deps build-linux build-darwin build-windows

.PHONY: build-linux
build-linux:
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 .

.PHONY: build-darwin
build-darwin:
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 .

.PHONY: build-windows
build-windows:
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe .

# Run tests
# Adjust the package path(s) to match your backend's layout, e.g.:
#   go test ./internal/tools ./internal/config -v
#   go test ./mypackage -v
.PHONY: test
test:
	go test ./internal/... -v

# Clean build artifacts
.PHONY: clean
clean:
	rm -f $(BUILD_DIR)/$(BINARY_NAME)
	rm -f $(BUILD_DIR)/$(BINARY_NAME)-*

# Install locally
.PHONY: install
install: build
	go install $(LDFLAGS) .

# Show help
.PHONY: help
help:
	@echo "<Service Name> MCP Server"
	@echo ""
	@echo "Usage:"
	@echo "  make              - Download dependencies and build binary"
	@echo "  make deps         - Download and verify dependencies"
	@echo "  make build        - Build the binary"
	@echo "  make build-all    - Build for all platforms (Linux, macOS, Windows)"
	@echo "  make test         - Run tests"
	@echo "  make clean        - Remove build artifacts"
	@echo "  make install      - Install binary to GOPATH/bin"
	@echo "  make help         - Show this help message"
```

**Notes:**
- `GOPROXY=direct GOSUMDB=off` in `deps` is required when `go.mod` contains a `replace` directive pointing to a local path (e.g. the local mcp-framework during development). Without it, `go mod tidy` will attempt to resolve the replaced module from the internet and fail.
- Cross-platform binaries are output to `BUILD_DIR` (`.` by default), not a `dist/` subdirectory.
- `build-all` targets linux/amd64, darwin/arm64, and windows/amd64 only. Add arm64 Linux or amd64 Darwin if your deployment requires it.
- The `test` target must reference the specific package paths that contain tests, not `./...`. Using `./...` will also attempt to build `main.go` as a test binary, which fails without the binary's runtime env vars.

### Why `-trimpath -ldflags "-s -w"`

| Flag | Effect |
|---|---|
| `-trimpath` | Removes all local filesystem paths from the binary (prevents leaking developer machine paths in stack traces) |
| `-s` | Strips the symbol table |
| `-w` | Strips DWARF debug info |
| Combined | ~30% smaller binary, no embedded path leakage |

---

## Direct Run from GitHub

Every backend must be runnable without cloning:

```bash
# Run directly (latest main)
go run github.com/karldane/my-mcp@latest

# Run a specific version
go run github.com/karldane/my-mcp@v1.2.0

# Install to PATH
go install github.com/karldane/my-mcp@latest
```

**Requirements for this to work:**
- `main` package must be at the repository root (not in `cmd/`)
- `go.mod` module path must exactly match the GitHub URL
- All dependencies must be committed to `go.sum`
- No `replace` directives in `go.mod` that point to local paths
- The repo must be public

Test this works before tagging a release:
```bash
cd /tmp && go run github.com/karldane/my-mcp@latest --help
```

---

## Cross-Platform Builds

Because MCP backends use **pure Go with no CGo**, cross-compilation works out of the box from any single platform. No additional toolchain is needed.

### Building Locally

```bash
make build-all
# Produces dist/ with binaries for all 5 targets
```

### GitHub Actions Release Pipeline

Add `.github/workflows/release.yml` to automate release binaries:

```yaml
name: Release

on:
  push:
    tags: ["v*"]

jobs:
  build:
    runs-on: ubuntu-latest
    strategy:
      matrix:
        include:
          - goos: linux   
            goarch: amd64
          - goos: linux   
            goarch: arm64
          - goos: darwin  
            goarch: amd64
          - goos: darwin  
            goarch: arm64
          - goos: windows 
            goarch: amd64

    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.22"

      - name: Build
        env:
          GOOS: ${{ matrix.goos }}
          GOARCH: ${{ matrix.goarch }}
        run: |
          EXT=""
          [ "$GOOS" = "windows" ] && EXT=".exe"
          go build -trimpath -ldflags "-s -w -X main.version=${{ github.ref_name }}" \
            -o dist/${{ github.event.repository.name }}-${GOOS}-${GOARCH}${EXT} .

      - uses: actions/upload-artifact@v4
        with:
          name: ${{ matrix.goos }}-${{ matrix.goarch }}
          path: dist/

  release:
    needs: build
    runs-on: ubuntu-latest
    steps:
      - uses: actions/download-artifact@v4
        with:
          path: dist/
          merge-multiple: true

      - uses: softprops/action-gh-release@v2
        with:
          files: dist/*
```

**Key points:**
- No special build environment needed — `ubuntu-latest` can cross-compile to macOS and Windows
- CGo must be disabled if any dependency pulls in C code; for pure-Go backends this is not needed
- The matrix approach builds all targets in parallel — total pipeline time ~2 minutes
- `softprops/action-gh-release` attaches all binaries to the GitHub release automatically

---

## README Template

Every backend README must follow this structure:

```markdown
# <Service Name> MCP Server

One sentence describing what service this connects to and what it provides.

## Features
- Bullet list of capability groups (not individual tools)

## Safety Features
- `GetEnforcerProfile()` on every tool
- `--readonly` flag behaviour
- `--write-enabled` requirement for mutating tools

## Requirements
- Go version
- Credentials / account type needed

## Installation
make build / make install / go install ... @latest

## Configuration

| Variable | Description | Required | Default |
|---|---|---|---|

## CLI Flags

| Flag | Description |
|---|---|

## Available Tools

Group tools by category. For each group, use a markdown table:

| Tool | Description | Risk | Impact | Approval |
|---|---|---|---|---|

## Testing
make test / go test ./internal/... -v

## License
Functional Source License, Version 1.1, ALv2 Future License.
Copyright <year> Karl Dane
```

## devdocs/ Convention

Every backend must have a `devdocs/` directory at the repo root. This directory is the source of truth for AI agents and developers resuming work on the backend.

```
devdocs/
├── SPEC_<NAME>_BACKEND.md   # What to build: requirements, phases, API notes, constraints
└── AGENTS.md                # Working session state: what's done, what's next, key decisions, gotchas
```

**`SPEC_<NAME>_BACKEND.md`** is written at project start and updated when requirements change. It describes what the backend does, its phases (if phased), and any service-specific constraints (auth model, API quirks, env var semantics).

**`AGENTS.md`** is a living document updated at the end of every working session. It must contain:

- **Accomplished** — what has been completed (files, tests, decisions)
- **In Progress / Just Started** — work that was interrupted mid-flight
- **Not Yet Done** — remaining tasks with enough detail to resume without re-reading the full conversation history
- **Discoveries** — API quirks, dependency version constraints, or environment facts that are not obvious from the code
- **Relevant files** — absolute paths to key files so an agent can jump directly to them

An agent starting on a backend must read `devdocs/AGENTS.md` before doing anything else.

---

## Checklist for a New Backend

Before opening a PR or tagging a release:

- [ ] `main.go` contains only wiring — no business logic
- [ ] All tools implement `GetEnforcerProfile()` with accurate values
- [ ] `--readonly` disables mutating tools without panicking
- [ ] `--write-enabled` is required for any tool with `Impact >= Write`
- [ ] All required args validated at top of `Handle()` before I/O
- [ ] Tests cover EnforcerProfile, schema, Handle validation, Handle success, errors
- [ ] `make test` passes with no race conditions
- [ ] `make build-all` produces binaries for all 3 platform targets
- [ ] `go run github.com/karldane/<repo>@latest` works from a clean temp directory
- [ ] No local paths embedded: `strings -a <binary> | grep /home` returns nothing
- [ ] README follows the standard template with env var table and tool table
- [ ] `devdocs/AGENTS.md` exists and captures: current status, key decisions, known gotchas, and what's next
