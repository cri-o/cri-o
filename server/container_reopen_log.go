package server

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/kubernetes-incubator/cri-o/oci"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	"golang.org/x/sys/unix"
	pb "k8s.io/kubernetes/pkg/kubelet/apis/cri/runtime/v1alpha2"
)

// ReopenContainerLog reopens the containers log file
func (s *Server) ReopenContainerLog(ctx context.Context, req *pb.ReopenContainerLogRequest) (resp *pb.ReopenContainerLogResponse, err error) {
	const operation = "container_reopen_log"
	defer func() {
		recordOperation(operation, time.Now())
		recordError(operation, err)
	}()

	logrus.Debugf("ReopenContainerLogRequest %+v", req)
	containerID := req.ContainerId
	c := s.GetContainer(containerID)

	if c == nil {
		return nil, fmt.Errorf("could not find container %q", containerID)
	}

	if err := s.ContainerServer.Runtime().UpdateStatus(c); err != nil {
		return nil, err
	}

	cState := s.ContainerServer.Runtime().ContainerStatus(c)
	if !(cState.Status == oci.ContainerStateRunning || cState.Status == oci.ContainerStateCreated) {
		return nil, fmt.Errorf("container is not created or running")
	}

	controlPath := filepath.Join(c.BundlePath(), "ctl")
	controlFile, err := os.OpenFile(controlPath, unix.O_WRONLY, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to open container ctl file: %v", err)
	}
	defer controlFile.Close()

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("Failed to create new watch: %v", err)
	}
	defer watcher.Close()

	done := make(chan struct{})
	errorCh := make(chan error)
	go func() {
		for {
			select {
			case event := <-watcher.Events:
				logrus.Debugf("event: %v", event)
				if event.Op&fsnotify.Create == fsnotify.Create || event.Op&fsnotify.Write == fsnotify.Write {
					logrus.Debugf("file created %s", event.Name)
					if event.Name == c.LogPath() {
						logrus.Debugf("expected log file created")
						close(done)
						return
					}
				}
			case err := <-watcher.Errors:
				errorCh <- fmt.Errorf("watch error for container log reopen %v: %v", containerID, err)
				return
			case <-s.monitorsChan:
				errorCh <- fmt.Errorf("closing log reopen monitor for %v", containerID)
				return
			}
		}
	}()
	cLogDir := filepath.Dir(c.LogPath())
	if err := watcher.Add(cLogDir); err != nil {
		logrus.Errorf("watcher.Add(%q) failed: %s", cLogDir, err)
		close(done)
	}

	if _, err = fmt.Fprintf(controlFile, "%d %d %d\n", 2, 0, 0); err != nil {
		logrus.Debugf("Failed to write to control file to reopen log file: %v", err)
	}

	select {
	case err := <-errorCh:
		return nil, err
	case <-done:
		resp = &pb.ReopenContainerLogResponse{}
	}

	logrus.Debugf("ReopenContainerLogResponse %s: %+v", containerID, resp)
	return resp, nil
}
