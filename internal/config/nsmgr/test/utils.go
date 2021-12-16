package nsmgr_test

import (
	"path/filepath"
	"time"

	"github.com/cri-o/cri-o/internal/config/nsmgr"
	"github.com/cri-o/cri-o/internal/oci"
	"github.com/opencontainers/runtime-spec/specs-go"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

type SpoofedNamespace struct {
	NsType    nsmgr.NSType
	EmptyPath bool
}

func (s *SpoofedNamespace) Type() nsmgr.NSType {
	return s.NsType
}

func (s *SpoofedNamespace) Remove() error {
	return nil
}

func (s *SpoofedNamespace) Path() string {
	if s.EmptyPath {
		return ""
	}
	return filepath.Join("tmp", string(s.NsType))
}

func (s *SpoofedNamespace) Close() error {
	return nil
}

var AllSpoofedNamespaces = []nsmgr.Namespace{
	&SpoofedNamespace{
		NsType: nsmgr.IPCNS,
	},
	&SpoofedNamespace{
		NsType: nsmgr.UTSNS,
	},
	&SpoofedNamespace{
		NsType: nsmgr.NETNS,
	},
	&SpoofedNamespace{
		NsType: nsmgr.USERNS,
	},
}

func ContainerWithPid(pid int) (*oci.Container, error) {
	testContainer, err := oci.NewContainer("testid", "testname", "",
		"/container/logs", map[string]string{},
		map[string]string{}, map[string]string{}, "image",
		"imageName", "imageRef", &types.ContainerMetadata{},
		"testsandboxid", false, false, false, "",
		"/root/for/container", time.Now(), "SIGKILL")
	if err != nil {
		return nil, err
	}
	cstate := &oci.ContainerState{}
	cstate.State = specs.State{
		Pid: pid,
	}
	// eat error here because callers may send invalid pids to test against
	_ = cstate.SetInitPid(pid) // nolint:errcheck
	testContainer.SetState(cstate)

	return testContainer, nil
}
