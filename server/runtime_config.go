package server

import (
	"context"

	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// RuntimeConfig returns configuration information of the runtime.
func (s *Server) RuntimeConfig(_ context.Context, req *types.RuntimeConfigRequest) (*types.RuntimeConfigResponse, error) {
	resp := &types.RuntimeConfigResponse{
		Linux: &types.LinuxRuntimeConfiguration{
			CgroupDriver: s.getCgroupDriver(),
		},
	}
	return resp, nil
}

func (s *Server) getCgroupDriver() types.CgroupDriver {
	if s.config.CgroupManager().IsSystemd() {
		return types.CgroupDriver_SYSTEMD
	}
	return types.CgroupDriver_CGROUPFS
}
