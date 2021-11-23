package v1

import (
	"github.com/cri-o/cri-o/server/cri/types"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func serverPodSandboxStatToCRI(from *types.PodSandboxStats) *pb.PodSandboxStats {
	to := &pb.PodSandboxStats{
		Linux: &pb.LinuxPodSandboxStats{},
	}

	if from.Attributes != nil {
		to.Attributes = &pb.PodSandboxAttributes{
			Id:          from.Attributes.ID,
			Labels:      from.Attributes.Labels,
			Annotations: from.Attributes.Annotations,
		}
		if from.Attributes.Metadata != nil {
			to.Attributes.Metadata = &pb.PodSandboxMetadata{
				Name:    from.Attributes.Metadata.Name,
				Attempt: from.Attributes.Metadata.Attempt,
			}
		}
	}
	to.Linux.Cpu = serverCPUStatToCRI(from.CPU)
	to.Linux.Memory = serverMemoryStatToCRI(from.Memory)
	to.Linux.Network = serverNetworkStatToCRI(from.Network)
	to.Linux.Process = serverProcessStatToCRI(from.Process)
	if from.Containers != nil {
		to.Linux.Containers = make([]*pb.ContainerStats, 0, len(from.Containers))
		for _, stat := range from.Containers {
			to.Linux.Containers = append(to.Linux.Containers, serverContainerStatToCRI(stat))
		}
	}
	return to
}

func serverContainerStatToCRI(from *types.ContainerStats) *pb.ContainerStats {
	to := &pb.ContainerStats{}

	if from.Attributes != nil {
		to.Attributes = &pb.ContainerAttributes{
			Id:          from.Attributes.ID,
			Labels:      from.Attributes.Labels,
			Annotations: from.Attributes.Annotations,
		}
		if from.Attributes.Metadata != nil {
			to.Attributes.Metadata = &pb.ContainerMetadata{
				Name:    from.Attributes.Metadata.Name,
				Attempt: from.Attributes.Metadata.Attempt,
			}
		}
	}
	to.Cpu = serverCPUStatToCRI(from.CPU)
	to.Memory = serverMemoryStatToCRI(from.Memory)
	to.WritableLayer = serverFilesystemStatToCRI(from.WritableLayer)
	return to
}

func serverCPUStatToCRI(from *types.CPUUsage) *pb.CpuUsage {
	if from == nil {
		return nil
	}
	to := &pb.CpuUsage{
		Timestamp: from.Timestamp,
	}
	if from.UsageCoreNanoSeconds != nil {
		to.UsageCoreNanoSeconds = &pb.UInt64Value{
			Value: from.UsageCoreNanoSeconds.Value,
		}
	}
	return to
}

func serverMemoryStatToCRI(from *types.MemoryUsage) *pb.MemoryUsage {
	if from == nil {
		return nil
	}
	to := &pb.MemoryUsage{
		Timestamp: from.Timestamp,
	}
	if from.WorkingSetBytes != nil {
		to.WorkingSetBytes = &pb.UInt64Value{
			Value: from.WorkingSetBytes.Value,
		}
	}
	return to
}

func serverFilesystemStatToCRI(from *types.FilesystemUsage) *pb.FilesystemUsage {
	if from == nil {
		return nil
	}
	to := &pb.FilesystemUsage{
		Timestamp: from.Timestamp,
	}
	if from.FsID != nil {
		to.FsId = &pb.FilesystemIdentifier{
			Mountpoint: from.FsID.Mountpoint,
		}
	}
	if from.UsedBytes != nil {
		to.UsedBytes = &pb.UInt64Value{
			Value: from.UsedBytes.Value,
		}
	}
	if from.InodesUsed != nil {
		to.InodesUsed = &pb.UInt64Value{
			Value: from.InodesUsed.Value,
		}
	}
	return to
}

func serverNetworkStatToCRI(from *types.NetworkUsage) *pb.NetworkUsage {
	if from == nil {
		return nil
	}
	to := &pb.NetworkUsage{
		Timestamp: from.Timestamp,
	}
	to.DefaultInterface = serverInterfaceStatToCRI(from.DefaultInterface)
	if from.Interfaces != nil {
		to.Interfaces = make([]*pb.NetworkInterfaceUsage, 0, len(from.Interfaces))
		for _, iface := range from.Interfaces {
			to.Interfaces = append(to.Interfaces, serverInterfaceStatToCRI(iface))
		}
	}
	return to
}

func serverInterfaceStatToCRI(from *types.NetworkInterfaceUsage) *pb.NetworkInterfaceUsage {
	if from == nil {
		return nil
	}
	to := &pb.NetworkInterfaceUsage{
		Name: from.Name,
	}
	if from.RxBytes != nil {
		to.RxBytes = &pb.UInt64Value{
			Value: from.RxBytes.Value,
		}
	}
	if from.RxErrors != nil {
		to.RxErrors = &pb.UInt64Value{
			Value: from.RxErrors.Value,
		}
	}
	if from.TxBytes != nil {
		to.TxBytes = &pb.UInt64Value{
			Value: from.TxBytes.Value,
		}
	}
	if from.TxErrors != nil {
		to.TxErrors = &pb.UInt64Value{
			Value: from.TxErrors.Value,
		}
	}
	return to
}

func serverProcessStatToCRI(from *types.ProcessUsage) *pb.ProcessUsage {
	if from == nil {
		return nil
	}
	to := &pb.ProcessUsage{
		Timestamp: from.Timestamp,
	}
	if from.ProcessCount != nil {
		to.ProcessCount = &pb.UInt64Value{
			Value: from.ProcessCount.Value,
		}
	}
	return to
}
