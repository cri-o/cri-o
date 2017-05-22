package server

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/docker/pkg/mount"
	"github.com/golang/glog"
	"golang.org/x/net/context"
	pb "k8s.io/kubernetes/pkg/kubelet/apis/cri/v1alpha1"
)

// ImageFsInfo returns information of the filesystem that is used to store images.
func (s *Server) ImageFsInfo(ctx context.Context, req *pb.ImageFsInfoRequest) (*pb.ImageFsInfoResponse, error) {
	logrus.Debugf("ImageFsInfoRequest: %+v", req)

	mountInfo, err := mount.GetMounts()
	if err != nil {
		return nil, err
	}
	parts := processMounts(mountInfo, nil)
	devPath, err := getDirFsDevice(parts, mountInfo, s.config.Root)
	if err != nil {
		return nil, err
	}

	part, ok := parts[devPath]
	if !ok {
		return nil, fmt.Errorf("couldn't look up %s in partitions", devPath)
	}
	var (
		total, avail       uint64
		inodes, inodesFree *uint64
	)
	switch part.fsType {
	case "devicemapper":
		total, _, avail, err = getDMStats(devPath, part.blockSize)
		logrus.Warnf("error getting DM stats from %s: %v", devPath, err)
	case "vfs":
		total, _, avail, inodes, inodesFree, err = getVfsStats(s.config.Root)
		logrus.Warnf("error getting stats from %s: %v", s.config.Root, err)
	default:
		logrus.Warnf("unknown or not supported fs type %s", part.fsType)
	}

	usedBytes, err := getDirDiskUsage(s.config.Root, time.Minute)
	if err != nil {
		return nil, err
	}
	inodesUsed, err := getDirInodeUsage(s.config.Root, time.Minute)
	if err != nil {
		return nil, err
	}
	resp := &pb.ImageFsInfoResponse{
		FsInfo: &pb.FsInfo{
			Device:         devPath,
			Path:           s.config.Root,
			CapacityBytes:  &pb.UInt64Value{Value: total},
			AvailableBytes: &pb.UInt64Value{Value: avail},
			UsedBytes:      &pb.UInt64Value{Value: usedBytes},
			InodesUsed:     &pb.UInt64Value{Value: inodesUsed},
		},
	}
	if inodes != nil {
		resp.FsInfo.InodesCapacity = &pb.UInt64Value{Value: *inodes}
	}
	if inodesFree != nil {
		resp.FsInfo.InodesAvailable = &pb.UInt64Value{Value: *inodesFree}
	}
	logrus.Debugf("ImageFsInfoResponse: %+v", resp)
	return resp, nil
}

//
// XXX: this works just for VFS! No devmapper nor ZFS is supported here
//
func getVfsStats(path string) (total uint64, free uint64, avail uint64, inodes *uint64, inodesFree *uint64, err error) {
	var s syscall.Statfs_t
	if err = syscall.Statfs(path, &s); err != nil {
		return 0, 0, 0, nil, nil, err
	}
	total = uint64(s.Frsize) * s.Blocks
	free = uint64(s.Frsize) * s.Bfree
	avail = uint64(s.Frsize) * s.Bavail
	inodesV := uint64(s.Files)
	inodesFreeV := uint64(s.Ffree)
	return total, free, avail, &inodesV, &inodesFreeV, nil
}

func getDirFsDevice(parts map[string]partition, mountInfo []*mount.Info, dir string) (string, error) {
	buf := new(syscall.Stat_t)
	err := syscall.Stat(dir, buf)
	if err != nil {
		return "", fmt.Errorf("stat failed on %s with error: %s", dir, err)
	}
	major := major(buf.Rdev)
	minor := minor(buf.Rdev)
	for device, partition := range parts {
		if partition.major == major && partition.minor == minor {
			return device, nil
		}
	}
	return "", fmt.Errorf("could not find device with major: %d, minor: %d in cached partitions map", major, minor)
}

type partition struct {
	mountpoint string
	major      uint
	minor      uint
	fsType     string
	blockSize  uint
}

func processMounts(mounts []*mount.Info, excludedMountpointPrefixes []string) map[string]partition {
	partitions := make(map[string]partition, 0)

	supportedFsType := map[string]bool{
		// all ext systems are checked through prefix.
		"btrfs": true,
		"xfs":   true,
		"zfs":   true,
	}

	for _, mount := range mounts {
		if !strings.HasPrefix(mount.Fstype, "ext") && !supportedFsType[mount.Fstype] {
			continue
		}
		// Avoid bind mounts.
		if _, ok := partitions[mount.Source]; ok {
			continue
		}

		hasPrefix := false
		for _, prefix := range excludedMountpointPrefixes {
			if strings.HasPrefix(mount.Mountpoint, prefix) {
				hasPrefix = true
				break
			}
		}
		if hasPrefix {
			continue
		}

		// btrfs fix: following workaround fixes wrong btrfs Major and Minor Ids reported in /proc/self/mountinfo.
		// instead of using values from /proc/self/mountinfo we use stat to get Ids from btrfs mount point
		if mount.Fstype == "btrfs" && mount.Major == 0 && strings.HasPrefix(mount.Source, "/dev/") {

			buf := new(syscall.Stat_t)
			err := syscall.Stat(mount.Source, buf)
			if err != nil {
				logrus.Warningf("stat failed on %s with error: %s", mount.Source, err)
			} else {
				logrus.Infof("btrfs mount %#v", mount)
				if buf.Mode&syscall.S_IFMT == syscall.S_IFBLK {
					err := syscall.Stat(mount.Mountpoint, buf)
					if err != nil {
						logrus.Warningf("stat failed on %s with error: %s", mount.Mountpoint, err)
					} else {
						logrus.Infof("btrfs dev major:minor %d:%d\n", int(major(buf.Dev)), int(minor(buf.Dev)))
						logrus.Infof("btrfs rdev major:minor %d:%d\n", int(major(buf.Rdev)), int(minor(buf.Rdev)))

						mount.Major = int(major(buf.Dev))
						mount.Minor = int(minor(buf.Dev))
					}
				}
			}
		}

		partitions[mount.Source] = partition{
			fsType:     mount.Fstype,
			mountpoint: mount.Mountpoint,
			major:      uint(mount.Major),
			minor:      uint(mount.Minor),
		}
	}

	return partitions
}

func major(devNumber uint64) uint {
	return uint((devNumber >> 8) & 0xfff)
}

func minor(devNumber uint64) uint {
	return uint((devNumber & 0xff) | ((devNumber >> 12) & 0xfff00))
}

// Simple io.Writer implementation that counts how many bytes were written.
type byteCounter struct{ bytesWritten uint64 }

func (b *byteCounter) Write(p []byte) (int, error) {
	b.bytesWritten += uint64(len(p))
	return len(p), nil
}

// The maximum number of `du` and `find` tasks that can be running at once.
const maxConcurrentOps = 20

// A pool for restricting the number of consecutive `du` and `find` tasks running.
var pool = make(chan struct{}, maxConcurrentOps)

func init() {
	for i := 0; i < maxConcurrentOps; i++ {
		releaseToken()
	}
}

func claimToken() {
	<-pool
}

func releaseToken() {
	pool <- struct{}{}
}

func getDirDiskUsage(dir string, timeout time.Duration) (uint64, error) {
	if dir == "" {
		return 0, fmt.Errorf("invalid directory")
	}
	claimToken()
	defer releaseToken()
	cmd := exec.Command("nice", "-n", "19", "du", "-s", dir)
	stdoutp, err := cmd.StdoutPipe()
	if err != nil {
		return 0, fmt.Errorf("failed to setup stdout for cmd %v - %v", cmd.Args, err)
	}
	stderrp, err := cmd.StderrPipe()
	if err != nil {
		return 0, fmt.Errorf("failed to setup stderr for cmd %v - %v", cmd.Args, err)
	}

	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("failed to exec du - %v", err)
	}
	timer := time.AfterFunc(timeout, func() {
		glog.Infof("killing cmd %v due to timeout(%s)", cmd.Args, timeout.String())
		cmd.Process.Kill()
	})
	stdoutb, souterr := ioutil.ReadAll(stdoutp)
	if souterr != nil {
		glog.Errorf("failed to read from stdout for cmd %v - %v", cmd.Args, souterr)
	}
	stderrb, _ := ioutil.ReadAll(stderrp)
	err = cmd.Wait()
	timer.Stop()
	if err != nil {
		return 0, fmt.Errorf("du command failed on %s with output stdout: %s, stderr: %s - %v", dir, string(stdoutb), string(stderrb), err)
	}
	stdout := string(stdoutb)
	usageInKb, err := strconv.ParseUint(strings.Fields(stdout)[0], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("cannot parse 'du' output %s - %s", stdout, err)
	}
	return usageInKb * 1024, nil
}

func getDirInodeUsage(dir string, timeout time.Duration) (uint64, error) {
	if dir == "" {
		return 0, fmt.Errorf("invalid directory")
	}
	var counter byteCounter
	var stderr bytes.Buffer
	claimToken()
	defer releaseToken()
	findCmd := exec.Command("find", dir, "-xdev", "-printf", ".")
	findCmd.Stdout, findCmd.Stderr = &counter, &stderr
	if err := findCmd.Start(); err != nil {
		return 0, fmt.Errorf("failed to exec cmd %v - %v; stderr: %v", findCmd.Args, err, stderr.String())
	}
	timer := time.AfterFunc(timeout, func() {
		glog.Infof("killing cmd %v due to timeout(%s)", findCmd.Args, timeout.String())
		findCmd.Process.Kill()
	})
	if err := findCmd.Wait(); err != nil {
		return 0, fmt.Errorf("cmd %v failed. stderr: %s; err: %v", findCmd.Args, stderr.String(), err)
	}
	timer.Stop()
	return counter.bytesWritten, nil
}

func getDMStats(poolName string, dataBlkSize uint) (uint64, uint64, uint64, error) {
	out, err := exec.Command("dmsetup", "status", poolName).Output()
	if err != nil {
		return 0, 0, 0, err
	}

	used, total, err := parseDMStatus(string(out))
	if err != nil {
		return 0, 0, 0, err
	}

	used *= 512 * uint64(dataBlkSize)
	total *= 512 * uint64(dataBlkSize)
	free := total - used

	return total, free, free, nil
}

func parseDMStatus(dmStatus string) (uint64, uint64, error) {
	dmStatus = strings.Replace(dmStatus, "/", " ", -1)
	dmFields := strings.Fields(dmStatus)

	if len(dmFields) < 8 {
		return 0, 0, fmt.Errorf("Invalid dmsetup status output: %s", dmStatus)
	}

	used, err := strconv.ParseUint(dmFields[6], 10, 64)
	if err != nil {
		return 0, 0, err
	}
	total, err := strconv.ParseUint(dmFields[7], 10, 64)
	if err != nil {
		return 0, 0, err
	}

	return used, total, nil
}
