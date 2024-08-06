package util

// Chkfatal panics if err is not nil
func Chkfatal(err error) {
	if err != nil {
		panic(err)
	}
}

// Must panics if err != nil, otherwise returns value.
func Must[T any](value T, err error) T {
	Chkfatal(err)
	return value
}
