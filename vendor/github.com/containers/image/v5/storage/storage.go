package storage

import internal "github.com/containers/image/v5/internal/storage"

// Note that the storage package is merely aliasing *some* parts of the
// `internal/storage` package.  This allows us to control the public parts of
// the API (i.e., in this file) and the c/image internal parts of the API
// (i.e., unaliased parts).  An immediate use case is to add new features
// (e.g., to the `copy` package) without having to worry too much about public
// facing changes and versioning.

var (
	// Transport is an ImageTransport that uses either a default
	// storage.Store or one that's it's explicitly told to use.
	Transport = internal.Transport
	// ErrInvalidReference is returned when ParseReference() is passed an
	// empty reference.
	ErrInvalidReference = internal.ErrInvalidReference
	// ErrPathNotAbsolute is returned when a graph root is not an absolute
	// path name.
	ErrPathNotAbsolute = internal.ErrPathNotAbsolute
)

// StoreTransport is an ImageTransport that uses a storage.Store to parse
// references, either its own default or one that it's told to use.
type StoreTransport = internal.StoreTransport

// A StorageReference holds an arbitrary name and/or an ID, which is a 32-byte
// value hex-encoded into a 64-character string, and a reference to a Store
// where an image is, or would be, kept.
// Either "named" or "id" must be set.
type StorageReference = internal.StorageReference
