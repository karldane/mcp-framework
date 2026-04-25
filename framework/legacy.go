package framework

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
)

// OldToolHandler is the previous ToolHandler interface, retained for migration.
// Backends implementing this interface can be wrapped with WrapLegacy.
type OldToolHandler interface {
	Name() string
	Description() string
	Schema() mcp.ToolInputSchema
	Handle(ctx context.Context, args map[string]interface{}) (string, error)
	GetEnforcerProfile() *EnforcerProfile
}

// WrapLegacy wraps an OldToolHandler as a current ToolHandler.
// The wrapped tool's string output is placed in ToolResult.RawText.
func WrapLegacy(h OldToolHandler) ToolHandler {
	return &legacyWrapper{inner: h}
}

type legacyWrapper struct {
	inner OldToolHandler
}

func (l *legacyWrapper) Name() string {
	return l.inner.Name()
}

func (l *legacyWrapper) Description() string {
	return l.inner.Description()
}

func (l *legacyWrapper) Schema() mcp.ToolInputSchema {
	return l.inner.Schema()
}

func (l *legacyWrapper) EnforcerProfile(args map[string]interface{}) *EnforcerProfile {
	// Legacy tools always return static profile, ignore args
	return l.inner.GetEnforcerProfile()
}

func (l *legacyWrapper) Handle(ctx CallContext, args map[string]interface{}) (ToolResult, error) {
	text, err := l.inner.Handle(ctx.Context, args)
	if err != nil {
		return ToolResult{RawText: text, IsError: true}, err
	}
	return ToolResult{RawText: text}, nil
}
