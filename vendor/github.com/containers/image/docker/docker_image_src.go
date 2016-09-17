package docker

import (
	"fmt"
	"io"
	"io/ioutil"
	"mime"
	"net/http"
	"strconv"

	"github.com/Sirupsen/logrus"
	"github.com/containers/image/manifest"
	"github.com/containers/image/types"
)

type errFetchManifest struct {
	statusCode int
	body       []byte
}

func (e errFetchManifest) Error() string {
	return fmt.Sprintf("error fetching manifest: status code: %d, body: %s", e.statusCode, string(e.body))
}

type dockerImageSource struct {
	ref                        dockerReference
	requestedManifestMIMETypes []string
	c                          *dockerClient
}

// newImageSource creates a new ImageSource for the specified image reference,
// asking the backend to use a manifest from requestedManifestMIMETypes if possible.
// nil requestedManifestMIMETypes means manifest.DefaultRequestedManifestMIMETypes.
// The caller must call .Close() on the returned ImageSource.
func newImageSource(ctx *types.SystemContext, ref dockerReference, requestedManifestMIMETypes []string) (*dockerImageSource, error) {
	c, err := newDockerClient(ctx, ref.ref.Hostname())
	if err != nil {
		return nil, err
	}
	if requestedManifestMIMETypes == nil {
		requestedManifestMIMETypes = manifest.DefaultRequestedManifestMIMETypes
	}
	return &dockerImageSource{
		ref: ref,
		requestedManifestMIMETypes: requestedManifestMIMETypes,
		c: c,
	}, nil
}

// Reference returns the reference used to set up this source, _as specified by the user_
// (not as the image itself, or its underlying storage, claims).  This can be used e.g. to determine which public keys are trusted for this image.
func (s *dockerImageSource) Reference() types.ImageReference {
	return s.ref
}

// Close removes resources associated with an initialized ImageSource, if any.
func (s *dockerImageSource) Close() {
}

// simplifyContentType drops parameters from a HTTP media type (see https://tools.ietf.org/html/rfc7231#section-3.1.1.1)
// Alternatively, an empty string is returned unchanged, and invalid values are "simplified" to an empty string.
func simplifyContentType(contentType string) string {
	if contentType == "" {
		return contentType
	}
	mimeType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return ""
	}
	return mimeType
}

func (s *dockerImageSource) GetManifest() ([]byte, string, error) {
	reference, err := s.ref.tagOrDigest()
	if err != nil {
		return nil, "", err
	}
	url := fmt.Sprintf(manifestURL, s.ref.ref.RemoteName(), reference)
	// TODO(runcom) set manifest version header! schema1 for now - then schema2 etc etc and v1
	// TODO(runcom) NO, switch on the resulter manifest like Docker is doing
	headers := make(map[string][]string)
	headers["Accept"] = s.requestedManifestMIMETypes
	res, err := s.c.makeRequest("GET", url, headers, nil)
	if err != nil {
		return nil, "", err
	}
	defer res.Body.Close()
	manblob, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, "", err
	}
	if res.StatusCode != http.StatusOK {
		return nil, "", errFetchManifest{res.StatusCode, manblob}
	}
	// We might validate manblob against the Docker-Content-Digest header here to protect against transport errors.
	return manblob, simplifyContentType(res.Header.Get("Content-Type")), nil
}

// GetBlob returns a stream for the specified blob, and the blob’s size (or -1 if unknown).
func (s *dockerImageSource) GetBlob(digest string) (io.ReadCloser, int64, error) {
	url := fmt.Sprintf(blobsURL, s.ref.ref.RemoteName(), digest)
	logrus.Debugf("Downloading %s", url)
	res, err := s.c.makeRequest("GET", url, nil, nil)
	if err != nil {
		return nil, 0, err
	}
	if res.StatusCode != http.StatusOK {
		// print url also
		return nil, 0, fmt.Errorf("Invalid status code returned when fetching blob %d", res.StatusCode)
	}
	size, err := strconv.ParseInt(res.Header.Get("Content-Length"), 10, 64)
	if err != nil {
		size = -1
	}
	return res.Body, size, nil
}

func (s *dockerImageSource) GetSignatures() ([][]byte, error) {
	return [][]byte{}, nil
}

// deleteImage deletes the named image from the registry, if supported.
func deleteImage(ctx *types.SystemContext, ref dockerReference) error {
	c, err := newDockerClient(ctx, ref.ref.Hostname())
	if err != nil {
		return err
	}

	// When retrieving the digest from a registry >= 2.3 use the following header:
	//   "Accept": "application/vnd.docker.distribution.manifest.v2+json"
	headers := make(map[string][]string)
	headers["Accept"] = []string{manifest.DockerV2Schema2MediaType}

	reference, err := ref.tagOrDigest()
	if err != nil {
		return err
	}
	getURL := fmt.Sprintf(manifestURL, ref.ref.RemoteName(), reference)
	get, err := c.makeRequest("GET", getURL, headers, nil)
	if err != nil {
		return err
	}
	defer get.Body.Close()
	body, err := ioutil.ReadAll(get.Body)
	if err != nil {
		return err
	}
	switch get.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return fmt.Errorf("Unable to delete %v. Image may not exist or is not stored with a v2 Schema in a v2 registry.", ref.ref)
	default:
		return fmt.Errorf("Failed to delete %v: %s (%v)", ref.ref, string(body), get.Status)
	}

	digest := get.Header.Get("Docker-Content-Digest")
	deleteURL := fmt.Sprintf(manifestURL, ref.ref.RemoteName(), digest)

	// When retrieving the digest from a registry >= 2.3 use the following header:
	//   "Accept": "application/vnd.docker.distribution.manifest.v2+json"
	delete, err := c.makeRequest("DELETE", deleteURL, headers, nil)
	if err != nil {
		return err
	}
	defer delete.Body.Close()

	body, err = ioutil.ReadAll(delete.Body)
	if err != nil {
		return err
	}
	if delete.StatusCode != http.StatusAccepted {
		return fmt.Errorf("Failed to delete %v: %s (%v)", deleteURL, string(body), delete.Status)
	}

	return nil
}
