package docker

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strconv"

	"github.com/Sirupsen/logrus"
	"github.com/containers/image/manifest"
	"github.com/containers/image/types"
)

type dockerImageDestination struct {
	ref dockerReference
	c   *dockerClient
}

// newImageDestination creates a new ImageDestination for the specified image reference.
func newImageDestination(ctx *types.SystemContext, ref dockerReference) (types.ImageDestination, error) {
	c, err := newDockerClient(ctx, ref.ref.Hostname())
	if err != nil {
		return nil, err
	}
	return &dockerImageDestination{
		ref: ref,
		c:   c,
	}, nil
}

// Reference returns the reference used to set up this destination.  Note that this should directly correspond to user's intent,
// e.g. it should use the public hostname instead of the result of resolving CNAMEs or following redirects.
func (d *dockerImageDestination) Reference() types.ImageReference {
	return d.ref
}

// Close removes resources associated with an initialized ImageDestination, if any.
func (d *dockerImageDestination) Close() {
}

func (d *dockerImageDestination) SupportedManifestMIMETypes() []string {
	return []string{
		// TODO(runcom): we'll add OCI as part of another PR here
		manifest.DockerV2Schema2MediaType,
		manifest.DockerV2Schema1SignedMediaType,
		manifest.DockerV2Schema1MediaType,
	}
}

// SupportsSignatures returns an error (to be displayed to the user) if the destination certainly can't store signatures.
// Note: It is still possible for PutSignatures to fail if SupportsSignatures returns nil.
func (d *dockerImageDestination) SupportsSignatures() error {
	return fmt.Errorf("Pushing signatures to a Docker Registry is not supported")
}

// PutBlob writes contents of stream and returns its computed digest and size.
// A digest can be optionally provided if known, the specific image destination can decide to play with it or not.
// The length of stream is expected to be expectedSize; if expectedSize == -1, it is not known.
// WARNING: The contents of stream are being verified on the fly.  Until stream.Read() returns io.EOF, the contents of the data SHOULD NOT be available
// to any other readers for download using the supplied digest.
// If stream.Read() at any time, ESPECIALLY at end of input, returns an error, PutBlob MUST 1) fail, and 2) delete any data stored so far.
func (d *dockerImageDestination) PutBlob(stream io.Reader, digest string, expectedSize int64) (string, int64, error) {
	if digest != "" {
		checkURL := fmt.Sprintf(blobsURL, d.ref.ref.RemoteName(), digest)

		logrus.Debugf("Checking %s", checkURL)
		res, err := d.c.makeRequest("HEAD", checkURL, nil, nil)
		if err != nil {
			return "", -1, err
		}
		defer res.Body.Close()
		if res.StatusCode == http.StatusOK {
			logrus.Debugf("... already exists, not uploading")
			blobLength, err := strconv.ParseInt(res.Header.Get("Content-Length"), 10, 64)
			if err != nil {
				return "", -1, err
			}
			return digest, blobLength, nil
		}
		logrus.Debugf("... failed, status %d", res.StatusCode)
	}

	// FIXME? Chunked upload, progress reporting, etc.
	uploadURL := fmt.Sprintf(blobUploadURL, d.ref.ref.RemoteName())
	logrus.Debugf("Uploading %s", uploadURL)
	res, err := d.c.makeRequest("POST", uploadURL, nil, nil)
	if err != nil {
		return "", -1, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusAccepted {
		logrus.Debugf("Error initiating layer upload, response %#v", *res)
		return "", -1, fmt.Errorf("Error initiating layer upload to %s, status %d", uploadURL, res.StatusCode)
	}
	uploadLocation, err := res.Location()
	if err != nil {
		return "", -1, fmt.Errorf("Error determining upload URL: %s", err.Error())
	}

	h := sha256.New()
	tee := io.TeeReader(stream, h)
	res, err = d.c.makeRequestToResolvedURL("PATCH", uploadLocation.String(), map[string][]string{"Content-Type": {"application/octet-stream"}}, tee, expectedSize)
	if err != nil {
		logrus.Debugf("Error uploading layer chunked, response %#v", *res)
		return "", -1, err
	}
	defer res.Body.Close()
	hash := h.Sum(nil)
	computedDigest := "sha256:" + hex.EncodeToString(hash[:])

	uploadLocation, err = res.Location()
	if err != nil {
		return "", -1, fmt.Errorf("Error determining upload URL: %s", err.Error())
	}

	// FIXME: DELETE uploadLocation on failure

	locationQuery := uploadLocation.Query()
	// TODO: check digest == computedDigest https://github.com/containers/image/pull/70#discussion_r77646717
	locationQuery.Set("digest", computedDigest)
	uploadLocation.RawQuery = locationQuery.Encode()
	res, err = d.c.makeRequestToResolvedURL("PUT", uploadLocation.String(), map[string][]string{"Content-Type": {"application/octet-stream"}}, nil, -1)
	if err != nil {
		return "", -1, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		logrus.Debugf("Error uploading layer, response %#v", *res)
		return "", -1, fmt.Errorf("Error uploading layer to %s, status %d", uploadLocation, res.StatusCode)
	}

	logrus.Debugf("Upload of layer %s complete", digest)
	return computedDigest, res.Request.ContentLength, nil
}

func (d *dockerImageDestination) PutManifest(m []byte) error {
	// FIXME: This only allows upload by digest, not creating a tag.  See the
	// corresponding comment in openshift.NewImageDestination.
	digest, err := manifest.Digest(m)
	if err != nil {
		return err
	}
	url := fmt.Sprintf(manifestURL, d.ref.ref.RemoteName(), digest)

	headers := map[string][]string{}
	mimeType := manifest.GuessMIMEType(m)
	if mimeType != "" {
		headers["Content-Type"] = []string{mimeType}
	}
	res, err := d.c.makeRequest("PUT", url, headers, bytes.NewReader(m))
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		body, err := ioutil.ReadAll(res.Body)
		if err == nil {
			logrus.Debugf("Error body %s", string(body))
		}
		logrus.Debugf("Error uploading manifest, status %d, %#v", res.StatusCode, res)
		return fmt.Errorf("Error uploading manifest to %s, status %d", url, res.StatusCode)
	}
	return nil
}

func (d *dockerImageDestination) PutSignatures(signatures [][]byte) error {
	if len(signatures) != 0 {
		return fmt.Errorf("Pushing signatures to a Docker Registry is not supported")
	}
	return nil
}

// Commit marks the process of storing the image as successful and asks for the image to be persisted.
// WARNING: This does not have any transactional semantics:
// - Uploaded data MAY be visible to others before Commit() is called
// - Uploaded data MAY be removed or MAY remain around if Close() is called without Commit() (i.e. rollback is allowed but not guaranteed)
func (d *dockerImageDestination) Commit() error {
	return nil
}
