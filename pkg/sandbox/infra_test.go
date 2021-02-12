package sandbox_test

import (
	"github.com/cri-o/cri-o/pkg/config"
	"github.com/cri-o/cri-o/pkg/sandbox"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

var _ = Describe("Sandbox", func() {
	t.Describe("PauseCommand", func() {
		var cfg *config.Config

		BeforeEach(func() {
			// Given
			var err error
			cfg, err = config.DefaultConfig()
			Expect(err).To(BeNil())
		})

		It("should succeed with default config", func() {
			// When
			_, err := sandbox.PauseCommand(cfg, nil)

			// Then
			Expect(err).To(BeNil())
		})

		It("should succeed with Entrypoint", func() {
			// Given
			cfg.PauseCommand = ""
			entrypoint := []string{"/custom-pause"}
			image := &v1.Image{Config: v1.ImageConfig{Entrypoint: entrypoint}}

			// When
			res, err := sandbox.PauseCommand(cfg, image)

			// Then
			Expect(err).To(BeNil())
			Expect(res).To(Equal(entrypoint))
		})

		It("should succeed with Cmd", func() {
			// Given
			cfg.PauseCommand = ""
			cmd := []string{"some-cmd"}
			image := &v1.Image{Config: v1.ImageConfig{Cmd: cmd}}

			// When
			res, err := sandbox.PauseCommand(cfg, image)

			// Then
			Expect(err).To(BeNil())
			Expect(res).To(Equal(cmd))
		})

		It("should succeed with Entrypoint and Cmd", func() {
			// Given
			cfg.PauseCommand = ""
			entrypoint := "/custom-pause"
			cmd := "some-cmd"
			image := &v1.Image{Config: v1.ImageConfig{
				Entrypoint: []string{entrypoint},
				Cmd:        []string{cmd},
			}}

			// When
			res, err := sandbox.PauseCommand(cfg, image)

			// Then
			Expect(err).To(BeNil())
			Expect(res).To(HaveLen(2))
			Expect(res[0]).To(Equal(entrypoint))
			Expect(res[1]).To(Equal(cmd))
		})

		It("should fail if config is nil", func() {
			// When
			res, err := sandbox.PauseCommand(nil, nil)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(res).To(BeNil())
		})

		It("should fail if image config is nil", func() {
			// Given
			cfg.PauseCommand = ""

			// When
			res, err := sandbox.PauseCommand(cfg, nil)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(res).To(BeNil())
		})
	})
})
