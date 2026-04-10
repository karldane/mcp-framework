
# Developer Handbook

**Language:** Go 1.25.x preferred, standard library first

## Mission

Build Go-based MCP servers using the MCP Framework. Follow project specifications and maintain extensibility.

## Source of Truth

Always follow these documents in order:
1. **Primary spec** — Functional and architectural specification.
2. **README.md** — Repository overview and setup.
3. **MCP-safety-reporting-spec.md** — Safety metadata contract.
4. **MCP Framework docs/code** — Framework patterns.

Update specs first if implementation conflicts arise.

## Go Version Policy

Use **Go 1.25.x**:
- Module `go` version: `1.25`
- Develop/test on Go 1.25.8
- Prefer stable stdlib features
- Avoid 1.26+ features
- Prioritise clarity over cleverness

## Non-Negotiables

### 1. TDD Always (80%+ Coverage)

**TDD without exception**:
- No production code without failing test first
- **Red → Green → Refactor** for all changes
- Smallest failing test proving next increment
- Regression test for every bug
- Refactor only after green tests

**Coverage targets:**
- `internal/tools`: 80%+
- `internal/config`: 60%+
- `internal/client`: 60%+
- `internal/normalize`: 90%+

### 2. Safety Metadata Mandatory

Every tool implements `GetEnforcerProfile()` accurately:
- Risk level
- Impact scope  
- Resource cost
- PII exposure
- Idempotence
- Human approval requirement

No exceptions — functional requirement for bridge enforcement.

### 3. Readonly Mode Real

`--readonly` disables mutating tools at runtime:
- Tools visible in `tools/list`
- Fail deterministically when readonly
- Tests prove enforcement for all mutating categories
- Annotations reflect true nature even when readonly

### 4. Git Checkpoints Required

Commit at logical boundaries:
- After each green test slice
- No batching unrelated work
- Messages explain **why**, not just what
- Never large uncommitted states except mid-red

### 5. Subtask Boundaries

Use subtasks for natural splits:
- Config/startup
- Authenticated client
- Normalization models
- Tool groups (by domain)
- Readonly enforcement
- Safety verification
- Live tests (optional)

## Engineering Guidelines

### Go Style
- Standard library first
- Minimal justified dependencies
- Narrow package responsibilities
- Duplication OK until patterns stabilise
- Structured errors with stable messages
- No logging secrets/tokens/sensitive data

### Package Layout
```
my-mcp/
├── main.go                 # Wiring only
├── Makefile                # Standard targets
├── README.md               # Standard format
├── internal/
│   ├── config/             # Env/flag parsing
│   ├── client/             # Service API client
│   ├── normalize/          # Response formatting
│   ├── readonly/           # Enforcement logic
│   ├── tools/              # Tool groups
│   └── testutil/           # Mock helpers
└── docs/
    └── TOOLS.md            # Tool reference
```

### Configuration
- Support env vars + flags (flags override)
- Test parsing + precedence
- Document in README table format
- Split composite keys in `FromEnv()`

### HTTP Client
- Centralise auth/headers/retries/timeouts
- Mock with `httptest` before tools
- Preserve upstream errors
- Normalise status outcomes

## Testing Strategy

**Run frequently:**
```bash
go test ./... -count=1 -race
```

**Required coverage:**
- Config parsing/precedence
- Readonly enforcement
- Auth header generation
- Client error mapping
- Pagination
- Entity normalisation
- Tool input validation
- Tool output mapping
- Safety annotation correctness
- Bug regression tests

**Live tests:** Opt-in via env vars, skip cleanly, avoid destruction.

## Tooling Guidelines

### Naming
- No `service_` prefix (bridge adds namespace)
- Concise, domain-oriented: `list_apps`, `start_scan`, `search_findings`

### Readonly Tools
- Guard in `Handle()` — don't hide from `tools/list`
- `--readonly` takes precedence over `--write-enabled`

## Definition of Done

✅ Failing test written  
✅ Implementation passes targeted tests  
✅ Full suite passes  
✅ Safety metadata correct  
✅ Docs updated  
✅ Formatted  
✅ Committed at checkpoint  

## Working Style

1. Name subtask
2. Read relevant spec
3. Write failing test
4. Smallest passing change
5. Refactor
6. Package tests
7. Full suite
8. Commit
9. Next subtask

## Escalate When

- Tenant-specific API quirks
- Retry strategy unclear
- Binary retrieval unexpectedly complex
- Spec item deferred/promoted

## Single Runner Constraint (if applicable)

Queued status = valid outcome, not error.
