package runtimehandlerhooks

import (
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"unicode"

	"github.com/cri-o/cri-o/utils/cmdrunner"
	"github.com/sirupsen/logrus"
	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"
)

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
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
	l := len(h)
	var hexin string
	if l%2 != 0 {
		// expect even number of chars
		hexin = "0" + h
	} else {
		hexin = h
	}

	breversed, err := hex.DecodeString(hexin)
	if err != nil {
		return nil, err
	}

	l = len(breversed)
	var barray []byte
	var rindex int
	for i := 0; i < l; i++ {
		rindex = l - i - 1
		barray = append(barray, breversed[rindex])
	}
	return barray, nil
}

func mapByteToHexChar(b []byte) string {
	var breversed []byte
	var rindex int
	l := len(b)
	// align it to 8 byte
	if l%8 != 0 {
		lfill := 8 - l%8
		l += lfill
		for i := 0; i < lfill; i++ {
			b = append(b, byte(0))
		}
	}

	for i := 0; i < l; i++ {
		rindex = l - i - 1
		breversed = append(breversed, b[rindex])
	}
	return hex.EncodeToString(breversed)
}

// take a byte array and invert each byte
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
func UpdateIRQSmpAffinityMask(cpus, current string, set bool) (cpuMask, bannedCPUMask string, err error) {
	podcpuset, err := cpuset.Parse(cpus)
	if err != nil {
		return cpus, "", err
	}

	// only ascii string supported
	if !isASCII(current) {
		return cpus, "", fmt.Errorf("non ascii character detected: %s", current)
	}

	// remove ","; now each element is "0-9,a-f"
	s := strings.ReplaceAll(current, ",", "")

	// the index 0 corresponds to the cpu 0-7
	currentMaskArray, err := mapHexCharToByte(s)
	if err != nil {
		return cpus, "", err
	}
	invertedMaskArray := invertByteArray(currentMaskArray)

	for _, cpu := range podcpuset.ToSlice() {
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
	invertedMaskString := mapByteToHexChar(invertedMaskArray)

	maskStringWithComma := maskString[0:8]
	invertedMaskStringWithComma := invertedMaskString[0:8]
	for i := 8; i+8 <= len(maskString); i += 8 {
		maskStringWithComma = maskStringWithComma + "," + maskString[i:i+8]
		invertedMaskStringWithComma = invertedMaskStringWithComma + "," + invertedMaskString[i:i+8]
	}
	return maskStringWithComma, invertedMaskStringWithComma, nil
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

func updateIrqBalanceConfigFile(irqBalanceConfigFile, newIRQBalanceSetting string) error {
	input, err := ioutil.ReadFile(irqBalanceConfigFile)
	if err != nil {
		return err
	}
	lines := strings.Split(string(input), "\n")
	found := false
	for i, line := range lines {
		if strings.HasPrefix(line, irqBalanceBannedCpus+"=") {
			lines[i] = irqBalanceBannedCpus + "=" + "\"" + newIRQBalanceSetting + "\"" + "\n"
			found = true
		}
	}
	output := strings.Join(lines, "\n")
	if !found {
		output = output + "\n" + irqBalanceBannedCpus + "=" + "\"" + newIRQBalanceSetting + "\"" + "\n"
	}
	if err := ioutil.WriteFile(irqBalanceConfigFile, []byte(output), 0644); err != nil {
		return err
	}
	return nil
}

func retrieveIrqBannedCPUMasks(irqBalanceConfigFile string) (string, error) {
	input, err := ioutil.ReadFile(irqBalanceConfigFile)
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(input), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, irqBalanceBannedCpus+"=") {
			return strings.Trim(strings.Split(line, "=")[1], "\""), nil
		}
	}
	return "", nil
}

func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}
