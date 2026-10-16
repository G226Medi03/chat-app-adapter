package chat

// Logger is the minimal logging surface adapters use. Any logger with these
// methods satisfies it, including a host application's own logger. A nil
// Logger passed to an adapter means logging is disabled.
type Logger interface {
	Debugf(format string, args ...any)
	Infof(format string, args ...any)
	Warnf(format string, args ...any)
	Errorf(format string, args ...any)
}
