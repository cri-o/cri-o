package runtimehandlerhooks

import (
	"bufio"
	"fmt"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Utils", func() {
	Describe("UpdateIRQSmpAffinityMask", func() {
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
				mask, invMask, err := UpdateIRQSmpAffinityMask(c.input.cpus, c.input.mask, c.input.set)
				Expect(err).ToNot(HaveOccurred())
				Expect(mask).To(Equal(c.expected.mask))
				Expect(invMask).To(Equal(c.expected.invMask))
			},
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
			Entry("clear a single bit that was one when odd mask is present", TestData{
				input:    Input{cpus: "9", mask: "fff", set: false},
				expected: Expected{mask: "00000dff", invMask: "0000f200"},
			}),
			Entry("clear two bits from a short mask", TestData{
				input:    Input{cpus: "2-3", mask: "ffffff", set: false},
				expected: Expected{mask: "00fffff3", invMask: "0000000c"},
			}),
		)

		Context("UpdateIRQBalanceConfigFile", func() {
			It("Should not let the file grow unbounded", func() {
				fakeFile, err := writeTempFile(confTemplate)
				Expect(err).ToNot(HaveOccurred())
				defer os.Remove(fakeFile)

				fakeData := "000000000,0000000fa" // doesn't need to be valid
				err = updateIrqBalanceConfigFile(fakeFile, fakeData)
				Expect(err).ToNot(HaveOccurred())

				refLineCount, err := countLines(fakeFile)
				Expect(err).ToNot(HaveOccurred())

				attempts := 10 // random number, no special meaning
				for idx := 0; idx < attempts; idx++ {
					data := fmt.Sprintf("000000000,0000000%02x", idx)
					err = updateIrqBalanceConfigFile(fakeFile, data)
					Expect(err).ToNot(HaveOccurred())

					curLineCount, err := countLines(fakeFile)
					Expect(err).ToNot(HaveOccurred())

					// we should replace the line in place
					Expect(curLineCount).To(Equal(refLineCount), "irqbalance file grown from %d to %d lines", refLineCount, curLineCount)
				}
			})
		})
	})
})

func countLines(fileName string) (int, error) {
	file, err := os.Open(fileName)
	if err != nil {
		return -1, err
	}
	defer file.Close()
	fileScanner := bufio.NewScanner(file)
	lineCount := 0
	for fileScanner.Scan() {
		lineCount++
	}
	return lineCount, nil
}

func writeTempFile(content string) (string, error) {
	f, err := os.CreateTemp("", "test-irqbalance-conf")
	if err != nil {
		return "", err
	}

	if _, err := f.WriteString(content); err != nil {
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	return f.Name(), nil
}

const confTemplate = `# irqbalance is a daemon process that distributes interrupts across
# CPUS on SMP systems. The default is to rebalance once every 10
# seconds. This is the environment file that is specified to systemd via the
# EnvironmentFile key in the service unit file (or via whatever method the init
# system you're using has.
#
# ONESHOT=yes
# after starting, wait for a minute, then look at the interrupt
# load and balance it once; after balancing exit and do not change
# it again.
#IRQBALANCE_ONESHOT=

#
# IRQBALANCE_BANNED_CPUS
# 64 bit bitmask which allows you to indicate which cpu's should
# be skipped when reblancing irqs. Cpu numbers which have their
# corresponding bits set to one in this mask will not have any
# irq's assigned to them on rebalance
#
#IRQBALANCE_BANNED_CPUS=

#
# IRQBALANCE_ARGS
# append any args here to the irqbalance daemon as documented in the man page
#
#IRQBALANCE_ARGS=

IRQBALANCE_BANNED_CPUS=
`
