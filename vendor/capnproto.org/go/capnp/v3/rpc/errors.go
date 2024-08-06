package rpc

import (
	"errors"

	"capnproto.org/go/capnp/v3/exc"
)

var (
	rpcerr = exc.Annotator("rpc")

	// Base errors
	ErrConnClosed        = errors.New("connection closed")
	ErrNotACapability    = errors.New("not a capability")
	ErrCapTablePopulated = errors.New("capability table already populated")

	// RPC exceptions
	ExcClosed = rpcerr.Disconnected(ErrConnClosed)
)

type errReporter struct {
	Logger Logger
}

func (er errReporter) Debug(msg string, args ...any) {
	if er.Logger != nil {
		er.Logger.Debug(msg, args...)
	}
}

func (er errReporter) Info(msg string, args ...any) {
	if er.Logger != nil {
		er.Logger.Info(msg, args...)
	}
}

func (er errReporter) Warn(msg string, args ...any) {
	if er.Logger != nil {
		er.Logger.Warn(msg, args...)
	}
}

func (er errReporter) Error(msg string, args ...any) {
	if er.Logger != nil {
		er.Logger.Error(msg, args...)
	}
}

func (er errReporter) ReportError(err error) {
	if err != nil {
		er.Error(err.Error())
	}
}
