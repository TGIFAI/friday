package cli

import (
	"context"

	"github.com/tgifai/friday/internal/agent/session"
)

type ctxKey struct{}

func WithSession(ctx context.Context, sess *session.Session) context.Context {
	return context.WithValue(ctx, ctxKey{}, sess)
}

func SessionFromCtx(ctx context.Context) *session.Session {
	s, _ := ctx.Value(ctxKey{}).(*session.Session)
	return s
}
