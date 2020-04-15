package libpod

import (
	"fmt"
	"strings"

	"github.com/containers/common/pkg/config"
	"github.com/containers/libpod/libpod/define"
	"github.com/containers/libpod/libpod/driver"
	"github.com/containers/libpod/pkg/util"
	spec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/opencontainers/runtime-tools/generate"
	"github.com/opencontainers/runtime-tools/validate"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/syndtr/gocapability/capability"
)

const (
	// InspectAnnotationCIDFile is used by Inspect to determine if a
	// container ID file was created for the container.
	// If an annotation with this key is found in the OCI spec, it will be
	// used in the output of Inspect().
	InspectAnnotationCIDFile = "io.podman.annotations.cid-file"
	// InspectAnnotationAutoremove is used by Inspect to determine if a
	// container will be automatically removed on exit.
	// If an annotation with this key is found in the OCI spec and is one of
	// the two supported boolean values (InspectResponseTrue and
	// InspectResponseFalse) it will be used in the output of Inspect().
	InspectAnnotationAutoremove = "io.podman.annotations.autoremove"
	// InspectAnnotationVolumesFrom is used by Inspect to identify
	// containers whose volumes are are being used by this container.
	// It is expected to be a comma-separated list of container names and/or
	// IDs.
	// If an annotation with this key is found in the OCI spec, it will be
	// used in the output of Inspect().
	InspectAnnotationVolumesFrom = "io.podman.annotations.volumes-from"
	// InspectAnnotationPrivileged is used by Inspect to identify containers
	// which are privileged (IE, running with elevated privileges).
	// It is expected to be a boolean, populated by one of
	// InspectResponseTrue or InspectResponseFalse.
	// If an annotation with this key is found in the OCI spec, it will be
	// used in the output of Inspect().
	InspectAnnotationPrivileged = "io.podman.annotations.privileged"
	// InspectAnnotationPublishAll is used by Inspect to identify containers
	// which have all the ports from their image published.
	// It is expected to be a boolean, populated by one of
	// InspectResponseTrue or InspectResponseFalse.
	// If an annotation with this key is found in the OCI spec, it will be
	// used in the output of Inspect().
	InspectAnnotationPublishAll = "io.podman.annotations.publish-all"
	// InspectAnnotationInit is used by Inspect to identify containers that
	// mount an init binary in.
	// It is expected to be a boolean, populated by one of
	// InspectResponseTrue or InspectResponseFalse.
	// If an annotation with this key is found in the OCI spec, it will be
	// used in the output of Inspect().
	InspectAnnotationInit = "io.podman.annotations.init"
	// InspectAnnotationLabel is used by Inspect to identify containers with
	// special SELinux-related settings. It is used to populate the output
	// of the SecurityOpt setting.
	// If an annotation with this key is found in the OCI spec, it will be
	// used in the output of Inspect().
	InspectAnnotationLabel = "io.podman.annotations.label"
	// InspectAnnotationSeccomp is used by Inspect to identify containers
	// with special Seccomp-related settings. It is used to populate the
	// output of the SecurityOpt setting in Inspect.
	// If an annotation with this key is found in the OCI spec, it will be
	// used in the output of Inspect().
	InspectAnnotationSeccomp = "io.podman.annotations.seccomp"
	// InspectAnnotationApparmor is used by Inspect to identify containers
	// with special Apparmor-related settings. It is used to populate the
	// output of the SecurityOpt setting.
	// If an annotation with this key is found in the OCI spec, it will be
	// used in the output of Inspect().
	InspectAnnotationApparmor = "io.podman.annotations.apparmor"

	// InspectResponseTrue is a boolean True response for an inspect
	// annotation.
	InspectResponseTrue = "TRUE"
	// InspectResponseFalse is a boolean False response for an inspect
	// annotation.
	InspectResponseFalse = "FALSE"
)

// inspectLocked inspects a container for low-level information.
// The caller must held c.lock.
func (c *Container) inspectLocked(size bool) (*define.InspectContainerData, error) {
	storeCtr, err := c.runtime.store.Container(c.ID())
	if err != nil {
		return nil, errors.Wrapf(err, "error getting container from store %q", c.ID())
	}
	layer, err := c.runtime.store.Layer(storeCtr.LayerID)
	if err != nil {
		return nil, errors.Wrapf(err, "error reading information about layer %q", storeCtr.LayerID)
	}
	driverData, err := driver.GetDriverData(c.runtime.store, layer.ID)
	if err != nil {
		return nil, errors.Wrapf(err, "error getting graph driver info %q", c.ID())
	}
	return c.getContainerInspectData(size, driverData)
}

// Inspect a container for low-level information
func (c *Container) Inspect(size bool) (*define.InspectContainerData, error) {
	if !c.batched {
		c.lock.Lock()
		defer c.lock.Unlock()

		if err := c.syncContainer(); err != nil {
			return nil, err
		}
	}

	return c.inspectLocked(size)
}

func (c *Container) getContainerInspectData(size bool, driverData *driver.Data) (*define.InspectContainerData, error) {
	config := c.config
	runtimeInfo := c.state
	ctrSpec, err := c.specFromState()
	if err != nil {
		return nil, err
	}

	// Process is allowed to be nil in the stateSpec
	args := []string{}
	if config.Spec.Process != nil {
		args = config.Spec.Process.Args
	}
	var path string
	if len(args) > 0 {
		path = args[0]
	}
	if len(args) > 1 {
		args = args[1:]
	}

	execIDs := []string{}
	for id := range c.state.ExecSessions {
		execIDs = append(execIDs, id)
	}

	resolvPath := ""
	hostsPath := ""
	hostnamePath := ""
	if c.state.BindMounts != nil {
		if getPath, ok := c.state.BindMounts["/etc/resolv.conf"]; ok {
			resolvPath = getPath
		}
		if getPath, ok := c.state.BindMounts["/etc/hosts"]; ok {
			hostsPath = getPath
		}
		if getPath, ok := c.state.BindMounts["/etc/hostname"]; ok {
			hostnamePath = getPath
		}
	}

	namedVolumes, mounts := c.sortUserVolumes(ctrSpec)
	inspectMounts, err := c.getInspectMounts(ctrSpec, namedVolumes, mounts)
	if err != nil {
		return nil, err
	}

	data := &define.InspectContainerData{
		ID:      config.ID,
		Created: config.CreatedTime,
		Path:    path,
		Args:    args,
		State: &define.InspectContainerState{
			OciVersion: ctrSpec.Version,
			Status:     runtimeInfo.State.String(),
			Running:    runtimeInfo.State == define.ContainerStateRunning,
			Paused:     runtimeInfo.State == define.ContainerStatePaused,
			OOMKilled:  runtimeInfo.OOMKilled,
			Dead:       runtimeInfo.State.String() == "bad state",
			Pid:        runtimeInfo.PID,
			ConmonPid:  runtimeInfo.ConmonPID,
			ExitCode:   runtimeInfo.ExitCode,
			Error:      "", // can't get yet
			StartedAt:  runtimeInfo.StartedTime,
			FinishedAt: runtimeInfo.FinishedTime,
		},
		Image:           config.RootfsImageID,
		ImageName:       config.RootfsImageName,
		ExitCommand:     config.ExitCommand,
		Namespace:       config.Namespace,
		Rootfs:          config.Rootfs,
		Pod:             config.Pod,
		ResolvConfPath:  resolvPath,
		HostnamePath:    hostnamePath,
		HostsPath:       hostsPath,
		StaticDir:       config.StaticDir,
		LogPath:         config.LogPath,
		LogTag:          config.LogTag,
		OCIRuntime:      config.OCIRuntime,
		ConmonPidFile:   config.ConmonPidFile,
		Name:            config.Name,
		RestartCount:    int32(runtimeInfo.RestartCount),
		Driver:          driverData.Name,
		MountLabel:      config.MountLabel,
		ProcessLabel:    config.ProcessLabel,
		EffectiveCaps:   ctrSpec.Process.Capabilities.Effective,
		BoundingCaps:    ctrSpec.Process.Capabilities.Bounding,
		AppArmorProfile: ctrSpec.Process.ApparmorProfile,
		ExecIDs:         execIDs,
		GraphDriver:     driverData,
		Mounts:          inspectMounts,
		Dependencies:    c.Dependencies(),
		IsInfra:         c.IsInfra(),
	}

	if c.state.ConfigPath != "" {
		data.OCIConfigPath = c.state.ConfigPath
	}

	if c.config.HealthCheckConfig != nil {
		// This container has a healthcheck defined in it; we need to add it's state
		healthCheckState, err := c.GetHealthCheckLog()
		if err != nil {
			// An error here is not considered fatal; no health state will be displayed
			logrus.Error(err)
		} else {
			data.State.Healthcheck = healthCheckState
		}
	}

	networkConfig, err := c.getContainerNetworkInfo()
	if err != nil {
		return nil, err
	}
	data.NetworkSettings = networkConfig

	inspectConfig, err := c.generateInspectContainerConfig(ctrSpec)
	if err != nil {
		return nil, err
	}
	data.Config = inspectConfig

	hostConfig, err := c.generateInspectContainerHostConfig(ctrSpec, namedVolumes, mounts)
	if err != nil {
		return nil, err
	}
	data.HostConfig = hostConfig

	if size {
		rootFsSize, err := c.rootFsSize()
		if err != nil {
			logrus.Errorf("error getting rootfs size %q: %v", config.ID, err)
		}
		data.SizeRootFs = rootFsSize

		rwSize, err := c.rwSize()
		if err != nil {
			logrus.Errorf("error getting rw size %q: %v", config.ID, err)
		}
		data.SizeRw = &rwSize
	}
	return data, nil
}

// Get inspect-formatted mounts list.
// Only includes user-specified mounts. Only includes bind mounts and named
// volumes, not tmpfs volumes.
func (c *Container) getInspectMounts(ctrSpec *spec.Spec, namedVolumes []*ContainerNamedVolume, mounts []spec.Mount) ([]define.InspectMount, error) {
	inspectMounts := []define.InspectMount{}

	// No mounts, return early
	if len(c.config.UserVolumes) == 0 {
		return inspectMounts, nil
	}

	for _, volume := range namedVolumes {
		mountStruct := define.InspectMount{}
		mountStruct.Type = "volume"
		mountStruct.Destination = volume.Dest
		mountStruct.Name = volume.Name

		// For src and driver, we need to look up the named
		// volume.
		volFromDB, err := c.runtime.state.Volume(volume.Name)
		if err != nil {
			return nil, errors.Wrapf(err, "error looking up volume %s in container %s config", volume.Name, c.ID())
		}
		mountStruct.Driver = volFromDB.Driver()
		mountStruct.Source = volFromDB.MountPoint()

		parseMountOptionsForInspect(volume.Options, &mountStruct)

		inspectMounts = append(inspectMounts, mountStruct)
	}
	for _, mount := range mounts {
		// It's a mount.
		// Is it a tmpfs? If so, discard.
		if mount.Type == "tmpfs" {
			continue
		}

		mountStruct := define.InspectMount{}
		mountStruct.Type = "bind"
		mountStruct.Source = mount.Source
		mountStruct.Destination = mount.Destination

		parseMountOptionsForInspect(mount.Options, &mountStruct)

		inspectMounts = append(inspectMounts, mountStruct)
	}

	return inspectMounts, nil
}

// Parse mount options so we can populate them in the mount structure.
// The mount passed in will be modified.
func parseMountOptionsForInspect(options []string, mount *define.InspectMount) {
	isRW := true
	mountProp := ""
	zZ := ""
	otherOpts := []string{}

	// Some of these may be overwritten if the user passes us garbage opts
	// (for example, [ro,rw])
	// We catch these on the Podman side, so not a problem there, but other
	// users of libpod who do not properly validate mount options may see
	// this.
	// Not really worth dealing with on our end - garbage in, garbage out.
	for _, opt := range options {
		switch opt {
		case "ro":
			isRW = false
		case "rw":
			// Do nothing, silently discard
		case "shared", "slave", "private", "rshared", "rslave", "rprivate":
			mountProp = opt
		case "z", "Z":
			zZ = opt
		default:
			otherOpts = append(otherOpts, opt)
		}
	}

	mount.RW = isRW
	mount.Propagation = mountProp
	mount.Mode = zZ
	mount.Options = otherOpts
}

// Generate the InspectContainerConfig struct for the Config field of Inspect.
func (c *Container) generateInspectContainerConfig(spec *spec.Spec) (*define.InspectContainerConfig, error) {
	ctrConfig := new(define.InspectContainerConfig)

	ctrConfig.Hostname = c.Hostname()
	ctrConfig.User = c.config.User
	if spec.Process != nil {
		ctrConfig.Tty = spec.Process.Terminal
		ctrConfig.Env = []string{}
		ctrConfig.Env = append(ctrConfig.Env, spec.Process.Env...)
		ctrConfig.WorkingDir = spec.Process.Cwd
	}

	ctrConfig.OpenStdin = c.config.Stdin
	ctrConfig.Image = c.config.RootfsImageName

	// Leave empty is not explicitly overwritten by user
	if len(c.config.Command) != 0 {
		ctrConfig.Cmd = []string{}
		ctrConfig.Cmd = append(ctrConfig.Cmd, c.config.Command...)
	}

	// Leave empty if not explicitly overwritten by user
	if len(c.config.Entrypoint) != 0 {
		ctrConfig.Entrypoint = strings.Join(c.config.Entrypoint, " ")
	}

	if len(c.config.Labels) != 0 {
		ctrConfig.Labels = make(map[string]string)
		for k, v := range c.config.Labels {
			ctrConfig.Labels[k] = v
		}
	}

	if len(spec.Annotations) != 0 {
		ctrConfig.Annotations = make(map[string]string)
		for k, v := range spec.Annotations {
			ctrConfig.Annotations[k] = v
		}
	}

	ctrConfig.StopSignal = c.config.StopSignal
	// TODO: should JSON deep copy this to ensure internal pointers don't
	// leak.
	ctrConfig.Healthcheck = c.config.HealthCheckConfig

	ctrConfig.CreateCommand = c.config.CreateCommand

	return ctrConfig, nil
}

// Generate the InspectContainerHostConfig struct for the HostConfig field of
// Inspect.
func (c *Container) generateInspectContainerHostConfig(ctrSpec *spec.Spec, namedVolumes []*ContainerNamedVolume, mounts []spec.Mount) (*define.InspectContainerHostConfig, error) {
	hostConfig := new(define.InspectContainerHostConfig)

	logConfig := new(define.InspectLogConfig)
	logConfig.Type = c.config.LogDriver
	hostConfig.LogConfig = logConfig

	restartPolicy := new(define.InspectRestartPolicy)
	restartPolicy.Name = c.config.RestartPolicy
	restartPolicy.MaximumRetryCount = c.config.RestartRetries
	hostConfig.RestartPolicy = restartPolicy
	if c.config.NoCgroups {
		hostConfig.Cgroups = "disabled"
	} else {
		hostConfig.Cgroups = "default"
	}

	hostConfig.Dns = make([]string, 0, len(c.config.DNSServer))
	for _, dns := range c.config.DNSServer {
		hostConfig.Dns = append(hostConfig.Dns, dns.String())
	}

	hostConfig.DnsOptions = make([]string, 0, len(c.config.DNSOption))
	hostConfig.DnsOptions = append(hostConfig.DnsOptions, c.config.DNSOption...)

	hostConfig.DnsSearch = make([]string, 0, len(c.config.DNSSearch))
	hostConfig.DnsSearch = append(hostConfig.DnsSearch, c.config.DNSSearch...)

	hostConfig.ExtraHosts = make([]string, 0, len(c.config.HostAdd))
	hostConfig.ExtraHosts = append(hostConfig.ExtraHosts, c.config.HostAdd...)

	hostConfig.GroupAdd = make([]string, 0, len(c.config.Groups))
	hostConfig.GroupAdd = append(hostConfig.GroupAdd, c.config.Groups...)

	hostConfig.SecurityOpt = []string{}
	if ctrSpec.Process != nil {
		if ctrSpec.Process.OOMScoreAdj != nil {
			hostConfig.OomScoreAdj = *ctrSpec.Process.OOMScoreAdj
		}
		if ctrSpec.Process.NoNewPrivileges {
			hostConfig.SecurityOpt = append(hostConfig.SecurityOpt, "no-new-privileges")
		}
	}

	hostConfig.ReadonlyRootfs = ctrSpec.Root.Readonly
	hostConfig.ShmSize = c.config.ShmSize
	hostConfig.Runtime = "oci"

	// This is very expensive to initialize.
	// So we don't want to initialize it unless we absolutely have to - IE,
	// there are things that require a major:minor to path translation.
	var deviceNodes map[string]string

	// Annotations
	if ctrSpec.Annotations != nil {
		hostConfig.ContainerIDFile = ctrSpec.Annotations[InspectAnnotationCIDFile]
		if ctrSpec.Annotations[InspectAnnotationAutoremove] == InspectResponseTrue {
			hostConfig.AutoRemove = true
		}
		if ctrs, ok := ctrSpec.Annotations[InspectAnnotationVolumesFrom]; ok {
			hostConfig.VolumesFrom = strings.Split(ctrs, ",")
		}
		if ctrSpec.Annotations[InspectAnnotationPrivileged] == InspectResponseTrue {
			hostConfig.Privileged = true
		}
		if ctrSpec.Annotations[InspectAnnotationInit] == InspectResponseTrue {
			hostConfig.Init = true
		}
		if label, ok := ctrSpec.Annotations[InspectAnnotationLabel]; ok {
			hostConfig.SecurityOpt = append(hostConfig.SecurityOpt, fmt.Sprintf("label=%s", label))
		}
		if seccomp, ok := ctrSpec.Annotations[InspectAnnotationSeccomp]; ok {
			hostConfig.SecurityOpt = append(hostConfig.SecurityOpt, fmt.Sprintf("seccomp=%s", seccomp))
		}
		if apparmor, ok := ctrSpec.Annotations[InspectAnnotationApparmor]; ok {
			hostConfig.SecurityOpt = append(hostConfig.SecurityOpt, fmt.Sprintf("apparmor=%s", apparmor))
		}
	}

	// Resource limits
	if ctrSpec.Linux != nil {
		if ctrSpec.Linux.Resources != nil {
			if ctrSpec.Linux.Resources.CPU != nil {
				if ctrSpec.Linux.Resources.CPU.Shares != nil {
					hostConfig.CpuShares = *ctrSpec.Linux.Resources.CPU.Shares
				}
				if ctrSpec.Linux.Resources.CPU.Period != nil {
					hostConfig.CpuPeriod = *ctrSpec.Linux.Resources.CPU.Period
				}
				if ctrSpec.Linux.Resources.CPU.Quota != nil {
					hostConfig.CpuQuota = *ctrSpec.Linux.Resources.CPU.Quota
				}
				if ctrSpec.Linux.Resources.CPU.RealtimePeriod != nil {
					hostConfig.CpuRealtimePeriod = *ctrSpec.Linux.Resources.CPU.RealtimePeriod
				}
				if ctrSpec.Linux.Resources.CPU.RealtimeRuntime != nil {
					hostConfig.CpuRealtimeRuntime = *ctrSpec.Linux.Resources.CPU.RealtimeRuntime
				}
				hostConfig.CpusetCpus = ctrSpec.Linux.Resources.CPU.Cpus
				hostConfig.CpusetMems = ctrSpec.Linux.Resources.CPU.Mems
			}
			if ctrSpec.Linux.Resources.Memory != nil {
				if ctrSpec.Linux.Resources.Memory.Limit != nil {
					hostConfig.Memory = *ctrSpec.Linux.Resources.Memory.Limit
				}
				if ctrSpec.Linux.Resources.Memory.Kernel != nil {
					hostConfig.KernelMemory = *ctrSpec.Linux.Resources.Memory.Kernel
				}
				if ctrSpec.Linux.Resources.Memory.Reservation != nil {
					hostConfig.MemoryReservation = *ctrSpec.Linux.Resources.Memory.Reservation
				}
				if ctrSpec.Linux.Resources.Memory.Swap != nil {
					hostConfig.MemorySwap = *ctrSpec.Linux.Resources.Memory.Swap
				}
				if ctrSpec.Linux.Resources.Memory.Swappiness != nil {
					hostConfig.MemorySwappiness = int64(*ctrSpec.Linux.Resources.Memory.Swappiness)
				} else {
					// Swappiness has a default of -1
					hostConfig.MemorySwappiness = -1
				}
				if ctrSpec.Linux.Resources.Memory.DisableOOMKiller != nil {
					hostConfig.OomKillDisable = *ctrSpec.Linux.Resources.Memory.DisableOOMKiller
				}
			}
			if ctrSpec.Linux.Resources.Pids != nil {
				hostConfig.PidsLimit = ctrSpec.Linux.Resources.Pids.Limit
			}
			if ctrSpec.Linux.Resources.BlockIO != nil {
				if ctrSpec.Linux.Resources.BlockIO.Weight != nil {
					hostConfig.BlkioWeight = *ctrSpec.Linux.Resources.BlockIO.Weight
				}
				hostConfig.BlkioWeightDevice = []define.InspectBlkioWeightDevice{}
				for _, dev := range ctrSpec.Linux.Resources.BlockIO.WeightDevice {
					key := fmt.Sprintf("%d:%d", dev.Major, dev.Minor)
					// TODO: how do we handle LeafWeight vs
					// Weight? For now, ignore anything
					// without Weight set.
					if dev.Weight == nil {
						logrus.Warnf("Ignoring weight device %s as it lacks a weight", key)
						continue
					}
					if deviceNodes == nil {
						nodes, err := util.FindDeviceNodes()
						if err != nil {
							return nil, err
						}
						deviceNodes = nodes
					}
					path, ok := deviceNodes[key]
					if !ok {
						logrus.Warnf("Could not locate weight device %s in system devices", key)
						continue
					}
					weightDev := define.InspectBlkioWeightDevice{}
					weightDev.Path = path
					weightDev.Weight = *dev.Weight
					hostConfig.BlkioWeightDevice = append(hostConfig.BlkioWeightDevice, weightDev)
				}

				handleThrottleDevice := func(devs []spec.LinuxThrottleDevice) ([]define.InspectBlkioThrottleDevice, error) {
					out := []define.InspectBlkioThrottleDevice{}
					for _, dev := range devs {
						key := fmt.Sprintf("%d:%d", dev.Major, dev.Minor)
						if deviceNodes == nil {
							nodes, err := util.FindDeviceNodes()
							if err != nil {
								return nil, err
							}
							deviceNodes = nodes
						}
						path, ok := deviceNodes[key]
						if !ok {
							logrus.Warnf("Could not locate throttle device %s in system devices", key)
							continue
						}
						throttleDev := define.InspectBlkioThrottleDevice{}
						throttleDev.Path = path
						throttleDev.Rate = dev.Rate
						out = append(out, throttleDev)
					}
					return out, nil
				}

				readBps, err := handleThrottleDevice(ctrSpec.Linux.Resources.BlockIO.ThrottleReadBpsDevice)
				if err != nil {
					return nil, err
				}
				hostConfig.BlkioDeviceReadBps = readBps

				writeBps, err := handleThrottleDevice(ctrSpec.Linux.Resources.BlockIO.ThrottleWriteBpsDevice)
				if err != nil {
					return nil, err
				}
				hostConfig.BlkioDeviceWriteBps = writeBps

				readIops, err := handleThrottleDevice(ctrSpec.Linux.Resources.BlockIO.ThrottleReadIOPSDevice)
				if err != nil {
					return nil, err
				}
				hostConfig.BlkioDeviceReadIOps = readIops

				writeIops, err := handleThrottleDevice(ctrSpec.Linux.Resources.BlockIO.ThrottleWriteIOPSDevice)
				if err != nil {
					return nil, err
				}
				hostConfig.BlkioDeviceWriteIOps = writeIops
			}
		}
	}

	// NanoCPUs.
	// This is only calculated if CpuPeriod == 100000.
	// It is given in nanoseconds, versus the microseconds used elsewhere -
	// so multiply by 10000 (not sure why, but 1000 is off by 10).
	if hostConfig.CpuPeriod == 100000 {
		hostConfig.NanoCpus = 10000 * hostConfig.CpuQuota
	}

	// Bind mounts, formatted as src:dst.
	// We'll be appending some options that aren't necessarily in the
	// original command line... but no helping that from inside libpod.
	binds := []string{}
	tmpfs := make(map[string]string)
	for _, namedVol := range namedVolumes {
		if len(namedVol.Options) > 0 {
			binds = append(binds, fmt.Sprintf("%s:%s:%s", namedVol.Name, namedVol.Dest, strings.Join(namedVol.Options, ",")))
		} else {
			binds = append(binds, fmt.Sprintf("%s:%s", namedVol.Name, namedVol.Dest))
		}
	}
	for _, mount := range mounts {
		if mount.Type == "tmpfs" {
			tmpfs[mount.Destination] = strings.Join(mount.Options, ",")
		} else {
			// TODO - maybe we should parse for empty source/destination
			// here. Would be confusing if we print just a bare colon.
			if len(mount.Options) > 0 {
				binds = append(binds, fmt.Sprintf("%s:%s:%s", mount.Source, mount.Destination, strings.Join(mount.Options, ",")))
			} else {
				binds = append(binds, fmt.Sprintf("%s:%s", mount.Source, mount.Destination))
			}
		}
	}
	hostConfig.Binds = binds
	hostConfig.Tmpfs = tmpfs

	// Network mode parsing.
	networkMode := ""
	switch {
	case c.config.CreateNetNS:
		networkMode = "default"
	case c.config.NetNsCtr != "":
		networkMode = fmt.Sprintf("container:%s", c.config.NetNsCtr)
	default:
		// Find the spec's network namespace.
		// If there is none, it's host networking.
		// If there is one and it has a path, it's "ns:".
		foundNetNS := false
		for _, ns := range ctrSpec.Linux.Namespaces {
			if ns.Type == spec.NetworkNamespace {
				foundNetNS = true
				if ns.Path != "" {
					networkMode = fmt.Sprintf("ns:%s", ns.Path)
				} else {
					networkMode = "none"
				}
				break
			}
		}
		if !foundNetNS {
			networkMode = "host"
		}
	}
	hostConfig.NetworkMode = networkMode

	// Port bindings.
	// Only populate if we're using CNI to configure the network.
	portBindings := make(map[string][]define.InspectHostPort)
	if c.config.CreateNetNS {
		for _, port := range c.config.PortMappings {
			key := fmt.Sprintf("%d/%s", port.ContainerPort, port.Protocol)
			hostPorts := portBindings[key]
			if hostPorts == nil {
				hostPorts = []define.InspectHostPort{}
			}
			hostPorts = append(hostPorts, define.InspectHostPort{
				HostIP:   port.HostIP,
				HostPort: fmt.Sprintf("%d", port.HostPort),
			})
			portBindings[key] = hostPorts
		}
	}
	hostConfig.PortBindings = portBindings

	// Cap add and cap drop.
	// We need a default set of capabilities to compare against.
	// The OCI generate package has one, and is commonly used, so we'll
	// use it.
	// Problem: there are 5 sets of capabilities.
	// Use the bounding set for this computation, it's the most encompassing
	// (but still not perfect).
	capAdd := []string{}
	capDrop := []string{}
	// No point in continuing if we got a spec without a Process block...
	if ctrSpec.Process != nil {
		// Max an O(1) lookup table for default bounding caps.
		boundingCaps := make(map[string]bool)
		g, err := generate.New("linux")
		if err != nil {
			return nil, err
		}
		if !hostConfig.Privileged {
			for _, cap := range g.Config.Process.Capabilities.Bounding {
				boundingCaps[cap] = true
			}
		} else {
			// If we are privileged, use all caps.
			for _, cap := range capability.List() {
				if g.HostSpecific && cap > validate.LastCap() {
					continue
				}
				boundingCaps[fmt.Sprintf("CAP_%s", strings.ToUpper(cap.String()))] = true
			}
		}
		// Iterate through spec caps.
		// If it's not in default bounding caps, it was added.
		// If it is, delete from the default set. Whatever remains after
		// we finish are the dropped caps.
		for _, cap := range ctrSpec.Process.Capabilities.Bounding {
			if _, ok := boundingCaps[cap]; ok {
				delete(boundingCaps, cap)
			} else {
				capAdd = append(capAdd, cap)
			}
		}
		for cap := range boundingCaps {
			capDrop = append(capDrop, cap)
		}
	}
	hostConfig.CapAdd = capAdd
	hostConfig.CapDrop = capDrop

	// IPC Namespace mode
	ipcMode := ""
	if c.config.IPCNsCtr != "" {
		ipcMode = fmt.Sprintf("container:%s", c.config.IPCNsCtr)
	} else {
		// Locate the spec's IPC namespace.
		// If there is none, it's ipc=host.
		// If there is one and it has a path, it's "ns:".
		// If no path, it's default - the empty string.
		foundIPCNS := false
		for _, ns := range ctrSpec.Linux.Namespaces {
			if ns.Type == spec.IPCNamespace {
				foundIPCNS = true
				if ns.Path != "" {
					ipcMode = fmt.Sprintf("ns:%s", ns.Path)
				}
				break
			}
		}
		if !foundIPCNS {
			ipcMode = "host"
		}
	}
	hostConfig.IpcMode = ipcMode

	// CGroup parent
	// Need to check if it's the default, and not print if so.
	defaultCgroupParent := ""
	switch c.runtime.config.Engine.CgroupManager {
	case config.CgroupfsCgroupsManager:
		defaultCgroupParent = CgroupfsDefaultCgroupParent
	case config.SystemdCgroupsManager:
		defaultCgroupParent = SystemdDefaultCgroupParent
	}
	if c.config.CgroupParent != defaultCgroupParent {
		hostConfig.CgroupParent = c.config.CgroupParent
	}

	// PID namespace mode
	pidMode := ""
	if c.config.PIDNsCtr != "" {
		pidMode = fmt.Sprintf("container:%s", c.config.PIDNsCtr)
	} else {
		// Locate the spec's PID namespace.
		// If there is none, it's pid=host.
		// If there is one and it has a path, it's "ns:".
		// If there is no path, it's default - the empty string.
		foundPIDNS := false
		for _, ns := range ctrSpec.Linux.Namespaces {
			if ns.Type == spec.PIDNamespace {
				foundPIDNS = true
				if ns.Path != "" {
					pidMode = fmt.Sprintf("ns:%s", ns.Path)
				}
				break
			}
		}
		if !foundPIDNS {
			pidMode = "host"
		}
	}
	hostConfig.PidMode = pidMode

	// UTS namespace mode
	utsMode := ""
	if c.config.UTSNsCtr != "" {
		utsMode = fmt.Sprintf("container:%s", c.config.UTSNsCtr)
	} else {
		// Locate the spec's UTS namespace.
		// If there is none, it's uts=host.
		// If there is one and it has a path, it's "ns:".
		// If there is no path, it's default - the empty string.
		foundUTSNS := false
		for _, ns := range ctrSpec.Linux.Namespaces {
			if ns.Type == spec.UTSNamespace {
				foundUTSNS = true
				if ns.Path != "" {
					utsMode = fmt.Sprintf("ns:%s", ns.Path)
				}
				break
			}
		}
		if !foundUTSNS {
			utsMode = "host"
		}
	}
	hostConfig.UTSMode = utsMode

	// User namespace mode
	usernsMode := ""
	if c.config.UserNsCtr != "" {
		usernsMode = fmt.Sprintf("container:%s", c.config.UserNsCtr)
	} else {
		// Locate the spec's user namespace.
		// If there is none, it's default - the empty string.
		// If there is one, it's "private" if no path, or "ns:" if
		// there's a path.
		for _, ns := range ctrSpec.Linux.Namespaces {
			if ns.Type == spec.UserNamespace {
				if ns.Path != "" {
					usernsMode = fmt.Sprintf("ns:%s", ns.Path)
				} else {
					usernsMode = "private"
				}
			}
		}
	}
	hostConfig.UsernsMode = usernsMode

	// Devices
	// Do not include if privileged - assumed that all devices will be
	// included.
	hostConfig.Devices = []define.InspectDevice{}
	if ctrSpec.Linux != nil && !hostConfig.Privileged {
		for _, dev := range ctrSpec.Linux.Devices {
			key := fmt.Sprintf("%d:%d", dev.Major, dev.Minor)
			if deviceNodes == nil {
				nodes, err := util.FindDeviceNodes()
				if err != nil {
					return nil, err
				}
				deviceNodes = nodes
			}
			path, ok := deviceNodes[key]
			if !ok {
				logrus.Warnf("Could not locate device %s on host", key)
				continue
			}
			newDev := define.InspectDevice{}
			newDev.PathOnHost = path
			newDev.PathInContainer = dev.Path
			hostConfig.Devices = append(hostConfig.Devices, newDev)
		}
	}

	// Ulimits
	hostConfig.Ulimits = []define.InspectUlimit{}
	if ctrSpec.Process != nil {
		for _, limit := range ctrSpec.Process.Rlimits {
			newLimit := define.InspectUlimit{}
			newLimit.Name = limit.Type
			newLimit.Soft = limit.Soft
			newLimit.Hard = limit.Hard
			hostConfig.Ulimits = append(hostConfig.Ulimits, newLimit)
		}
	}

	// Terminal size
	// We can't actually get this for now...
	// So default to something sane.
	// TODO: Populate this.
	hostConfig.ConsoleSize = []uint{0, 0}

	return hostConfig, nil
}
