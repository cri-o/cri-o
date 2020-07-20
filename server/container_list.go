package server

import (
	"github.com/cri-o/cri-o/internal/log"
	oci "github.com/cri-o/cri-o/internal/oci"
	"golang.org/x/net/context"
	"k8s.io/apimachinery/pkg/fields"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
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

// filterContainerList applies a protobuf-defined filter to retrieve only intended containers. Not matching
// the filter is not considered an error but will return an empty response.
func (s *Server) filterContainerList(ctx context.Context, filter *pb.ContainerFilter, origCtrList []*oci.Container) []*oci.Container {
	// Filter using container id and pod id first.
	if filter.Id != "" {
		id, err := s.CtrIDIndex().Get(filter.Id)
		if err != nil {
			// If we don't find a container ID with a filter, it should not
			// be considered an error.  Log a warning and return an empty struct
			log.Warnf(ctx, "unable to find container ID %s", filter.Id)
			return []*oci.Container{}
		}
		c := s.ContainerServer.GetContainer(id)
		if c != nil {
			switch {
			case filter.PodSandboxId == "":
				return []*oci.Container{c}
			case c.Sandbox() == filter.PodSandboxId:
				return []*oci.Container{c}
			default:
				return []*oci.Container{}
			}
		}
	} else if filter.PodSandboxId != "" {
		pod := s.ContainerServer.GetSandbox(filter.PodSandboxId)
		if pod == nil {
			return []*oci.Container{}
		}
		return pod.Containers().List()
	}
	log.Debugf(ctx, "no filters were applied, returning full container list")
	return origCtrList
}

// ListContainers lists all containers by filters.
func (s *Server) ListContainers(ctx context.Context, req *pb.ListContainersRequest) (*pb.ListContainersResponse, error) {
	var ctrs []*pb.Container
	filter := req.GetFilter()
	ctrList, err := s.ContainerServer.ListContainers()
	if err != nil {
		return nil, err
	}

	if filter != nil {
		ctrList = s.filterContainerList(ctx, filter, ctrList)
	}

	for _, ctr := range ctrList {
		// Skip over containers that are still being created
		if !ctr.Created() {
			continue
		}
		podSandboxID := ctr.Sandbox()
		cState := ctr.StateNoLock()
		created := ctr.CreatedAt().UnixNano()
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
			ImageRef:     ctr.ImageRef(),
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

	return &pb.ListContainersResponse{
		Containers: ctrs,
	}, nil
}
