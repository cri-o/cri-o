package server

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/containers/storage"
	"github.com/cri-o/cri-o/internal/pkg/log"
	pkgstorage "github.com/cri-o/cri-o/internal/pkg/storage"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// ImageStatus returns the status of the image.
func (s *Server) ImageStatus(ctx context.Context, req *pb.ImageStatusRequest) (resp *pb.ImageStatusResponse, err error) {
	image := ""
	img := req.GetImage()
	if img != nil {
		image = img.Image
	}
	if image == "" {
		return nil, fmt.Errorf("no image specified")
	}
	images, err := s.StorageImageServer().ResolveNames(s.systemContext, image)
	if err != nil {
		if err == pkgstorage.ErrCannotParseImageID {
			images = append(images, image)
		} else {
			return nil, err
		}
	}
	var (
		notfound bool
		lastErr  error
	)
	for _, image := range images {
		status, err := s.StorageImageServer().ImageStatus(s.systemContext, image)
		if err != nil {
			if errors.Cause(err) == storage.ErrImageUnknown {
				log.Warnf(ctx, "imageStatus: can't find %s", image)
				notfound = true
				continue
			}
			log.Warnf(ctx, "imageStatus: error getting status from %s: %v", image, err)
			lastErr = err
			continue
		}

		// Ensure that size is already defined
		var size uint64
		if status.Size == nil {
			size = 0
		} else {
			size = *status.Size
		}

		resp = &pb.ImageStatusResponse{
			Image: &pb.Image{
				Id:          status.ID,
				RepoTags:    status.RepoTags,
				RepoDigests: status.RepoDigests,
				Size_:       size,
			},
		}
		uid, username := getUserFromImage(status.User)
		if uid != nil {
			resp.Image.Uid = &pb.Int64Value{Value: *uid}
		}
		resp.Image.Username = username
		break
	}
	if lastErr != nil && resp == nil {
		return nil, lastErr
	}
	if notfound && resp == nil {
		return &pb.ImageStatusResponse{}, nil
	}
	return resp, nil
}

// getUserFromImage gets uid or user name of the image user.
// If user is numeric, it will be treated as uid; or else, it is treated as user name.
func getUserFromImage(user string) (id *int64, username string) {
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
