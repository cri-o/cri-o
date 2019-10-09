// +build linux

package libpod

import (
	"context"
	"strings"

	"github.com/containers/libpod/libpod/define"
	"github.com/containers/libpod/libpod/image"
	"github.com/containers/libpod/pkg/rootless"
	"github.com/containers/libpod/pkg/util"
	"github.com/opencontainers/image-spec/specs-go/v1"
	spec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/opencontainers/runtime-tools/generate"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	// IDTruncLength is the length of the pod's id that will be used to make the
	// infra container name
	IDTruncLength = 12
)

func (r *Runtime) makeInfraContainer(ctx context.Context, p *Pod, imgName, imgID string, config *v1.ImageConfig) (*Container, error) {

	// Set up generator for infra container defaults
	g, err := generate.New("linux")
	if err != nil {
		return nil, err
	}

	// Set Pod hostname
	g.Config.Hostname = p.config.Hostname

	isRootless := rootless.IsRootless()

	entryCmd := []string{r.config.InfraCommand}
	// I've seen circumstances where config is being passed as nil.
	// Let's err on the side of safety and make sure it's safe to use.
	if config != nil {
		setEntrypoint := false
		// default to entrypoint in image if there is one
		if len(config.Entrypoint) > 0 {
			entryCmd = config.Entrypoint
			setEntrypoint = true
		}
		if len(config.Cmd) > 0 {
			// We can't use the default pause command, since we're
			// sourcing from the image. If we didn't already set an
			// entrypoint, set one now.
			if !setEntrypoint {
				// Use the Docker default "/bin/sh -c"
				// entrypoint, as we're overriding command.
				// If an image doesn't want this, it can
				// override entrypoint too.
				entryCmd = []string{"/bin/sh", "-c"}
			}
			entryCmd = append(entryCmd, config.Cmd...)
		}
		if len(config.Env) > 0 {
			for _, nameValPair := range config.Env {
				nameValSlice := strings.Split(nameValPair, "=")
				if len(nameValSlice) < 2 {
					return nil, errors.Errorf("Invalid environment variable structure in pause image")
				}
				g.AddProcessEnv(nameValSlice[0], nameValSlice[1])
			}
		}
	}

	g.SetRootReadonly(true)
	g.SetProcessArgs(entryCmd)

	logrus.Debugf("Using %q as infra container entrypoint", entryCmd)

	if isRootless {
		g.RemoveMount("/dev/pts")
		devPts := spec.Mount{
			Destination: "/dev/pts",
			Type:        "devpts",
			Source:      "devpts",
			Options:     []string{"private", "nosuid", "noexec", "newinstance", "ptmxmode=0666", "mode=0620"},
		}
		g.AddMount(devPts)
	}

	containerName := p.ID()[:IDTruncLength] + "-infra"
	var options []CtrCreateOption
	options = append(options, r.WithPod(p))
	options = append(options, WithRootFSFromImage(imgID, imgName, false))
	options = append(options, WithName(containerName))
	options = append(options, withIsInfra())

	// Since user namespace sharing is not implemented, we only need to check if it's rootless
	networks := make([]string, 0)
	netmode := "bridge"
	if isRootless {
		netmode = "slirp4netns"
	}
	// PostConfigureNetNS should not be set since user namespace sharing is not implemented
	// and rootless networking no longer supports post configuration setup
	options = append(options, WithNetNS(p.config.InfraContainer.PortBindings, false, netmode, networks))

	return r.newContainer(ctx, g.Config, options...)
}

// createInfraContainer wrap creates an infra container for a pod.
// An infra container becomes the basis for kernel namespace sharing between
// containers in the pod.
func (r *Runtime) createInfraContainer(ctx context.Context, p *Pod) (*Container, error) {
	if !r.valid {
		return nil, define.ErrRuntimeStopped
	}

	newImage, err := r.ImageRuntime().New(ctx, r.config.InfraImage, "", "", nil, nil, image.SigningOptions{}, nil, util.PullImageMissing)
	if err != nil {
		return nil, err
	}

	data, err := newImage.Inspect(ctx)
	if err != nil {
		return nil, err
	}
	imageName := newImage.Names()[0]
	imageID := data.ID

	return r.makeInfraContainer(ctx, p, imageName, imageID, data.Config)
}
