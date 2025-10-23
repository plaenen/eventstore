package runner

// Logger is a simple logging interface that the runner uses.
// Implementations can wrap any logging library (zap, logrus, slog, etc).
type Logger interface {
	Info(msg string, keysAndValues ...interface{})
	Error(msg string, keysAndValues ...interface{})
	Debug(msg string, keysAndValues ...interface{})
}

// noopLogger is a no-op logger implementation.
type noopLogger struct{}

// NewNoopLogger returns a no-op logger.
func NewNoopLogger() Logger {
	return noopLogger{}
}

func (noopLogger) Info(msg string, keysAndValues ...interface{})  {}
func (noopLogger) Error(msg string, keysAndValues ...interface{}) {}
func (noopLogger) Debug(msg string, keysAndValues ...interface{}) {}
