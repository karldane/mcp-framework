package framework

import (
	"context"
)

// CallContext carries both the standard context and typed caller identity.
// It embeds context.Context so it satisfies all existing context usage.
type CallContext struct {
	context.Context

	UserID string

	TrustLevel int

	BackendID string

	SessionID string
}

func Background() CallContext {
	return CallContext{Context: context.Background()}
}

func (c CallContext) WithContext(ctx context.Context) CallContext {
	c.Context = ctx
	return c
}
