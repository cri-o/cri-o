package runtimehandlerhooks

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"
)

var _ = Describe("Utils", func() {
	Describe("computeCPUmask", func() {
		type Input struct {
			cpus string
			mask string
			set  bool
		}
		type Expected struct {
			mask    string
			invMask string
		}
		type TestData struct {
			input    Input
			expected Expected
		}

		DescribeTable("testing cpu mask",
			func(c TestData) {
				mask, invMask, err := computeCPUmask(c.input.cpus, c.input.mask, c.input.set)
				Expect(err).To(BeNil())
				Expect(mask).To(Equal(c.expected.mask))
				Expect(invMask).To(Equal(c.expected.invMask))
			},
			Entry("empty mask, set is true", TestData{
				input:    Input{cpus: "0-7", mask: "", set: true},
				expected: Expected{mask: "00000000,000000ff", invMask: "00000000,00000000"},
			}),
			Entry("empty mask, set is false", TestData{
				input:    Input{cpus: "0-7", mask: "", set: false},
				expected: Expected{mask: "00000000,00000000", invMask: "00000000,000000ff"},
			}),
			Entry("empty mask, nonsequencial cpus, set is true", TestData{
				input:    Input{cpus: "7,31-33", mask: "", set: true},
				expected: Expected{mask: "00000003,80000080", invMask: "000000fc,7fffff7f"},
			}),
			Entry("clear a single bit that was one", TestData{
				input:    Input{cpus: "0", mask: "0000,00003003", set: false},
				expected: Expected{mask: "00000000,00003002", invMask: "0000ffff,ffffcffd"},
			}),
			Entry("set a single bit that was zero", TestData{
				input:    Input{cpus: "4", mask: "0000,00003003", set: true},
				expected: Expected{mask: "00000000,00003013", invMask: "0000ffff,ffffcfec"},
			}),
			Entry("clear a set of bits", TestData{
				input:    Input{cpus: "4-13", mask: "ffff,ffffffff", set: false},
				expected: Expected{mask: "0000ffff,ffffc00f", invMask: "00000000,00003ff0"},
			}),
			Entry("set a set of bits", TestData{
				input:    Input{cpus: "4-13", mask: "ffff,ffffc00f", set: true},
				expected: Expected{mask: "0000ffff,ffffffff", invMask: "00000000,00000000"},
			}),
		)
	})
	Describe("cpuMaskToCPUSet", func() {
		type listToMask struct {
			cpuList string
			cpuMask string
		}

		cpuListToMask := []listToMask{
			{"0", "00000001"},
			{"2-3", "0000000c"},
			{"3,4,53-55,61-63", "e0e00000,00000018"},
			{"0-127", "ffffffff,ffffffff,ffffffff,ffffffff"},
			{"0-255", "ffffffff,ffffffff,ffffffff,ffffffff,ffffffff,ffffffff,ffffffff,ffffffff"},
		}

		Context("convert CPU mask to CPU set", func() {
			It("should generate a valid CPU set from a CPU mask", func() {
				for _, cpuEntry := range cpuListToMask {
					cpuSetFromList, err := cpuset.Parse(cpuEntry.cpuList)
					Expect(err).ToNot(HaveOccurred())
					cpuSetFromMask, err := cpuMaskToCPUSet(cpuEntry.cpuMask)
					Expect(err).ToNot(HaveOccurred())

					Expect(cpuSetFromList).Should(Equal(cpuSetFromMask))
				}
			})
		})
	})
})
