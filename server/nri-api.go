package server

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cri-o/cri-o/internal/log"
	"github.com/intel/goresctrl/pkg/blockio"

	"github.com/cri-o/cri-o/internal/config/cgmgr"
	"github.com/cri-o/cri-o/internal/config/node"
	"github.com/cri-o/cri-o/internal/config/rdt"
	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/pkg/annotations"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/opencontainers/runtime-tools/generate"
	cri "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/containerd/nri/pkg/api"
	nrigen "github.com/containerd/nri/pkg/runtime-tools/generate"
	"github.com/cri-o/cri-o/internal/nri"
)

type nriAPI struct {
	cri *Server
	nri nri.API
}

func (a *nriAPI) start() error {
	if !a.isEnabled() {
		return nil
	}

	nri.SetDomain(a)

	return a.nri.Start()
}

func (a *nriAPI) isEnabled() bool {
	return a != nil && a.nri != nil && a.nri.IsEnabled()
}

//
// CRI 'downward' interface for NRI
//
// These functions are used in the CRI plugin to hook NRI processing into
// the corresponding CRI pod and container lifecycle events.
//

func (a *nriAPI) runPodSandbox(ctx context.Context, criPod *sandbox.Sandbox) error {
	if !a.isEnabled() {
		return nil
	}

	pod := nriPodSandbox(ctx, criPod)
	err := a.nri.RunPodSandbox(ctx, pod)
	if err != nil {
		if undoErr := a.nri.StopPodSandbox(ctx, pod); undoErr != nil {
			log.Warnf(ctx, "Undo stop of failed NRI pod start failed: %v", undoErr)
		}
		if undoErr := a.nri.RemovePodSandbox(ctx, pod); undoErr != nil {
			log.Warnf(ctx, "Undo remove of failed NRI pod start failed: %v", undoErr)
		}
	}

	return err
}

func (a *nriAPI) stopPodSandbox(ctx context.Context, criPod *sandbox.Sandbox) error {
	if !a.isEnabled() {
		return nil
	}

	pod := nriPodSandbox(ctx, criPod)

	return a.nri.StopPodSandbox(ctx, pod)
}

func (a *nriAPI) removePodSandbox(ctx context.Context, criPod *sandbox.Sandbox) error {
	if !a.isEnabled() {
		return nil
	}

	pod := nriPodSandbox(ctx, criPod)

	return a.nri.RemovePodSandbox(ctx, pod)
}

func (a *nriAPI) createContainer(ctx context.Context, specgen *generate.Generator, criPod *sandbox.Sandbox, criCtr *oci.Container) error {
	if !a.isEnabled() {
		return nil
	}

	pod := nriPodSandbox(ctx, criPod)
	ctr := &criContainer{
		api:  a,
		ctr:  criCtr,
		spec: specgen.Config,
	}

	adjust, err := a.nri.CreateContainer(ctx, pod, ctr)
	if err != nil {
		return err
	}

	wrapgen := nrigen.SpecGenerator(specgen,
		nrigen.WithAnnotationFilter(
			func(values map[string]string) (map[string]string, error) {
				annotations, handler := criPod.Annotations(), criPod.RuntimeHandler()
				if err := a.cri.FilterDisallowedAnnotations(annotations, values, handler); err != nil {
					return nil, fmt.Errorf("disallowed annotations in NRI adjustment: %w", err)
				}
				return values, nil
			},
		),
		nrigen.WithResourceChecker(
			func(r *rspec.LinuxResources) error {
				if r == nil {
					return nil
				}
				if mem := r.Memory; mem != nil {
					if mem.Limit != nil {
						containerMinMemory, err := a.cri.Runtime().GetContainerMinMemory(criPod.RuntimeHandler())
						if err != nil {
							return err
						}
						if err := cgmgr.VerifyMemoryIsEnough(*mem.Limit, containerMinMemory); err != nil {
							return err
						}
					}
					if !node.CgroupHasMemorySwap() {
						mem.Swap = nil
					}
				}
				if !node.CgroupHasHugetlb() {
					r.HugepageLimits = nil
				}
				return nil
			},
		),
		nrigen.WithBlockIOResolver(
			func(className string) (*rspec.LinuxBlockIO, error) {
				if !a.cri.Config().BlockIO().Enabled() || className == "" {
					return nil, nil
				}
				if blockIO, err := blockio.OciLinuxBlockIO(className); err == nil {
					return blockIO, nil
				}
				return nil, nil
			},
		),
		nrigen.WithRdtResolver(
			func(className string) (*rspec.LinuxIntelRdt, error) {
				if className == "" || className == "/PodQos" {
					return nil, nil
				}
				return &rspec.LinuxIntelRdt{
					ClosID: rdt.ResctrlPrefix + className,
				}, nil
			},
		),
	)

	if err := wrapgen.Adjust(adjust); err != nil {
		return fmt.Errorf("failed to adjust container %s: %w", ctr.GetID(), err)
	}

	return nil
}

func (a *nriAPI) postCreateContainer(ctx context.Context, criPod *sandbox.Sandbox, criCtr *oci.Container) error {
	if !a.isEnabled() {
		return nil
	}

	pod := nriPodSandbox(ctx, criPod)
	ctr := &criContainer{
		api: a,
		ctr: criCtr,
	}

	return a.nri.PostCreateContainer(ctx, pod, ctr)
}

func (a *nriAPI) startContainer(ctx context.Context, criPod *sandbox.Sandbox, criCtr *oci.Container) error {
	if !a.isEnabled() {
		return nil
	}

	pod := nriPodSandbox(ctx, criPod)
	ctr := &criContainer{
		api: a,
		ctr: criCtr,
	}

	return a.nri.StartContainer(ctx, pod, ctr)
}

func (a *nriAPI) postStartContainer(ctx context.Context, criPod *sandbox.Sandbox, criCtr *oci.Container) error {
	if !a.isEnabled() {
		return nil
	}

	pod := nriPodSandbox(ctx, criPod)
	ctr := &criContainer{
		api: a,
		ctr: criCtr,
	}

	return a.nri.PostStartContainer(ctx, pod, ctr)
}

func (a *nriAPI) updateContainer(ctx context.Context, criCtr *oci.Container, req *cri.LinuxContainerResources) (*cri.LinuxContainerResources, error) {
	if !a.isEnabled() {
		return req, nil
	}

	const noOomAdj = 0

	criPod := a.cri.getSandbox(ctx, criCtr.Sandbox())
	pod := nriPodSandbox(ctx, criPod)
	ctr := &criContainer{
		api: a,
		ctr: criCtr,
	}

	r, err := a.nri.UpdateContainer(ctx, pod, ctr, api.FromCRILinuxResources(req))
	if err != nil {
		return nil, err
	}

	return r.ToCRI(noOomAdj), nil
}

func (a *nriAPI) postUpdateContainer(ctx context.Context, criCtr *oci.Container) error {
	if !a.isEnabled() {
		return nil
	}

	criPod := a.cri.getSandbox(ctx, criCtr.Sandbox())
	pod := nriPodSandbox(ctx, criPod)
	ctr := &criContainer{
		api: a,
		ctr: criCtr,
	}

	return a.nri.PostUpdateContainer(ctx, pod, ctr)
}

func (a *nriAPI) stopContainer(ctx context.Context, criPod *sandbox.Sandbox, criCtr *oci.Container) error {
	if !a.isEnabled() {
		return nil
	}

	ctr := &criContainer{
		api: a,
		ctr: criCtr,
	}

	if criPod == nil {
		sandboxID, err := a.cri.PodIDIndex().Get(ctr.GetPodSandboxID())
		if err != nil {
			log.Errorf(ctx, "Failed to stop CRI container %q: %v", ctr.GetID(), err)
			return nil
		}

		criPod = a.cri.ContainerServer.GetSandbox(sandboxID)
		if criPod == nil {
			log.Errorf(ctx, "Failed to stop CRI container %q: can't find pod %q",
				ctr.GetID(), sandboxID)
			return nil
		}
	}

	pod := nriPodSandbox(ctx, criPod)

	return a.nri.StopContainer(ctx, pod, ctr)
}

func (a *nriAPI) removeContainer(ctx context.Context, criPod *sandbox.Sandbox, criCtr *oci.Container) error {
	if !a.isEnabled() {
		return nil
	}

	pod := nriPodSandbox(ctx, criPod)
	ctr := &criContainer{
		api: a,
		ctr: criCtr,
	}

	return a.nri.RemoveContainer(ctx, pod, ctr)
}

func (a *nriAPI) undoCreateContainer(ctx context.Context, specgen *generate.Generator, criPod *sandbox.Sandbox, criCtr *oci.Container) {
	if !a.isEnabled() {
		return
	}

	pod := nriPodSandbox(ctx, criPod)
	ctr := &criContainer{
		api:  a,
		ctr:  criCtr,
		spec: specgen.Config,
	}

	if err := a.nri.StopContainer(ctx, pod, ctr); err != nil {
		log.Errorf(ctx, "NRI failed to undo creation (stop): %v", err)
	}

	if err := a.nri.RemoveContainer(ctx, pod, ctr); err != nil {
		log.Errorf(ctx, "NRI failed to undo creation (remove): %v", err)
	}
}

//
// CRI 'upward' interface for NRI
//
// This implements the 'CRI domain' for the common NRI interface plugin.
// It takes care of the CRI-specific details of interfacing from NRI to
// CRI (container and pod discovery, container adjustment and updates).
//

const (
	nriDomain = "k8s.io"
)

func (a *nriAPI) GetName() string {
	return nriDomain
}

func (a *nriAPI) ListPodSandboxes(ctx context.Context) []nri.PodSandbox {
	pods := []nri.PodSandbox{}
	for _, pod := range a.cri.ContainerServer.ListSandboxes() {
		if pod.Created() {
			pods = append(pods, nriPodSandbox(ctx, pod))
		}
	}
	return pods
}

func (a *nriAPI) ListContainers() []nri.Container {
	containers := []nri.Container{}
	ctrList, err := a.cri.ContainerServer.ListContainers()
	if err != nil {
		log.Warnf(context.TODO(), "Failed to list containers: %v", err)
	}
	for _, ctr := range ctrList {
		switch ctr.State().Status {
		case oci.ContainerStateCreated, oci.ContainerStateRunning, oci.ContainerStatePaused:
			containers = append(containers, &criContainer{
				api: a,
				ctr: ctr,
			})
		}
	}
	return containers
}

func (a *nriAPI) GetPodSandbox(ctx context.Context, id string) (nri.PodSandbox, bool) {
	sandboxID, err := a.cri.PodIDIndex().Get(id)
	if err != nil {
		return nil, false
	}

	pod := a.cri.ContainerServer.GetSandbox(sandboxID)
	if pod == nil {
		return nil, false
	}

	return nriPodSandbox(ctx, pod), true
}

func (a *nriAPI) GetContainer(id string) (nri.Container, bool) {
	ctr, err := a.cri.GetContainerFromShortID(context.TODO(), id)
	if err != nil {
		return nil, false
	}

	return &criContainer{
		api: a,
		ctr: ctr,
	}, true
}

func (a *nriAPI) UpdateContainer(ctx context.Context, u *api.ContainerUpdate) error {
	ctr, err := a.cri.GetContainerFromShortID(context.TODO(), u.ContainerId)
	if err != nil {
		// We blindly assume container with given ID not found and ignore it.
		log.Errorf(ctx, "Failed to update CRI container %q: %v", u.ContainerId, err)
		return nil
	}

	if s := ctr.State().Status; s != oci.ContainerStateRunning && s != oci.ContainerStateCreated {
		return nil
	}

	resources := u.Linux.Resources.ToOCI()
	if err = a.cri.Runtime().UpdateContainer(ctx, ctr, resources); err != nil {
		log.Errorf(ctx, "Failed to update CRI container %q: %v", u.ContainerId, err)
		if u.IgnoreFailure {
			return nil
		}
		return fmt.Errorf("failed to update CRI container %q: %w", u.ContainerId, err)
	}

	a.cri.UpdateContainerLinuxResources(ctr, resources)

	return nil
}

func (a *nriAPI) EvictContainer(ctx context.Context, e *api.ContainerEviction) error {
	ctr, err := a.cri.GetContainerFromShortID(context.TODO(), e.ContainerId)
	if err != nil {
		// We blindly assume container with given ID not found and ignore it.
		log.Errorf(ctx, "Failed to evict CRI container %q: %v", e.ContainerId, err)
		return nil
	}
	if err = a.cri.stopContainer(ctx, ctr, 0); err != nil {
		log.Errorf(ctx, "Failed to evict CRI container %q: %v", e.ContainerId, err)
		return err
	}

	return nil
}

//
// NRI integration wrapper for CRI Pods
//

type criPodSandbox struct {
	*sandbox.Sandbox
	spec *rspec.Spec
	pid  int
}

func nriPodSandbox(ctx context.Context, pod *sandbox.Sandbox) *criPodSandbox {
	criPod := &criPodSandbox{
		Sandbox: pod,
		spec:    &rspec.Spec{},
	}

	if ic := pod.InfraContainer(); ic != nil {
		spec := ic.Spec()
		if !ic.Spoofed() {
			pid, err := ic.Pid()
			if err != nil {
				log.Debugf(ctx, "Failed to get pid for pod infra container: %v", err)
			} else {
				criPod.pid = pid
			}
		}
		criPod.spec = &spec
	}

	return criPod
}

func (p *criPodSandbox) GetDomain() string {
	return nriDomain
}

func (p *criPodSandbox) GetID() string {
	if p.Sandbox == nil {
		return ""
	}
	return p.ID()
}

func (p *criPodSandbox) GetName() string {
	if p.Sandbox == nil {
		return ""
	}
	return p.Metadata().Name
}

func (p *criPodSandbox) GetUID() string {
	if p.Sandbox == nil {
		return ""
	}
	return p.Metadata().GetUid()
}

func (p *criPodSandbox) GetNamespace() string {
	if p.Sandbox == nil {
		return ""
	}
	return p.Metadata().Namespace
}

func (p *criPodSandbox) GetAnnotations() map[string]string {
	if p.Sandbox == nil {
		return nil
	}
	anns := map[string]string{}
	for key, value := range p.Annotations() {
		anns[key] = value
	}
	return anns
}

func (p *criPodSandbox) GetLabels() map[string]string {
	if p.Sandbox == nil {
		return nil
	}
	labels := map[string]string{}
	for key, value := range p.Labels() {
		labels[key] = value
	}
	return labels
}

func (p *criPodSandbox) GetRuntimeHandler() string {
	if p.Sandbox == nil {
		return ""
	}
	return p.RuntimeHandler()
}

func (p *criPodSandbox) GetLinuxPodSandbox() nri.LinuxPodSandbox {
	return p
}

func (p *criPodSandbox) GetLinuxNamespaces() []*api.LinuxNamespace {
	if p.spec.Linux == nil {
		return nil
	}
	return api.FromOCILinuxNamespaces(p.spec.Linux.Namespaces)
}

func (p *criPodSandbox) GetPodLinuxOverhead() *api.LinuxResources {
	if p.Sandbox == nil {
		return nil
	}

	return api.FromCRILinuxResources(p.Sandbox.PodLinuxOverhead())
}

func (p *criPodSandbox) GetPodLinuxResources() *api.LinuxResources {
	if p.Sandbox == nil {
		return nil
	}

	return api.FromCRILinuxResources(p.Sandbox.PodLinuxResources())
}

func (p *criPodSandbox) GetLinuxResources() *api.LinuxResources {
	if p.spec.Linux == nil {
		return nil
	}
	return api.FromOCILinuxResources(p.spec.Linux.Resources, nil)
}

func (p *criPodSandbox) GetCgroupParent() string {
	if p.Sandbox == nil {
		return ""
	}
	return p.CgroupParent()
}

func (p *criPodSandbox) GetCgroupsPath() string {
	if p.spec.Linux == nil {
		return ""
	}
	return p.spec.Linux.CgroupsPath
}

func (p *criPodSandbox) GetPid() uint32 {
	return uint32(p.pid)
}

//
// NRI integration wrapper for CRI Containers
//

type criContainer struct {
	api  *nriAPI
	ctr  *oci.Container
	spec *rspec.Spec
}

func (c *criContainer) GetDomain() string {
	return nriDomain
}

func (c *criContainer) GetID() string {
	if c.ctr == nil {
		return ""
	}
	return c.GetSpec().Annotations[annotations.ContainerID]
}

func (c *criContainer) GetPodSandboxID() string {
	return c.GetSpec().Annotations[annotations.SandboxID]
}

func (c *criContainer) GetName() string {
	return c.GetSpec().Annotations["io.kubernetes.container.name"]
}

func (c *criContainer) GetState() api.ContainerState {
	if c.ctr != nil {
		switch c.ctr.State().Status {
		case oci.ContainerStateCreated:
			return api.ContainerState_CONTAINER_CREATED
		case oci.ContainerStatePaused:
			return api.ContainerState_CONTAINER_PAUSED
		case oci.ContainerStateRunning:
			return api.ContainerState_CONTAINER_RUNNING
		case oci.ContainerStateStopped:
			return api.ContainerState_CONTAINER_STOPPED
		}
	}

	return api.ContainerState_CONTAINER_UNKNOWN
}

func (c *criContainer) GetLabels() map[string]string {
	if blob, ok := c.GetSpec().Annotations[annotations.Labels]; ok {
		labels := map[string]string{}
		if err := json.Unmarshal([]byte(blob), &labels); err == nil {
			return labels
		}
	}
	return nil
}

func (c *criContainer) GetAnnotations() map[string]string {
	return c.GetSpec().Annotations
}

func (c *criContainer) GetArgs() []string {
	if p := c.GetSpec().Process; p != nil {
		return api.DupStringSlice(p.Args)
	}
	return nil
}

func (c *criContainer) GetEnv() []string {
	if p := c.GetSpec().Process; p != nil {
		return api.DupStringSlice(p.Env)
	}
	return nil
}

func (c *criContainer) GetMounts() []*api.Mount {
	return api.FromOCIMounts(c.GetSpec().Mounts)
}

func (c *criContainer) GetHooks() *api.Hooks {
	return api.FromOCIHooks(c.GetSpec().Hooks)
}

func (c *criContainer) GetLinuxContainer() nri.LinuxContainer {
	return c
}

func (c *criContainer) GetLinuxNamespaces() []*api.LinuxNamespace {
	spec := c.GetSpec()
	if spec.Linux != nil {
		return api.FromOCILinuxNamespaces(spec.Linux.Namespaces)
	}
	return nil
}

func (c *criContainer) GetLinuxDevices() []*api.LinuxDevice {
	spec := c.GetSpec()
	if spec.Linux != nil {
		return api.FromOCILinuxDevices(spec.Linux.Devices)
	}
	return nil
}

func (c *criContainer) GetLinuxResources() *api.LinuxResources {
	spec := c.GetSpec()
	if spec.Linux == nil {
		return nil
	}
	return api.FromOCILinuxResources(spec.Linux.Resources, spec.Annotations)
}

func (c *criContainer) GetOOMScoreAdj() *int {
	if c.GetSpec().Process != nil {
		return c.GetSpec().Process.OOMScoreAdj
	}
	return nil
}

func (c *criContainer) GetCgroupsPath() string {
	if c.GetSpec().Linux == nil {
		return ""
	}
	return c.GetSpec().Linux.CgroupsPath
}

func (c *criContainer) GetSpec() *rspec.Spec {
	if c.spec != nil {
		return c.spec
	}
	if c.ctr != nil {
		spec := c.ctr.Spec()
		return &spec
	}
	return &rspec.Spec{}
}
