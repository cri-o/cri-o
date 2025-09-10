package references

import (
	"fmt"

	"go.podman.io/image/v5/docker/reference"
)

// RegistryImageReference is a name of a specific image location on a registry.
// The image may or may not exist, and, in general, what image the name points to may change
// over time.
//
// More specifically:
// - The name always specifies a registry; it is not an alias nor a short name input to a search
// - The name contains a tag xor digest; it does not specify just a repo.
//
// This is intended to be a value type; if a value exists, it contains a valid reference.
type RegistryImageReference struct {
	// privateNamed is INTENTIONALLY ENCAPSULATED to provide strong type safety and strong syntax/semantics guarantees.
	// Use typed values, not strings, everywhere it is even remotely possible.
	privateNamed reference.Named // Satisfies !reference.IsNameOnly
}

// RegistryImageReferenceFromRaw is an internal constructor of a RegistryImageReference.
//
// This should only be called from internal/storage.
// It will modify the reference if both digest and tag are specified, stripping the tag and leaving the digest.
// It will also verifies the image is not only a name. If it is only a name, the function errors.
func RegistryImageReferenceFromRaw(rawNamed reference.Named) RegistryImageReference {
	_, isTagged := rawNamed.(reference.NamedTagged)
	canonical, isDigested := rawNamed.(reference.Canonical)
	// Strip the tag from ambiguous image references that have a
	// digest as well (e.g.  `image:tag@sha256:123...`).  Such
	// image references are supported by docker but, due to their
	// ambiguity, explicitly not by containers/image.
	if isTagged && isDigested {
		canonical, err := reference.WithDigest(reference.TrimNamed(rawNamed), canonical.Digest())
		if err != nil {
			panic("internal error, reference.WithDigest was not passed a digest, which should not be possible")
		}

		rawNamed = canonical
	}
	// Ideally this would be better encapsulated, e.g. in internal/storage/internal, but
	// that would require using a type defined with the internal package with a public alias,
	// and as of 2023-10 mockgen creates code that refers to the internal target of the alias,
	// which doesn’t compile.
	if reference.IsNameOnly(rawNamed) {
		panic(fmt.Sprintf("internal error, NewRegistryImageReference with a NameOnly %q", rawNamed.String()))
	}

	return RegistryImageReference{privateNamed: rawNamed}
}

// ParseRegistryImageReferenceFromOutOfProcessData constructs a RegistryImageReference from a string.
//
// It is only intended for communication with OUT-OF-PROCESS APIs,
// like registry references provided by CRI by Kubelet.
// It will modify the reference if both digest and tag are specified, stripping the tag and leaving the digest.
// It will also verifies the image is not only a name. If it is only a name, the `latest` tag will be added.
func ParseRegistryImageReferenceFromOutOfProcessData(input string) (RegistryImageReference, error) {
	// Alternatively, should we provide two parsers, one with docker.io/library and :latest defaulting,
	// and one only accepting fully-specified reference.Named.String() values?
	ref, err := reference.ParseNormalizedNamed(input)
	if err != nil {
		return RegistryImageReference{}, err
	}

	ref = reference.TagNameOnly(ref)

	return RegistryImageReferenceFromRaw(ref), nil
}

func (ref RegistryImageReference) ensureInitialized() {
	// It’s deeply disappointing that we need to check this at runtime, instead of just
	// requiring a constructor to be called.
	if ref.privateNamed == nil {
		panic("internal error, use of an uninitialized RegistryImageReference")
	}
}

// StringForOutOfProcessConsumptionOnly is only intended for communication with OUT-OF-PROCESS APIs,
// like image names in CRI status objects.
//
// RegistryImageReference intentionally does not implement String(). Use typed values wherever possible.
func (ref RegistryImageReference) StringForOutOfProcessConsumptionOnly() string {
	ref.ensureInitialized()

	return ref.privateNamed.String()
}

// Format() is implemented so that log entries can be written, without providing a convenient String() method.
func (ref RegistryImageReference) Format(f fmt.State, verb rune) {
	ref.ensureInitialized()
	fmt.Fprintf(f, fmt.FormatString(f, verb), ref.privateNamed.String())
}

// Registry returns the host[:port] part of the reference.
func (ref RegistryImageReference) Registry() string {
	ref.ensureInitialized()

	return reference.Domain(ref.privateNamed)
}

// Raw returns the underlying reference.Named.
//
// The return value is !IsNameOnly, and the repo is registry-qualified.
//
// This should only be called from internal/storage.
func (ref RegistryImageReference) Raw() reference.Named {
	// See the comment in RegistryImageReferenceFromRaw about better encapsulation.
	ref.ensureInitialized()

	return ref.privateNamed
}
