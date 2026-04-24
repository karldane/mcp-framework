package framework

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
)

type OldToolHandler interface {
	Name() string
	Description() string
	Schema() mcp.ToolInputSchema
	Handle(ctx context.Context, args map[string]interface{}) (string, error)
	GetEnforcerProfile() *EnforcerProfile
}

type legacyWrapper struct {
	inner OldToolHandler
}

func WrapLegacy(h OldToolHandler) ToolHandler {
	return &legacyWrapper{inner: h}
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

func (l *legacyWrapper) GetEnforcerProfile() *EnforcerProfile {
	return l.inner.GetEnforcerProfile()
}

func (l *legacyWrapper) Handle(ctx context.Context, args map[string]interface{}) (ToolResult, error) {
	text, err := l.inner.Handle(ctx, args)
	if err != nil {
		return ToolResult{RawText: text, IsError: true}, err
	}
	return ToolResult{RawText: text}, nil
}
