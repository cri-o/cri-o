package server

import (
	"encoding/json"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/kubernetes-incubator/cri-o/oci"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"golang.org/x/net/context"
	pb "k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
)

// ContainerStatus returns status of the container.
func (s *Server) ContainerStatus(ctx context.Context, req *pb.ContainerStatusRequest) (*pb.ContainerStatusResponse, error) {
	logrus.Debugf("ContainerStatusRequest %+v", req)
	c, err := s.getContainerFromRequest(req.ContainerId)
	if err != nil {
		return nil, err
	}

	if err = s.runtime.UpdateStatus(c); err != nil {
		logrus.Debugf("failed to get container status for %s: %v", c.ID, err)
	}

	containerID := c.ID()
	image := c.Image
	resp := &pb.ContainerStatusResponse{
		Status: &pb.ContainerStatus{
			Id:          containerID,
			Metadata:    c.Metadata,
			Labels:      c.Labels,
			Annotations: c.Annotations,
			Image:       image,
		},
	}

	mounts, err := s.getMounts(containerID)
	if err != nil {
		return nil, err
	}
	resp.Status.Mounts = mounts

	status, err := s.storageImageServer.ImageStatus(s.imageContext, image.Image)
	if err != nil {
		return nil, err
	}

	resp.Status.ImageRef = status.ID

	cState := s.runtime.ContainerStatus(c)
	rStatus := pb.ContainerState_CONTAINER_UNKNOWN

	if cState != nil {
		switch cState.Status {
		case oci.ContainerStateCreated:
			rStatus = pb.ContainerState_CONTAINER_CREATED
			created := cState.Created.UnixNano()
			resp.Status.CreatedAt = created
		case oci.ContainerStateRunning:
			rStatus = pb.ContainerState_CONTAINER_RUNNING
			created := cState.Created.UnixNano()
			resp.Status.CreatedAt = created
			started := cState.Started.UnixNano()
			resp.Status.StartedAt = started
		case oci.ContainerStateStopped:
			rStatus = pb.ContainerState_CONTAINER_EXITED
			created := cState.Created.UnixNano()
			resp.Status.CreatedAt = created
			started := cState.Started.UnixNano()
			resp.Status.StartedAt = started
			finished := cState.Finished.UnixNano()
			resp.Status.FinishedAt = finished
			resp.Status.ExitCode = cState.ExitCode
		default:
			resp.Status.CreatedAt = time.Time{}.UnixNano()
			resp.Status.StartedAt = time.Time{}.UnixNano()
		}
	}

	resp.Status.State = rStatus

	logrus.Debugf("ContainerStatusResponse: %+v", resp)
	return resp, nil
}

func (s *Server) getMounts(id string) ([]*pb.Mount, error) {
	config, err := s.store.GetFromContainerDirectory(id, "config.json")
	if err != nil {
		return nil, err
	}
	var m rspec.Spec
	if err = json.Unmarshal(config, &m); err != nil {
		return nil, err
	}
	isRO := func(m rspec.Mount) bool {
		var ro bool
		for _, o := range m.Options {
			if o == "ro" {
				ro = true
				break
			}
		}
		return ro
	}
	isBind := func(m rspec.Mount) bool {
		var bind bool
		for _, o := range m.Options {
			if o == "bind" || o == "rbind" {
				bind = true
				break
			}
		}
		return bind
	}
	mounts := []*pb.Mount{}
	for _, b := range m.Mounts {
		if !isBind(b) {
			continue
		}
		mounts = append(mounts, &pb.Mount{
			ContainerPath: b.Destination,
			HostPath:      b.Source,
			Readonly:      isRO(b),
		})
	}
	return mounts, nil
}
