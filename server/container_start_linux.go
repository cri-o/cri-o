package server

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/cri-o/cri-o/internal/oci"
	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"
)

const (
	annotationCPUloadBalancing    = "cpu-load-balancing.crio.io"
	runtimeHandlerHighPerformance = "high-performance"
	schedDomainDir                = "/proc/sys/kernel/sched_domain"
)

func shouldCPULoadBalancingBeDisabled(annotations fields.Set) bool {
	value, ok := annotations[annotationCPUloadBalancing]
	if !ok {
		return false
	}

	return value == "true"
}

func setCPUSLoadBalancing(c *oci.Container, enable bool, schedDomainDir string) error {
	if c.Spec().Linux == nil ||
		c.Spec().Linux.Resources == nil ||
		c.Spec().Linux.Resources.CPU == nil ||
		c.Spec().Linux.Resources.CPU.Cpus == "" {
		return fmt.Errorf("failed to find the container %q CPUs", c.ID())
	}

	cpus, err := cpuset.Parse(c.Spec().Linux.Resources.CPU.Cpus)
	if err != nil {
		return err
	}

	for _, cpu := range cpus.ToSlice() {
		cpuSchedDomainDir := fmt.Sprintf("%s/cpu%d", schedDomainDir, cpu)
		err := filepath.Walk(cpuSchedDomainDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				logrus.Errorf("path %q does not exist: %v", path, err)
				return err
			}

			if path == cpuSchedDomainDir {
				return nil
			}

			if !strings.Contains(path, "flags") {
				return nil
			}

			content, err := ioutil.ReadFile(path)
			if err != nil {
				return err
			}

			flags, err := strconv.Atoi(strings.Trim(string(content), "\n"))
			if err != nil {
				return err
			}

			var newContent string
			if enable {
				newContent = strconv.Itoa(flags | 1)
			} else {
				// we should set the LSB to 0 to disable the load balancing for the specified CPU
				// in case of sched domain all flags can be represented by the binary number 111111111111111 that equals
				// to 32767 in the decimal form
				// see https://github.com/torvalds/linux/blob/0fe5f9ca223573167c4c4156903d751d2c8e160e/include/linux/sched/topology.h#L14
				// for more information regarding the sched domain flags
				newContent = strconv.Itoa(flags & 32766)
			}

			err = ioutil.WriteFile(path, []byte(newContent), 0644)
			return err
		})

		if err != nil {
			return err
		}
	}

	return nil
}
