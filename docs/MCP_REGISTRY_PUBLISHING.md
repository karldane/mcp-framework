# Publishing to the MCP Registry

Lessons learned from publishing `io.github.karldane/newrelic-mcp` (55-tool New Relic MCP server).

## The `mcp-publisher` CLI

**Use the binary from GitHub releases, NOT the snap or npm packages.**

| Source | Version | Works? |
|--------|---------|--------|
| Snap (`snap install mcp-publisher`) | v1.1.0 (2025-09-24) | **No** — outdated schema validation rejects current `$schema` URLs as "deprecated" |
| npm (`npx mcp-publisher`) | v0.4.2 | **No** — this is an MCP server, not a CLI |
| GitHub releases | v1.7.9+ (2026-05-12) | **Yes** |

**Install:**
```bash
# Download latest Linux amd64 binary
curl -sL https://github.com/modelcontextprotocol/registry/releases/download/v1.7.9/mcp-publisher_linux_amd64.tar.gz \
  | tar -xzf - -C /usr/local/bin/
```

**Commands:**
- `mcp-publisher login github` — GitHub device-code auth flow
- `mcp-publisher validate server.json` — validate without publishing
- `mcp-publisher publish --server-name <name> server.json` — publish
- `mcp-publisher logout` — clear auth

**Token storage:** `~/.config/mcp-publisher/` (newer versions). Old snap stored in project `.mcpregistry_*` files. Re-login required after upgrading the binary.

## `server.json` — Current Schema (2025-12-11)

### Required Fields

| Level | Field | Notes |
|-------|-------|-------|
| Top | `$schema` | `https://static.modelcontextprotocol.io/schemas/2025-12-11/server.schema.json` |
| Top | `name` | Reverse-DNS: `namespace/server`, e.g. `io.github.user/repo` |
| Top | `description` | **Max 100 characters** (registry API enforces this) |
| Top | `version` | Exact semver, e.g. `1.0.0`. No ranges (`^`, `~`). No `latest`. |
| Package | `registryType` | `"mcpb"` for standalone binaries |
| Package | `identifier` | URL to release asset (must contain `"mcp"` — `.mcpb` extension suffices) |
| Package | `transport` | `{"type": "stdio"}` |
| Package (MCPB) | `fileSha256` | SHA-256 hex hash of the `.mcpb` file |

### Gotchas

1. **`description` ≤ 100 chars** — Our original 178-char description was rejected by the registry API
2. **Package `version` must be explicit** — The package object needs its own `"version": "1.0.0"` field, not inherited from the top-level version (even though the schema says it's optional for MCPB, the CLI serializes it and the API rejects empty strings)
3. **`$schema` must match CLI expectations** — The old snap CLI rejects any schema URL it doesn't know. Use the GitHub release binary (v1.7.9+) which understands `2025-12-11`
4. **GitHub release must exist before publishing** — The registry validates that the `.mcpb` URL is reachable
5. **MCPB identifier must contain "mcp"** — The repo name `newrelic-mcp` satisfies this, as does the `.mcpb` extension
6. **No special archive format** — `.mcpb` is just the binary renamed with a `.mcpb` extension

## Complete Publish Workflow

```bash
# 1. Build the binary
make build

# 2. Create .mcpb artifact and compute hash
cp newrelic-mcp newrelic-mcp-linux-amd64.mcpb
openssl dgst -sha256 newrelic-mcp-linux-amd64.mcpb
# Copy the SHA256 hash

# 3. Create GitHub release with the .mcpb asset
gh release create v1.0.0 newrelic-mcp-linux-amd64.mcpb \
  --title "v1.0.0" \
  --notes "Release notes here"

# 4. Update server.json:
#    - Set fileSha256 to the hash from step 2
#    - Ensure description ≤100 chars
#    - Ensure package has explicit version
#    - Ensure $schema is 2025-12-11

# 5. Auth (one time)
mcp-publisher login github

# 6. Validate then publish
mcp-publisher validate server.json
mcp-publisher publish --server-name io.github.user/repo server.json
```

## Verify Publication

- **Browse**: https://registry.modelcontextprotocol.io (search for your server)
- **API**: `curl https://registry.modelcontextprotocol.io/api/servers/io.github.user/repo`

## Example `server.json` (MCPB)

```json
{
  "$schema": "https://static.modelcontextprotocol.io/schemas/2025-12-11/server.schema.json",
  "name": "io.github.karldane/newrelic-mcp",
  "title": "New Relic MCP",
  "description": "New Relic MCP server with 55 tools for APM, alerts, dashboards, synthetics, workflows, and more.",
  "repository": {
    "url": "https://github.com/karldane/newrelic-mcp",
    "source": "github",
    "id": "1187449102"
  },
  "version": "1.0.0",
  "packages": [
    {
      "registryType": "mcpb",
      "identifier": "https://github.com/karldane/newrelic-mcp/releases/download/v1.0.0/newrelic-mcp-linux-amd64.mcpb",
      "version": "1.0.0",
      "fileSha256": "f41678277789f34aeff1f9413f07c1ec0632237584d062a97447a822de541a31",
      "transport": { "type": "stdio" },
      "environmentVariables": [
        {
          "name": "NEWRELIC_API_KEY",
          "description": "New Relic API key (User or Admin)",
          "isRequired": true,
          "isSecret": true,
          "format": "string"
        },
        {
          "name": "NEWRELIC_REGION",
          "description": "Data center region (us or eu)",
          "isRequired": false,
          "default": "us",
          "format": "string"
        }
      ],
      "packageArguments": [
        {
          "type": "named",
          "name": "-write-enabled",
          "description": "Enable write operations (create, update, delete resources)",
          "isRequired": false
        }
      ]
    }
  ]
}
```
