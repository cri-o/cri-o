package directory

import (
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

func (d *dirImageDestination) SupportedManifestMIMETypes() []string {
	return nil
}

func (d *dirImageDestination) PutManifest(manifest []byte) error {
	return ioutil.WriteFile(d.ref.manifestPath(), manifest, 0644)
}

func (d *dirImageDestination) PutBlob(digest string, stream io.Reader) error {
	layerFile, err := os.Create(d.ref.layerPath(digest))
	if err != nil {
		return err
	}
	defer layerFile.Close()
	if _, err := io.Copy(layerFile, stream); err != nil {
		return err
	}
	if err := layerFile.Sync(); err != nil {
		return err
	}
	return nil
}

func (d *dirImageDestination) PutSignatures(signatures [][]byte) error {
	for i, sig := range signatures {
		if err := ioutil.WriteFile(d.ref.signaturePath(i), sig, 0644); err != nil {
			return err
		}
	}
	return nil
}
