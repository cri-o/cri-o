//go:build freebsd
// +build freebsd

package ocicni

import (
	"fmt"
	"net"
	"os/exec"
	"strings"
)

type nsManager struct{}

func (nsm *nsManager) init() error {
	return nil
}

func getContainerDetails(nsm *nsManager, netnsJailName, interfaceName, addrType string) (*net.IPNet, *net.HardwareAddr, error) {
	// Try to retrieve ip inside container network namespace
	if addrType == "-4" {
		addrType = "inet"
	} else {
		addrType = "inet6"
	}
	output, err := exec.Command(
		"ifconfig", "-j", netnsJailName,
		"-f", "inet:cidr,inet6:cidr",
		interfaceName,
		addrType).CombinedOutput()
	if err != nil {
		return nil, nil, fmt.Errorf("Unexpected command output %s with error: %v", output, err)
	}

	lines := strings.Split(string(output), "\n")
	if len(lines) < 3 {
		return nil, nil, fmt.Errorf("Unexpected command output %s", output)
	}
	fields := strings.Fields(strings.TrimSpace(lines[2]))
	if len(fields) < 2 {
		return nil, nil, fmt.Errorf("Unexpected address output %s ", lines[0])
	}
	ip, ipNet, err := net.ParseCIDR(fields[1])
	if err != nil {
		return nil, nil, fmt.Errorf("CNI failed to parse ip from output %s due to %v", output, err)
	}
	if ip.To4() == nil {
		ipNet.IP = ip
	} else {
		ipNet.IP = ip.To4()
	}

	// Try to retrieve MAC inside container network namespace
	output, err = exec.Command(
		"ifconfig", "-j", netnsJailName, "-f", "inet:cidr,inet6:cidr",
		interfaceName,
		"ether").CombinedOutput()
	if err != nil {
		return nil, nil, fmt.Errorf("unexpected ifconfig command output %s with error: %v", output, err)
	}

	lines = strings.Split(string(output), "\n")
	if len(lines) < 3 {
		return nil, nil, fmt.Errorf("unexpected ifconfig command output %s", output)
	}
	fields = strings.Fields(strings.TrimSpace(lines[2]))
	if len(fields) < 2 {
		return nil, nil, fmt.Errorf("unexpected ether output %s ", lines[0])
	}
	mac, err := net.ParseMAC(fields[1])
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse MAC from output %s due to %v", output, err)
	}

	return ipNet, &mac, nil
}

func bringUpLoopback(netns string) error {
	if err := exec.Command("ifconfig", "-j", netns, "lo0", "inet", "127.0.0.1").Run(); err != nil {
		return fmt.Errorf("failed to initialize loopback: %w", err)
	}
	return nil
}

func checkLoopback(netns string) error {
	return nil
}
