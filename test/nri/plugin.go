package nri

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/containerd/nri/pkg/api"
	"github.com/containerd/nri/pkg/stub"
)

type PluginOption func(*plugin)

type plugin struct {
	sync.Mutex
	namespace string
	options   []stub.Option
	stub      stub.Stub
	name      string
	idx       string
	eventW    chan *event
	eventR    chan *event
	stopOnce  sync.Once
	doneC     chan struct{}
	pods      map[string]*api.PodSandbox
	ctrs      map[string]*api.Container

	createContainer     func(*plugin, *api.PodSandbox, *api.Container) (*api.ContainerAdjustment, []*api.ContainerUpdate, error)
	postCreateContainer func(*plugin, *api.PodSandbox, *api.Container) error
	updateContainer     func(*plugin, *api.PodSandbox, *api.Container) ([]*api.ContainerUpdate, error)
	stopContainer       func(*plugin, *api.PodSandbox, *api.Container) ([]*api.ContainerUpdate, error)
}

type event struct {
	kind string
	pods []*api.PodSandbox
	ctrs []*api.Container
	pod  *api.PodSandbox
	ctr  *api.Container
	err  error
}

func WithStubOptions(options ...stub.Option) PluginOption {
	return func(p *plugin) {
		p.options = append(p.options, options...)
	}
}

func WithTestNamespace(namespace string) PluginOption {
	return func(p *plugin) {
		p.namespace = namespace
	}
}

func WithCreateHandler(fn func(*plugin, *api.PodSandbox, *api.Container) (*api.ContainerAdjustment, []*api.ContainerUpdate, error)) PluginOption {
	return func(p *plugin) {
		p.createContainer = fn
	}
}

func WithPostCreateHandler(fn func(*plugin, *api.PodSandbox, *api.Container) error) PluginOption {
	return func(p *plugin) {
		p.postCreateContainer = fn
	}
}

func WithStopHandler(fn func(*plugin, *api.PodSandbox, *api.Container) ([]*api.ContainerUpdate, error)) PluginOption {
	return func(p *plugin) {
		p.stopContainer = fn
	}
}

func WithUpdateHandler(fn func(*plugin, *api.PodSandbox, *api.Container) ([]*api.ContainerUpdate, error)) PluginOption {
	return func(p *plugin) {
		p.updateContainer = fn
	}
}

func NewPlugin(namespace string, options ...PluginOption) *plugin {
	p := &plugin{
		namespace: namespace,
		name:      "test-plugin",
		idx:       "00",
		eventW:    make(chan *event),
		eventR:    make(chan *event),
		doneC:     make(chan struct{}),
		pods:      make(map[string]*api.PodSandbox),
		ctrs:      make(map[string]*api.Container),
	}

	for _, opt := range options {
		opt(p)
	}

	return p
}

func (p *plugin) Name() string {
	return p.idx + "-" + p.name
}

func (p *plugin) inNamespace(namespace string) bool {
	return p.namespace == namespace
}

func (p *plugin) Start() error {
	opts := []stub.Option{
		stub.WithPluginName(p.name),
		stub.WithPluginIdx(p.idx),
		stub.WithSocketPath(strings.TrimPrefix(*nriSocket, "unix://")),
		stub.WithOnClose(p.onClose),
	}

	s, err := stub.New(p, opts...)
	if err != nil {
		return fmt.Errorf("failed to create plugin: %w", err)
	}
	p.stub = s

	go p.pumpEvents()

	err = p.stub.Start(context.Background())
	if err != nil {
		return fmt.Errorf("failed to start plugin: %w", err)
	}

	return nil
}

func (p *plugin) Stop() {
	p.stopOnce.Do(func() {
		close(p.doneC)
		if p.stub != nil {
			p.stub.Stop()
			p.stub.Wait()
		}
	})
}

func (p *plugin) onClose() {
	p.emitEvent(PluginClosedEvent)
	p.Stop()
}

func (p *plugin) Configure(cfg, name, version string) (stub.EventMask, error) {
	p.emitEvent(PluginConfigEvent)
	return 0, nil
}

func (p *plugin) Synchronize(pods []*api.PodSandbox, ctrs []*api.Container) ([]*api.ContainerUpdate, error) {
	p.Lock()
	defer p.Unlock()

	var (
		podIDs = map[string]*api.PodSandbox{}
		nsPods []*api.PodSandbox
		nsCtrs []*api.Container
	)

	for _, pod := range pods {
		if p.inNamespace(pod.GetNamespace()) {
			podIDs[pod.GetId()] = pod
			p.pods[pod.GetId()] = pod
			nsPods = append(nsPods, pod)
		}
	}

	for _, ctr := range ctrs {
		if _, ok := podIDs[ctr.GetPodSandboxId()]; ok {
			nsCtrs = append(nsCtrs, ctr)
			p.ctrs[ctr.GetId()] = ctr
		}
	}

	p.emitEvent(
		&event{
			kind: "Synchronize",
			pods: nsPods,
			ctrs: nsCtrs,
		},
	)
	return nil, nil
}

func (p *plugin) RunPodSandbox(pod *api.PodSandbox) error {
	if !p.inNamespace(pod.GetNamespace()) {
		return nil
	}

	p.Lock()
	defer p.Unlock()

	p.pods[pod.GetId()] = pod

	p.emitEvent(
		&event{
			kind: "RunPodSandbox",
			pod:  pod,
		},
	)
	return nil
}

func (p *plugin) StopPodSandbox(pod *api.PodSandbox) error {
	if !p.inNamespace(pod.GetNamespace()) {
		return nil
	}

	p.emitEvent(
		&event{
			kind: "StopPodSandbox",
			pod:  pod,
		},
	)
	return nil
}

func (p *plugin) RemovePodSandbox(pod *api.PodSandbox) error {
	if !p.inNamespace(pod.GetNamespace()) {
		return nil
	}

	p.Lock()
	defer p.Unlock()

	delete(p.pods, pod.GetId())

	p.emitEvent(
		&event{
			kind: "RemovePodSandbox",
			pod:  pod,
		},
	)
	return nil
}

func (p *plugin) CreateContainer(pod *api.PodSandbox, ctr *api.Container) (*api.ContainerAdjustment, []*api.ContainerUpdate, error) {
	if !p.inNamespace(pod.GetNamespace()) {
		return nil, nil, nil
	}

	p.Lock()
	defer p.Unlock()

	var (
		adjust *api.ContainerAdjustment
		update []*api.ContainerUpdate
		err    error
	)

	if p.createContainer != nil {
		adjust, update, err = p.createContainer(p, pod, ctr)
	}

	p.ctrs[ctr.GetId()] = ctr

	p.emitEvent(
		&event{
			kind: "CreateContainer",
			pod:  pod,
			ctr:  ctr,
			err:  err,
		},
	)
	return adjust, update, err
}

func (p *plugin) PostCreateContainer(pod *api.PodSandbox, ctr *api.Container) error {
	if !p.inNamespace(pod.GetNamespace()) {
		return nil
	}

	var err error

	if p.postCreateContainer != nil {
		err = p.postCreateContainer(p, pod, ctr)
	}

	p.emitEvent(
		&event{
			kind: "PostCreateContainer",
			pod:  pod,
			ctr:  ctr,
		},
	)
	return err
}

func (p *plugin) StartContainer(pod *api.PodSandbox, ctr *api.Container) error {
	if !p.inNamespace(pod.GetNamespace()) {
		return nil
	}

	p.emitEvent(
		&event{
			kind: "StartContainer",
			pod:  pod,
			ctr:  ctr,
		},
	)
	return nil
}

func (p *plugin) PostStartContainer(pod *api.PodSandbox, ctr *api.Container) error {
	if !p.inNamespace(pod.GetNamespace()) {
		return nil
	}

	p.emitEvent(
		&event{
			kind: "PostStartContainer",
			pod:  pod,
			ctr:  ctr,
		},
	)
	return nil
}

func (p *plugin) UpdateContainer(pod *api.PodSandbox, ctr *api.Container) ([]*api.ContainerUpdate, error) {
	if !p.inNamespace(pod.GetNamespace()) {
		return nil, nil
	}

	p.Lock()
	defer p.Unlock()

	var (
		update []*api.ContainerUpdate
		err    error
	)

	if p.updateContainer != nil {
		update, err = p.updateContainer(p, pod, ctr)
	}

	p.ctrs[ctr.GetId()] = ctr

	p.emitEvent(
		&event{
			kind: "UpdateContainer",
			pod:  pod,
			ctr:  ctr,
			err:  err,
		},
	)
	return update, err
}

func (p *plugin) PostUpdateContainer(pod *api.PodSandbox, ctr *api.Container) error {
	if !p.inNamespace(pod.GetNamespace()) {
		return nil
	}

	p.emitEvent(
		&event{
			kind: "PostUpdateContainer",
			pod:  pod,
			ctr:  ctr,
		},
	)
	return nil
}

func (p *plugin) StopContainer(pod *api.PodSandbox, ctr *api.Container) ([]*api.ContainerUpdate, error) {
	if !p.inNamespace(pod.GetNamespace()) {
		return nil, nil
	}

	var (
		update []*api.ContainerUpdate
		err    error
	)

	if p.stopContainer != nil {
		update, err = p.stopContainer(p, pod, ctr)
	}

	p.emitEvent(
		&event{
			kind: "StopContainer",
			pod:  pod,
			ctr:  ctr,
			err:  err,
		},
	)
	return update, err
}

func (p *plugin) RemoveContainer(pod *api.PodSandbox, ctr *api.Container) error {
	if !p.inNamespace(pod.GetNamespace()) {
		return nil
	}

	p.Lock()
	defer p.Unlock()

	delete(p.ctrs, ctr.GetId())

	p.emitEvent(
		&event{
			kind: "RemoveContainer",
			pod:  pod,
			ctr:  ctr,
		},
	)
	return nil
}

func (p *plugin) GetPod(id string) (*api.PodSandbox, bool) {
	p.Lock()
	defer p.Unlock()

	pod, ok := p.pods[id]
	return pod, ok
}

func (p *plugin) GetContainer(id string) (*api.Container, bool) {
	p.Lock()
	defer p.Unlock()

	ctr, ok := p.ctrs[id]
	return ctr, ok
}

func (p *plugin) pumpEvents() {
	var (
		eventQ = []*event{}
		eventR chan *event
		next   *event
	)
	for {
		if next == nil {
			if len(eventQ) > 0 {
				next = eventQ[0]
				eventQ = eventQ[1:]
				eventR = p.eventR
			} else {
				eventR = nil
			}
		}

		select {
		case <-p.doneC:
			return
		case e, ok := <-p.eventW:
			if !ok {
				return
			}
			eventQ = append(eventQ, e)
		case eventR <- next:
			next = nil
			continue
		}
	}
}

func (p *plugin) emitEvent(e *event) {
	p.eventW <- e
}

func (p *plugin) PollEvent(timeout time.Duration) *event {
	select {
	case e, ok := <-p.eventR:
		if ok {
			return e
		}
	case <-time.After(timeout):
	}
	return nil
}

func (p *plugin) WaitEvent(evt *event, timeout time.Duration) *event {
	deadline := time.After(timeout)
	for {
		e := p.PollEvent(timeout)
		if e != nil && (evt == nil || e.Matches(evt)) {
			return e
		}
		select {
		case <-deadline:
			return nil
		default:
		}
	}
}

func (p *plugin) VerifyEventStream(events []*event, exact bool, timeout time.Duration) error {
	var (
		deadline = time.After(timeout)
		i        int
	)
	for {
		select {
		case evt, ok := <-p.eventR:
			if !ok {
				return fmt.Errorf("receiving plugin event failed")
			}
			if evt.Matches(events[i]) {
				i++
				if i == len(events) {
					return nil
				}
			}
		case <-deadline:
			return fmt.Errorf("timeout waiting for event %s", events[i].String())
		}
	}
}

func (e *event) String() string {
	if e == nil {
		return "<nil event>"
	}
	var (
		pod string
		ctr string
	)

	if e.pod != nil {
		pod = "/" + e.pod.GetId()
	}
	if e.ctr != nil {
		ctr = ":" + e.ctr.GetId()
	}
	return "<" + e.kind + pod + ctr + ">"
}

func (e *event) IsEvent(kind string) bool {
	return e.kind == kind
}

func (e *event) IsPodEvent(kind, podID string) bool {
	if !e.IsEvent(kind) {
		return false
	}
	return e.pod != nil && e.pod.GetId() == podID
}

func (e *event) IsContainerEvent(kind, podID, ctrID string) bool {
	if !e.IsEvent(kind) {
		return false
	}
	if podID != "" && e.pod == nil || e.pod.GetId() != podID {
		return false
	}
	return e.ctr != nil && e.ctr.GetId() == ctrID
}

func (e *event) Matches(o *event) bool {
	if e == nil || o == nil {
		return false
	}

	if e.kind != o.kind {
		return false
	}
	if (e.pod == nil && o.pod != nil) || (e.pod != nil && o.pod == nil) {
		return false
	}
	if e.pod != nil && e.pod.GetId() != o.pod.GetId() {
		return false
	}
	if (e.ctr == nil && o.ctr != nil) || (e.ctr != nil && o.ctr == nil) {
		return false
	}
	if e.ctr != nil && e.ctr.GetId() != o.ctr.GetId() {
		return false
	}

	return true
}

var (
	PluginConfigEvent = &event{kind: "Configure"}
	PluginSyncedEvent = &event{kind: "Synchronize"}
	PluginClosedEvent = &event{kind: "Closed"}
)

func RunPodEvent(pod string) *event {
	return PodEvent("RunPodSandbox", pod)
}

func StopPodEvent(pod string) *event {
	return PodEvent("StopPodSandbox", pod)
}

func RemovePodEvent(pod string) *event {
	return PodEvent("RemovePodSandbox", pod)
}

func PodEvent(kind, pod string) *event {
	return &event{
		kind: kind,
		pod: &api.PodSandbox{
			Id: pod,
		},
	}
}

func CreateContainerEvent(pod, ctr string) *event {
	return ContainerEvent("CreateContainer", pod, ctr)
}

func PostCreateContainerEvent(pod, ctr string) *event {
	return ContainerEvent("PostCreateContainer", pod, ctr)
}

func StartContainerEvent(pod, ctr string) *event {
	return ContainerEvent("StartContainer", pod, ctr)
}

func PostStartContainerEvent(pod, ctr string) *event {
	return ContainerEvent("PostStartContainer", pod, ctr)
}

func UpdateContainerEvent(pod, ctr string) *event {
	return ContainerEvent("UpdateContainer", pod, ctr)
}

func PostUpdateContainerEvent(pod, ctr string) *event {
	return ContainerEvent("PostUpdateContainer", pod, ctr)
}

func StopContainerEvent(pod, ctr string) *event {
	return ContainerEvent("StopContainer", pod, ctr)
}

func RemoveContainerEvent(pod, ctr string) *event {
	return ContainerEvent("RemoveContainer", pod, ctr)
}

func ContainerEvent(kind, pod, ctr string) *event {
	return &event{
		kind: kind,
		pod: &api.PodSandbox{
			Id: pod,
		},
		ctr: &api.Container{
			Id: ctr,
		},
	}
}
