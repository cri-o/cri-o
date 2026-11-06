//go:build !linux

package server

// dedupReflinkSupported is a no-op on platforms where reflink-based
// deduplication is not available; the storage library handles those.
func dedupReflinkSupported(_ string) error {
	return nil
}
