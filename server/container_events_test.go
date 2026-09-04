package server_test

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"
	"google.golang.org/protobuf/proto"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	containereventservermock "github.com/cri-o/cri-o/test/mocks/containereventserver"
)

// eventTimeout is how long to wait for the events to be delivered. Everything
// here is in-memory and takes way less than that, but a CI machine running the
// race detector under load can be slow, and this is not a benchmark.
const eventTimeout = 30 * time.Second

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

// heartbeat is only used to detect that a client has been registered by the
// server, so it must not collide with any of the events above.
var heartbeat = types.ContainerEventResponse{
	ContainerId: "heartbeat",
}

// protoMatcher matches a protobuf message using proto.Equal.
type protoMatcher struct {
	want *types.ContainerEventResponse
}

func (m *protoMatcher) Matches(x any) bool {
	got, ok := x.(*types.ContainerEventResponse)

	return ok && proto.Equal(m.want, got)
}

func (m *protoMatcher) String() string {
	return fmt.Sprintf("is equal to %v", m.want)
}

// sendOf matches a single event sent to a client.
//
// gomock.Eq (which is what passing the message itself would use) compares with
// reflect.DeepEqual, which is not a valid way to compare protobuf messages: it
// also compares their internal state, which is populated lazily. The server
// hands the very same message to every client, so as soon as anything looks at
// it through the protobuf reflection, DeepEqual stops matching for all the
// remaining clients.
//
// The message is cloned because both comparing and printing it populate that
// internal state as well, which would race with the specs sending their own
// copy of it.
func sendOf(want *types.ContainerEventResponse) gomock.Matcher {
	clone := &types.ContainerEventResponse{}
	proto.Merge(clone, want)

	return &protoMatcher{want: clone}
}

// waitFor blocks until wg is done, failing the spec rather than hanging
// forever if that never happens.
func waitFor(wg *sync.WaitGroup) {
	GinkgoHelper()

	done := make(chan struct{})

	go func() {
		defer GinkgoRecover()

		wg.Wait()
		close(done)
	}()

	Eventually(done).WithTimeout(eventTimeout).Should(BeClosed())
}

var _ = t.Describe("ContainerEvents", func() {
	BeforeEach(func() {
		beforeEach()
		setupSUT()
	})

	AfterEach(afterEach)

	t.Describe("ContainerEvents", func() {
		It("should send events to single client", func() {
			cesMock := containereventservermock.NewMockRuntimeService_GetContainerEventsServer[string](mockCtrl)

			var sent sync.WaitGroup

			sent.Add(len(events))

			for i := range events {
				cesMock.EXPECT().Send(sendOf(&events[i])).
					Do(func(*types.ContainerEventResponse) { sent.Done() }).
					Return(nil)
			}

			for i := range events {
				sut.ContainerEventsChan <- &events[i]
			}

			// GetContainerEvents only returns once the events channel is
			// closed, so close it as soon as everything has been delivered.
			// The channel is captured because sut already belongs to the next
			// spec by the time this goroutine runs.
			eventsChan := sut.ContainerEventsChan

			go func() {
				defer GinkgoRecover()

				sent.Wait()
				close(eventsChan)
			}()

			err := sut.GetContainerEvents(nil, cesMock)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should send events all events to both clients", func() {
			clients := []*containereventservermock.MockRuntimeService_GetContainerEventsServer[string]{
				containereventservermock.NewMockRuntimeService_GetContainerEventsServer[string](mockCtrl),
				containereventservermock.NewMockRuntimeService_GetContainerEventsServer[string](mockCtrl),
			}

			var (
				sent       sync.WaitGroup
				registered = make([]atomic.Bool, len(clients))
			)

			sent.Add(len(clients) * len(events))

			for n, client := range clients {
				client.EXPECT().Send(sendOf(&heartbeat)).
					Do(func(*types.ContainerEventResponse) { registered[n].Store(true) }).
					Return(nil).AnyTimes()

				for i := range events {
					client.EXPECT().Send(sendOf(&events[i])).
						Do(func(*types.ContainerEventResponse) { sent.Done() }).
						Return(nil)
				}
			}

			var received sync.WaitGroup

			received.Add(len(clients))

			for _, client := range clients {
				go func() {
					defer GinkgoRecover()
					defer received.Done()

					err := sut.GetContainerEvents(nil, client)
					Expect(err).ToNot(HaveOccurred())
				}()
			}

			// The clients register asynchronously, so keep sending heartbeats
			// until every one of them received one. Otherwise an event could
			// be sent while a client is not registered yet, and reach only
			// some of them.
			Eventually(func() bool {
				sut.ContainerEventsChan <- &heartbeat

				for n := range registered {
					if !registered[n].Load() {
						return false
					}
				}

				return true
			}).WithTimeout(eventTimeout).Should(BeTrue())

			for i := range events {
				sut.ContainerEventsChan <- &events[i]
			}

			// Everything has to be delivered before the spec ends, otherwise
			// the mock controller verification races the in-flight sends.
			waitFor(&sent)

			close(sut.ContainerEventsChan)
			waitFor(&received)
		})
	})
})
