package server

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"

	"github.com/containerd/nri/pkg/api"
	nrigen "github.com/containerd/nri/pkg/runtime-tools/generate"
	"github.com/intel/goresctrl/pkg/blockio"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/opencontainers/runtime-tools/generate"
	cri "k8s.io/cri-api/pkg/apis/runtime/v1"
	"tags.cncf.io/container-device-interface/pkg/cdi"

	"github.com/cri-o/cri-o/internal/annotations"
	"github.com/cri-o/cri-o/internal/config/cgmgr"
	"github.com/cri-o/cri-o/internal/config/node"
	"github.com/cri-o/cri-o/internal/config/rdt"
	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/internal/nri"
	"github.com/cri-o/cri-o/internal/oci"
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

func (a *nriAPI) updatePodSandbox(ctx context.Context, criPod *sandbox.Sandbox, overhead, resources *cri.LinuxContainerResources) error {
	if !a.isEnabled() {
		return nil
	}

	pod := nriPodSandbox(ctx, criPod)

	return a.nri.UpdatePodSandbox(ctx, pod, fromCRILinuxResources(overhead), fromCRILinuxResources(resources))
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

	if adjust == nil {
		return nil
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
						containerMinMemory, err := a.cri.ContainerServer.Runtime().GetContainerMinMemory(criPod.RuntimeHandler())
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
				if !a.cri.ContainerServer.Config().BlockIO().Enabled() || className == "" {
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
		nrigen.WithCDIDeviceInjector(
			func(s *rspec.Spec, devices []string) error {
				if err := cdi.Refresh(); err != nil {
					// We don't consider a refresh failure a fatal error.
					// For instance, a dynamically generated invalid CDI Spec file for
					// any particular vendor shouldn't prevent injection of devices of
					// different vendors. CDI itself knows better and it will fail the
					// injection if necessary.
					log.Warnf(context.TODO(), "CDI registry has errors: %v", err)
				}

				if _, err := cdi.InjectDevices(s, devices...); err != nil {
					return fmt.Errorf("CDI device injection failed: %w", err)
				}

				return nil
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

	r, err := a.nri.UpdateContainer(ctx, pod, ctr, fromCRILinuxResources(req))
	if err != nil {
		return nil, err
	}

	return toCRIResources(r, noOomAdj), nil
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
		sandboxID, err := a.cri.ContainerServer.PodIDIndex().Get(ctr.GetPodSandboxID())
		if err != nil {
			log.Errorf(ctx, "Failed to stop CRI container %q: %v", ctr.GetID(), err)

			return nil
		}

		criPod = a.cri.GetSandbox(sandboxID)
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

	for _, pod := range a.cri.ListSandboxes() {
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
	sandboxID, err := a.cri.ContainerServer.PodIDIndex().Get(id)
	if err != nil {
		return nil, false
	}

	pod := a.cri.GetSandbox(sandboxID)
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
	ctr, err := a.cri.GetContainerFromShortID(context.TODO(), u.GetContainerId())
	if err != nil {
		// We blindly assume container with given ID not found and ignore it.
		log.Errorf(ctx, "Failed to update CRI container %q: %v", u.GetContainerId(), err)

		return nil
	}

	if s := ctr.State().Status; s != oci.ContainerStateRunning && s != oci.ContainerStateCreated {
		return nil
	}

	resources := u.GetLinux().GetResources().ToOCI()
	if err = a.cri.ContainerServer.Runtime().UpdateContainer(ctx, ctr, resources); err != nil {
		log.Errorf(ctx, "Failed to update CRI container %q: %v", u.GetContainerId(), err)

		if u.GetIgnoreFailure() {
			return nil
		}

		return fmt.Errorf("failed to update CRI container %q: %w", u.GetContainerId(), err)
	}

	a.cri.UpdateContainerLinuxResources(ctr, resources)

	return nil
}

func (a *nriAPI) EvictContainer(ctx context.Context, e *api.ContainerEviction) error {
	ctr, err := a.cri.GetContainerFromShortID(context.TODO(), e.GetContainerId())
	if err != nil {
		// We blindly assume container with given ID not found and ignore it.
		log.Errorf(ctx, "Failed to evict CRI container %q: %v", e.GetContainerId(), err)

		return nil
	}

	if err = a.cri.stopContainer(ctx, ctr, 0); err != nil {
		log.Errorf(ctx, "Failed to evict CRI container %q: %v", e.GetContainerId(), err)

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

	return p.Sandbox.Metadata().GetName()
}

func (p *criPodSandbox) GetUID() string {
	if p.Sandbox == nil {
		return ""
	}

	return p.Sandbox.Metadata().GetUid()
}

func (p *criPodSandbox) GetNamespace() string {
	if p.Sandbox == nil {
		return ""
	}

	return p.Sandbox.Metadata().GetNamespace()
}

func (p *criPodSandbox) GetAnnotations() map[string]string {
	if p.Sandbox == nil {
		return nil
	}

	anns := map[string]string{}
	maps.Copy(anns, p.Annotations())

	return anns
}

func (p *criPodSandbox) GetLabels() map[string]string {
	if p.Sandbox == nil {
		return nil
	}

	labels := map[string]string{}
	maps.Copy(labels, p.Labels())

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

	return fromCRILinuxResources(p.PodLinuxOverhead())
}

func (p *criPodSandbox) GetPodLinuxResources() *api.LinuxResources {
	if p.Sandbox == nil {
		return nil
	}

	return fromCRILinuxResources(p.PodLinuxResources())
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

func (c *criContainer) GetIOPriority() *api.LinuxIOPriority {
	spec := c.GetSpec()
	if spec.Process == nil {
		return nil
	}

	return api.FromOCILinuxIOPriority(spec.Process.IOPriority)
}

func (c *criContainer) GetScheduler() *api.LinuxScheduler {
	spec := c.GetSpec()
	if spec.Process == nil || spec.Process.Scheduler == nil {
		return nil
	}

	return api.FromOCILinuxScheduler(spec.Process.Scheduler)
}

func (c *criContainer) GetNetDevices() map[string]*api.LinuxNetDevice {
	spec := c.GetSpec()
	if spec.Linux == nil {
		return nil
	}

	return api.FromOCILinuxNetDevices(spec.Linux.NetDevices)
}

func (c *criContainer) GetRdt() *api.LinuxRdt {
	spec := c.GetSpec()
	if spec.Linux == nil || spec.Linux.IntelRdt == nil {
		return nil
	}

	return &api.LinuxRdt{
		ClosId:           api.String(spec.Linux.IntelRdt.ClosID),
		Schemata:         api.RepeatedString(spec.Linux.IntelRdt.Schemata),
		EnableMonitoring: api.Bool(spec.Linux.IntelRdt.EnableMonitoring),
	}
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

//
// conversion to/from CRI types
//

// fromCRILinuxResources converts linux container resources from CRI to NRI representation.
func fromCRILinuxResources(c *cri.LinuxContainerResources) *api.LinuxResources {
	if c == nil {
		return nil
	}

	shares, quota, period := uint64(c.GetCpuShares()), c.GetCpuQuota(), uint64(c.GetCpuPeriod())
	r := &api.LinuxResources{
		Cpu: &api.LinuxCPU{
			Shares: api.UInt64(&shares),
			Quota:  api.Int64(&quota),
			Period: api.UInt64(&period),
			Cpus:   c.GetCpusetCpus(),
			Mems:   c.GetCpusetMems(),
		},
		Memory: &api.LinuxMemory{
			Limit: api.Int64(&c.MemoryLimitInBytes),
		},
	}

	for _, l := range c.GetHugepageLimits() {
		r.HugepageLimits = append(r.HugepageLimits,
			&api.HugepageLimit{
				PageSize: l.GetPageSize(),
				Limit:    l.GetLimit(),
			})
	}

	if u := c.GetUnified(); len(u) != 0 {
		r.Unified = make(map[string]string)
		maps.Copy(r.GetUnified(), u)
	}

	return r
}

// toCRIResources converts linux container resources from NRI to CRI representation.
func toCRIResources(r *api.LinuxResources, oomScoreAdj int64) *cri.LinuxContainerResources {
	if r == nil {
		return nil
	}

	o := &cri.LinuxContainerResources{}
	if mem := r.GetMemory(); mem != nil {
		o.MemoryLimitInBytes = mem.GetLimit().GetValue()
		o.OomScoreAdj = oomScoreAdj
	}

	if cpu := r.GetCpu(); cpu != nil {
		o.CpuShares = int64(cpu.GetShares().GetValue())
		o.CpuPeriod = int64(cpu.GetPeriod().GetValue())
		o.CpuQuota = cpu.GetQuota().GetValue()
		o.CpusetCpus = cpu.GetCpus()
		o.CpusetMems = cpu.GetMems()
	}

	for _, l := range r.GetHugepageLimits() {
		o.HugepageLimits = append(o.HugepageLimits, &cri.HugepageLimit{
			PageSize: l.GetPageSize(),
			Limit:    l.GetLimit(),
		})
	}

	if u := r.GetUnified(); len(u) != 0 {
		o.Unified = make(map[string]string)
		maps.Copy(o.GetUnified(), u)
	}

	return o
}
