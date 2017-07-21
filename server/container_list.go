package server

import (
	"github.com/Sirupsen/logrus"
	"github.com/kubernetes-incubator/cri-o/oci"
	"golang.org/x/net/context"
	"k8s.io/apimachinery/pkg/fields"
	pb "k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
)

// filterContainer returns whether passed container matches filtering criteria
func filterContainer(c *pb.Container, filter *pb.ContainerFilter) bool {
	if filter != nil {
		if filter.State != nil {
			if c.State != filter.State.State {
				return false
			}
		}
		if filter.LabelSelector != nil {
			sel := fields.SelectorFromSet(filter.LabelSelector)
			if !sel.Matches(fields.Set(c.Labels)) {
				return false
			}
		}
	}
	return true
}

// ListContainers lists all containers by filters.
func (s *Server) ListContainers(ctx context.Context, req *pb.ListContainersRequest) (*pb.ListContainersResponse, error) {
	logrus.Debugf("ListContainersRequest %+v", req)
	var ctrs []*pb.Container
	filter := req.Filter
	ctrList := s.ContainerServer.ListContainers()

	// Filter using container id and pod id first.
	if filter != nil {
		if filter.Id != "" {
			id, err := s.CtrIDIndex().Get(filter.Id)
			if err != nil {
				return nil, err
			}
			c := s.ContainerServer.GetContainer(id)
			if c != nil {
				if filter.PodSandboxId != "" {
					if c.Sandbox() == filter.PodSandboxId {
						ctrList = []*oci.Container{c}
					} else {
						ctrList = []*oci.Container{}
					}

				} else {
					ctrList = []*oci.Container{c}
				}
			}
		} else {
			if filter.PodSandboxId != "" {
				pod := s.state.sandboxes[filter.PodSandboxId]
				if pod == nil {
					ctrList = []*oci.Container{}
				} else {
					ctrList = pod.Containers().List()
				}
			}
		}
	}

	for _, ctr := range ctrList {
		if err := s.Runtime().UpdateStatus(ctr); err != nil {
			return nil, err
		}

		podSandboxID := ctr.Sandbox()
		cState := s.Runtime().ContainerStatus(ctr)
		created := cState.Created.UnixNano()
		rState := pb.ContainerState_CONTAINER_UNKNOWN
		cID := ctr.ID()
		img := &pb.ImageSpec{
			Image: ctr.Image(),
		}
		c := &pb.Container{
			Id:           cID,
			PodSandboxId: podSandboxID,
			CreatedAt:    created,
			Labels:       ctr.Labels(),
			Metadata:     ctr.Metadata(),
			Annotations:  ctr.Annotations(),
			Image:        img,
		}

		switch cState.Status {
		case oci.ContainerStateCreated:
			rState = pb.ContainerState_CONTAINER_CREATED
		case oci.ContainerStateRunning:
			rState = pb.ContainerState_CONTAINER_RUNNING
		case oci.ContainerStateStopped:
			rState = pb.ContainerState_CONTAINER_EXITED
		}
		c.State = rState

		// Filter by other criteria such as state and labels.
		if filterContainer(c, req.Filter) {
			ctrs = append(ctrs, c)
		}
	}

	resp := &pb.ListContainersResponse{
		Containers: ctrs,
	}
	logrus.Debugf("ListContainersResponse: %+v", resp)
	return resp, nil
}
