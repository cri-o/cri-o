package directory

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"os"

	"github.com/containers/image/types"
)

type dirImageDestination struct {
	ref dirReference
}

// newImageDestination returns an ImageDestination for writing to an existing directory.
func newImageDestination(ref dirReference) types.ImageDestination {
	return &dirImageDestination{ref}
}

// Reference returns the reference used to set up this destination.  Note that this should directly correspond to user's intent,
// e.g. it should use the public hostname instead of the result of resolving CNAMEs or following redirects.
func (d *dirImageDestination) Reference() types.ImageReference {
	return d.ref
}

// Close removes resources associated with an initialized ImageDestination, if any.
func (d *dirImageDestination) Close() {
}

func (d *dirImageDestination) SupportedManifestMIMETypes() []string {
	return nil
}

// SupportsSignatures returns an error (to be displayed to the user) if the destination certainly can't store signatures.
// Note: It is still possible for PutSignatures to fail if SupportsSignatures returns nil.
func (d *dirImageDestination) SupportsSignatures() error {
	return nil
}

// PutBlob writes contents of stream and returns its computed digest and size.
// A digest can be optionally provided if known, the specific image destination can decide to play with it or not.
// The length of stream is expected to be expectedSize; if expectedSize == -1, it is not known.
// WARNING: The contents of stream are being verified on the fly.  Until stream.Read() returns io.EOF, the contents of the data SHOULD NOT be available
// to any other readers for download using the supplied digest.
// If stream.Read() at any time, ESPECIALLY at end of input, returns an error, PutBlob MUST 1) fail, and 2) delete any data stored so far.
func (d *dirImageDestination) PutBlob(stream io.Reader, digest string, expectedSize int64) (string, int64, error) {
	blobFile, err := ioutil.TempFile(d.ref.path, "dir-put-blob")
	if err != nil {
		return "", -1, err
	}
	succeeded := false
	defer func() {
		blobFile.Close()
		if !succeeded {
			os.Remove(blobFile.Name())
		}
	}()

	h := sha256.New()
	tee := io.TeeReader(stream, h)

	size, err := io.Copy(blobFile, tee)
	if err != nil {
		return "", -1, err
	}
	computedDigest := hex.EncodeToString(h.Sum(nil))
	if expectedSize != -1 && size != expectedSize {
		return "", -1, fmt.Errorf("Size mismatch when copying %s, expected %d, got %d", computedDigest, expectedSize, size)
	}
	if err := blobFile.Sync(); err != nil {
		return "", -1, err
	}
	if err := blobFile.Chmod(0644); err != nil {
		return "", -1, err
	}
	blobPath := d.ref.layerPath(computedDigest)
	if err := os.Rename(blobFile.Name(), blobPath); err != nil {
		return "", -1, err
	}
	succeeded = true
	return "sha256:" + computedDigest, size, nil
}

func (d *dirImageDestination) PutManifest(manifest []byte) error {
	return ioutil.WriteFile(d.ref.manifestPath(), manifest, 0644)
}

func (d *dirImageDestination) PutSignatures(signatures [][]byte) error {
	for i, sig := range signatures {
		if err := ioutil.WriteFile(d.ref.signaturePath(i), sig, 0644); err != nil {
			return err
		}
	}
	return nil
}

// Commit marks the process of storing the image as successful and asks for the image to be persisted.
// WARNING: This does not have any transactional semantics:
// - Uploaded data MAY be visible to others before Commit() is called
// - Uploaded data MAY be removed or MAY remain around if Close() is called without Commit() (i.e. rollback is allowed but not guaranteed)
func (d *dirImageDestination) Commit() error {
	return nil
}
