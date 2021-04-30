package sandbox

import (
	"fmt"

	"github.com/cri-o/cri-o/internal/storage"
	libconfig "github.com/cri-o/cri-o/pkg/config"
	"github.com/cri-o/cri-o/pkg/container"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/opencontainers/runtime-tools/generate"
)

func (s *sandbox) InitInfraContainer(serverConfig *libconfig.Config, podContainer *storage.ContainerInfo) error {
	var err error
	s.infra, err = container.New()
	if err != nil {
		return err
	}

	// determine pause command before initializing generator
	// so a failure will not result in a half configured generator
	pauseCommand, err := PauseCommand(serverConfig, podContainer.Config)
	if err != nil {
		return err
	}

	g := s.infra.Spec()
	g.HostSpecific = true

	// setup defaults for the pod sandbox
	g.SetRootReadonly(true)

	// configure default ulimits
	for _, u := range serverConfig.Ulimits() {
		g.AddProcessRlimits(u.Name, u.Hard, u.Soft)
	}
	g.SetProcessArgs(pauseCommand)

	return nil
}

// Spec can only be called after a successful call to InitInfraContainer
func (s *sandbox) Spec() *generate.Generator {
	return s.infra.Spec()
}

// PauseCommand returns the pause command for the provided image configuration.
func PauseCommand(cfg *libconfig.Config, image *v1.Image) ([]string, error) {
	if cfg == nil {
		return nil, fmt.Errorf("provided configuration is nil")
	}

	// This has been explicitly set by the user, since the configuration
	// default is `/pause`
	if cfg.PauseCommand != "" {
		return []string{cfg.PauseCommand}, nil
	}
	if image == nil || (len(image.Config.Entrypoint) == 0 && len(image.Config.Cmd) == 0) {
		return nil, fmt.Errorf(
			"unable to run pause image %q: %s",
			cfg.PauseImage,
			"neither Cmd nor Entrypoint specified",
		)
	}
	cmd := []string{}
	cmd = append(cmd, image.Config.Entrypoint...)
	cmd = append(cmd, image.Config.Cmd...)
	return cmd, nil
}
