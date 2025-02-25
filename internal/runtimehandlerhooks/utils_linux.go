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

func mapHexCharToByte(h string) ([]byte, error) {
	var hexin string
	// remove ","; now each element is "0-9,a-f"
	s := strings.ReplaceAll(h, ",", "")

	l := len(s)

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
// to a cpuset.CPUSet representation.
func mapByteToCPUSet(b []byte) cpuset.CPUSet {
	result := cpuset.New()

	for i, chunk := range b {
		start := 8 * i // (len(b) - 1 - i) // First cpu in the chunk

		for bit := range 8 {
			if chunk&(1<<bit) != 0 { // Check if this bit is set
				result = result.Union(cpuset.New(start + bit))
			}
		}
	}

	return result
}

func mapHexCharToCPUSet(s string) (cpuset.CPUSet, error) {
	toByte, err := mapHexCharToByte(s)
	if err != nil {
		return cpuset.New(), err
	}

	return mapByteToCPUSet(toByte), nil
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
func UpdateIRQSmpAffinityMask(cpus, current string, size int, set bool) (string, cpuset.CPUSet, error) {
	var updatedSMPAffinityCPUs cpuset.CPUSet

	podCPUSet, err := cpuset.Parse(cpus)
	if err != nil {
		return cpus, cpuset.New(), err
	}

	// only ascii string supported
	if !isASCII(current) {
		return cpus, cpuset.New(), fmt.Errorf("non ascii character detected: %s", current)
	}

	currentCPUs, err := mapHexCharToCPUSet(current)
	if err != nil {
		return cpus, cpuset.New(), err
	}

	allCPUs, err := cpuset.Parse(fmt.Sprintf("0-%d", size-1))
	if err != nil {
		return cpus, cpuset.New(), err
	}

	if set {
		updatedSMPAffinityCPUs = currentCPUs.Union(podCPUSet)
	} else {
		updatedSMPAffinityCPUs = currentCPUs.Difference(podCPUSet)
	}

	return toMask(size, updatedSMPAffinityCPUs), allCPUs.Difference(updatedSMPAffinityCPUs), nil
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
		if strings.HasPrefix(line, irqBalanceBannedCpusLegacy+"=") {
			lines[i] = "#" + line
		}

		if strings.HasPrefix(line, irqBalanceBannedCPUs+"=") {
			lines[i] = irqBalanceBannedCPUs + "=" + "\"" + newIRQBalanceSetting.String() + "\""
			found = true
		}
	}

	output := strings.Join(lines, "\n")
	if !found {
		output = output + "\n" + irqBalanceBannedCPUs + "=" + "\"" + newIRQBalanceSetting.String() + "\"" + "\n"
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

	setFromOldValue := cpuset.New()
	setFromNewValue := cpuset.New()

	lines := strings.Split(string(input), "\n")
	for _, line := range lines {
		// try to read from the legacy variable first and merge both values.
		// after the first iteration, it'll comment this line, so
		// it will not enter this flow again (because the prefix won't match).
		if strings.HasPrefix(line, irqBalanceBannedCpusLegacy+"=") {
			list := strings.Trim(strings.Split(line, "=")[1], "\"")

			setFromOldValue, err = mapHexCharToCPUSet(list)
			if err != nil {
				return cpuset.New(), err
			}
		}

		if strings.HasPrefix(line, irqBalanceBannedCPUs+"=") {
			list := strings.Trim(strings.Split(line, "=")[1], "\"")

			setFromNewValue, err = cpuset.Parse(list)
			if err != nil {
				return cpuset.New(), err
			}
		}
	}

	return setFromOldValue.Union(setFromNewValue), nil
}

func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}

	return !info.IsDir()
}

// calculateCPUSizeFromMask return the number of total cpus
// given a CPU mask
// this is useful for finding the complement of a mask.
func calculateCPUSizeFromMask(mask string) int {
	noCommaMask := strings.ReplaceAll(mask, ",", "")
	maskLen := len(noCommaMask)
	// if mask is odd, round up
	if maskLen%2 == 1 {
		maskLen++
	}
	// each hexadecimal char represents 4 CPUs
	return maskLen * 4
}

// toMask convert cpuset which is represented as a CPU list format
// into mask format
// the size is number of bits that will be presented in the mask.
func toMask(size int, set cpuset.CPUSet) string {
	arraySize := size / 8
	if size%8 != 0 {
		arraySize += 1
	}

	byteArray := make([]byte, arraySize)

	for i := range size {
		if set.Contains(i) {
			byteArray[i/8] |= 1 << (i % 8)
		}
	}

	maskString := mapByteToHexChar(byteArray)

	maskStringWithComma := maskString[0:8]

	for i := 8; i+8 <= len(maskString); i += 8 {
		maskStringWithComma = maskStringWithComma + "," + maskString[i:i+8]
	}

	return maskStringWithComma
}
