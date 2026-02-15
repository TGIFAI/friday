package logs

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/fatih/color"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"gopkg.in/natefinch/lumberjack.v2"
)

type ctxKey string

const ctxKeyLogID ctxKey = "log_id"

type Options struct {
	Level      string
	Format     string
	Output     string
	File       string
	MaxSize    int
	MaxBackups int
	MaxAge     int
	Compress   bool
}

var logger Logger = newDefaultLogger()

// SetLogger sets global logger.
// Note that this method is not concurrent-safe.
func SetLogger(l Logger) {
	if l == nil {
		return
	}
	logger = l
}

// SetLogLevel sets minimum output level.
func SetLogLevel(level LogLevel) {
	logger.SetLevel(level)
}

func DefaultLogger() Logger {
	return logger
}

func Init(opts Options) error {
	l, err := newConfiguredLogger(opts)
	if err != nil {
		return err
	}
	SetLogger(l)
	return nil
}

func Debug(format string, v ...interface{}) {
	logger.Debug(format, v...)
}

func Info(format string, v ...interface{}) {
	logger.Info(format, v...)
}

func Warn(format string, v ...interface{}) {
	logger.Warn(format, v...)
}

func Error(format string, v ...interface{}) {
	logger.Error(format, v...)
}

func Fatal(format string, v ...interface{}) {
	logger.Fatal(format, v...)
}

func CtxDebug(ctx context.Context, format string, v ...interface{}) {
	logger.CtxDebug(ctx, format, v...)
}

func CtxInfo(ctx context.Context, format string, v ...interface{}) {
	logger.CtxInfo(ctx, format, v...)
}

func CtxWarn(ctx context.Context, format string, v ...interface{}) {
	logger.CtxWarn(ctx, format, v...)
}

func CtxError(ctx context.Context, format string, v ...interface{}) {
	logger.CtxError(ctx, format, v...)
}

func CtxFatal(ctx context.Context, format string, v ...interface{}) {
	logger.CtxFatal(ctx, format, v...)
}

func NewLogID() string {
	return logger.NewLogID()
}

func GetLogID(ctx context.Context) string {
	return logger.GetLogID(ctx)
}

func SetLogID(ctx context.Context, logID string) context.Context {
	return logger.SetLogID(ctx, logID)
}

func Flush() {
	logger.Flush()
}

type defaultLogger struct {
	log *logrus.Logger
}

func (l *defaultLogger) NewLogID() string {
	return uuid.New().String()
}

func (l *defaultLogger) GetLogID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	logID, _ := ctx.Value(ctxKeyLogID).(string)
	return logID
}

func (l *defaultLogger) SetLogID(ctx context.Context, logID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, ctxKeyLogID, logID)
}

func newDefaultLogger() Logger {
	log := logrus.New()
	log.SetFormatter(&customFormatter{enableColor: shouldColorizeStdout("stdout")})
	log.SetLevel(logrus.InfoLevel)
	return &defaultLogger{log: log}
}

func newConfiguredLogger(opts Options) (Logger, error) {
	log := logrus.New()

	output := strings.ToLower(strings.TrimSpace(opts.Output))
	if output == "" {
		output = "stdout"
	}
	w, err := buildWriter(opts, output)
	if err != nil {
		return nil, err
	}
	log.SetOutput(w)

	format := strings.ToLower(strings.TrimSpace(opts.Format))
	if format == "json" {
		log.SetFormatter(&logrus.JSONFormatter{})
	} else {
		log.SetFormatter(&customFormatter{enableColor: shouldColorizeStdout(output)})
	}

	log.SetLevel(parseLogLevel(opts.Level))
	return &defaultLogger{log: log}, nil
}

func buildWriter(opts Options, output string) (io.Writer, error) {
	switch output {
	case "stdout":
		return os.Stdout, nil
	case "file":
		w, err := newRotateWriter(opts)
		if err != nil {
			return nil, err
		}
		return w, nil
	case "both":
		w, err := newRotateWriter(opts)
		if err != nil {
			return nil, err
		}
		return &dualWriter{
			stdout: os.Stdout,
			file:   w,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported log output: %s", output)
	}
}

type dualWriter struct {
	stdout io.Writer
	file   io.Writer
}

func (w *dualWriter) Write(p []byte) (int, error) {
	if _, err := w.stdout.Write(p); err != nil {
		return 0, err
	}
	if _, err := w.file.Write(stripANSI(p)); err != nil {
		return 0, err
	}
	return len(p), nil
}

func newRotateWriter(opts Options) (io.Writer, error) {
	if strings.TrimSpace(opts.File) == "" {
		return nil, fmt.Errorf("log file is required when output includes file")
	}
	dir := filepath.Dir(opts.File)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create log dir failed: %w", err)
		}
	}

	maxSize := opts.MaxSize
	if maxSize <= 0 {
		maxSize = 100
	}
	maxBackups := opts.MaxBackups
	if maxBackups < 0 {
		maxBackups = 0
	}
	maxAge := opts.MaxAge
	if maxAge < 0 {
		maxAge = 0
	}

	return &lumberjack.Logger{
		Filename:   opts.File,
		MaxSize:    maxSize,
		MaxBackups: maxBackups,
		MaxAge:     maxAge,
		Compress:   opts.Compress,
	}, nil
}

func parseLogLevel(level string) logrus.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return logrus.DebugLevel
	case "warn", "warning":
		return logrus.WarnLevel
	case "error":
		return logrus.ErrorLevel
	case "fatal":
		return logrus.FatalLevel
	default:
		return logrus.InfoLevel
	}
}

func (l *defaultLogger) GetLevel() LogLevel {
	switch l.log.GetLevel() {
	case logrus.DebugLevel:
		return DebugLevel
	case logrus.InfoLevel:
		return InfoLevel
	case logrus.WarnLevel:
		return WarnLevel
	case logrus.ErrorLevel:
		return ErrorLevel
	case logrus.FatalLevel:
		return FatalLevel
	default:
		return InfoLevel
	}
}

func (l *defaultLogger) SetLevel(level LogLevel) {
	switch level {
	case DebugLevel:
		l.log.SetLevel(logrus.DebugLevel)
	case InfoLevel:
		l.log.SetLevel(logrus.InfoLevel)
	case WarnLevel:
		l.log.SetLevel(logrus.WarnLevel)
	case ErrorLevel:
		l.log.SetLevel(logrus.ErrorLevel)
	case FatalLevel:
		l.log.SetLevel(logrus.FatalLevel)
	}
}

func (l *defaultLogger) Debug(format string, v ...interface{}) {
	l.log.Debugf(format, v...)
}

func (l *defaultLogger) Info(format string, v ...interface{}) {
	l.log.Infof(format, v...)
}

func (l *defaultLogger) Warn(format string, v ...interface{}) {
	l.log.Warnf(format, v...)
}

func (l *defaultLogger) Error(format string, v ...interface{}) {
	l.log.Errorf(format, v...)
}

func (l *defaultLogger) Fatal(format string, v ...interface{}) {
	l.log.Fatalf(format, v...)
}

func (l *defaultLogger) CtxDebug(ctx context.Context, format string, v ...interface{}) {
	l.log.WithContext(ctx).Debugf(format, v...)
}

func (l *defaultLogger) CtxInfo(ctx context.Context, format string, v ...interface{}) {
	l.log.WithContext(ctx).Infof(format, v...)
}

func (l *defaultLogger) CtxWarn(ctx context.Context, format string, v ...interface{}) {
	l.log.WithContext(ctx).Warnf(format, v...)
}

func (l *defaultLogger) CtxError(ctx context.Context, format string, v ...interface{}) {
	l.log.WithContext(ctx).Errorf(format, v...)
}

func (l *defaultLogger) CtxFatal(ctx context.Context, format string, v ...interface{}) {
	l.log.WithContext(ctx).Fatalf(format, v...)
}

func (l *defaultLogger) Flush() {}

type customFormatter struct {
	enableColor bool
}

func (f *customFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	timestamp := entry.Time.Format("2006-01-02 15:04:05,000")
	level := strings.ToUpper(entry.Level.String())
	if f.enableColor {
		level = colorizeLevel(entry.Level, level)
	}

	skip := 9
	if entry.Context != nil {
		skip = 8
	}
	_, file, line, ok := runtime.Caller(skip)
	if ok {
		file = shortFilePath(file)
	}

	var logID any
	logID = ""
	if entry.Context != nil {
		if id := entry.Context.Value(ctxKeyLogID); id != nil {
			logID = id
		}
	}

	logLine := fmt.Sprintf("%s %s %s:%d %s %s\n",
		level,
		timestamp,
		file,
		line,
		logID,
		entry.Message,
	)

	return []byte(logLine), nil
}

// shortFilePath returns "dir/file.go" (two-level) when a parent directory
// exists, otherwise just "file.go".
func shortFilePath(fullPath string) string {
	dir, file := filepath.Split(fullPath)
	if dir == "" {
		return file
	}
	dir = filepath.Clean(dir)
	parent := filepath.Base(dir)
	return parent + "/" + file
}

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(p []byte) []byte {
	return ansiPattern.ReplaceAll(p, nil)
}

func shouldColorizeStdout(output string) bool {
	if output == "file" {
		return false
	}
	return !color.NoColor
}

var (
	colorDebug = color.New(color.FgCyan)
	colorInfo  = color.New(color.FgGreen)
	colorWarn  = color.New(color.FgYellow)
	colorError = color.New(color.FgRed)
)

func colorizeLevel(level logrus.Level, text string) string {
	switch level {
	case logrus.DebugLevel:
		return colorDebug.Sprint(text)
	case logrus.InfoLevel:
		return colorInfo.Sprint(text)
	case logrus.WarnLevel:
		return colorWarn.Sprint(text)
	case logrus.ErrorLevel, logrus.FatalLevel, logrus.PanicLevel:
		return colorError.Sprint(text)
	default:
		return text
	}
}
