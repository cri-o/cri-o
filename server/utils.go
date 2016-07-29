package server

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/mrunalp/ocid/utils"
	"github.com/opencontainers/ocitools/generate"
)

func getGPRCVersion() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("Failed to recover the caller information.")
	}

	ocidRoot := filepath.Dir(filepath.Dir(file))
	p := filepath.Join(ocidRoot, "Godeps/Godeps.json")

	grepCmd := fmt.Sprintf(`grep -r "\"google.golang.org/grpc\"" %s -A 1 | grep "\"Rev\"" | cut -d: -f2 | tr -d ' "\n'`, p)

	out, err := utils.ExecCmd("bash", "-c", grepCmd)
	if err != nil {
		return "", err
	}
	return out, nil
}

func copyFile(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return nil
}

func removeFile(path string) error {
	if _, err := os.Stat(path); err == nil {
		if err := os.Remove(path); err != nil {
			return err
		}
	}
	return nil
}

func parseDNSOptions(servers, searches []string, path string) error {
	nServers := len(servers)
	nSearches := len(searches)
	if nServers == 0 && nSearches == 0 {
		return copyFile("/etc/resolv.conf", path)
	}

	if nSearches > maxDNSSearches {
		return fmt.Errorf("DNSOption.Searches has more than 6 domains")
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	if nSearches > 0 {
		data := fmt.Sprintf("search %s\n", strings.Join(searches, " "))
		_, err = f.Write([]byte(data))
		if err != nil {
			return err
		}
	}

	if nServers > 0 {
		data := fmt.Sprintf("nameserver %s\n", strings.Join(servers, "\nnameserver "))
		_, err = f.Write([]byte(data))
		if err != nil {
			return err
		}
	}

	return nil
}

// kubernetes compute resources - CPU: http://kubernetes.io/docs/user-guide/compute-resources/#meaning-of-cpu
func setResourcesCPU(limits, requests, defaultCores float64, g generate.Generator) error {
	if requests > limits {
		return fmt.Errorf("CPU.Requests should not be greater than CPU.Limits")
	}

	cores := defaultCores
	if limits != 0 || requests != 0 {
		if limits > requests {
			cores = limits
		} else {
			cores = requests
		}
	}

	period := uint64(defaultCPUCFSPeriod)
	quota := uint64(float64(period) * cores)

	if quota < minCPUCFSQuota {
		quota = minCPUCFSQuota
	}

	// adjust quota and period for the case where multiple CPUs are requested
	// so that cpu.cfs_quota_us <= maxCPUCFSQuota.
	for quota > maxCPUCFSQuota {
		quota /= 10
		period /= 10
	}

	g.SetLinuxResourcesCPUPeriod(period)
	g.SetLinuxResourcesCPUQuota(quota)
	return nil
}

// kubernetes compute resources - Memory: http://kubernetes.io/docs/user-guide/compute-resources/#meaning-of-memory
func setResourcesMemory(limits, requests, defaultMem float64, g generate.Generator) error {
	if requests > limits {
		return fmt.Errorf("Memory.Requests should not be greater than Memory.Limits")
	}

	if limits != 0 {
		if requests == 0 {
			requests = limits
		}
	} else {
		if requests == 0 {
			// set the default values of limits and requests
			requests = defaultMem
			limits = defaultMem
		} else {
			limits = requests
		}
	}

	g.SetLinuxResourcesMemoryLimit(uint64(limits))
	g.SetLinuxResourcesMemoryReservation(uint64(requests))
	return nil
}
