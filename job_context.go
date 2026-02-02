package taskbus

import "context"

type jobContextKey struct{}

type jobContextValue struct {
	key string
}

func withJobKey(ctx context.Context, key string) context.Context {
	if ctx == nil || key == "" {
		return ctx
	}
	return context.WithValue(ctx, jobContextKey{}, jobContextValue{key: key})
}

func jobKeyFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(jobContextKey{}).(jobContextValue); ok {
		return v.key
	}
	return ""
}
