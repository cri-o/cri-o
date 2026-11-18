package server_test

import (
	"context"
	"fmt"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/pkg/config"
)

// The actual test suite.
var _ = t.Describe("Status", func() {
	// Prepare the sut
	BeforeEach(beforeEach)

	JustBeforeEach(setupSUT)

	AfterEach(afterEach)

	t.Describe("Status", func() {
		It("should succeed", func() {
			// When
			response, err := sut.Status(context.Background(),
				&types.StatusRequest{})

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(response).NotTo(BeNil())
			Expect(len(response.GetStatus().GetConditions())).To(BeEquivalentTo(2))

			for _, condition := range response.GetStatus().GetConditions() {
				Expect(condition.GetStatus()).To(BeTrue())
			}
		})

		It("should succeed when CNI plugin status errors", func() {
			// When
			response, err := sut.Status(context.Background(),
				&types.StatusRequest{})

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(response).NotTo(BeNil())
			Expect(len(response.GetStatus().GetConditions())).To(BeEquivalentTo(2))

			for _, condition := range response.GetStatus().GetConditions() {
				Expect(condition.GetStatus()).To(BeTrue())
			}
		})

		It("should return info as part of a verbose response", func() {
			// When
			response, err := sut.Status(context.Background(),
				&types.StatusRequest{Verbose: true})

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(response).NotTo(BeNil())
			Expect(response.GetInfo()).NotTo(BeNil())
		})
	})

	DescribeTableSubtree(
		"runtime handler ordering",
		func(names []string, defaultRuntime string, expectedNames []string) {
			capabilities := map[string]*types.RuntimeHandlerFeatures{
				"alpha": {RecursiveReadOnlyMounts: false, UserNamespaces: true},
				"crun":  {RecursiveReadOnlyMounts: true, UserNamespaces: false},
				"runc":  {RecursiveReadOnlyMounts: true, UserNamespaces: true},
				"zeta":  {RecursiveReadOnlyMounts: false, UserNamespaces: false},
			}

			BeforeEach(func() {
				serverConfig.Runtimes = nil
				if names != nil {
					serverConfig.Runtimes = make(config.Runtimes, len(names))
				}

				for _, name := range names {
					features := capabilities[name]
					handler := &config.RuntimeHandler{}
					Expect(handler.LoadRuntimeFeatures(fmt.Appendf(
						nil,
						`{"ociVersionMin":"1.0.0","ociVersionMax":"1.2.0","linux":{"mountExtensions":{"idmap":{"enabled":%t}}}}`,
						features.GetUserNamespaces(),
					))).To(Succeed())
					handler.SetRuntimeSupportsRROMounts(features.GetRecursiveReadOnlyMounts())
					serverConfig.Runtimes[name] = handler
				}

				serverConfig.DefaultRuntime = defaultRuntime
			})

			DescribeTable("returns stable capabilities",
				func(verbose bool) {
					var expected []*types.RuntimeHandler
					if expectedNames != nil {
						expected = make([]*types.RuntimeHandler, 0, len(expectedNames))
					}

					for _, name := range expectedNames {
						featureName := name
						if name == "" {
							featureName = defaultRuntime
						}

						expected = append(expected, &types.RuntimeHandler{
							Name: name, Features: capabilities[featureName],
						})
					}

					// Exact order catches the regression deterministically: the default
					// alias must precede its named handler, regardless of map iteration.
					// Repeat calls to also catch order changes with unchanged config.
					for range 32 {
						response, err := sut.Status(
							context.Background(),
							&types.StatusRequest{Verbose: verbose},
						)
						Expect(err).NotTo(HaveOccurred())
						Expect(response.GetRuntimeHandlers()).To(Equal(expected))

						if verbose {
							Expect(response.GetInfo()).NotTo(BeEmpty())
						} else {
							Expect(response.GetInfo()).To(BeEmpty())
						}
					}

					var callers sync.WaitGroup
					for range 8 {
						callers.Go(func() {
							defer GinkgoRecover()

							response, err := sut.Status(
								context.Background(),
								&types.StatusRequest{Verbose: verbose},
							)
							Expect(err).NotTo(HaveOccurred())
							Expect(response.GetRuntimeHandlers()).To(Equal(expected))
						})
					}

					callers.Wait()

					if len(expectedNames) > 0 {
						response, err := sut.Status(
							context.Background(),
							&types.StatusRequest{Verbose: verbose},
						)
						Expect(err).NotTo(HaveOccurred())

						// A caller modifying one response must not change later responses
						// or the capabilities stored in the configuration.
						handler := response.GetRuntimeHandlers()[0]
						handler.Name = "modified"
						handler.Features.RecursiveReadOnlyMounts = !handler.GetFeatures().
							GetRecursiveReadOnlyMounts()
						handler.Features.UserNamespaces = !handler.GetFeatures().GetUserNamespaces()

						response, err = sut.Status(
							context.Background(),
							&types.StatusRequest{Verbose: verbose},
						)
						Expect(err).NotTo(HaveOccurred())
						Expect(response.GetRuntimeHandlers()).To(Equal(expected))
					}

					// Status must not insert the default alias into the config map.
					Expect(serverConfig.Runtimes).To(HaveLen(len(names)))
					Expect(serverConfig.DefaultRuntime).To(Equal(defaultRuntime))
				},
				Entry("without verbose info", false),
				Entry("with verbose info", true),
			)
		},
		Entry("nil handlers", nil, "runc", nil),
		Entry("empty handlers", []string{}, "runc", nil),
		Entry("one handler and its default alias", []string{"runc"}, "runc", []string{"", "runc"}),
		Entry(
			"default sorts first",
			[]string{"zeta", "runc", "crun", "alpha"},
			"alpha",
			[]string{"", "alpha", "crun", "runc", "zeta"},
		),
		Entry(
			"default supports only recursive read-only mounts",
			[]string{"zeta", "runc", "crun", "alpha"},
			"crun",
			[]string{"", "alpha", "crun", "runc", "zeta"},
		),
		Entry(
			"default supports both features",
			[]string{"zeta", "runc", "crun", "alpha"},
			"runc",
			[]string{"", "alpha", "crun", "runc", "zeta"},
		),
		Entry(
			"default sorts last",
			[]string{"zeta", "runc", "crun", "alpha"},
			"zeta",
			[]string{"", "alpha", "crun", "runc", "zeta"},
		),
	)
})
