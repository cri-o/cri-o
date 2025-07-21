package server_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	containereventservermock "github.com/cri-o/cri-o/test/mocks/containereventserver"
)

var events = []types.ContainerEventResponse{
	{
		ContainerId: "1",
	},
	{
		ContainerId: "2",
	},
	{
		ContainerId: "3",
	},
}

var _ = t.Describe("ContainerEvents", func() {
	BeforeEach(func() {
		beforeEach()
		setupSUT()

		// close after all events have been processed,
		// so we are not waiting for move events to come.
		go func() {
			time.Sleep(2 * time.Second)
			close(sut.ContainerEventsChan)
		}()
	})

	AfterEach(afterEach)

	t.Describe("ContainerEvents", func() {
		It("should send events to single client", func() {
			cesMock := containereventservermock.NewMockRuntimeService_GetContainerEventsServer[string](mockCtrl)
			// EXPECT expects the exact object, so we can't use the copy range gives us
			for i := range events {
				cesMock.EXPECT().Send(&events[i]).Return(nil)
			}

			//nolint:govet // copylock is not harmful for this implementation
			for _, event := range events {
				sut.ContainerEventsChan <- event
			}

			err := sut.GetContainerEvents(nil, cesMock)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should send events all events to both clients", func() {
			client1 := containereventservermock.NewMockRuntimeService_GetContainerEventsServer[string](mockCtrl)
			client2 := containereventservermock.NewMockRuntimeService_GetContainerEventsServer[string](mockCtrl)

			for i := range events {
				client1.EXPECT().Send(&events[i]).Return(nil)
				client2.EXPECT().Send(&events[i]).Return(nil)
			}

			recv := func(ces types.RuntimeService_GetContainerEventsServer) {
				err := sut.GetContainerEvents(nil, ces)
				Expect(err).ToNot(HaveOccurred())
			}

			go recv(client1)
			go recv(client2)

			// wait so that both goroutines are ready
			// when we send the events
			time.Sleep(1 * time.Second)

			//nolint:govet // copylock is not harmful for this implementation
			for _, event := range events {
				sut.ContainerEventsChan <- event
			}
		})
	})
})
