package storage

import (
	"fmt"

	"go.podman.io/image/v5/docker/reference"
	istorage "go.podman.io/image/v5/storage"
	"go.podman.io/image/v5/types"
	"go.podman.io/storage"
)

// StorageImageID is a stable identifier for a (deduplicated) image in a local storage.
// The image referenced by the ID is _mostly_ immutable, notably the layers and config
// will never change; the names and some other metadata may change (as images are deduplicated).
//
// An ID might not refer to an image (e.g. if the image was deleted, or if the ID never referred
// to an image in the first place).
//
// This is intended to be a value type; if a value exists, it is a correctly-formatted ID.
// The values can be compared for equality, or used as map keys.
type StorageImageID struct {
	// privateID is INTENTIONALLY ENCAPSULATED to provide strong type safety and strong syntax/semantics guarantees.
	// Use typed values, not strings, everywhere it is even remotely possible.
	privateID string // Always a full *storage.Image.ID value (not a prefix); but there might not be any image with this ID
}

// newExactStorageImageID is a private constructor of a StorageImageID.
func newExactStorageImageID(rawImageID string) StorageImageID {
	if !reference.IsFullIdentifier(rawImageID) {
		panic(fmt.Sprintf("internal error, invalid input %q to newExactStorageImageID", rawImageID))
	}

	return StorageImageID{privateID: rawImageID}
}

// ParseStorageImageIDFromOutOfProcessData constructs a StorageImageID from a string.
// It is only intended for communication with OUT-OF-PROCESS APIs,
// like image IDs provided by CRI by Kubelet (who got it from CRI-O’s
// StorageImageID.IDStringForOutOfProcessConsumptionOnly() in the first place).
func ParseStorageImageIDFromOutOfProcessData(input string) (StorageImageID, error) {
	return parseStorageImageID(input)
}

// parseStorageImageID is an private constructor of a StorageImageID.
// Most callers should use ParseStorageImageIDFromOutOfProcessData ,
// or preferably not parse strings in the first place.
func parseStorageImageID(input string) (StorageImageID, error) {
	if !reference.IsFullIdentifier(input) {
		return StorageImageID{}, fmt.Errorf("%q is not a valid image ID", input)
	}

	return newExactStorageImageID(input), nil
}

// storageImageIDFromImage is an internal constructor of a StorageImageID.
func storageImageIDFromImage(image *storage.Image) StorageImageID {
	return newExactStorageImageID(image.ID)
}

func (id StorageImageID) ensureInitialized() {
	// It’s deeply disappointing that we need to check this at runtime, instead of just
	// requiring a constructor to be called.
	if id.privateID == "" {
		panic("internal error, use of an uninitialized StorageImageID")
	}
}

// IDStringForOutOfProcessConsumptionOnly is only intended for communication with OUT-OF-PROCESS APIs,
// like image IDs in CRI to provide stable identifiers to Kubelet.
//
// StorageImageID intentionally does not implement String(). Use typed values wherever possible.
func (id StorageImageID) IDStringForOutOfProcessConsumptionOnly() string {
	id.ensureInitialized()

	return id.privateID
}

// Format() is implemented so that log entries can be written, without providing a convenient String() method.
func (id StorageImageID) Format(f fmt.State, verb rune) {
	id.ensureInitialized()
	fmt.Fprintf(f, fmt.FormatString(f, verb), id.privateID)
}

// imageRef(svc) returns a types.ImageReference for id
// This succeeds even if the image does not exist.
func (id StorageImageID) imageRef(svc *imageService) (types.ImageReference, error) {
	id.ensureInitialized()
	// This is never expected to fail, we have validated privateID.
	return istorage.Transport.NewStoreReference(svc.store, nil, id.privateID)
}
