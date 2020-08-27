package hostport_test

import (
	"net"
	"time"

	"github.com/cri-o/cri-o/internal/hostport"
	"github.com/cri-o/cri-o/internal/lib/sandbox"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
	k8sHostport "k8s.io/kubernetes/pkg/kubelet/dockershim/network/hostport"
)

// The actual test suite
var _ = t.Describe("Hostport", func() {
	var sut *hostport.Manager

	BeforeEach(func() {
		sut = hostport.New()
		Expect(sut).NotTo(BeNil())
	})

	newSandbox := func(mappings []*k8sHostport.PortMapping) *sandbox.Sandbox {
		sb, err := sandbox.New("sandboxID", "", "", "", "",
			make(map[string]string), make(map[string]string), "", "",
			&pb.PodSandboxMetadata{}, "", "", false, "", "", "",
			mappings, false, time.Now(), "")

		Expect(err).To(BeNil())
		Expect(sb).NotTo(BeNil())

		return sb
	}

	t.Describe("Add", func() {
		It("should succeed", func() {
			// Given
			sb := newSandbox(nil)

			// When
			err := sut.Add(sb, net.IPv4zero)

			// Then
			Expect(err).To(BeNil())
		})

		It("should fail when sandbox is nil", func() {
			// Given
			// When
			err := sut.Add(nil, net.IPv4zero)

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail for if mapping IP is missing", func() {
			// Given
			sb := newSandbox([]*k8sHostport.PortMapping{{
				Name:          "test",
				HostPort:      8080,
				ContainerPort: 8080,
				Protocol:      v1.ProtocolTCP,
			}})

			// When
			err := sut.Add(sb, net.IPv4zero)

			// Then
			Expect(err).NotTo(BeNil())
		})
	})

	t.Describe("Remove", func() {
		It("should succeed", func() {
			// Given
			sb := newSandbox(nil)

			// When
			err := sut.Remove(sb)

			// Then
			Expect(err).To(BeNil())
		})

		It("should fail when sandbox is nil", func() {
			// Given
			// When
			err := sut.Remove(nil)

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail for if mapping IP is missing", func() {
			// Given
			sb := newSandbox([]*k8sHostport.PortMapping{{
				Name:          "test",
				HostPort:      8080,
				ContainerPort: 8080,
				Protocol:      v1.ProtocolTCP,
			}})

			// When
			err := sut.Remove(sb)

			// Then
			Expect(err).NotTo(BeNil())
		})
	})
})
