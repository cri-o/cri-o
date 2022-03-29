package runtimehandlerhooks

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestHighPerformanceHooks(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "high_performance_hooks Suite")
}
