package server

import (
	"encoding/base64"
	"strings"
	"time"

	"github.com/containers/image/copy"
	"github.com/containers/image/types"
	"github.com/cri-o/cri-o/pkg/storage"
	"github.com/cri-o/cri-o/server/useragent"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// PullImage pulls a image with authentication config.
func (s *Server) PullImage(ctx context.Context, req *pb.PullImageRequest) (resp *pb.PullImageResponse, err error) {
	const operation = "pull_image"
	defer func() {
		recordOperation(operation, time.Now())
		recordError(operation, err)
	}()

	logrus.Debugf("PullImageRequest: %+v", req)
	// TODO: what else do we need here? (Signatures when the story isn't just pulling from docker://)
	image := ""
	img := req.GetImage()
	if img != nil {
		image = img.Image
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
		sourceCtx := *s.systemContext // A shallow copy we can modify
		sourceCtx.AuthFilePath = s.config.GlobalAuthFile

		if req.GetAuth() != nil {
			username := req.GetAuth().Username
			password := req.GetAuth().Password
			if req.GetAuth().Auth != "" {
				username, password, err = decodeDockerAuth(req.GetAuth().Auth)
				if err != nil {
					logrus.Debugf("error decoding authentication for image %s: %v", img, err)
					continue
				}
			}
		}
		sourceCtx := *s.systemContext // A shallow copy we can modify
		sourceCtx.DockerRegistryUserAgent = useragent.Get(ctx)
		sourceCtx.AuthFilePath = s.config.GlobalAuthFile

		// Specifying a username indicates the user intends to send authentication to the registry.
		if username != "" {
			sourceCtx.DockerAuthConfig = &types.DockerAuthConfig{
				Username: username,
				Password: password,
			}
		}

		var tmpImg types.ImageCloser
		tmpImg, err = s.StorageImageServer().PrepareImage(&sourceCtx, img)
		if err != nil {
			logrus.Debugf("error preparing image %s: %v", img, err)
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
				logrus.Debugf("image config digest is empty, re-pulling image")
			} else if tmpImgConfigDigest.String() == storedImage.ConfigDigest.String() {
				logrus.Debugf("image %s already in store, skipping pull", img)
				pulled = img
				break
			}
			logrus.Debugf("image in store has different ID, re-pulling %s", img)
		}

		_, err = s.StorageImageServer().PullImage(s.systemContext, img, &copy.Options{
			SourceCtx:      &sourceCtx,
			DestinationCtx: s.systemContext,
		})
		if err != nil {
			logrus.Debugf("error pulling image %s: %v", img, err)
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
	logrus.Debugf("PullImageResponse: %+v", resp)
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
