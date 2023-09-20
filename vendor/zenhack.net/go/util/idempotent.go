package util

// Idempotent returns a function which calls f() the first time it is called,
// and then no-ops thereafter.
func Idempotent(f func()) func() {
	var called bool
	return func() {
		if !called {
			called = true
			f()
		}
	}
}
