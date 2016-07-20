package types

import (
	"io"
	"time"

	"github.com/docker/docker/reference"
)

// ImageTransport is a top-level namespace for ways to to store/load an image.
// It should generally correspond to ImageSource/ImageDestination implementations.
//
// Note that ImageTransport is based on "ways the users refer to image storage", not necessarily on the underlying physical transport.
// For example, all Docker References would be used within a single "docker" transport, regardless of whether the images are pulled over HTTP or HTTPS
// (or, even, IPv4 or IPv6).
//
// OTOH all images using the same transport should (apart from versions of the image format), be interoperable.
// For example, several different ImageTransport implementations may be based on local filesystem paths,
// but using completely different formats for the contents of that path (a single tar file, a directory containing tarballs, a fully expanded container filesystem, ...)
//
// See also transports.KnownTransports.
type ImageTransport interface {
	// Name returns the name of the transport, which must be unique among other transports.
	Name() string
	// ParseReference converts a string, which should not start with the ImageTransport.Name prefix, into an ImageReference.
	ParseReference(reference string) (ImageReference, error)
	// ValidatePolicyConfigurationScope checks that scope is a valid name for a signature.PolicyTransportScopes keys
	// (i.e. a valid PolicyConfigurationIdentity() or PolicyConfigurationNamespaces() return value).
	// It is acceptable to allow an invalid value which will never be matched, it can "only" cause user confusion.
	// scope passed to this function will not be "", that value is always allowed.
	ValidatePolicyConfigurationScope(scope string) error
}

// ImageReference is an abstracted way to refer to an image location, namespaced within an ImageTransport.
//
// The object should preferably be immutable after creation, with any parsing/state-dependent resolving happening
// within an ImageTransport.ParseReference() or equivalent API creating the reference object.
// That's also why the various identification/formatting methods of this type do not support returning errors.
//
// WARNING: While this design freezes the content of the reference within this process, it can not freeze the outside
// world: paths may be replaced by symlinks elsewhere, HTTP APIs may start returning different results, and so on.
type ImageReference interface {
	Transport() ImageTransport
	// StringWithinTransport returns a string representation of the reference, which MUST be such that
	// reference.Transport().ParseReference(reference.StringWithinTransport()) returns an equivalent reference.
	// NOTE: The returned string is not promised to be equal to the original input to ParseReference;
	// e.g. default attribute values omitted by the user may be filled in in the return value, or vice versa.
	// WARNING: Do not use the return value in the UI to describe an image, it does not contain the Transport().Name() prefix;
	// instead, see transports.ImageName().
	StringWithinTransport() string

	// DockerReference returns a Docker reference associated with this reference
	// (fully explicit, i.e. !reference.IsNameOnly, but reflecting user intent,
	// not e.g. after redirect or alias processing), or nil if unknown/not applicable.
	DockerReference() reference.Named

	// PolicyConfigurationIdentity returns a string representation of the reference, suitable for policy lookup.
	// This MUST reflect user intent, not e.g. after processing of third-party redirects or aliases;
	// The value SHOULD be fully explicit about its semantics, with no hidden defaults, AND canonical
	// (i.e. various references with exactly the same semantics should return the same configuration identity)
	// It is fine for the return value to be equal to StringWithinTransport(), and it is desirable but
	// not required/guaranteed that it will be a valid input to Transport().ParseReference().
	// Returns "" if configuration identities for these references are not supported.
	PolicyConfigurationIdentity() string

	// PolicyConfigurationNamespaces returns a list of other policy configuration namespaces to search
	// for if explicit configuration for PolicyConfigurationIdentity() is not set.  The list will be processed
	// in order, terminating on first match, and an implicit "" is always checked at the end.
	// It is STRONGLY recommended for the first element, if any, to be a prefix of PolicyConfigurationIdentity(),
	// and each following element to be a prefix of the element preceding it.
	PolicyConfigurationNamespaces() []string

	// NewImage returns a types.Image for this reference.
	NewImage(certPath string, tlsVerify bool) (Image, error)
	// NewImageSource returns a types.ImageSource for this reference.
	NewImageSource(certPath string, tlsVerify bool) (ImageSource, error)
	// NewImageDestination returns a types.ImageDestination for this reference.
	NewImageDestination(certPath string, tlsVerify bool) (ImageDestination, error)
}

// ImageSource is a service, possibly remote (= slow), to download components of a single image.
// This is primarily useful for copying images around; for examining their properties, Image (below)
// is usually more useful.
type ImageSource interface {
	// Reference returns the reference used to set up this source, _as specified by the user_
	// (not as the image itself, or its underlying storage, claims).  This can be used e.g. to determine which public keys are trusted for this image.
	Reference() ImageReference
	// GetManifest returns the image's manifest along with its MIME type. The empty string is returned if the MIME type is unknown. The slice parameter indicates the supported mime types the manifest should be when getting it.
	// It may use a remote (= slow) service.
	GetManifest([]string) ([]byte, string, error)
	// Note: Calling GetBlob() may have ordering dependencies WRT other methods of this type. FIXME: How does this work with (docker save) on stdin?
	// the second return value is the size of the blob. If not known 0 is returned
	GetBlob(digest string) (io.ReadCloser, int64, error)
	// GetSignatures returns the image's signatures.  It may use a remote (= slow) service.
	GetSignatures() ([][]byte, error)
	// Delete image from registry, if operation is supported
	Delete() error
}

// ImageDestination is a service, possibly remote (= slow), to store components of a single image.
type ImageDestination interface {
	// Reference returns the reference used to set up this destination.  Note that this should directly correspond to user's intent,
	// e.g. it should use the public hostname instead of the result of resolving CNAMEs or following redirects.
	Reference() ImageReference
	// FIXME? This should also receive a MIME type if known, to differentiate between schema versions.
	PutManifest([]byte) error
	// Note: Calling PutBlob() and other methods may have ordering dependencies WRT other methods of this type. FIXME: Figure out and document.
	PutBlob(digest string, stream io.Reader) error
	PutSignatures(signatures [][]byte) error
	// SupportedManifestMIMETypes tells which manifest mime types the destination supports
	// If an empty slice or nil it's returned, then any mime type can be tried to upload
	SupportedManifestMIMETypes() []string
}

// Image is the primary API for inspecting properties of images.
type Image interface {
	// Reference returns the reference used to set up this source, _as specified by the user_
	// (not as the image itself, or its underlying storage, claims).  This can be used e.g. to determine which public keys are trusted for this image.
	Reference() ImageReference
	// ref to repository?
	// Manifest is like ImageSource.GetManifest, but the result is cached; it is OK to call this however often you need.
	// NOTE: It is essential for signature verification that Manifest returns the manifest from which BlobDigests is computed.
	Manifest() ([]byte, string, error)
	// Signatures is like ImageSource.GetSignatures, but the result is cached; it is OK to call this however often you need.
	Signatures() ([][]byte, error)
	// BlobDigests returns a list of blob digests referenced by this image.
	// The list will not contain duplicates; it is not intended to correspond to the "history" or "parent chain" of a Docker image.
	// NOTE: It is essential for signature verification that BlobDigests is computed from the same manifest which is returned by Manifest().
	BlobDigests() ([]string, error)
	// Inspect returns various information for (skopeo inspect) parsed from the manifest and configuration.
	Inspect() (*ImageInspectInfo, error)
}

// ImageInspectInfo is a set of metadata describing Docker images, primarily their manifest and configuration.
// The Tag field is a legacy field which is here just for the Docker v2s1 manifest. It won't be supported
// for other manifest types.
type ImageInspectInfo struct {
	Tag           string
	Created       time.Time
	DockerVersion string
	Labels        map[string]string
	Architecture  string
	Os            string
	Layers        []string
}
