package runtimehandlerhooks

import (
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"unicode"

	"github.com/sirupsen/logrus"
	"k8s.io/utils/cpuset"

	"github.com/cri-o/cri-o/utils/cmdrunner"
)

func isASCII(s string) bool {
	for i := range len(s) {
		if s[i] > unicode.MaxASCII {
			return false
		}
	}
	return true
}

func cpuMaskByte(c int) byte {
	return byte(1 << c)
}

func mapHexCharToByte(h string) ([]byte, error) {
	// remove ","; now each element is "0-9,a-f"
	s := strings.ReplaceAll(h, ",", "")

	l := len(s)
	var hexin string
	if l%2 != 0 {
		// expect even number of chars
		hexin = "0" + s
	} else {
		hexin = s
	}

	breversed, err := hex.DecodeString(hexin)
	if err != nil {
		return nil, err
	}

	l = len(breversed)
	barray := make([]byte, 0, l)
	var rindex int
	for i := range l {
		rindex = l - i - 1
		barray = append(barray, breversed[rindex])
	}
	return barray, nil
}

func mapByteToHexChar(b []byte) string {
	// The kernel will not accept longer bit mask than the count of cpus
	// on the system rounded up to the closest 32 bit multiple.
	// See https://bugzilla.redhat.com/show_bug.cgi?id=2181546

	var rindex int
	l := len(b)
	breversed := make([]byte, 0, l)
	// align it to 4 byte
	if l%4 != 0 {
		lfill := 4 - l%4
		l += lfill
		for range lfill {
			b = append(b, byte(0))
		}
	}

	for i := range l {
		rindex = l - i - 1
		breversed = append(breversed, b[rindex])
	}
	return hex.EncodeToString(breversed)
}

// Convert byte encoded cpu mask (converted from hex, no commas)
// to a cpuset.CPUSet representation
func mapByteToCpuSet(b []byte) cpuset.CPUSet {
	result := cpuset.New()

	for i, chunk := range b {
		start := 8 * i // (len(b) - 1 - i) // First cpu in the chunk
		for cpu := range 8 {
			if chunk&0x1 == 1 {
				result = result.Union(cpuset.New(cpu + start))
			}
		}
	}
	return result
}

// take a byte array and invert each byte.
func invertByteArray(in []byte) (out []byte) {
	for _, b := range in {
		out = append(out, byte(0xff)-b)
	}
	return
}

// take a byte array and returns true when bits of every byte element
// set to 1, otherwise returns false.
func isAllBitSet(in []byte) bool {
	for _, b := range in {
		if b&(b+1) != 0 {
			return false
		}
	}
	return true
}

// UpdateIRQSmpAffinityMask take input cpus that need to change irq affinity mask and
// the current mask string, return an update mask string and inverted mask, with those cpus
// enabled or disable in the mask.
func UpdateIRQSmpAffinityMask(cpus, current string, set bool) (cpuMask string, banned cpuset.CPUSet, err error) {
	podcpuset, err := cpuset.Parse(cpus)
	if err != nil {
		return cpus, cpuset.New(), err
	}

	// only ascii string supported
	if !isASCII(current) {
		return cpus, cpuset.New(), fmt.Errorf("non ascii character detected: %s", current)
	}

	// the index 0 corresponds to the cpu 0-7
	// the LSb (right-most bit) represents the lowest cpu id from the byte
	// and the MSb (left-most bit) represents the highest cpu id from the byte
	currentMaskArray, err := mapHexCharToByte(current)
	if err != nil {
		return cpus, cpuset.New(), err
	}
	invertedMaskArray := invertByteArray(currentMaskArray)

	for _, cpu := range podcpuset.List() {
		if set {
			// each byte represent 8 cpus
			currentMaskArray[cpu/8] |= cpuMaskByte(cpu % 8)
			invertedMaskArray[cpu/8] &^= cpuMaskByte(cpu % 8)
		} else {
			currentMaskArray[cpu/8] &^= cpuMaskByte(cpu % 8)
			invertedMaskArray[cpu/8] |= cpuMaskByte(cpu % 8)
		}
	}

	maskString := mapByteToHexChar(currentMaskArray)
	bannedCpuSet := mapByteToCpuSet(invertedMaskArray)

	maskStringWithComma := maskString[0:8]
	for i := 8; i+8 <= len(maskString); i += 8 {
		maskStringWithComma = maskStringWithComma + "," + maskString[i:i+8]
	}
	return maskStringWithComma, bannedCpuSet, nil
}

func restartIrqBalanceService() error {
	return cmdrunner.Command("systemctl", "restart", "irqbalance").Run()
}

func isServiceEnabled(serviceName string) bool {
	cmd := cmdrunner.Command("systemctl", "is-enabled", serviceName)
	status, err := cmd.CombinedOutput()
	if err != nil {
		logrus.Infof("Service %s is-enabled check returned with: %v", serviceName, err)
		return false
	}
	if strings.TrimSpace(string(status)) == "enabled" {
		return true
	}
	return false
}

func updateIrqBalanceConfigFile(irqBalanceConfigFile string, newIRQBalanceSetting cpuset.CPUSet) error {
	input, err := os.ReadFile(irqBalanceConfigFile)
	if err != nil {
		return err
	}
	lines := strings.Split(string(input), "\n")
	found := false
	for i, line := range lines {
		// Comment out the old deprecated variable
		if strings.HasPrefix(line, irqBalanceBannedCpus+"=") {
			lines[i] = "#" + line
		}

		if strings.HasPrefix(line, irqBalanceBannedCpuList+"=") {
			lines[i] = irqBalanceBannedCpuList + "=" + "\"" + newIRQBalanceSetting.String() + "\""
			found = true
		}
	}
	output := strings.Join(lines, "\n")
	if !found {
		output = output + "\n" + irqBalanceBannedCpuList + "=" + "\"" + newIRQBalanceSetting.String() + "\"" + "\n"
	}
	if err := os.WriteFile(irqBalanceConfigFile, []byte(output), 0o644); err != nil {
		return err
	}
	return nil
}

func retrieveIrqBannedCPUList(irqBalanceConfigFile string) (cpuset.CPUSet, error) {
	input, err := os.ReadFile(irqBalanceConfigFile)
	if err != nil {
		return cpuset.New(), err
	}
	lines := strings.Split(string(input), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, irqBalanceBannedCpuList+"=") {
			list := strings.Trim(strings.Split(line, "=")[1], "\"")
			return cpuset.Parse(list)
		}
	}
	return cpuset.New(), nil
}

func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}
