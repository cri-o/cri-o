package server

import (
	"fmt"
	"time"

	"github.com/cri-o/cri-o/pkg/storage"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// RemoveImage removes the image. If the image does not exist, then this
// function should not error at all.
//
// For further documentation, see:
// https://github.com/kubernetes/cri-api/blob/261df499b74595bc2ce546130f1edfcc6f05d2c1/pkg/apis/runtime/v1alpha2/api.proto#L123-L124
// https://github.com/kubernetes/kubernetes/blob/71a7be41e0518a7df549eb1fa7b51e7c647d985b/pkg/kubelet/dockershim/docker_image.go#L122-L156
func (s *Server) RemoveImage(ctx context.Context, req *pb.RemoveImageRequest) (resp *pb.RemoveImageResponse, err error) {
	const operation = "remove_image"
	defer func() {
		recordOperation(operation, time.Now())
		recordError(operation, err)
	}()

	logrus.Debugf("%s: request: %+v", operation, req)
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
		if err == storage.ErrCannotParseImageID {
			images = append(images, image)
		} else {
			return nil, err
		}
	}
	for _, img := range images {
		err = s.StorageImageServer().UntagImage(s.systemContext, img)
		if err != nil {
			logrus.Infof("%s: error deleting image %s: %v", operation, img, err)
			continue
		}
		break
	}
	resp = &pb.RemoveImageResponse{}
	logrus.Debugf("%s: response: %+v", operation, resp)
	return resp, nil
}
