package oci

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func stopWatcherCount(c *Container) int {
	c.stopLock.Lock()
	defer c.stopLock.Unlock()

	return len(c.stopWatchers)
}

var _ = Describe("Container stop watchers", func() {
	var c *Container

	BeforeEach(func() {
		c = &Container{stopTimeoutChan: make(chan int64, 10)}
		c.SetAsStopping()
	})

	It("should release the watcher of a canceled request", func() {
		for range 100 {
			ctx, cancel := context.WithCancel(context.Background())
			result := make(chan error, 1)

			go func() { result <- c.WaitOnStopTimeout(ctx, 30) }()

			Eventually(func() int { return stopWatcherCount(c) }).Should(Equal(1))
			cancel()
			Eventually(result, 5*time.Second).Should(Receive(MatchError(context.Canceled)))
			Eventually(func() int { return stopWatcherCount(c) }).Should(BeZero())
		}
	})

	It("should keep the watchers of other requests when one is canceled", func() {
		ctx, cancel := context.WithCancel(context.Background())
		canceled := make(chan error, 1)
		waiting := make(chan error, 1)

		go func() { canceled <- c.WaitOnStopTimeout(ctx, 30) }()
		go func() { waiting <- c.WaitOnStopTimeout(context.Background(), 30) }()

		Eventually(func() int { return stopWatcherCount(c) }).Should(Equal(2))

		cancel()
		Eventually(canceled, 5*time.Second).Should(Receive(MatchError(context.Canceled)))
		Expect(stopWatcherCount(c)).To(Equal(1))
		Consistently(waiting, 50*time.Millisecond).ShouldNot(Receive())

		c.SetAsDoneStopping()
		Eventually(waiting, 5*time.Second).Should(Receive(BeNil()))
		Expect(stopWatcherCount(c)).To(BeZero())
	})
})

var _ = Describe("Container setFinishedIfUnset", func() {
	It("should keep a finish time that is already known", func() {
		known := time.Now().Add(-time.Hour)
		c := &Container{state: &ContainerState{}}
		c.state.Finished = known

		c.setFinishedIfUnset()

		Expect(c.state.Finished).To(Equal(known))
	})

	It("should record the finish time when none is known", func() {
		c := &Container{state: &ContainerState{}}

		c.setFinishedIfUnset()

		Expect(c.state.Finished).NotTo(BeZero())
	})
})
