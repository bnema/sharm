package service

type nopLogger struct{}

func (nopLogger) Infof(string, ...any)  {}
func (nopLogger) Errorf(string, ...any) {}
func (nopLogger) Debugf(string, ...any) {}
func (nopLogger) Warnf(string, ...any)  {}
