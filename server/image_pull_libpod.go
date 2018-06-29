// +build libpod

package server

import (
	"encoding/base64"
	"io"
	"strings"
	"time"

	li "github.com/projectatomic/libpod/libpod/image"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	pb "k8s.io/kubernetes/pkg/kubelet/apis/cri/runtime/v1alpha2"
)

// PullImage pulls a image with authentication config.
func (s *Server) PullImage(ctx context.Context, req *pb.PullImageRequest) (resp *pb.PullImageResponse, err error) {
	var (
		// assumption is that pull status is not shown
		writer io.Writer
	)

	const operation = "pull_image"
	defer func() {
		recordOperation(operation, time.Now())
		recordError(operation, err)
	}()

	logrus.Debugf("PullImageRequest: %+v", req)

	// TODO Reimplement userid and password authorization mechanisms
	dockerRegistryOptions := li.DockerRegistryOptions{}

	image, err := s.ImageRuntime().New(ctx, req.Image.Image, "", "", writer, &dockerRegistryOptions, li.SigningOptions{}, false, true)
	if err != nil {
		return nil, err
	}
	imageRef := image.ID()
	resp = &pb.PullImageResponse{
		ImageRef: imageRef,
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
