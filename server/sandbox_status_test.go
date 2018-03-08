package server

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/kubernetes-incubator/cri-o/lib"
	"github.com/kubernetes-incubator/cri-o/lib/sandbox"
	"github.com/kubernetes-incubator/cri-o/oci"
	"github.com/kubernetes-incubator/cri-o/version"
	pb "k8s.io/kubernetes/pkg/kubelet/apis/cri/runtime/v1alpha2"
)

func init() {
	rand.Seed(time.Now().UTC().UnixNano())
}

func newTestContainerServerOrFailNow(t *testing.T) (cs *lib.ContainerServer, dirsToCleanUp []string) {
	tmpdir := os.Getenv("TMPDIR")

	config := lib.DefaultConfig()
	runRoot, err := ioutil.TempDir(tmpdir, "")
	if err != nil {
		t.Fatal(err)
	}
	config.RootConfig.RunRoot = runRoot
	root, err := ioutil.TempDir(tmpdir, "")
	if err != nil {
		t.Fatal(err)
	}
	config.RootConfig.Root = root
	config.RootConfig.Storage = "vfs"
	cs, err = lib.New(config)
	if err != nil {
		t.Fatal(err)
	}
	return cs, []string{runRoot, root}
}

func newTestSandboxOrFailNow(t *testing.T) (string, *sandbox.Sandbox) {
	id := fmt.Sprintf("id-for-sandbox-%d", rand.Int())

	sb, err := sandbox.New(id, "", "", "", "", nil, nil, "", "", nil, "", "", false, false, "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	return id, sb
}

func newTestContainerOrFailNow(t *testing.T) *oci.Container {
	id := fmt.Sprintf("id-for-container-%d", rand.Int())

	c, err := oci.NewContainer(id, "", "", "", nil, nil, nil, nil, "", "", "", nil, "", false, false, false, false, false, "", time.Now(), "")
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func setupServer(t *testing.T) (*Server, string, func()) {
	containerServer, fs := newTestContainerServerOrFailNow(t)
	teardown := func() {
		for _, f := range fs {
			defer os.RemoveAll(f)
		}
	}

	server := &Server{ContainerServer: containerServer}
	sandboxID, sb := newTestSandboxOrFailNow(t)
	sb.SetInfraContainer(newTestContainerOrFailNow(t))
	server.PodIDIndex().Add(sandboxID)
	server.ContainerServer.AddSandbox(sb)

	return server, sandboxID, teardown
}

func TestPodSandboxStatus(t *testing.T) {
	server, sandboxID, teardown := setupServer(t)
	defer teardown()

	t.Run("Without verbose information", func(t *testing.T) {
		resp, err := server.PodSandboxStatus(nil, &pb.PodSandboxStatusRequest{
			PodSandboxId: sandboxID,
		})
		if err != nil {
			t.Fatal(err)
		}

		if resp.Status == nil {
			t.Error("expected non nil resp.Status")
		}
		if resp.Info != nil {
			t.Error("expected nil resp.Info")
		}
	})

	t.Run("With verbose information", func(t *testing.T) {
		resp, err := server.PodSandboxStatus(nil, &pb.PodSandboxStatusRequest{
			PodSandboxId: sandboxID,
			Verbose:      true,
		})
		if err != nil {
			t.Fatal(err)
		}

		marshaledVersion := resp.Info["version"]
		var versionPayload VersionPayload
		must(t, json.Unmarshal([]byte(marshaledVersion), &versionPayload))

		if version.Version != versionPayload.Version {
			t.Errorf("expected: %s\ngot: %s", version.Version, versionPayload.Version)
		}
	})
}
