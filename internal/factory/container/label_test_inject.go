//go:build test
// +build test

// All *_inject.go files are meant to be used by tests only. Purpose of this
// files is to provide a way to inject mocked data into the current setup.

package container

func (label *secLabel) SetImpl(impl Impl) {
	label.impl = impl
}
