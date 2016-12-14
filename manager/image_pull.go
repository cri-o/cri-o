package manager

import (
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/containers/image/directory"
	"github.com/containers/image/image"
	"github.com/containers/image/transports"
	pb "k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
)

// PullImage pulls a image with authentication config.
func (m *Manager) PullImage(imageSpec *pb.ImageSpec, auth *pb.AuthConfig, sandboxConfig *pb.PodSandboxConfig) error {
	img := imageSpec.GetImage()
	if img == "" {
		return errors.New("got empty imagespec name")
	}

	// TODO(runcom): deal with AuthConfig in req.GetAuth()

	// TODO(mrunalp,runcom): why do we need the SandboxConfig here?
	// how do we pull in a specified sandbox?
	tr, err := transports.ParseImageName(img)
	if err != nil {
		return err
	}
	// TODO(runcom): figure out the ImageContext story in containers/image instead of passing ("", true)
	src, err := tr.NewImageSource(nil, nil)
	if err != nil {
		return err
	}
	i := image.FromSource(src)
	blobs, err := i.BlobDigests()
	if err != nil {
		return err
	}

	if err = os.Mkdir(filepath.Join(m.config.ImageDir, tr.StringWithinTransport()), 0755); err != nil {
		return err
	}
	dir, err := directory.NewReference(filepath.Join(m.config.ImageDir, tr.StringWithinTransport()))
	if err != nil {
		return err
	}
	// TODO(runcom): figure out the ImageContext story in containers/image instead of passing ("", true)
	dest, err := dir.NewImageDestination(nil)
	if err != nil {
		return err
	}
	// save blobs (layer + config for docker v2s2, layers only for docker v2s1 [the config is in the manifest])
	for _, b := range blobs {
		// TODO(runcom,nalin): we need do-then-commit to later purge on error
		var r io.ReadCloser
		r, _, err = src.GetBlob(b)
		if err != nil {
			return err
		}
		if _, _, err = dest.PutBlob(r, b, -1); err != nil {
			r.Close()
			return err
		}
		r.Close()
	}
	// save manifest
	mf, _, err := i.Manifest()
	if err != nil {
		return err
	}
	if err := dest.PutManifest(mf); err != nil {
		return err
	}

	// TODO: what else do we need here? (Signatures when the story isn't just pulling from docker://)

	return nil
}
