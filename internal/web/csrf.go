package web

import "context"

type csrfKey struct{}

// WithCSRF stores the per-session CSRF token in the context (set by middleware).
func WithCSRF(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, csrfKey{}, token)
}

// CSRF returns the CSRF token from the context (empty if none).
func CSRF(ctx context.Context) string {
	s, _ := ctx.Value(csrfKey{}).(string)
	return s
}
