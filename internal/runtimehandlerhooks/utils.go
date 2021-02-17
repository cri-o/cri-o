package runtimehandlerhooks

import (
	"encoding/hex"
	"fmt"
	"strings"
	"unicode"

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
