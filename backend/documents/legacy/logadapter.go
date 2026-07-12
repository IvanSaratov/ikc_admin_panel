package legacy

import "go.uber.org/zap"

// FieldLogger — минимальная поверхность logger, которую использует legacy
// generator. Она совпадает с нужной частью zap.SugaredLogger и не даёт legacy-коду
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

type zapAdapter struct {
	log *zap.SugaredLogger
}

func newZapAdapter(log *zap.Logger) FieldLogger {
	if log == nil {
		log = zap.NewNop()
	}
	return &zapAdapter{log: log.Sugar()}
}

// NewZapAdapter возвращает logger для legacy generator.
func NewZapAdapter() FieldLogger {
	return newZapAdapter(nil)
}

// NewZapAdapterWithLogger подключает legacy generator к runtime logger приложения.
func NewZapAdapterWithLogger(log *zap.Logger) FieldLogger {
	return newZapAdapter(log)
}

func (a *zapAdapter) Infof(format string, args ...any)  { a.log.Infof(format, args...) }
func (a *zapAdapter) Debugf(format string, args ...any) { a.log.Debugf(format, args...) }
func (a *zapAdapter) Warnf(format string, args ...any)  { a.log.Warnf(format, args...) }
func (a *zapAdapter) Errorf(format string, args ...any) { a.log.Errorf(format, args...) }
func (a *zapAdapter) Info(args ...any)                  { a.log.Info(args...) }
func (a *zapAdapter) Debug(args ...any)                 { a.log.Debug(args...) }
func (a *zapAdapter) Warn(args ...any)                  { a.log.Warn(args...) }
func (a *zapAdapter) Error(args ...any)                 { a.log.Error(args...) }
