package nsmgr_test

import (
	"path/filepath"
	"time"

	"github.com/opencontainers/runtime-spec/specs-go"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/config/nsmgr"
	"github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/internal/storage"
	"github.com/cri-o/cri-o/internal/storage/references"
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
	imageName, err := references.ParseRegistryImageReferenceFromOutOfProcessData("example.com/some-image:latest")
	if err != nil {
		return nil, err
	}

	imageID, err := storage.ParseStorageImageIDFromOutOfProcessData("2a03a6059f21e150ae84b0973863609494aad70f0a80eaeb64bddd8d92465812")
	if err != nil {
		return nil, err
	}

	testContainer, err := oci.NewContainer("testid", "testname", "",
		"/container/logs", map[string]string{},
		map[string]string{}, map[string]string{}, "image",
		&imageName, &imageID, "", &types.ContainerMetadata{},
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
	_ = cstate.SetInitPid(pid) //nolint:errcheck
	testContainer.SetState(cstate)

	return testContainer, nil
}
