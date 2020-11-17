package runtimehandlerhooks

import (
	"encoding/hex"
	"fmt"
	"strconv"
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

// computeCPUmask takes input a set of cpus and a mask returning a CPU mask
// with the masked cpus included/excluded based on the set argument and the inverted mask
func computeCPUmask(cpus, mask string, set bool) (cpuMask, invertedCPUMask string, err error) {
	inputCPUs, err := cpuset.Parse(cpus)
	if err != nil {
		return cpus, "", err
	}

	// only ascii string supported
	if !isASCII(mask) {
		return cpus, "", fmt.Errorf("non ascii character detected: %s", mask)
	}

	// remove ","; now each element is "0-9,a-f"
	s := strings.ReplaceAll(mask, ",", "")

	// the index 0 corresponds to the cpu 0-7
	maskArray, err := mapHexCharToByte(s)
	if err != nil {
		return cpus, "", err
	}

	// handle empty input mask, prepare an empty array
	if len(maskArray) == 0 {
		slice := inputCPUs.ToSlice()
		maxCPUs := slice[len(slice)-1] + 1
		maskLen := maxCPUs / 8
		if maxCPUs%8 != 0 {
			maskLen++
		}
		maskArray = make([]byte, maskLen)
	}

	invertedMaskArray := invertByteArray(maskArray)

	for _, cpu := range inputCPUs.ToSlice() {
		if set {
			// each byte represent 8 cpus
			maskArray[cpu/8] |= cpuMaskByte(cpu % 8)
			invertedMaskArray[cpu/8] &^= cpuMaskByte(cpu % 8)
		} else {
			maskArray[cpu/8] &^= cpuMaskByte(cpu % 8)
			invertedMaskArray[cpu/8] |= cpuMaskByte(cpu % 8)
		}
	}

	maskString := mapByteToHexChar(maskArray)
	invertedMaskString := mapByteToHexChar(invertedMaskArray)

	maskStringWithComma := maskString[0:8]
	invertedMaskStringWithComma := invertedMaskString[0:8]
	for i := 8; i+8 <= len(maskString); i += 8 {
		maskStringWithComma = maskStringWithComma + "," + maskString[i:i+8]
		invertedMaskStringWithComma = invertedMaskStringWithComma + "," + invertedMaskString[i:i+8]
	}
	return maskStringWithComma, invertedMaskStringWithComma, nil
}

// cpuMaskToCPUSet parses a CPUSet received in a Mask Format, see:
// https://man7.org/linux/man-pages/man7/cpuset.7.html#FORMATS
func cpuMaskToCPUSet(cpuMask string) (cpuset.CPUSet, error) {
	chunks := strings.Split(cpuMask, ",")

	// reverse the chunks order
	n := len(chunks)
	for i := 0; i < n/2; i++ {
		chunks[i], chunks[n-i-1] = chunks[n-i-1], chunks[i]
	}

	builder := cpuset.NewBuilder()
	for i, chunk := range chunks {
		mask, err := strconv.ParseUint(chunk, 16, 32)
		if err != nil {
			return cpuset.NewCPUSet(), fmt.Errorf("failed to parse the CPU mask %s: %v", cpuMask, err)
		}
		for j := 0; j < 32; j++ {
			if mask&1 == 1 {
				builder.Add(i*32 + j)
			}
			mask >>= 1
		}
	}

	return builder.Result(), nil
}
