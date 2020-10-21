package ffi

// #cgo CFLAGS: -I ../../rust/include
// #cgo LDFLAGS: ${SRCDIR}/../../rust/target/release/libexample.a -ldl
// #include <api.h>
import "C"

func Hello() {
	C.hello()
}
