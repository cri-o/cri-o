package server

import (
	"encoding/base64"
	"strings"

	"github.com/containers/image/v5/copy"
	"github.com/containers/image/v5/types"
	"github.com/cri-o/cri-o/internal/pkg/log"
	"github.com/cri-o/cri-o/internal/pkg/storage"
	"golang.org/x/net/context"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// PullImage pulls a image with authentication config.
func (s *Server) PullImage(ctx context.Context, req *pb.PullImageRequest) (resp *pb.PullImageResponse, err error) {
	// TODO: what else do we need here? (Signatures when the story isn't just pulling from docker://)
	image := ""
	img := req.GetImage()
	if img != nil {
		image = img.Image
	}

	sourceCtx := *s.systemContext // A shallow copy we can modify
	if req.GetAuth() != nil {
		username := req.GetAuth().Username
		password := req.GetAuth().Password
		if req.GetAuth().Auth != "" {
			username, password, err = decodeDockerAuth(req.GetAuth().Auth)
			if err != nil {
				log.Debugf(ctx, "error decoding authentication for image %s: %v", img, err)
				return nil, err
			}
		}
		// Specifying a username indicates the user intends to send authentication to the registry.
		if username != "" {
			sourceCtx.DockerAuthConfig = &types.DockerAuthConfig{
				Username: username,
				Password: password,
			}
		}
	}

	var (
		images []string
		pulled string
	)
	images, err = s.StorageImageServer().ResolveNames(s.systemContext, image)
	if err != nil {
		return nil, err
	}
	for _, img := range images {
		var tmpImg types.ImageCloser
		tmpImg, err = s.StorageImageServer().PrepareImage(&sourceCtx, img)
		if err != nil {
			log.Debugf(ctx, "error preparing image %s: %v", img, err)
			continue
		}
		defer tmpImg.Close()

		var storedImage *storage.ImageResult
		storedImage, err = s.StorageImageServer().ImageStatus(s.systemContext, img)
		if err == nil {
			tmpImgConfigDigest := tmpImg.ConfigInfo().Digest
			if tmpImgConfigDigest.String() == "" {
				// this means we are playing with a schema1 image, in which
				// case, we're going to repull the image in any case
				log.Debugf(ctx, "image config digest is empty, re-pulling image")
			} else if tmpImgConfigDigest.String() == storedImage.ConfigDigest.String() {
				log.Debugf(ctx, "image %s already in store, skipping pull", img)
				pulled = img
				break
			}
			log.Debugf(ctx, "image in store has different ID, re-pulling %s", img)
		}

		_, err = s.StorageImageServer().PullImage(s.systemContext, img, &copy.Options{
			SourceCtx:      &sourceCtx,
			DestinationCtx: s.systemContext,
		})
		if err != nil {
			log.Debugf(ctx, "error pulling image %s: %v", img, err)
			continue
		}
		pulled = img
		break
	}
	if pulled == "" && err != nil {
		return nil, err
	}
	status, err := s.StorageImageServer().ImageStatus(s.systemContext, pulled)
	if err != nil {
		return nil, err
	}
	imageRef := status.ID
	if len(status.RepoDigests) > 0 {
		imageRef = status.RepoDigests[0]
	}
	resp = &pb.PullImageResponse{
		ImageRef: imageRef,
	}
	return resp, nil
}

func decodeDockerAuth(s string) (user, password string, err error) {
	decoded, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return "", "", err
	}
	parts := strings.SplitN(string(decoded), ":", 2)
	if len(parts) != 2 {
		// if it's invalid just skip, as docker does
		return "", "", nil
	}
	user = parts[0]
	password = strings.Trim(parts[1], "\x00")
	return user, password, nil
}
