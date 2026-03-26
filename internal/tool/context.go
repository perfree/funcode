package tool

import "context"

type callIDContextKey struct{}

func WithCallID(ctx context.Context, callID string) context.Context {
	if callID == "" {
		return ctx
	}
	return context.WithValue(ctx, callIDContextKey{}, callID)
}

func CallIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if value, ok := ctx.Value(callIDContextKey{}).(string); ok {
		return value
	}
	return ""
}
