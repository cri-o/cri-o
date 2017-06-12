package server

import (
	"encoding/json"
	"fmt"

	"github.com/Sirupsen/logrus"
	"github.com/docker/distribution/reference"
	"github.com/kubernetes-incubator/cri-o/oci"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"golang.org/x/net/context"
	pb "k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
)

const (
	oomKilledReason = "OOMKilled"
	completedReason = "Completed"
	errorReason     = "Error"
)

// ContainerStatus returns status of the container.
func (s *Server) ContainerStatus(ctx context.Context, req *pb.ContainerStatusRequest) (*pb.ContainerStatusResponse, error) {
	logrus.Debugf("ContainerStatusRequest %+v", req)
	c, err := s.getContainerFromRequest(req.ContainerId)
	if err != nil {
		return nil, err
	}

	if err = s.runtime.UpdateStatus(c); err != nil {
		return nil, err
	}
	s.containerStateToDisk(c)

	containerID := c.ID()
	resp := &pb.ContainerStatusResponse{
		Status: &pb.ContainerStatus{
			Id:          containerID,
			Metadata:    c.Metadata(),
			Labels:      c.Labels(),
			Annotations: c.Annotations(),
		},
	}

	mounts, err := s.getMounts(containerID)
	if err != nil {
		return nil, err
	}
	resp.Status.Mounts = mounts

	imageName := c.Image().Image
	status, err := s.storageImageServer.ImageStatus(s.imageContext, imageName)
	if err != nil {
		return nil, err
	}

	imageRef := status.ID
	//
	// TODO: https://github.com/kubernetes-incubator/cri-o/issues/531
	//
	//for _, n := range status.Names {
	//r, err := reference.ParseNormalizedNamed(n)
	//if err != nil {
	//return nil, fmt.Errorf("failed to normalize image name for ImageRef: %v", err)
	//}
	//if digested, isDigested := r.(reference.Canonical); isDigested {
	//imageRef = reference.FamiliarString(digested)
	//break
	//}
	//}
	resp.Status.ImageRef = imageRef

	for _, n := range status.Names {
		r, err := reference.ParseNormalizedNamed(n)
		if err != nil {
			return nil, fmt.Errorf("failed to normalize image name for Image: %v", err)
		}
		if tagged, isTagged := r.(reference.Tagged); isTagged {
			imageName = reference.FamiliarString(tagged)
			break
		}
	}
	resp.Status.Image = &pb.ImageSpec{Image: imageName}

	cState := s.runtime.ContainerStatus(c)
	rStatus := pb.ContainerState_CONTAINER_UNKNOWN

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
		switch {
		case cState.OOMKilled:
			resp.Status.Reason = oomKilledReason
		case cState.ExitCode == 0:
			resp.Status.Reason = completedReason
		default:
			resp.Status.Reason = errorReason
			resp.Status.Message = cState.Error
		}
	}

	resp.Status.State = rStatus

	logrus.Debugf("ContainerStatusResponse: %+v", resp)
	return resp, nil
}

func (s *Server) getMounts(id string) ([]*pb.Mount, error) {
	config, err := s.store.FromContainerDirectory(id, "config.json")
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
