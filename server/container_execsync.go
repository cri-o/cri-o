package server

import (
	"fmt"

	"github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/internal/pkg/log"
	"golang.org/x/net/context"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// ExecSync runs a command in a container synchronously.
func (s *Server) ExecSync(ctx context.Context, req *pb.ExecSyncRequest) (resp *pb.ExecSyncResponse, err error) {
	c, err := s.GetContainerFromShortID(req.ContainerId)
	if err != nil {
		return nil, err
	}

	if err := s.Runtime().UpdateContainerStatus(c); err != nil {
		return nil, err
	}

	cState := c.State()
	if !(cState.Status == oci.ContainerStateRunning || cState.Status == oci.ContainerStateCreated) {
		return nil, fmt.Errorf("container is not created or running")
	}

	cmd := req.Cmd
	if cmd == nil {
		return nil, fmt.Errorf("exec command cannot be empty")
	}

	execResp, err := s.Runtime().ExecSyncContainer(c, cmd, req.Timeout)
	if err != nil {
		return nil, err
	}
	resp = &pb.ExecSyncResponse{
		Stdout:   execResp.Stdout,
		Stderr:   execResp.Stderr,
		ExitCode: execResp.ExitCode,
	}

	log.Infof(ctx, "exec'd %s in %s", cmd, c.Description())
	return resp, nil
}
