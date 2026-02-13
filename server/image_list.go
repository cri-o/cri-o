package server

import (
	"context"
	"errors"

	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/internal/ociartifact"
	"github.com/cri-o/cri-o/internal/storage"
)

// ListImages lists existing images.
func (s *Server) ListImages(ctx context.Context, req *types.ListImagesRequest) (*types.ListImagesResponse, error) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()

	images, err := s.listImages(ctx, req.GetFilter())
	if err != nil {
		return nil, err
	}

	return &types.ListImagesResponse{
		Images: images,
	}, nil
}

// StreamImages returns a stream of images.
func (s *Server) StreamImages(req *types.StreamImagesRequest, stream types.ImageService_StreamImagesServer) error {
	ctx := stream.Context()

	images, err := s.listImages(ctx, req.GetFilter())
	if err != nil {
		return err
	}

	for _, image := range images {
		if err := stream.Send(&types.StreamImagesResponse{
			Image: image,
		}); err != nil {
			return err
		}
	}

	return nil
}

// listImages returns a filtered list of images.
func (s *Server) listImages(ctx context.Context, filter *types.ImageFilter) ([]*types.Image, error) {
	if filter != nil {
		if filterImage := filter.GetImage(); filterImage != nil && filterImage.GetImage() != "" {
			// Historically CRI-O has interpreted the "filter" as a single image to look up.
			// Also, the type of the value is types.ImageSpec, the value used to refer to a single image.
			// And, ultimately, Kubelet never uses the filter.
			// So, fall back to existing code instead of having an extra code path doing some kind of filtering.
			status, err := s.storageImageStatus(ctx, filterImage)
			if err != nil {
				return nil, err
			}

			var images []*types.Image

			if status != nil {
				images = append(images, ConvertImage(status))
			}

			if artifact, err := s.ArtifactStore().Status(ctx, filterImage.GetImage()); err == nil {
				images = append(images, artifact.CRIImage())
			} else if !errors.Is(err, ociartifact.ErrNotFound) {
				log.Errorf(ctx, "Unable to get filtered artifact: %v", err)
			}

			return images, nil
		}
	}

	results, err := s.ContainerServer.StorageImageServer().ListImages(s.config.SystemContext)
	if err != nil {
		return nil, err
	}

	var images []*types.Image

	for i := range results {
		image := ConvertImage(&results[i])
		images = append(images, image)
	}

	artifacts, err := s.ArtifactStore().List(ctx)
	if err != nil {
		log.Warnf(ctx, "Unable to list artifacts: %v", err)
	}

	for _, a := range artifacts {
		images = append(images, a.CRIImage())
	}

	return images, nil
}

// ConvertImage takes an containers/storage ImageResult and converts it into a
// CRI protobuf type. More information about the "why"s of this function can be
// found in ../cri.md.
func ConvertImage(from *storage.ImageResult) *types.Image {
	if from == nil {
		return nil
	}

	repoTags := []string{}
	repoDigests := []string{}

	if len(from.RepoTags) > 0 {
		repoTags = from.RepoTags
	}

	if len(from.RepoDigests) > 0 {
		repoDigests = from.RepoDigests
	} else if from.PreviousName != "" && from.Digest != "" {
		repoDigests = []string{from.PreviousName + "@" + string(from.Digest)}
	}

	to := &types.Image{
		Id:          from.ID.IDStringForOutOfProcessConsumptionOnly(),
		RepoTags:    repoTags,
		RepoDigests: repoDigests,
		Pinned:      from.Pinned,
	}

	uid, username := getUserFromImage(from.User)
	to.Username = username

	if uid != nil {
		to.Uid = &types.Int64Value{Value: *uid}
	}

	if from.Size != nil {
		to.Size = *from.Size
	}

	return to
}
