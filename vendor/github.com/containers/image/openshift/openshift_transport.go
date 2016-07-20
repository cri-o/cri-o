package openshift

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"

	"github.com/Sirupsen/logrus"
	"github.com/containers/image/docker/policyconfiguration"
	"github.com/containers/image/types"
	"github.com/docker/docker/reference"
)

// Transport is an ImageTransport for directory paths.
var Transport = openshiftTransport{}

type openshiftTransport struct{}

func (t openshiftTransport) Name() string {
	return "atomic"
}

// ParseReference converts a string, which should not start with the ImageTransport.Name prefix, into an ImageReference.
func (t openshiftTransport) ParseReference(reference string) (types.ImageReference, error) {
	return ParseReference(reference)
}

// Note that imageNameRegexp is namespace/stream:tag, this
// is HOSTNAME/namespace/stream:tag or parent prefixes.
// Keep this in sync with imageNameRegexp!
var scopeRegexp = regexp.MustCompile("^[^/]*(/[^:/]*(/[^:/]*(:[^:/]*)?)?)?$")

// ValidatePolicyConfigurationScope checks that scope is a valid name for a signature.PolicyTransportScopes keys
// (i.e. a valid PolicyConfigurationIdentity() or PolicyConfigurationNamespaces() return value).
// It is acceptable to allow an invalid value which will never be matched, it can "only" cause user confusion.
// scope passed to this function will not be "", that value is always allowed.
func (t openshiftTransport) ValidatePolicyConfigurationScope(scope string) error {
	if scopeRegexp.FindStringIndex(scope) == nil {
		return fmt.Errorf("Invalid scope name %s", scope)
	}
	return nil
}

// openshiftReference is an ImageReference for OpenShift images.
type openshiftReference struct {
	baseURL         *url.URL
	namespace       string
	stream          string
	tag             string
	dockerReference reference.Named // Computed from the above in advance, so that later references can not fail.
}

// FIXME: Is imageName like this a good way to refer to OpenShift images?
// Keep this in sync with scopeRegexp!
var imageNameRegexp = regexp.MustCompile("^([^:/]*)/([^:/]*):([^:/]*)$")

// ParseReference converts a string, which should not start with the ImageTransport.Name prefix, into an OpenShift ImageReference.
func ParseReference(reference string) (types.ImageReference, error) {
	// Overall, this is modelled on openshift/origin/pkg/cmd/util/clientcmd.New().ClientConfig() and openshift/origin/pkg/client.
	cmdConfig := defaultClientConfig()
	logrus.Debugf("cmdConfig: %#v", cmdConfig)
	restConfig, err := cmdConfig.ClientConfig()
	if err != nil {
		return nil, err
	}
	// REMOVED: SetOpenShiftDefaults (values are not overridable in config files, so hard-coded these defaults.)
	logrus.Debugf("restConfig: %#v", restConfig)
	baseURL, _, err := restClientFor(restConfig)
	if err != nil {
		return nil, err
	}
	logrus.Debugf("URL: %#v", *baseURL)

	m := imageNameRegexp.FindStringSubmatch(reference)
	if m == nil || len(m) != 4 {
		return nil, fmt.Errorf("Invalid image reference %s, %#v", reference, m)
	}

	return NewReference(baseURL, m[1], m[2], m[3])
}

// NewReference returns an OpenShift reference for a base URL, namespace, stream and tag.
func NewReference(baseURL *url.URL, namespace, stream, tag string) (types.ImageReference, error) {
	// Precompute also dockerReference so that later references can not fail.
	//
	// This discards ref.baseURL.Path, which is unexpected for a “base URL”;
	// but openshiftClient.doRequest actually completely overrides url.Path
	// (and defaultServerURL rejects non-trivial Path values), so it is OK for
	// us to ignore it as well.
	//
	// FIXME: This is, strictly speaking, a namespace conflict with images placed in a Docker registry running on the same host.
	// Do we need to do something else, perhaps disambiguate (port number?) or namespace Docker and OpenShift separately?
	dockerRef, err := reference.WithName(fmt.Sprintf("%s/%s/%s", baseURL.Host, namespace, stream))
	if err != nil {
		return nil, err
	}
	dockerRef, err = reference.WithTag(dockerRef, tag)
	if err != nil {
		return nil, err
	}

	return openshiftReference{
		baseURL:         baseURL,
		namespace:       namespace,
		stream:          stream,
		tag:             tag,
		dockerReference: dockerRef,
	}, nil
}

func (ref openshiftReference) Transport() types.ImageTransport {
	return Transport
}

// StringWithinTransport returns a string representation of the reference, which MUST be such that
// reference.Transport().ParseReference(reference.StringWithinTransport()) returns an equivalent reference.
// NOTE: The returned string is not promised to be equal to the original input to ParseReference;
// e.g. default attribute values omitted by the user may be filled in in the return value, or vice versa.
// WARNING: Do not use the return value in the UI to describe an image, it does not contain the Transport().Name() prefix.
func (ref openshiftReference) StringWithinTransport() string {
	return fmt.Sprintf("%s/%s:%s", ref.namespace, ref.stream, ref.tag)
}

// DockerReference returns a Docker reference associated with this reference
// (fully explicit, i.e. !reference.IsNameOnly, but reflecting user intent,
// not e.g. after redirect or alias processing), or nil if unknown/not applicable.
func (ref openshiftReference) DockerReference() reference.Named {
	return ref.dockerReference
}

// PolicyConfigurationIdentity returns a string representation of the reference, suitable for policy lookup.
// This MUST reflect user intent, not e.g. after processing of third-party redirects or aliases;
// The value SHOULD be fully explicit about its semantics, with no hidden defaults, AND canonical
// (i.e. various references with exactly the same semantics should return the same configuration identity)
// It is fine for the return value to be equal to StringWithinTransport(), and it is desirable but
// not required/guaranteed that it will be a valid input to Transport().ParseReference().
// Returns "" if configuration identities for these references are not supported.
func (ref openshiftReference) PolicyConfigurationIdentity() string {
	res, err := policyconfiguration.DockerReferenceIdentity(ref.dockerReference)
	if res == "" || err != nil { // Coverage: Should never happen, NewReference constructs a valid tagged reference.
		panic(fmt.Sprintf("Internal inconsistency: policyconfiguration.DockerReferenceIdentity returned %#v, %v", res, err))
	}
	return res
}

// PolicyConfigurationNamespaces returns a list of other policy configuration namespaces to search
// for if explicit configuration for PolicyConfigurationIdentity() is not set.  The list will be processed
// in order, terminating on first match, and an implicit "" is always checked at the end.
// It is STRONGLY recommended for the first element, if any, to be a prefix of PolicyConfigurationIdentity(),
// and each following element to be a prefix of the element preceding it.
func (ref openshiftReference) PolicyConfigurationNamespaces() []string {
	return policyconfiguration.DockerReferenceNamespaces(ref.dockerReference)
}

// NewImage returns a types.Image for this reference.
func (ref openshiftReference) NewImage(certPath string, tlsVerify bool) (types.Image, error) {
	return nil, errors.New("Full Image support not implemented for atomic: image names")
}

// NewImageSource returns a types.ImageSource for this reference.
func (ref openshiftReference) NewImageSource(certPath string, tlsVerify bool) (types.ImageSource, error) {
	return newImageSource(ref, certPath, tlsVerify)
}

// NewImageDestination returns a types.ImageDestination for this reference.
func (ref openshiftReference) NewImageDestination(certPath string, tlsVerify bool) (types.ImageDestination, error) {
	return newImageDestination(ref, certPath, tlsVerify)
}
