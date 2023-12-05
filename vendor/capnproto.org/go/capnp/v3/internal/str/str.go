// Helpers for formatting strings. We avoid the use of the fmt package for the benefit
// of environments that care about minimizing executable size.
package str

import (
	"strconv"
	"strings"

	"unsafe" // Only for formatting pointers as integers; we don't actually do anything unsafe.
)

// Utod formats unsigned integers as decimals.
func Utod[T Uint](n T) string {
	return strconv.FormatUint(uint64(n), 10)
}

// Itod formats signed integers as decimals.
func Itod[T Int](n T) string {
	return strconv.FormatInt(int64(n), 10)
}

// UToHex returns n formatted in hexidecimal.
func UToHex[T Uint](n T) string {
	return strconv.FormatUint(uint64(n), 16)
}

func PtrToHex[T any](p *T) string {
	return UToHex(uintptr(unsafe.Pointer(p)))
}

// ZeroPad pads value to the left with zeros, making the resulting string
// count bytes long.
func ZeroPad(count int, value string) string {
	pad := count - len(value)
	if pad < 0 {
		panic("ZeroPad: count is less than len(value)")
	}
	buf := make([]byte, count)
	for i := 0; i < pad; i++ {
		buf[i] = '0'
	}
	copy(buf[:pad], value[:])
	return string(buf)
}

// Slice formats a slice of values which themselves implement Stringer.
func Slice[T Stringer](s []T) string {
	var b strings.Builder
	b.WriteRune('{')
	for i, v := range s {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(v.String())
	}
	b.WriteRune('}')
	return b.String()
}

// Stringer is equivalent to fmt.Stringer
type Stringer interface {
	String() string
}

type Uint interface {
	~uint8 | ~uint16 | ~uint32 | ~uint64 | ~uint | ~uintptr
}

type Int interface {
	~int8 | ~int16 | ~int32 | ~int64 | ~int
}
