package server

import (
	"fmt"
	"math"
	"net/http"
	"net/http/pprof"

	"github.com/containers/storage/pkg/idtools"
	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/pkg/types"
	"github.com/go-zoo/bone"
	json "github.com/json-iterator/go"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
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

func (s *Server) getContainerInfo(id string, getContainerFunc, getInfraContainerFunc func(id string) *oci.Container, getSandboxFunc func(id string) *sandbox.Sandbox) (types.ContainerInfo, error) {
	ctr := getContainerFunc(id)
	isInfra := false
	if ctr == nil {
		ctr = getInfraContainerFunc(id)
		if ctr == nil {
			return types.ContainerInfo{}, errCtrNotFound
		}
		isInfra = true
	}
	// TODO(mrunalp): should we call UpdateStatus()?
	ctrState := ctr.State()
	if ctrState == nil {
		return types.ContainerInfo{}, errCtrStateNil
	}
	sb := getSandboxFunc(ctr.Sandbox())
	if sb == nil {
		logrus.Debugf("Can't find sandbox %s for container %s", ctr.Sandbox(), id)
		return types.ContainerInfo{}, errSandboxNotFound
	}
	image := ctr.Image()
	if s.ContainerServer != nil && s.ContainerServer.StorageImageServer() != nil {
		if status, err := s.ContainerServer.StorageImageServer().ImageStatus(s.config.SystemContext, ctr.ImageRef()); err == nil {
			image = status.Name
		}
	}

	pidToReturn := ctrState.Pid
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
		Image:           image,
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
)

// GetInfoMux returns the mux used to serve info requests
func (s *Server) GetInfoMux(enableProfile bool) *bone.Mux {
	mux := bone.New()

	mux.Get(InspectConfigEndpoint, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		b, err := s.config.ToBytes()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/toml")
		if _, err := w.Write(b); err != nil {
			http.Error(w, fmt.Sprintf("unable to write TOML: %v", err),
				http.StatusInternalServerError)
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
			http.Error(w, fmt.Sprintf("unable to write JSON: %v", err), http.StatusInternalServerError)
		}
	}))

	mux.Get(InspectContainersEndpoint+"/:id", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		containerID := bone.GetValue(req, "id")
		ci, err := s.getContainerInfo(containerID, s.GetContainer, s.getInfraContainer, s.getSandbox)
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
			http.Error(w, fmt.Sprintf("unable to write JSON: %v", err), http.StatusInternalServerError)
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
