package server

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/pprof"

	"github.com/containers/storage/pkg/idtools"
	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/pkg/types"
	"github.com/go-chi/chi/v5"
	json "github.com/json-iterator/go"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
)

func (s *Server) getIDMappingsInfo() types.IDMappings {
	max := int64(int(^uint(0) >> 1))
	if max > math.MaxUint32 {
		max = math.MaxUint32
	}

	if s.defaultIDMappings == nil {
		fullMapping := idtools.IDMap{
			ContainerID: 0,
			HostID:      0,
			Size:        int(max),
		}
		return types.IDMappings{
			Uids: []idtools.IDMap{fullMapping},
			Gids: []idtools.IDMap{fullMapping},
		}
	}
	return types.IDMappings{
		Uids: s.defaultIDMappings.UIDs(),
		Gids: s.defaultIDMappings.GIDs(),
	}
}

func (s *Server) getInfo() types.CrioInfo {
	return types.CrioInfo{
		StorageDriver:     s.config.Storage,
		StorageRoot:       s.config.Root,
		CgroupDriver:      s.config.CgroupManager().Name(),
		DefaultIDMappings: s.getIDMappingsInfo(),
	}
}

var (
	errCtrNotFound     = errors.New("container not found")
	errCtrStateNil     = errors.New("container state is nil")
	errSandboxNotFound = errors.New("sandbox for container not found")
)

func (s *Server) getContainerInfo(ctx context.Context, id string, getContainerFunc, getInfraContainerFunc func(ctx context.Context, id string) *oci.Container, getSandboxFunc func(ctx context.Context, id string) *sandbox.Sandbox) (types.ContainerInfo, error) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	ctr := getContainerFunc(ctx, id)
	isInfra := false
	if ctr == nil {
		ctr = getInfraContainerFunc(ctx, id)
		if ctr == nil {
			return types.ContainerInfo{}, errCtrNotFound
		}
		isInfra = true
	}
	// TODO(mrunalp): should we call UpdateStatus()?
	ctrState := ctr.StateNoLock()
	if ctrState == nil {
		return types.ContainerInfo{}, errCtrStateNil
	}
	sb := getSandboxFunc(ctx, ctr.Sandbox())
	if sb == nil {
		log.Debugf(ctx, "Can't find sandbox %s for container %s", ctr.Sandbox(), id)
		return types.ContainerInfo{}, errSandboxNotFound
	}

	pidToReturn := ctrState.InitPid
	if isInfra && pidToReturn == 0 {
		// It is possible the infra container doesn't report a PID.
		// That can either happen if we're using a vm based runtime,
		// or if we've dropped the infra container.
		// Since the Pid is used exclusively to find the network stats,
		// and pods share their network (whether it's host or pod level)
		// we can return the pid of a running container in the pod.
		for _, c := range sb.Containers().List() {
			ctrPid, err := c.Pid()
			if ctrPid > 0 && err == nil {
				pidToReturn = ctrPid
				break
			}
		}
	}
	return types.ContainerInfo{
		Name:            ctr.Name(),
		Pid:             pidToReturn,
		Image:           ctr.ImageName(),
		ImageRef:        ctr.ImageRef(),
		CreatedTime:     ctrState.Created.UnixNano(),
		Labels:          ctr.Labels(),
		Annotations:     ctr.Annotations(),
		CrioAnnotations: ctr.CrioAnnotations(),
		Root:            ctr.MountPoint(),
		LogPath:         ctr.LogPath(),
		Sandbox:         ctr.Sandbox(),
		IPs:             sb.IPs(),
	}, nil
}

const (
	InspectConfigEndpoint     = "/config"
	InspectContainersEndpoint = "/containers"
	InspectInfoEndpoint       = "/info"
	InspectPauseEndpoint      = "/pause"
	InspectUnpauseEndpoint    = "/unpause"
)

// GetExtendInterfaceMux returns the mux used to serve extend interface requests
func (s *Server) GetExtendInterfaceMux(enableProfile bool) *chi.Mux {
	mux := chi.NewMux()

	mux.Get(InspectConfigEndpoint, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		b, err := s.config.ToBytes()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/toml")
		if _, err := w.Write(b); err != nil {
			logrus.Errorf("Unable to write response TOML: %v", err)
		}
	}))

	mux.Get(InspectInfoEndpoint, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		ci := s.getInfo()
		js, err := json.Marshal(ci)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write(js); err != nil {
			logrus.Errorf("Unable to write response JSON: %v", err)
		}
	}))

	mux.Get(InspectContainersEndpoint+"/{id}", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		ctx := context.TODO()
		containerID := chi.URLParam(req, "id")
		ci, err := s.getContainerInfo(ctx, containerID, s.GetContainer, s.getInfraContainer, s.getSandbox)
		if err != nil {
			switch err {
			case errCtrNotFound:
				http.Error(w, fmt.Sprintf("can't find the container with id %s", containerID), http.StatusNotFound)
			case errCtrStateNil:
				http.Error(w, fmt.Sprintf("can't find container state for container with id %s", containerID), http.StatusInternalServerError)
			case errSandboxNotFound:
				http.Error(w, fmt.Sprintf("can't find the sandbox for container id %s", containerID), http.StatusNotFound)
			default:
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}
		js, err := json.Marshal(ci)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write(js); err != nil {
			logrus.Errorf("Unable to write response JSON: %v", err)
		}
	}))

	mux.Get(InspectPauseEndpoint+"/{id}", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		containerID := chi.URLParam(req, "id")
		ctx := context.TODO()
		ctr := s.GetContainer(ctx, containerID)

		if ctr == nil {
			http.Error(w, fmt.Sprintf("can't find the container with id %s", containerID), http.StatusNotFound)
			return
		}
		ctrStatus := ctr.State().Status
		if ctrStatus != oci.ContainerStateRunning && ctrStatus != oci.ContainerStateCreated {
			http.Error(w,
				fmt.Sprintf("container is not in running or created state, now is %s", ctrStatus),
				http.StatusConflict)
			return
		}
		if err := s.Runtime().PauseContainer(s.stream.ctx, ctr); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := s.Runtime().UpdateContainerStatus(s.stream.ctx, ctr); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		if _, err := w.Write([]byte("200 OK")); err != nil {
			logrus.Errorf("Unable to write response JSON: %v", err)
		}
	}))

	mux.Get(InspectUnpauseEndpoint+"/{id}", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		containerID := chi.URLParam(req, "id")
		ctx := context.TODO()
		ctr := s.GetContainer(ctx, containerID)

		if ctr == nil {
			http.Error(w, fmt.Sprintf("can't find the container with id %s", containerID), http.StatusNotFound)
			return
		}
		ctrStatus := ctr.State().Status
		if ctrStatus != oci.ContainerStatePaused {
			http.Error(w,
				fmt.Sprintf("container is not in paused state, now is %s", ctrStatus),
				http.StatusConflict)
			return
		}
		if err := s.Runtime().UnpauseContainer(s.stream.ctx, ctr); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := s.Runtime().UpdateContainerStatus(s.stream.ctx, ctr); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		if _, err := w.Write([]byte("200 OK")); err != nil {
			logrus.Errorf("Unable to write response JSON: %v", err)
		}
	}))

	// Add pprof handlers
	if enableProfile {
		mux.Get("/debug/pprof/cmdline", http.HandlerFunc(pprof.Cmdline))
		mux.Get("/debug/pprof/profile", http.HandlerFunc(pprof.Profile))
		mux.Get("/debug/pprof/symbol", http.HandlerFunc(pprof.Symbol))
		mux.Get("/debug/pprof/trace", http.HandlerFunc(pprof.Trace))
		mux.Get("/debug/pprof/*", http.HandlerFunc(pprof.Index))
	}

	return mux
}
