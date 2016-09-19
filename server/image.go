package server

import (
	"errors"

	ic "github.com/containers/image/copy"
	"github.com/containers/image/docker"
	"github.com/containers/image/signature"
	"github.com/containers/image/storage"
	"github.com/containers/image/transports"
	pb "github.com/kubernetes/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
	"golang.org/x/net/context"
)

// ListImages lists existing images.
func (s *Server) ListImages(ctx context.Context, req *pb.ListImagesRequest) (*pb.ListImagesResponse, error) {
	images, err := s.storage.Images()
	if err != nil {
		return nil, err
	}
	resp := pb.ListImagesResponse{}
	for _, image := range images {
		idCopy := image.ID
		resp.Images = append(resp.Images, &pb.Image{
			Id:       &idCopy,
			RepoTags: image.Names,
		})
	}
	return &resp, nil
}

// ImageStatus returns the status of the image.
func (s *Server) ImageStatus(ctx context.Context, req *pb.ImageStatusRequest) (*pb.ImageStatusResponse, error) {
	image, err := s.storage.GetImage(*(req.Image.Image))
	if err != nil {
		return nil, err
	}
	resp := pb.ImageStatusResponse{}
	idCopy := image.ID
	resp.Image = &pb.Image{
		Id:       &idCopy,
		RepoTags: image.Names,
	}
	return &resp, nil
}

// PullImage pulls a image with authentication config.
func (s *Server) PullImage(ctx context.Context, req *pb.PullImageRequest) (*pb.PullImageResponse, error) {
	img := req.GetImage().GetImage()
	if img == "" {
		return nil, errors.New("got empty imagespec name")
	}

	// TODO(runcom): deal with AuthConfig in req.GetAuth()
	// FIXME: pretend that the image coming from k8s is in docker format
	// because k8s haven't yet figured out a way for multiple image
	// formats - https://github.com/kubernetes/features/issues/63
	sr, err := transports.ParseImageName(docker.Transport.Name() + "://" + img)
	if err != nil {
		return nil, err
	}

	dr, err := transports.ParseImageName(storage.Transport.Name() + ":" + sr.DockerReference().String())
	if err != nil {
		return nil, err
	}

	policy, err := signature.DefaultPolicy(s.imageContext)
	if err != nil {
		return nil, err
	}

	pc, err := signature.NewPolicyContext(policy)
	if err != nil {
		return nil, err
	}

	err = ic.Image(s.imageContext, pc, dr, sr, &ic.Options{})
	if err != nil {
		return nil, err
	}

	return &pb.PullImageResponse{}, nil
}

// RemoveImage removes the image.
func (s *Server) RemoveImage(ctx context.Context, req *pb.RemoveImageRequest) (*pb.RemoveImageResponse, error) {
	_, err := s.storage.DeleteImage(*(req.Image.Image), true)
	return &pb.RemoveImageResponse{}, err
}
