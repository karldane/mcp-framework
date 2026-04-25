package framework

import (
	"context"
	"testing"
)

func TestCallContextBackground(t *testing.T) {
	cc := Background()
	if cc.Context != context.Background() {
		t.Error("Background() should embed context.Background()")
	}
	if cc.UserID != "" {
		t.Error("Background() should have empty UserID")
	}
	if cc.TrustLevel != 0 {
		t.Error("Background() should have zero TrustLevel")
	}
	if cc.BackendID != "" {
		t.Error("Background() should have empty BackendID")
	}
	if cc.SessionID != "" {
		t.Error("Background() should have empty SessionID")
	}
}

func TestCallContextWithContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cc := CallContext{
		Context:    ctx,
		UserID:     "user-123",
		TrustLevel: 5,
		BackendID:  "oracle-mcp",
		SessionID:  "sess-abc",
	}

	updated := cc.WithContext(context.Background())

	if updated.UserID != "user-123" {
		t.Errorf("WithContext should preserve UserID, got %q", updated.UserID)
	}
	if updated.TrustLevel != 5 {
		t.Errorf("WithContext should preserve TrustLevel, got %d", updated.TrustLevel)
	}
	if updated.BackendID != "oracle-mcp" {
		t.Errorf("WithContext should preserve BackendID, got %q", updated.BackendID)
	}
	if updated.SessionID != "sess-abc" {
		t.Errorf("WithContext should preserve SessionID, got %q", updated.SessionID)
	}
	if updated.Context != context.Background() {
		t.Error("WithContext should replace underlying context")
	}
}

func TestCallContextEmbedsContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cc := CallContext{Context: ctx}

	select {
	case <-cc.Done():
		t.Error("CallContext should not be done before cancel")
	default:
		// expected
	}

	if cc.Err() != nil {
		t.Error("un-cancelled context should have no error")
	}
}

func TestCallContextValue(t *testing.T) {
	cc := CallContext{Context: context.Background()}

	result := cc.Value("key")
	if result != nil {
		t.Error("Value should return nil for unset key")
	}

	ctx := context.WithValue(context.Background(), "key", "value")
	cc = cc.WithContext(ctx)

	result = cc.Value("key")
	if result != "value" {
		t.Errorf("Value should return stored value, got %v", result)
	}
}
