# Cap'n Proto bindings for Go

![License](https://img.shields.io/badge/license-MIT-brightgreen?style=flat-square)
[![CodeQuality](https://goreportcard.com/badge/capnproto.org/go/capnp)](https://goreportcard.com/report/capnproto.org/go/capnp/v3)
[![Go](https://github.com/capnproto/go-capnproto2/actions/workflows/go.yml/badge.svg)](https://github.com/capnproto/go-capnproto2/actions/workflows/go.yml)
[![GoDoc](https://godoc.org/capnproto.org/go/capnp/v3?status.svg)][godoc]
[![Matrix](https://img.shields.io/matrix/go-capnp:matrix.org?color=lightpink&label=Get%20Help&logo=matrix&style=flat-square)](https://matrix.to/#/#go-capnp:matrix.org)

[Cap’n Proto](https://capnproto.org/) is an insanely fast data interchange format similar to [Protocol Buffers](https://github.com/protocolbuffers/protobuf), but much faster.

It also includes a sophisticated RPC system based on [Object Capabilities](https://en.wikipedia.org/wiki/Object-capability_model), ideal for secure, low-latency applications.

This package provides:
- Go code-generation for Cap'n Proto
- Runtime support for the Go language
- Level 1 support for the [Cap'n Proto RPC](https://capnproto.org/rpc.html) protocol

Support for Level 3 RPC is [planned](https://github.com/capnproto/go-capnproto2/issues/160).

## Getting Started

Read the ["Getting Started" guide](docs/Getting-Started.md#remote-calls-using-interfaces)
for a high-level introduction to the package API and workflow.

## Help and Support

You can find us on Matrix:   [Go Cap'n Proto](https://matrix.to/#/!pLcnVUHHRZrUPscloW:matrix.org?via=matrix.org)

## API Reference

Available on [pkg.go.dev][godoc]

## API Compatibility

Until the official Cap'n Proto spec is finalized, this repository should be considered <u>beta software</u>.

We use [semantic versioning](https://semver.org) to track compatibility and signal breaking changes.  In the spirit of the [Go 1 compatibility guarantee][gocompat], we will make every effort to avoid making breaking API changes within major version numbers, but nevertheless reserve the right to introduce breaking changes for reasons related to:

- Security.
- Changes in the Cap'n Proto specification.
- Bugs.

An exception to this rule is currently in place for the `pogs` package, which is relatively new and may change over time.  However, its functionality has been well-tested, and breaking changes are relatively unlikely.

Note also we may merge breaking changes to the `main` branch without notice.  Users are encouraged to pin their dependencies to a major version, e.g. using the semver-aware features of `go get`.

## License

MIT - see [LICENSE][] file

[godoc]: http://pkg.go.dev/capnproto.org/go/capnp/v3
[gocompat]: https://golang.org/doc/go1compat
[LICENSE]: https://github.com/capnproto/go-capnproto2/blob/master/LICENSE
[getting-started]: docs/Getting-Started.md
