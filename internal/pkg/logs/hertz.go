package logs

import (
	"context"
	"fmt"
	"io"

	"github.com/cloudwego/hertz/pkg/common/hlog"
)

// hlogAdapter wraps our Logger to satisfy hertz's hlog.FullLogger interface,
// so hertz internal logging is routed through Friday's unified log pipeline.
type hlogAdapter struct {
	l Logger
}

var _ hlog.FullLogger = (*hlogAdapter)(nil)

// NewHlogLogger returns a hertz FullLogger backed by the given Logger.
func NewHlogLogger(l Logger) hlog.FullLogger {
	return &hlogAdapter{l: l}
}

// --- Logger (non-format) ---

func (a *hlogAdapter) Trace(v ...interface{})  { a.l.Debug(fmt.Sprint(v...)) }
func (a *hlogAdapter) Debug(v ...interface{})  { a.l.Debug(fmt.Sprint(v...)) }
func (a *hlogAdapter) Info(v ...interface{})   { a.l.Info(fmt.Sprint(v...)) }
func (a *hlogAdapter) Notice(v ...interface{}) { a.l.Info(fmt.Sprint(v...)) }
func (a *hlogAdapter) Warn(v ...interface{})   { a.l.Warn(fmt.Sprint(v...)) }
func (a *hlogAdapter) Error(v ...interface{})  { a.l.Error(fmt.Sprint(v...)) }
func (a *hlogAdapter) Fatal(v ...interface{})  { a.l.Fatal(fmt.Sprint(v...)) }

// --- FormatLogger ---

func (a *hlogAdapter) Tracef(format string, v ...interface{})  { a.l.Debug(format, v...) }
func (a *hlogAdapter) Debugf(format string, v ...interface{})  { a.l.Debug(format, v...) }
func (a *hlogAdapter) Infof(format string, v ...interface{})   { a.l.Info(format, v...) }
func (a *hlogAdapter) Noticef(format string, v ...interface{}) { a.l.Info(format, v...) }
func (a *hlogAdapter) Warnf(format string, v ...interface{})   { a.l.Warn(format, v...) }
func (a *hlogAdapter) Errorf(format string, v ...interface{})  { a.l.Error(format, v...) }
func (a *hlogAdapter) Fatalf(format string, v ...interface{})  { a.l.Fatal(format, v...) }

// --- CtxLogger ---

func (a *hlogAdapter) CtxTracef(ctx context.Context, format string, v ...interface{}) {
	a.l.CtxDebug(ctx, format, v...)
}
func (a *hlogAdapter) CtxDebugf(ctx context.Context, format string, v ...interface{}) {
	a.l.CtxDebug(ctx, format, v...)
}
func (a *hlogAdapter) CtxInfof(ctx context.Context, format string, v ...interface{}) {
	a.l.CtxInfo(ctx, format, v...)
}
func (a *hlogAdapter) CtxNoticef(ctx context.Context, format string, v ...interface{}) {
	a.l.CtxInfo(ctx, format, v...)
}
func (a *hlogAdapter) CtxWarnf(ctx context.Context, format string, v ...interface{}) {
	a.l.CtxWarn(ctx, format, v...)
}
func (a *hlogAdapter) CtxErrorf(ctx context.Context, format string, v ...interface{}) {
	a.l.CtxError(ctx, format, v...)
}
func (a *hlogAdapter) CtxFatalf(ctx context.Context, format string, v ...interface{}) {
	a.l.CtxFatal(ctx, format, v...)
}

// --- Control ---

func (a *hlogAdapter) SetLevel(level hlog.Level) {
	switch level {
	case hlog.LevelTrace, hlog.LevelDebug:
		a.l.SetLevel(DebugLevel)
	case hlog.LevelInfo, hlog.LevelNotice:
		a.l.SetLevel(InfoLevel)
	case hlog.LevelWarn:
		a.l.SetLevel(WarnLevel)
	case hlog.LevelError:
		a.l.SetLevel(ErrorLevel)
	case hlog.LevelFatal:
		a.l.SetLevel(FatalLevel)
	}
}

// SetOutput is a no-op; output is managed by our Logger's own configuration.
func (a *hlogAdapter) SetOutput(_ io.Writer) {}
