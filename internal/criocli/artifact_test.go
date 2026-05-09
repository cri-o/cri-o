package criocli_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/urfave/cli/v2"

	"github.com/cri-o/cri-o/internal/criocli"
	libconfig "github.com/cri-o/cri-o/pkg/config"
)

var _ = t.Describe("ArtifactCommand", func() {
	newApp := func() *cli.App {
		cfg, err := libconfig.DefaultConfig()
		Expect(err).NotTo(HaveOccurred())

		app := cli.NewApp()
		app.Metadata = map[string]any{"config": cfg}
		app.Commands = []*cli.Command{criocli.ArtifactCommand}

		return app
	}

	It("should error when no reference is given", func() {
		err := newApp().Run([]string{"crio", "artifact", "pull"})
		Expect(err).To(MatchError(ContainSubstring("usage: crio artifact pull <reference>")))
	})

	It("should error when more than one reference is given", func() {
		err := newApp().Run([]string{"crio", "artifact", "pull", "ref1", "ref2"})
		Expect(err).To(MatchError(ContainSubstring("usage: crio artifact pull <reference>")))
	})

	It("should error when the reference cannot be parsed", func() {
		err := newApp().Run([]string{"crio", "artifact", "pull", ":::bad:::"})
		Expect(err).To(MatchError(ContainSubstring("invalid reference")))
	})

	It("should error when GetConfigFromContext fails", func() {
		app := cli.NewApp()
		app.Commands = []*cli.Command{criocli.ArtifactCommand}
		err := app.Run([]string{"crio", "artifact", "pull", "registry.example.com/models/llama:v1"})
		Expect(err).To(MatchError(ContainSubstring("type assertion error")))
	})
})
