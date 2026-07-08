package legacy

import "github.com/sirupsen/logrus"

// FieldLogger — минимальная поверхность logger, которую использует legacy
// generator. Она совпадает с нужной частью logrus и не даёт legacy-коду
// диктовать остальную logging-архитектуру приложения.
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

type logrusAdapter struct {
	log logrus.FieldLogger
}

func newLogrusAdapter(log logrus.FieldLogger) FieldLogger {
	if log == nil {
		log = logrus.StandardLogger()
	}
	return &logrusAdapter{log: log}
}

// NewLogrusAdapter возвращает logger для legacy generator.
func NewLogrusAdapter() FieldLogger {
	return newLogrusAdapter(nil)
}

func (a *logrusAdapter) Infof(format string, args ...any)  { a.log.Infof(format, args...) }
func (a *logrusAdapter) Debugf(format string, args ...any) { a.log.Debugf(format, args...) }
func (a *logrusAdapter) Warnf(format string, args ...any)  { a.log.Warnf(format, args...) }
func (a *logrusAdapter) Errorf(format string, args ...any) { a.log.Errorf(format, args...) }
func (a *logrusAdapter) Info(args ...any)                  { a.log.Info(args...) }
func (a *logrusAdapter) Debug(args ...any)                 { a.log.Debug(args...) }
func (a *logrusAdapter) Warn(args ...any)                  { a.log.Warn(args...) }
func (a *logrusAdapter) Error(args ...any)                 { a.log.Error(args...) }
