package nri

import (
	"context"
	"flag"
	"fmt"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	cri "k8s.io/cri-api/pkg/apis/runtime/v1"
)

const (
	ciImage        = "quay.io/crio/fedora-crio-ci:latest"
	connectTimeout = 3 * time.Second
	requestTimeout = 10 * time.Second
	pullimgTimeout = 300 * time.Second
)

type runtime struct {
	sync.Mutex
	cc         *grpc.ClientConn
	runtime    cri.RuntimeServiceClient
	image      cri.ImageServiceClient
	podConfigs map[string]*cri.PodSandboxConfig
	pods       map[string]string
	containers map[string]string
	images     *imageRefs
}

type imageRefs struct {
	busybox string
}

var (
	crioSocket = flag.String("crio-socket", "", "cri-o socket to use")
	nriSocket  = flag.String("nri-socket", "", "NRI socket to use")
	cgroupMgr  = flag.String("cgroup-manager", "systemd", "cgroup manager used by cri-o")
)

func ConnectRuntime() (*runtime, error) {
	ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
	defer cancel()
	dialOpts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
		grpc.FailOnNonTempDialError(true),
	}

	cc, err := grpc.DialContext(ctx, *crioSocket, dialOpts...)
	if err != nil {
		return nil, fmt.Errorf("runtime connection failed: %w", err)
	}

	return &runtime{
		cc:         cc,
		runtime:    cri.NewRuntimeServiceClient(cc),
		image:      cri.NewImageServiceClient(cc),
		podConfigs: make(map[string]*cri.PodSandboxConfig),
		pods:       make(map[string]string),
		containers: make(map[string]string),
	}, nil
}

func (r *runtime) PullImages() error {
	if r.images != nil {
		return nil
	}

	imgRefs := &imageRefs{}

	for name, setRef := range map[string]func(string){
		ciImage: func(ref string) { imgRefs.busybox = ref },
	} {
		ref, err := r.PullImage(name)
		if err != nil {
			return err
		}
		setRef(ref)
	}

	r.images = imgRefs

	return nil
}

func (r *runtime) Disconnect() {
	if r == nil || r.cc == nil {
		return
	}
	r.cc.Close()
	r.runtime = nil
	r.image = nil
}

func (r *runtime) PullImage(image string) (string, error) {
	listCtx, cancelList := context.WithTimeout(context.Background(), requestTimeout)
	defer cancelList()

	listReply, err := r.image.ListImages(listCtx, &cri.ListImagesRequest{
		Filter: &cri.ImageFilter{
			Image: &cri.ImageSpec{
				Image: image,
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to list images: %w", err)
	}

	for _, img := range listReply.Images {
		for _, tag := range img.RepoTags {
			if tag == image {
				return img.Id, nil
			}
		}
	}

	pullCtx, cancelPull := context.WithTimeout(context.Background(), pullimgTimeout)
	defer cancelPull()

	reply, err := r.image.PullImage(pullCtx, &cri.PullImageRequest{
		Image: &cri.ImageSpec{Image: image},
	})
	if err != nil {
		return "", fmt.Errorf("failed to pull image %s: %w", image, err)
	}

	return reply.ImageRef, nil
}

func (r *runtime) ListPods(namespace string) (ready, other []string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	reply, err := r.runtime.ListPodSandbox(ctx, &cri.ListPodSandboxRequest{})
	if err != nil {
		return nil, nil, err
	}

	for _, pod := range reply.Items {
		if pod.GetMetadata().GetNamespace() != namespace {
			continue
		}
		if pod.GetState() == cri.PodSandboxState_SANDBOX_READY {
			ready = append(ready, pod.GetId())
		} else {
			other = append(other, pod.GetId())
		}
	}

	return ready, other, nil
}

type PodOption func(*cri.PodSandboxConfig) error

func WithPodAnnotations(annotations map[string]string) PodOption {
	return func(cfg *cri.PodSandboxConfig) error {
		for k, v := range annotations {
			cfg.Annotations[k] = v
		}
		return nil
	}
}

func WithPodLabels(labels map[string]string) PodOption {
	return func(cfg *cri.PodSandboxConfig) error {
		for k, v := range labels {
			cfg.Labels[k] = v
		}
		return nil
	}
}

func (r *runtime) CreatePod(namespace, name, uid string, options ...PodOption) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	var cgroupParent string

	if *cgroupMgr == "systemd" {
		cgroupParent = "pod_123.slice"
	} else {
		cgroupParent = "pod_123"
	}

	config := &cri.PodSandboxConfig{
		Metadata: &cri.PodSandboxMetadata{
			Name:      name,
			Uid:       uid,
			Namespace: namespace,
			Attempt:   1,
		},
		Hostname: "crio-nri-tests",
		DnsConfig: &cri.DNSConfig{
			Servers: []string{
				"8.8.8.8",
			},
		},
		PortMappings: []*cri.PortMapping{},
		Labels:       map[string]string{},
		Annotations:  map[string]string{},
		Linux: &cri.LinuxPodSandboxConfig{
			CgroupParent: cgroupParent,
			SecurityContext: &cri.LinuxSandboxSecurityContext{
				NamespaceOptions: &cri.NamespaceOption{
					Network: 0,
					Pid:     1,
					Ipc:     0,
				},
				SelinuxOptions: &cri.SELinuxOption{
					User:  "system_u",
					Role:  "system_r",
					Type:  "svirt_lxc_net_t",
					Level: "s0:c4,c5",
				},
			},
		},
	}
	for _, o := range options {
		if err := o(config); err != nil {
			return "", fmt.Errorf("failed to create pod %s/%s=%s: %w", namespace, name, uid, err)
		}
	}

	reply, err := r.runtime.RunPodSandbox(ctx, &cri.RunPodSandboxRequest{
		Config: config,
	})
	if err != nil {
		return "", fmt.Errorf("failed to create pod %s/%s=%s: %w", namespace, name, uid, err)
	}

	r.Lock()
	defer r.Unlock()
	id := reply.PodSandboxId
	r.podConfigs[id] = config
	r.pods[uid] = id
	r.pods[id] = id

	return reply.PodSandboxId, nil
}

func (r *runtime) StopPod(pod string) error {
	id, ok := r.pods[pod]
	if !ok {
		id = pod
	}

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	_, err := r.runtime.StopPodSandbox(ctx, &cri.StopPodSandboxRequest{
		PodSandboxId: id,
	})
	if err != nil {
		return fmt.Errorf("failed to stop pod %s: %v", pod, err)
	}

	return nil
}

func (r *runtime) RemovePod(pod string) error {
	id, ok := r.pods[pod]
	if !ok {
		id = pod
	}

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	_, err := r.runtime.RemovePodSandbox(ctx, &cri.RemovePodSandboxRequest{
		PodSandboxId: id,
	})
	if err != nil {
		return fmt.Errorf("failed to remove pod %s: %v", pod, err)
	}

	r.Lock()
	defer r.Unlock()
	delete(r.pods, pod)
	delete(r.pods, id)

	return nil
}

func (r *runtime) ListContainers(namespace string) (running, other, readyPods, otherPods []string, err error) {
	readyPods, otherPods, err = r.ListPods(namespace)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	pods := map[string]struct{}{}
	for _, pod := range readyPods {
		pods[pod] = struct{}{}
	}
	for _, pod := range otherPods {
		pods[pod] = struct{}{}
	}

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	reply, err := r.runtime.ListContainers(ctx, &cri.ListContainersRequest{})
	if err != nil {
		return nil, nil, nil, nil, err
	}

	for _, ctr := range reply.Containers {
		pod := ctr.GetPodSandboxId()
		if _, ok := pods[pod]; !ok {
			continue
		}
		if ctr.GetState() == cri.ContainerState_CONTAINER_RUNNING {
			running = append(running, ctr.GetId())
		} else {
			other = append(other, ctr.GetId())
		}
	}

	return running, other, readyPods, otherPods, nil
}

type ContainerOption func(*cri.ContainerConfig) error

func WithImage(image string) ContainerOption {
	return func(cfg *cri.ContainerConfig) error {
		cfg.Image = &cri.ImageSpec{
			Image: image,
		}
		return nil
	}
}

func WithCommand(cmd ...string) ContainerOption {
	return func(cfg *cri.ContainerConfig) error {
		cfg.Command = cmd
		return nil
	}
}

func WithShellScript(cmd string) ContainerOption {
	return func(cfg *cri.ContainerConfig) error {
		cfg.Command = []string{"sh", "-c", cmd}
		return nil
	}
}

func WithArgs(args ...string) ContainerOption {
	return func(cfg *cri.ContainerConfig) error {
		cfg.Args = args
		return nil
	}
}

func WithEnv(envs []*cri.KeyValue) ContainerOption {
	return func(cfg *cri.ContainerConfig) error {
		cfg.Envs = envs
		return nil
	}
}

func WithAnnotations(annotations map[string]string) ContainerOption {
	return func(cfg *cri.ContainerConfig) error {
		for k, v := range annotations {
			cfg.Annotations[k] = v
		}
		return nil
	}
}

func WithLabels(labels map[string]string) ContainerOption {
	return func(cfg *cri.ContainerConfig) error {
		for k, v := range labels {
			cfg.Labels[k] = v
		}
		return nil
	}
}

func WithResources(r *cri.LinuxContainerResources) ContainerOption {
	return func(cfg *cri.ContainerConfig) error {
		cfg.Linux.Resources = r
		return nil
	}
}

func WithSecurityContext(c *cri.LinuxContainerSecurityContext) ContainerOption {
	return func(cfg *cri.ContainerConfig) error {
		cfg.Linux.SecurityContext = c
		return nil
	}
}

func (r *runtime) CreateContainer(pod, name, uid string, options ...ContainerOption) (string, error) {
	podConfig, ok := r.podConfigs[pod]
	if !ok {
		return "", fmt.Errorf("failed to create container %s:%s=%s, no pod config found",
			pod, name, uid)
	}

	config := &cri.ContainerConfig{
		Metadata: &cri.ContainerMetadata{
			Name:    name,
			Attempt: 1,
		},
		Image: &cri.ImageSpec{
			Image: r.images.busybox,
		},
		Command: []string{
			"sh",
			"-c",
			fmt.Sprintf("echo %s/%s/%s $(sleep 3600)",
				podConfig.Metadata.Namespace, podConfig.Metadata.Name, name),
		},
		WorkingDir:  "/",
		Labels:      make(map[string]string),
		Annotations: make(map[string]string),
		Linux: &cri.LinuxContainerConfig{
			Resources: &cri.LinuxContainerResources{},
			SecurityContext: &cri.LinuxContainerSecurityContext{
				NamespaceOptions: &cri.NamespaceOption{
					Pid: 1,
				},
			},
		},
	}

	for _, o := range options {
		if err := o(config); err != nil {
			return "", fmt.Errorf("failed to create container %s:%s=%s: %w", pod, name, uid, err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	reply, err := r.runtime.CreateContainer(ctx, &cri.CreateContainerRequest{
		PodSandboxId:  pod,
		Config:        config,
		SandboxConfig: podConfig,
	})
	if err != nil {
		return "", fmt.Errorf("failed to create container %s:%s=%s: %w", pod, name, uid, err)
	}

	r.Lock()
	defer r.Unlock()
	id := reply.ContainerId
	r.containers[uid] = id
	r.containers[id] = id

	return id, nil
}

func (r *runtime) StartContainer(container string) error {
	id, ok := r.containers[container]
	if !ok {
		return fmt.Errorf("can't start container %s: unknown", container)
	}

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	_, err := r.runtime.StartContainer(ctx, &cri.StartContainerRequest{
		ContainerId: id,
	})
	if err != nil {
		return fmt.Errorf("failed to start container %s: %v", container, err)
	}

	return nil
}

func (r *runtime) StopContainer(container string) error {
	id, ok := r.containers[container]
	if !ok {
		id = container
	}

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	_, err := r.runtime.StopContainer(ctx, &cri.StopContainerRequest{
		ContainerId: id,
	})
	if err != nil {
		return fmt.Errorf("failed to stop container %s: %v", container, err)
	}

	return nil
}

func (r *runtime) RemoveContainer(container string) error {
	id, ok := r.containers[container]
	if !ok {
		id = container
	}

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	_, err := r.runtime.RemoveContainer(ctx, &cri.RemoveContainerRequest{
		ContainerId: id,
	})
	if err != nil {
		return fmt.Errorf("failed to remove container %s: %v", container, err)
	}

	r.Lock()
	defer r.Unlock()
	delete(r.containers, container)
	delete(r.containers, id)

	return nil
}

func (r *runtime) ExecSync(ctr string, cmd []string) (stdout, stderr []byte, ec int32, err error) {
	var reply *cri.ExecSyncResponse

	reply, err = r.runtime.ExecSync(context.Background(), &cri.ExecSyncRequest{
		ContainerId: ctr,
		Cmd:         cmd,
	})
	if err != nil {
		return nil, nil, -1, fmt.Errorf("failed to exec command in container %s: %w", ctr, err)
	}

	return reply.Stdout, reply.Stderr, reply.ExitCode, nil
}
