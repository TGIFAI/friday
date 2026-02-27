package session

import "context"

type ctxKey struct{}

func WithContext(ctx context.Context, sess *Session) context.Context {
	return context.WithValue(ctx, ctxKey{}, sess)
}

func ExtractFromCtx(ctx context.Context) *Session {
	s, _ := ctx.Value(ctxKey{}).(*Session)
	return s
}
