package server

import (
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/Sirupsen/logrus"
	"github.com/containers/image/directory"
	"github.com/containers/image/image"
	"github.com/containers/image/transports"
	"golang.org/x/net/context"
	pb "k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
)

// ListImages lists existing images.
func (s *Server) ListImages(ctx context.Context, req *pb.ListImagesRequest) (*pb.ListImagesResponse, error) {
	logrus.Debugf("ListImages: %+v", req)
	// TODO
	// containers/storage will take care of this by looking inside /var/lib/ocid/images
	// and listing images.
	return &pb.ListImagesResponse{}, nil
}

// ImageStatus returns the status of the image.
func (s *Server) ImageStatus(ctx context.Context, req *pb.ImageStatusRequest) (*pb.ImageStatusResponse, error) {
	logrus.Debugf("ImageStatus: %+v", req)
	// TODO
	// containers/storage will take care of this by looking inside /var/lib/ocid/images
	// and getting the image status

	// HACK: Return fake status for now as the image pull actually happens in create container.
	image := req.GetImage().GetImage()
	return &pb.ImageStatusResponse{
		Image: &pb.Image{
			Id: &image,
		},
	}, nil
}

// PullImage pulls a image with authentication config.
func (s *Server) PullImage(ctx context.Context, req *pb.PullImageRequest) (*pb.PullImageResponse, error) {
	logrus.Debugf("PullImage: %+v", req)
	img := req.GetImage().GetImage()
	if img == "" {
		return nil, errors.New("got empty imagespec name")
	}

	// TODO(runcom): deal with AuthConfig in req.GetAuth()

	// TODO(mrunalp,runcom): why do we need the SandboxConfig here?
	// how do we pull in a specified sandbox?
	tr, err := transports.ParseImageName(img)
	if err != nil {
		return nil, err
	}
	// TODO(runcom): figure out the ImageContext story in containers/image instead of passing ("", true)
	src, err := tr.NewImageSource(nil, nil)
	if err != nil {
		return nil, err
	}
	i := image.FromSource(src)
	blobs, err := i.BlobDigests()
	if err != nil {
		return nil, err
	}

	if err = os.Mkdir(filepath.Join(s.config.ImageStore, tr.StringWithinTransport()), 0755); err != nil {
		return nil, err
	}
	dir, err := directory.NewReference(filepath.Join(s.config.ImageStore, tr.StringWithinTransport()))
	if err != nil {
		return nil, err
	}
	// TODO(runcom): figure out the ImageContext story in containers/image instead of passing ("", true)
	dest, err := dir.NewImageDestination(nil)
	if err != nil {
		return nil, err
	}
	// save blobs (layer + config for docker v2s2, layers only for docker v2s1 [the config is in the manifest])
	for _, b := range blobs {
		// TODO(runcom,nalin): we need do-then-commit to later purge on error
		var r io.ReadCloser
		r, _, err = src.GetBlob(b)
		if err != nil {
			return nil, err
		}
		if _, _, err = dest.PutBlob(r, b, -1); err != nil {
			r.Close()
			return nil, err
		}
		r.Close()
	}
	// save manifest
	m, _, err := i.Manifest()
	if err != nil {
		return nil, err
	}
	if err := dest.PutManifest(m); err != nil {
		return nil, err
	}

	// TODO: what else do we need here? (Signatures when the story isn't just pulling from docker://)

	return &pb.PullImageResponse{}, nil
}

// RemoveImage removes the image.
func (s *Server) RemoveImage(ctx context.Context, req *pb.RemoveImageRequest) (*pb.RemoveImageResponse, error) {
	logrus.Debugf("RemoveImage: %+v", req)
	return &pb.RemoveImageResponse{}, nil
}
