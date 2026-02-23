package logs

import (
	"context"
	"fmt"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

// larkAdapter wraps our Logger to satisfy larkcore.Logger interface,
// so Lark SDK internal logging is routed through Friday's unified log pipeline.
type larkAdapter struct {
	l Logger
}

var _ larkcore.Logger = (*larkAdapter)(nil)

// NewLarkLogger returns a Lark SDK Logger backed by the given Logger.
func NewLarkLogger(l Logger) larkcore.Logger {
	return &larkAdapter{l: l}
}

func (a *larkAdapter) Debug(ctx context.Context, args ...any) {
	a.l.CtxDebug(ctx, "[lark-sdk] %s", fmt.Sprint(args...))
}

func (a *larkAdapter) Info(ctx context.Context, args ...any) {
	a.l.CtxInfo(ctx, "[lark-sdk] %s", fmt.Sprint(args...))
}

func (a *larkAdapter) Warn(ctx context.Context, args ...any) {
	a.l.CtxWarn(ctx, "[lark-sdk] %s", fmt.Sprint(args...))
}

func (a *larkAdapter) Error(ctx context.Context, args ...any) {
	a.l.CtxError(ctx, "[lark-sdk] %s", fmt.Sprint(args...))
}
