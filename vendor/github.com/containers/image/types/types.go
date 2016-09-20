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
	// The caller must call .Close() on the returned Image.
	NewImage(ctx *SystemContext) (Image, error)
	// NewImageSource returns a types.ImageSource for this reference,
	// asking the backend to use a manifest from requestedManifestMIMETypes if possible.
	// nil requestedManifestMIMETypes means manifest.DefaultRequestedManifestMIMETypes.
	// The caller must call .Close() on the returned ImageSource.
	NewImageSource(ctx *SystemContext, requestedManifestMIMETypes []string) (ImageSource, error)
	// NewImageDestination returns a types.ImageDestination for this reference.
	// The caller must call .Close() on the returned ImageDestination.
	NewImageDestination(ctx *SystemContext) (ImageDestination, error)

	// DeleteImage deletes the named image from the registry, if supported.
	DeleteImage(ctx *SystemContext) error
}

// ImageSource is a service, possibly remote (= slow), to download components of a single image.
// This is primarily useful for copying images around; for examining their properties, Image (below)
// is usually more useful.
// Each ImageSource should eventually be closed by calling Close().
type ImageSource interface {
	// Reference returns the reference used to set up this source, _as specified by the user_
	// (not as the image itself, or its underlying storage, claims).  This can be used e.g. to determine which public keys are trusted for this image.
	Reference() ImageReference
	// Close removes resources associated with an initialized ImageSource, if any.
	Close()
	// GetManifest returns the image's manifest along with its MIME type. The empty string is returned if the MIME type is unknown.
	// It may use a remote (= slow) service.
	GetManifest() ([]byte, string, error)
	// GetBlob returns a stream for the specified blob, and the blob’s size (or -1 if unknown).
	GetBlob(digest string) (io.ReadCloser, int64, error)
	// GetSignatures returns the image's signatures.  It may use a remote (= slow) service.
	GetSignatures() ([][]byte, error)
}

// ImageDestination is a service, possibly remote (= slow), to store components of a single image.
//
// There is a specific required order for some of the calls:
// PutBlob on the various blobs, if any, MUST be called before PutManifest (manifest references blobs, which may be created or compressed only at push time)
// PutSignatures, if called, MUST be called after PutManifest (signatures reference manifest contents)
// Finally, Commit MUST be called if the caller wants the image, as formed by the components saved above, to persist.
//
// Each ImageDestination should eventually be closed by calling Close().
type ImageDestination interface {
	// Reference returns the reference used to set up this destination.  Note that this should directly correspond to user's intent,
	// e.g. it should use the public hostname instead of the result of resolving CNAMEs or following redirects.
	Reference() ImageReference
	// Close removes resources associated with an initialized ImageDestination, if any.
	Close()

	// SupportedManifestMIMETypes tells which manifest mime types the destination supports
	// If an empty slice or nil it's returned, then any mime type can be tried to upload
	SupportedManifestMIMETypes() []string
	// SupportsSignatures returns an error (to be displayed to the user) if the destination certainly can't store signatures.
	// Note: It is still possible for PutSignatures to fail if SupportsSignatures returns nil.
	SupportsSignatures() error

	// PutBlob writes contents of stream and returns its computed digest and size.
	// A digest can be optionally provided if known, the specific image destination can decide to play with it or not.
	// The length of stream is expected to be expectedSize; if expectedSize == -1, it is not known.
	// WARNING: The contents of stream are being verified on the fly.  Until stream.Read() returns io.EOF, the contents of the data SHOULD NOT be available
	// to any other readers for download using the supplied digest.
	// If stream.Read() at any time, ESPECIALLY at end of input, returns an error, PutBlob MUST 1) fail, and 2) delete any data stored so far.
	PutBlob(stream io.Reader, digest string, expectedSize int64) (string, int64, error)
	// FIXME? This should also receive a MIME type if known, to differentiate between schema versions.
	PutManifest([]byte) error
	PutSignatures(signatures [][]byte) error
	// Commit marks the process of storing the image as successful and asks for the image to be persisted.
	// WARNING: This does not have any transactional semantics:
	// - Uploaded data MAY be visible to others before Commit() is called
	// - Uploaded data MAY be removed or MAY remain around if Close() is called without Commit() (i.e. rollback is allowed but not guaranteed)
	Commit() error
}

// Image is the primary API for inspecting properties of images.
// Each Image should eventually be closed by calling Close().
type Image interface {
	// Reference returns the reference used to set up this source, _as specified by the user_
	// (not as the image itself, or its underlying storage, claims).  This can be used e.g. to determine which public keys are trusted for this image.
	Reference() ImageReference
	// Close removes resources associated with an initialized Image, if any.
	Close()
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

// SystemContext allows parametrizing access to implicitly-accessed resources,
// like configuration files in /etc and users' login state in their home directory.
// Various components can share the same field only if their semantics is exactly
// the same; if in doubt, add a new field.
// It is always OK to pass nil instead of a SystemContext.
type SystemContext struct {
	// If not "", prefixed to any absolute paths used by default by the library (e.g. in /etc/).
	// Not used for any of the more specific path overrides available in this struct.
	// Not used for any paths specified by users in config files (even if the location of the config file _was_ affected by it).
	// NOTE: If this is set, environment-variable overrides of paths are ignored (to keep the semantics simple: to create an /etc replacement, just set RootForImplicitAbsolutePaths .
	// and there is no need to worry about the environment.)
	// NOTE: This does NOT affect paths starting by $HOME.
	RootForImplicitAbsolutePaths string

	// === Global configuration overrides ===
	// If not "", overrides the system's default path for signature.Policy configuration.
	SignaturePolicyPath string
	// If not "", overrides the system's default path for registries.d (Docker signature storage configuration)
	RegistriesDirPath string

	// === docker.Transport overrides ===
	DockerCertPath              string // If not "", a directory containing "cert.pem" and "key.pem" used when talking to a Docker Registry
	DockerInsecureSkipTLSVerify bool   // Allow contacting docker registries over HTTP, or HTTPS with failed TLS verification. Note that this does not affect other TLS connections.
}
