package server

import (
	"encoding/base64"
	"strings"

	"github.com/Sirupsen/logrus"
	"github.com/containers/image/copy"
	"github.com/containers/image/types"
	"golang.org/x/net/context"
	pb "k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
)

// PullImage pulls a image with authentication config.
func (s *Server) PullImage(ctx context.Context, req *pb.PullImageRequest) (*pb.PullImageResponse, error) {
	logrus.Debugf("PullImageRequest: %+v", req)
	// TODO(runcom?): deal with AuthConfig in req.GetAuth()
	// TODO: what else do we need here? (Signatures when the story isn't just pulling from docker://)
	image := ""
	img := req.GetImage()
	if img != nil {
		image = img.Image
	}

	var (
		username string
		password string
	)
	if req.GetAuth() != nil {
		username = req.GetAuth().Username
		password = req.GetAuth().Password
		if req.GetAuth().Auth != "" {
			var err error
			username, password, err = decodeDockerAuth(req.GetAuth().Auth)
			if err != nil {
				return nil, err
			}
		}
	}
	options := &copy.Options{
		SourceCtx: &types.SystemContext{},
	}
	// a not empty username should be sufficient to decide whether to send auth
	// or not I guess
	if username != "" {
		options.SourceCtx = &types.SystemContext{
			DockerAuthConfig: &types.DockerAuthConfig{
				Username: username,
				Password: password,
			},
		}
	}

	canPull, err := s.storageImageServer.CanPull(image, options)
	if err != nil && !canPull {
		return nil, err
	}

	// let's be smart, docker doesn't repull if image already exists.
	if _, err := s.storageImageServer.ImageStatus(s.imageContext, image); err == nil {
		return &pb.PullImageResponse{
			ImageRef: image,
		}, nil
	}

	if _, err := s.storageImageServer.PullImage(s.imageContext, image, options); err != nil {
		return nil, err
	}
	resp := &pb.PullImageResponse{
		ImageRef: image,
	}
	logrus.Debugf("PullImageResponse: %+v", resp)
	return resp, nil
}

func decodeDockerAuth(s string) (string, string, error) {
	decoded, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return "", "", err
	}
	parts := strings.SplitN(string(decoded), ":", 2)
	if len(parts) != 2 {
		// if it's invalid just skip, as docker does
		return "", "", nil
	}
	user := parts[0]
	password := strings.Trim(parts[1], "\x00")
	return user, password, nil
}
