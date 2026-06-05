package runtimehandlerhooks

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/opencontainers/runtime-tools/generate"
)

var _ = Describe("GOMAXPROCS injection", func() {
	DescribeTable("injectGOMAXPROCS",
		func(specEnvs []string, maxProcs int64, expectSet bool, expectVal string) {
			g, err := generate.New("linux")
			Expect(err).NotTo(HaveOccurred())

			// Pre-populate the spec's process env to simulate image/pod envs.
			for _, env := range specEnvs {
				parts := strings.SplitN(env, "=", 2)
				g.AddProcessEnv(parts[0], parts[1])
			}

			envsBefore := len(g.Config.Process.Env)

			injectGOMAXPROCS(&g, maxProcs)

			envsAfter := len(g.Config.Process.Env)
			injected := envsAfter > envsBefore

			if expectSet {
				Expect(injected).To(BeTrue(), "expected GOMAXPROCS to be injected, but it was not")

				var found bool

				for _, env := range g.Config.Process.Env {
					if val, ok := strings.CutPrefix(env, "GOMAXPROCS="); ok {
						Expect(val).To(Equal(expectVal))

						found = true

						break
					}
				}

				Expect(found).To(BeTrue(), "GOMAXPROCS was injected but value not found")
			} else {
				Expect(injected).To(BeFalse(), "expected GOMAXPROCS to not be injected, but it was")
			}
		},
		Entry("injects when not set",
			[]string{"FOO=bar"}, int64(4), true, "4"),
		Entry("skips when set in spec envs",
			[]string{"GOMAXPROCS=16"}, int64(4), false, ""),
		Entry("skips when set via default_env (already merged into spec)",
			[]string{"FOO=bar", "GOMAXPROCS=8"}, int64(4), false, ""),
		Entry("skips when GOMAXPROCS prefix matches in envs",
			[]string{"GOMAXPROCS=0"}, int64(4), false, ""),
		Entry("injects with large value",
			nil, int64(128), true, "128"),
		Entry("injects with value 1",
			nil, int64(1), true, "1"),
	)
})

var _ = Describe("GOMAXPROCS calculation", func() {
	DescribeTable("calculateGOMAXPROCS",
		func(shares, fallbackMaxProcs, expectedMaxProcs int64) {
			got := calculateGOMAXPROCS(shares, fallbackMaxProcs)
			Expect(got).To(Equal(expectedMaxProcs),
				"shares=%d, floor=%d", shares, fallbackMaxProcs)
		},
		Entry("2 CPU request (shares=2048), floor=4 -> use floor",
			int64(2048), int64(4), int64(4)),
		Entry("500m request (shares=512), floor=4 -> use floor",
			int64(512), int64(4), int64(4)),
		Entry("8 CPU request (shares=8192), floor=4 -> use calculated",
			int64(8192), int64(4), int64(16)),
		Entry("1 CPU request (shares=1024), floor=4 -> use floor",
			int64(1024), int64(4), int64(4)),
		Entry("4 CPU request (shares=4096), floor=4 -> use calculated (double is past floor)",
			int64(4096), int64(4), int64(8)),
		Entry("16 CPU request (shares=16384), floor=4 -> use calculated",
			int64(16384), int64(4), int64(32)),
		Entry("best-effort (shares=2), floor=4 -> use floor",
			int64(2), int64(4), int64(4)),
		Entry("100m request (shares=102), floor=4 -> use floor",
			int64(102), int64(4), int64(4)),
		Entry("5 CPU request (shares=5120), floor=2 -> use calculated",
			int64(5120), int64(2), int64(10)),
		Entry("250m request (shares=256), floor=1 -> use floor",
			int64(256), int64(1), int64(1)),
		Entry("3 CPU request (shares=3072), floor=2 -> use calculated",
			int64(3072), int64(2), int64(6)),
	)
})
