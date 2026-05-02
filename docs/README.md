# mcp-framework Documentation

This directory contains all the documentation you need to build MCP backends with mcp-framework.

## Quick Start

For a minimal working example, see the `example/` directory at the repository root.

## Documentation Index

| Document | Description |
|----------|-------------|
| [SPEC_MCP_BACKEND.md](./SPEC_MCP_BACKEND.md) | Complete specification for building MCP backends |
| [MCP-safety-reporting-spec.md](./MCP-safety-reporting-spec.md) | Safety metadata and self-reporting specification |

## What's Here

### SPEC_MCP_BACKEND.md
The canonical reference for building MCP backends. Covers:
- Repository structure
- Config and tool patterns
- ToolHandler interface (all 5 required methods)
- **Migration guide** for bringing older backends up to date
- PII scanning
- Makefile and cross-platform builds
- README template

### MCP-safety-reporting-spec.md
Details the self-reporting mechanism for safety metadata:
- EnforcerProfile fields (risk, impact, resource cost, etc.)
- How profiles are transmitted to MCP Bridge
- When profiles are evaluated (tools/list vs tools/call)

## For New Backends

1. Read `SPEC_MCP_BACKEND.md` from start to finish
2. Copy the Makefile and README template from the spec
3. Use `example/main.go` as your main.go template

## For Existing Backends

If your backend was built against an older mcp-framework version:

1. Read the **Migration Guide** section in `SPEC_MCP_BACKEND.md`
2. Update to v0.2.8+ for BaseTool and --scan support
3. Add `framework.BaseTool` embedding to all tool structs
4. Test with `--scan` flag

## Version History

| Version | Key Changes |
|---------|--------------|
| v0.2.8 | BaseTool struct, --scan mode support |
| v0.2.7 | ScanModeOutput for --scan JSON format |
| v0.2.6 | PII pipeline improvements |
| v0.2.5 | OutputSchema required in interface |

## Getting Help

- Issues: https://github.com/karldane/mcp-framework/issues
- Examples: `example/` directory