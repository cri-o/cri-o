package server

import (
	"golang.org/x/net/context"

	"github.com/Sirupsen/logrus"
	pb "k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
)

// ListPodSandbox returns a list of SandBoxes.
func (s *Server) ListPodSandbox(ctx context.Context, req *pb.ListPodSandboxRequest) (*pb.ListPodSandboxResponse, error) {
	logrus.Debugf("ListPodSandboxRequest %+v", req)

	pods, err := s.manager.ListPodSandbox(req.GetFilter())
	if err != nil {
		return nil, err
	}

	resp := &pb.ListPodSandboxResponse{
		Items: pods,
	}
	logrus.Debugf("ListPodSandboxResponse %+v", resp)
	return resp, nil
}

// RemovePodSandbox deletes the sandbox. If there are any running containers in the
// sandbox, they should be force deleted.
func (s *Server) RemovePodSandbox(ctx context.Context, req *pb.RemovePodSandboxRequest) (*pb.RemovePodSandboxResponse, error) {
	logrus.Debugf("RemovePodSandboxRequest %+v", req)

	if err := s.manager.RemovePodSandbox(req.GetPodSandboxId()); err != nil {
		return nil, err
	}

	resp := &pb.RemovePodSandboxResponse{}
	logrus.Debugf("RemovePodSandboxResponse %+v", resp)
	return resp, nil
}

// RunPodSandbox creates and runs a pod-level sandbox.
func (s *Server) RunPodSandbox(ctx context.Context, req *pb.RunPodSandboxRequest) (resp *pb.RunPodSandboxResponse, err error) {
	logrus.Debugf("RunPodSandboxRequest %+v", req)

	id, err := s.manager.RunPodSandbox(req.GetConfig())
	if err != nil {
		return nil, err
	}

	resp = &pb.RunPodSandboxResponse{PodSandboxId: &id}
	logrus.Debugf("RunPodSandboxResponse: %+v", resp)
	return resp, nil
}

// PodSandboxStatus returns the Status of the PodSandbox.
func (s *Server) PodSandboxStatus(ctx context.Context, req *pb.PodSandboxStatusRequest) (*pb.PodSandboxStatusResponse, error) {
	logrus.Debugf("PodSandboxStatusRequest %+v", req)

	status, err := s.manager.PodSandboxStatus(req.GetPodSandboxId())
	if err != nil {
		return nil, err
	}

	resp := &pb.PodSandboxStatusResponse{
		Status: status,
	}

	logrus.Infof("PodSandboxStatusResponse: %+v", resp)
	return resp, nil
}

// StopPodSandbox stops the sandbox. If there are any running containers in the
// sandbox, they should be force terminated.
func (s *Server) StopPodSandbox(ctx context.Context, req *pb.StopPodSandboxRequest) (*pb.StopPodSandboxResponse, error) {
	logrus.Debugf("StopPodSandboxRequest %+v", req)

	if err := s.manager.StopPodSandbox(req.GetPodSandboxId()); err != nil {
		return nil, err
	}

	resp := &pb.StopPodSandboxResponse{}
	logrus.Debugf("StopPodSandboxResponse: %+v", resp)
	return resp, nil
}
