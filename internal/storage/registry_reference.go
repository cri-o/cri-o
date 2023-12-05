package storage

import "github.com/cri-o/cri-o/internal/storage/references"

// RegistryImageReference is a name of a specific image location on a registry.
// The image may or may not exist, and, in general, what image the name points to may change
// over time.
//
// More specifically:
// - The name always specifies a registry; it is not an alias nor a short name input to a search
// - The name contains a tag or digest; it does not specify just a repo.
//
// This is intended to be a value type; if a value exists, it contains a valid reference.
type RegistryImageReference = references.RegistryImageReference

// ^^ is in a separate subpackage to break a dependency loop; but conceptually it should
// be provided by the same package as StorageImageID.
