// Package logging provides structured logging for the server using log/slog.
//
// It supports JSON output for production (auto-indexed by Cloud Logging) and
// human-readable colorized text output for development. Log levels are configurable
// via command-line flags.
//
// Usage:
//
//	logger := logging.NewLogger(logging.Options{
//	    Level:  "info",  // Default: "info"
//	    Format: "json",  // Default: "json"
//	})
//	logger.Info("server started", "port", 8080)
//
// In RPC handlers, use LoggerWithContext to get a logger enriched with
// request context (request_id, user_id, operation):
//
//	func (s *Service) SaveGear(ctx context.Context, req *Request) (*Response, error) {
//	    logger := logging.LoggerWithContext(ctx)
//	    logger.Info("creating gear", "gear_name", req.Name)
//	    // ...
//	}
//
// Source location (file:line) can be enabled with AddSource option for debugging.
package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"time"
)

// Options configures the logger.
//
// Default values:
//   - Level: "info"
//   - Format: "json"
//   - Output: os.Stdout
//   - AddSource: false
type Options struct {
	// Level is the minimum log level to output.
	// Valid values: "debug", "info", "warn", "error".
	Level string

	// Format is the output format.
	// Valid values: "json", "text".
	// Text format includes colorized output for terminals.
	Format string

	// Output is the destination for log output.
	Output io.Writer

	// AddSource adds source file and line number to log entries.
	// Useful for debugging but adds overhead.
	AddSource bool
}

// Logger wraps slog.Logger with additional context and convenience methods.
type Logger struct {
	*slog.Logger
}

// NewLogger creates a new structured logger with the given options.
// Default values are applied for any unset options.
func NewLogger(opts Options) *Logger {
	// Apply defaults
	level := opts.Level
	if level == "" {
		level = "info"
	}

	format := strings.ToLower(opts.Format)
	if format == "" {
		format = "json"
	}

	output := opts.Output
	if output == nil {
		output = os.Stdout
	}

	handlerOpts := &slog.HandlerOptions{
		Level:     parseLevel(level),
		AddSource: opts.AddSource,
	}

	var handler slog.Handler
	switch format {
	case "text":
		handler = newPrettyHandler(output, handlerOpts)
	default:
		// Use JSON handler with Cloud Logging-compatible field names
		handlerOpts.ReplaceAttr = cloudLoggingReplaceAttr
		handler = slog.NewJSONHandler(output, handlerOpts)
	}

	return &Logger{
		Logger: slog.New(handler),
	}
}

// cloudLoggingReplaceAttr renames slog's default field names to match Cloud Logging's expectations.
// This allows Cloud Logging to properly recognize severity levels and display appropriate icons.
// See: https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry
func cloudLoggingReplaceAttr(groups []string, a slog.Attr) slog.Attr {
	// Only rename top-level attributes (not nested in groups)
	if len(groups) == 0 {
		switch a.Key {
		case slog.LevelKey:
			// Cloud Logging expects "severity" not "level"
			a.Key = "severity"
			// Cloud Logging expects "WARNING" not "WARN"
			if level, ok := a.Value.Any().(slog.Level); ok {
				switch level {
				case slog.LevelWarn:
					a.Value = slog.StringValue("WARNING")
				default:
					a.Value = slog.StringValue(level.String())
				}
			}
		case slog.MessageKey:
			// Cloud Logging expects "message" not "msg"
			a.Key = "message"
		}
	}
	return a
}

// parseLevel parses a log level string into a slog.Level.
// Returns slog.LevelInfo for unrecognized values.
func parseLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "info", "":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// With returns a new Logger with the given attributes added to all log entries.
func (l *Logger) With(args ...any) *Logger {
	return &Logger{
		Logger: l.Logger.With(args...),
	}
}

// WithGroup returns a new Logger with the given group name.
// All subsequent attributes will be nested under this group.
func (l *Logger) WithGroup(name string) *Logger {
	return &Logger{
		Logger: l.Logger.WithGroup(name),
	}
}

// Context keys for storing logger and request context.
type contextKey string

const (
	loggerKey     contextKey = "logger"
	requestIDKey  contextKey = "request_id"
	userIDKey     contextKey = "user_id"
	operationKey  contextKey = "operation"
	remoteAddrKey contextKey = "remote_addr"
)

// WithLogger returns a new context with the logger attached.
func WithLogger(ctx context.Context, logger *Logger) context.Context {
	return context.WithValue(ctx, loggerKey, logger)
}

// FromContext extracts the logger from the context.
// If no logger is found, returns the default logger.
func FromContext(ctx context.Context) *Logger {
	if logger, ok := ctx.Value(loggerKey).(*Logger); ok {
		return logger
	}
	return defaultLogger
}

// WithRequestID returns a new context with the request ID attached.
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDKey, requestID)
}

// RequestIDFromContext extracts the request ID from the context.
// Returns empty string if not found.
func RequestIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey).(string); ok {
		return id
	}
	return ""
}

// WithUserID returns a new context with the user ID attached.
func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userIDKey, userID)
}

// UserIDFromContext extracts the user ID from the context.
// Returns empty string if not found.
func UserIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(userIDKey).(string); ok {
		return id
	}
	return ""
}

// WithOperation returns a new context with the operation name (e.g., RPC method) attached.
func WithOperation(ctx context.Context, operation string) context.Context {
	return context.WithValue(ctx, operationKey, operation)
}

// OperationFromContext extracts the operation name from the context.
// Returns empty string if not found.
func OperationFromContext(ctx context.Context) string {
	if op, ok := ctx.Value(operationKey).(string); ok {
		return op
	}
	return ""
}

// WithRemoteAddr returns a new context with the client remote address attached.
// The value should be the resolved client IP (X-Forwarded-For first hop or TCP RemoteAddr).
func WithRemoteAddr(ctx context.Context, addr string) context.Context {
	return context.WithValue(ctx, remoteAddrKey, addr)
}

// RemoteAddrFromContext extracts the client remote address from the context.
// Returns empty string if not found.
func RemoteAddrFromContext(ctx context.Context) string {
	if addr, ok := ctx.Value(remoteAddrKey).(string); ok {
		return addr
	}
	return ""
}

// defaultLogger is used when no logger is found in context.
var defaultLogger = NewLogger(Options{})

// SetDefault sets the default logger used when no logger is in context.
// This also sets the logger as the default for the standard slog package.
func SetDefault(logger *Logger) {
	defaultLogger = logger
	slog.SetDefault(logger.Logger)
}

// Default returns the default logger.
func Default() *Logger {
	return defaultLogger
}

// GoSafe launches fn in a new goroutine with panic recovery. Any panic is
// caught, logged at Error level via LoggerWithContext(ctx), and does not
// propagate further. The name parameter identifies the goroutine in the log
// entry for easier triage.
func GoSafe(ctx context.Context, name string, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				LoggerWithContext(ctx).ErrorContext(ctx, "panic in goroutine",
					"goroutine", name,
					"panic", r,
					"stack", string(debug.Stack()),
				)
			}
		}()
		fn()
	}()
}

// LoggerWithContext returns a logger enriched with request context fields.
// This is the recommended way to get a logger for use in request handlers.
// It automatically includes request_id, user_id, and operation if present in context.
func LoggerWithContext(ctx context.Context) *Logger {
	logger := FromContext(ctx)

	// Add context fields if present
	var args []any
	if requestID := RequestIDFromContext(ctx); requestID != "" {
		args = append(args, "request_id", requestID)
	}
	if userID := UserIDFromContext(ctx); userID != "" {
		args = append(args, "user_id", userID)
	}
	if operation := OperationFromContext(ctx); operation != "" {
		args = append(args, "operation", operation)
	}

	if len(args) > 0 {
		return logger.With(args...)
	}
	return logger
}

// prettyHandler is a custom slog.Handler that formats log entries with emoji
// prefixes for easy visual scanning in text mode.
type prettyHandler struct {
	opts   *slog.HandlerOptions
	output io.Writer
	mu     *sync.Mutex
	attrs  []slog.Attr
	groups []string
}

// newPrettyHandler creates a new pretty handler for human-readable log output.
func newPrettyHandler(output io.Writer, opts *slog.HandlerOptions) *prettyHandler {
	if opts == nil {
		opts = &slog.HandlerOptions{}
	}
	return &prettyHandler{
		opts:   opts,
		output: output,
		mu:     &sync.Mutex{},
	}
}

// Enabled reports whether the handler handles records at the given level.
func (h *prettyHandler) Enabled(_ context.Context, level slog.Level) bool {
	minLevel := slog.LevelInfo
	if h.opts.Level != nil {
		minLevel = h.opts.Level.Level()
	}
	return level >= minLevel
}

// Handle formats and writes the log record.
func (h *prettyHandler) Handle(_ context.Context, r slog.Record) error {
	var buf strings.Builder

	// Emoji prefix based on level (includes trailing space)
	emoji := levelEmoji(r.Level)
	buf.WriteString(emoji)

	// Timestamp
	buf.WriteString(r.Time.Format(time.DateTime))
	buf.WriteString(" ")

	// Level (padded for alignment)
	fmt.Fprintf(&buf, "%-5s", r.Level.String())
	buf.WriteString(" ")

	// Source location if enabled
	if h.opts.AddSource && r.PC != 0 {
		frames := runtime.CallersFrames([]uintptr{r.PC})
		frame, _ := frames.Next()
		if frame.File != "" {
			// Use short file path (just filename, not full path)
			file := filepath.Base(frame.File)
			fmt.Fprintf(&buf, "[%s:%d] ", file, frame.Line)
		}
	}

	// Message
	buf.WriteString(r.Message)

	// Pre-configured attributes (from With calls)
	for _, attr := range h.attrs {
		h.appendAttr(&buf, attr)
	}

	// Record attributes
	r.Attrs(func(attr slog.Attr) bool {
		h.appendAttr(&buf, attr)
		return true
	})

	buf.WriteString("\n")

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := h.output.Write([]byte(buf.String()))
	return err
}

// appendAttr appends a single attribute to the buffer.
func (h *prettyHandler) appendAttr(buf *strings.Builder, attr slog.Attr) {
	// Skip empty attributes
	if attr.Equal(slog.Attr{}) {
		return
	}

	// Resolve the attribute value (handles LogValuer interface)
	attr.Value = attr.Value.Resolve()

	buf.WriteString(" ")

	// Add group prefix if any
	for _, g := range h.groups {
		buf.WriteString(g)
		buf.WriteString(".")
	}

	buf.WriteString(attr.Key)
	buf.WriteString("=")

	// Format value based on kind
	switch attr.Value.Kind() {
	case slog.KindString:
		// Quote strings that contain spaces
		s := attr.Value.String()
		if strings.ContainsAny(s, " \t\n") {
			fmt.Fprintf(buf, "%q", s)
		} else {
			buf.WriteString(s)
		}
	case slog.KindTime:
		buf.WriteString(attr.Value.Time().Format(time.RFC3339))
	case slog.KindDuration:
		buf.WriteString(attr.Value.Duration().String())
	default:
		fmt.Fprintf(buf, "%v", attr.Value.Any())
	}
}

// WithAttrs returns a new handler with the given attributes added.
func (h *prettyHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newAttrs := make([]slog.Attr, len(h.attrs), len(h.attrs)+len(attrs))
	copy(newAttrs, h.attrs)
	newAttrs = append(newAttrs, attrs...)
	return &prettyHandler{
		opts:   h.opts,
		output: h.output,
		mu:     h.mu,
		attrs:  newAttrs,
		groups: h.groups,
	}
}

// WithGroup returns a new handler with the given group name.
func (h *prettyHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	newGroups := make([]string, len(h.groups), len(h.groups)+1)
	copy(newGroups, h.groups)
	newGroups = append(newGroups, name)
	return &prettyHandler{
		opts:   h.opts,
		output: h.output,
		mu:     h.mu,
		attrs:  h.attrs,
		groups: newGroups,
	}
}

// levelEmoji returns an emoji prefix for the given log level.
// Each emoji includes a trailing space for consistent formatting.
func levelEmoji(level slog.Level) string {
	switch {
	case level < slog.LevelInfo:
		return "🐛 " // DEBUG
	case level < slog.LevelWarn:
		return "ℹ️  " // INFO (extra space due to variation selector)
	case level < slog.LevelError:
		return "⚠️  " // WARN (extra space due to variation selector)
	default:
		return "❌ " // ERROR
	}
}
