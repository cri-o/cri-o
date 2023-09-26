package statsserver_test

import (
	statsserver "github.com/cri-o/cri-o/internal/lib/stats"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = t.Describe("Metrics", func() {
	//ctx := context.TODO()
	BeforeEach(beforeEach)
	t.Describe("GetSandboxMetric", func() {
		It("should succeed", func() {
			sbMetrics := statsserver.NewSandboxMetrics(mySandbox)
			Expect(sbMetrics.GetMetric()).NotTo(BeNil())
			Expect(sbMetrics.GetMetric().Metrics).NotTo(BeNil())
		})
	})

})
