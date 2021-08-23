package process_test

import (
	"os/exec"
	"time"

	"github.com/cri-o/cri-o/internal/process"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// The actual test suite
var _ = t.Describe("ZombieMonitor", func() {
	It("should clean zombie", func() {
		cmd := createZombie()
		defer cmd.Wait() // nolint:errcheck

		monitor := process.NewZombieMonitor()
		defer monitor.Shutdown()

		Eventually(func() int {
			_, defunctChildren, err := process.ParseDefunctProcesses()
			Expect(err).To(BeNil())
			return len(defunctChildren)
		}, time.Second*10, time.Second).Should(Equal(0))
	})
})

func createZombie() *exec.Cmd {
	cmd := exec.Command("true")
	err := cmd.Start()
	Expect(err).To(BeNil())

	_, defunctChildren, err := process.ParseDefunctProcesses()
	Expect(err).To(BeNil())
	Expect(len(defunctChildren)).To(Equal(1))
	return cmd
}
