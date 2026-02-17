package agent

import "context"

type contextKey int

const (
	sessionIDKey contextKey = iota
	delegationDepthKey
	emitKey
)

func ContextWithSessionID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, sessionIDKey, id)
}

func SessionIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(sessionIDKey).(string); ok {
		return v
	}
	return ""
}

func ContextWithDelegationDepth(ctx context.Context, depth int) context.Context {
	return context.WithValue(ctx, delegationDepthKey, depth)
}

func DelegationDepthFromContext(ctx context.Context) int {
	if v, ok := ctx.Value(delegationDepthKey).(int); ok {
		return v
	}
	return 0
}

func ContextWithEmit(ctx context.Context, emit func(Event)) context.Context {
	return context.WithValue(ctx, emitKey, emit)
}

func EmitFromContext(ctx context.Context) func(Event) {
	if v, ok := ctx.Value(emitKey).(func(Event)); ok {
		return v
	}
	return nil
}
