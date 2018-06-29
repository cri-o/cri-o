// +build libpod

package server

import (
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	pb "k8s.io/kubernetes/pkg/kubelet/apis/cri/runtime/v1alpha2"
)

// ImageStatus returns the status of the image.
func (s *Server) ImageStatus(ctx context.Context, req *pb.ImageStatusRequest) (resp *pb.ImageStatusResponse, err error) {
	const operation = "image_status"
	defer func() {
		recordOperation(operation, time.Now())
		recordError(operation, err)
	}()

	logrus.Debugf("ImageStatusRequest: %+v", req)

	image, err := s.ImageRuntime().NewFromLocal(req.Image.Image)
	if err != nil {
		return nil, err
	}

	// Discarding the err here as I dont believe not getting size should be fatal
	size, _ := image.Size(ctx)
	resp = &pb.ImageStatusResponse{
		Image: &pb.Image{
			Id:          image.ID(),
			RepoTags:    image.Names(),
			RepoDigests: image.RepoDigests(),
			Size_:       *size,
		},
	}

	uid, username := getUserFromImage(image.User)
	if uid != nil {
		resp.Image.Uid = &pb.Int64Value{Value: *uid}
	}
	resp.Image.Username = username
	logrus.Debugf("ImageStatusResponse: %+v", resp)
	return resp, nil
}

// getUserFromImage gets uid or user name of the image user.
// If user is numeric, it will be treated as uid; or else, it is treated as user name.
func getUserFromImage(user string) (*int64, string) {
	// return both empty if user is not specified in the image.
	if user == "" {
		return nil, ""
	}
	// split instances where the id may contain user:group
	user = strings.Split(user, ":")[0]
	// user could be either uid or user name. Try to interpret as numeric uid.
	uid, err := strconv.ParseInt(user, 10, 64)
	if err != nil {
		// If user is non numeric, assume it's user name.
		return nil, user
	}
	// If user is a numeric uid.
	return &uid, ""
}
