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
	s.Update()
	var ctrs []*pb.Container
	filter := req.Filter
	ctrList, _ := s.state.GetAllContainers()

	// Filter using container id and pod id first.
	if filter != nil {
		if filter.Id != "" {
			c, err := s.state.LookupContainerByID(filter.Id)
			if err != nil {
				return nil, err
			}
			if filter.PodSandboxId != "" {
				if c.Sandbox() == filter.PodSandboxId {
					ctrList = []*oci.Container{c}
				} else {
					ctrList = []*oci.Container{}
				}

			} else {
				ctrList = []*oci.Container{c}
			}
		} else {
			if filter.PodSandboxId != "" {
				pod, err := s.state.GetSandbox(filter.PodSandboxId)
				// TODO check if this is a pod not found error, if not we should error out here
				if err != nil {
					ctrList = []*oci.Container{}
				} else {
					ctrList = pod.Containers()
				}
			}
		}
	}

	for _, ctr := range ctrList {
		if err := s.runtime.UpdateStatus(ctr); err != nil {
			return nil, err
		}

		podSandboxID := ctr.Sandbox()
		cState := s.runtime.ContainerStatus(ctr)
		created := cState.Created.UnixNano()
		rState := pb.ContainerState_CONTAINER_UNKNOWN
		cID := ctr.ID()

		c := &pb.Container{
			Id:           cID,
			PodSandboxId: podSandboxID,
			CreatedAt:    created,
			Labels:       ctr.Labels(),
			Metadata:     ctr.Metadata(),
			Annotations:  ctr.Annotations(),
			Image:        ctr.Image(),
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
