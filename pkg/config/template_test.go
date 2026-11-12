package config_test

import (
	"bytes"
	"context"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/cri-o/cri-o/pkg/config"
)

// The actual test suite.
var _ = t.Describe("Config", func() {
	t.Describe("WriteTemplate", func() {
		BeforeEach(beforeEach)
		It("should succeed", func() {
			// Given
			var wr bytes.Buffer

			// When
			err := sut.WriteTemplate(true, &wr)

			// Then
			Expect(err).ToNot(HaveOccurred())
		})

		DescribeTable("should round-trip stream_websockets",
			func(value *bool, rendered string) {
				// Given
				sut.Runtimes["pod"] = &config.RuntimeHandler{
					RuntimePath:      validFilePath,
					RuntimeType:      config.RuntimeTypePod,
					StreamWebsockets: value,
				}

				var wr bytes.Buffer

				// When
				Expect(sut.WriteTemplate(false, &wr)).To(Succeed())

				f := t.MustTempFile("config")
				Expect(os.WriteFile(f, wr.Bytes(), 0o644)).To(Succeed())

				cfg := defaultConfig()
				Expect(cfg.UpdateFromFile(context.Background(), f)).To(Succeed())

				// Then
				if rendered == "" {
					Expect(wr.String()).NotTo(ContainSubstring("\nstream_websockets ="))
					Expect(cfg.Runtimes["pod"].StreamWebsockets).To(BeNil())
				} else {
					Expect(wr.String()).To(ContainSubstring("\n" + rendered + "\n"))
					Expect(cfg.Runtimes["pod"].StreamWebsockets).To(HaveValue(Equal(*value)))
				}
			},
			Entry("unset", nil, ""),
			Entry("false", new(false), "stream_websockets = false"),
			Entry("true", new(true), "stream_websockets = true"),
		)
	})
	t.Describe("RuntimesEqual", func() {
		It("not equal if different length", func() {
			// When
			r1 := config.Runtimes{
				"1": &config.RuntimeHandler{},
				"2": &config.RuntimeHandler{},
			}
			r2 := config.Runtimes{
				"1": &config.RuntimeHandler{},
			}

			// Then
			Expect(config.RuntimesEqual(r1, r2)).To(BeFalse())
		})
		It("not equal if different keys", func() {
			// When
			r1 := config.Runtimes{
				"1": &config.RuntimeHandler{},
			}
			r2 := config.Runtimes{
				"2": &config.RuntimeHandler{},
			}

			// Then
			Expect(config.RuntimesEqual(r1, r2)).To(BeFalse())
		})
		It("not equal if different values", func() {
			// When
			r1 := config.Runtimes{
				"1": &config.RuntimeHandler{
					RuntimePath: "1",
				},
			}
			r2 := config.Runtimes{
				"1": &config.RuntimeHandler{
					RuntimePath: "2",
				},
			}

			// Then
			Expect(config.RuntimesEqual(r1, r2)).To(BeFalse())
		})
		It("not equal if different slice values", func() {
			// When
			r1 := config.Runtimes{
				"1": &config.RuntimeHandler{
					AllowedAnnotations: []string{"1"},
				},
			}
			r2 := config.Runtimes{
				"1": &config.RuntimeHandler{
					AllowedAnnotations: []string{"2"},
				},
			}

			// Then
			Expect(config.RuntimesEqual(r1, r2)).To(BeFalse())
		})
		It("equal if same values", func() {
			// When
			r1 := config.Runtimes{
				"1": &config.RuntimeHandler{
					RuntimePath: "1",
				},
			}
			r2 := config.Runtimes{
				"1": &config.RuntimeHandler{
					RuntimePath: "1",
				},
			}

			// Then
			Expect(config.RuntimesEqual(r1, r2)).To(BeTrue())
		})
	})
})
