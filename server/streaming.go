package server

import (
	"golang.org/x/net/context"

	"github.com/Sirupsen/logrus"
	pb "k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
)

// ExecSync runs a command in a container synchronously.
func (s *Server) ExecSync(ctx context.Context, req *pb.ExecSyncRequest) (*pb.ExecSyncResponse, error) {
	logrus.Debugf("ExecSyncRequest %+v", req)

	execResp, err := s.manager.ExecSync(req.GetContainerId(), req.GetCmd(), req.GetTimeout())
	if err != nil {
		return nil, err
	}

	resp := &pb.ExecSyncResponse{
		Stdout:   execResp.Stdout,
		Stderr:   execResp.Stderr,
		ExitCode: &execResp.ExitCode,
	}

	logrus.Debugf("ExecSyncResponse: %+v", resp)
	return resp, nil
}

// Attach prepares a streaming endpoint to attach to a running container.
func (s *Server) Attach(ctx context.Context, req *pb.AttachRequest) (*pb.AttachResponse, error) {
	return nil, nil
}

// Exec prepares a streaming endpoint to execute a command in the container.
func (s *Server) Exec(ctx context.Context, req *pb.ExecRequest) (*pb.ExecResponse, error) {
	return nil, nil
}

// PortForward prepares a streaming endpoint to forward ports from a PodSandbox.
func (s *Server) PortForward(ctx context.Context, req *pb.PortForwardRequest) (*pb.PortForwardResponse, error) {
	return nil, nil
}
