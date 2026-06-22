package legacy

import (
	"fmt"
	"log/slog"
)

// FieldLogger is the subset of logrus.FieldLogger we use in the vendored
// legacy code (CreateDocx + internalCreateDocx). It is the surface area
// that the adapter satisfies, kept narrow on purpose: anything more would
// imply a deeper runtime dependency on logrus.
//
// The methods are intentionally plain (no key/value pairs) because that is
// what the legacy call sites use today.
type FieldLogger interface {
	Infof(format string, args ...any)
	Debugf(format string, args ...any)
	Warnf(format string, args ...any)
	Errorf(format string, args ...any)
	Info(args ...any)
	Debug(args ...any)
	Warn(args ...any)
	Error(args ...any)
}

// slogAdapter bridges the project's stdlib *slog.Logger into the
// FieldLogger interface the vendored legacy code expects. It is used as
// the `log` argument of legacy.CreateDocx.
//
// Why not vendor logrus? The project standard is log/slog. Adding logrus
// purely to satisfy a 4-call dependency would inflate the dep graph and
// split logging configuration. The adapter keeps the legacy code untouched
// (apart from the package rename + import path) and centralises the bridge
// in one file so it can be deleted the day the legacy goes away.
//
// Nil safety: passing a nil *slog.Logger is allowed and routes everything
// through slog.Default(). This is convenient for tests and for the
// caller's defensive case ("log may be nil").
type slogAdapter struct {
	log *slog.Logger
}

// newSlogAdapter returns a FieldLogger backed by the supplied *slog.Logger.
// A nil logger falls back to slog.Default() so callers don't have to nil-
// check at every call site.
func newSlogAdapter(log *slog.Logger) FieldLogger {
	if log == nil {
		log = slog.Default()
	}
	return &slogAdapter{log: log}
}

// NewSlogAdapter is the exported alias of newSlogAdapter used by the
// adapter layer (backend/documents/adapter_legacy.go) and by tests
// that want to route legacy log output through slog.Default().
func NewSlogAdapter() FieldLogger {
	return newSlogAdapter(nil)
}

// Infof logs at LevelInfo with the formatted string as the message.
func (a *slogAdapter) Infof(format string, args ...any) {
	a.log.Info(fmt.Sprintf(format, args...))
}

// Debugf logs at LevelDebug.
func (a *slogAdapter) Debugf(format string, args ...any) {
	a.log.Debug(fmt.Sprintf(format, args...))
}

// Warnf logs at LevelWarn.
func (a *slogAdapter) Warnf(format string, args ...any) {
	a.log.Warn(fmt.Sprintf(format, args...))
}

// Errorf logs at LevelError.
func (a *slogAdapter) Errorf(format string, args ...any) {
	a.log.Error(fmt.Sprintf(format, args...))
}

// Info forwards to slog.Info with the first argument as message; remaining
// args (if any) are joined with spaces to mirror fmt.Sprint semantics. This
// is the closest logrus-compatible behaviour without dragging in logrus.
func (a *slogAdapter) Info(args ...any) {
	a.log.Info(joinArgs(args))
}

// Debug mirrors Info at LevelDebug.
func (a *slogAdapter) Debug(args ...any) {
	a.log.Debug(joinArgs(args))
}

// Warn mirrors Info at LevelWarn.
func (a *slogAdapter) Warn(args ...any) {
	a.log.Warn(joinArgs(args))
}

// Error mirrors Info at LevelError.
func (a *slogAdapter) Error(args ...any) {
	a.log.Error(joinArgs(args))
}

func joinArgs(args []any) string {
	switch len(args) {
	case 0:
		return ""
	case 1:
		if s, ok := args[0].(string); ok {
			return s
		}
	}
	// fmt.Sprint with mixed args keeps the behaviour obvious for tests and
	// for anyone reading the legacy call sites.
	return fmt.Sprint(args...)
}
