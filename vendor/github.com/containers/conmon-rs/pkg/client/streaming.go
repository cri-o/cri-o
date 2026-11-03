package client

import (
	"context"
	"fmt"
	"time"

	"github.com/containers/conmon-rs/internal/proto"
)

// ServeExecContainerConfig is the configuration for calling the ServeExecContainer method.
type ServeExecContainerConfig struct {
	// ID is the container identifier. A request for a container which the
	// server does not know fails with an error matching
	// [ErrContainerNotFound].
	ID string

	// Command is the command to be run.
	Command []string

	// Tty indicates if a tty should be used or not.
	Tty bool

	// Stdin indicates if stdin should be available or not. The stdin of the
	// process gets closed once the client closed its stdin, which only SPDY
	// and v5.channel.k8s.io can signal, the WebSocket protocols before v5
	// have no close message. A terminal is never closed, it gets its end of
	// file character instead, like from Ctrl-D.
	Stdin bool

	// Stdout indicates if stdout should be available or not.
	Stdout bool

	// Stderr indicates if stderr should be available or not.
	Stderr bool

	// CgroupManager can be used to select the cgroup manager.
	//
	// To use this option set [ConmonServerConfig.CgroupManager] to
	// [CgroupManagerPerCommand].
	CgroupManager CgroupManager

	// IdleTimeout fails a session whose connection got no frame of the
	// client and no data either way for this long, rounded up to whole
	// seconds, and kills the process then. WebSocket pings and pongs, the
	// pings of the server and the answers to them and other control frames
	// of the server do not count. Stdin counts once the server reads it,
	// stdin which backs up behind a process which does not read it stops
	// that. Zero disables the timeout. Servers without [FeatureIdleTimeout]
	// ignore it.
	IdleTimeout time.Duration
}

// ServeExecContainerResult is the result for calling the ServeExecContainer method.
type ServeExecContainerResult struct {
	// URL specifies the returned URL.
	URL string
}

// ServeExecContainer returns a URL on which the server executes the command
// within a running container once a client connected. Clients speak the
// Kubernetes remote command protocols: v5.channel.k8s.io over WebSocket like
// kubectl, and with FeatureStreamingProtocols also SPDY/3.1 like the kubelet
// and the older WebSocket versions down to channel.k8s.io.
//
// See the Streaming section of the package documentation for how long the
// URL is good for and how many may wait (ErrTooManyRequests), and usage.md
// in the repository for the protocols.
func (c *ConmonClient) ServeExecContainer(
	ctx context.Context,
	cfg *ServeExecContainerConfig,
) (_ *ServeExecContainerResult, retErr error) {
	ctx, span := c.startSpan(ctx, "ServeExecContainer")
	defer endSpan(span, &retErr)

	if cfg == nil {
		return nil, ErrMissingConfig
	}

	if err := c.validateCgroupManager(cfg.CgroupManager); err != nil {
		return nil, err
	}

	url, err := call(ctx, c, proto.Conmon.ServeExecContainer,
		func(req proto.Conmon_ServeExecContainerRequest) error {
			if err := req.SetId(cfg.ID); err != nil {
				return fmt.Errorf("set ID: %w", err)
			}

			if err := stringSliceToTextList(cfg.Command, req.NewCommand); err != nil {
				return fmt.Errorf("convert command to text list: %w", err)
			}

			req.SetTty(cfg.Tty)
			req.SetStdin(cfg.Stdin)
			req.SetStdout(cfg.Stdout)
			req.SetStderr(cfg.Stderr)
			req.SetIdleTimeoutSec(idleTimeoutSeconds(cfg.IdleTimeout))

			c.setCgroupManager(cfg.CgroupManager, req)

			return nil
		},
		readURL,
	)
	if err != nil {
		return nil, err
	}

	return &ServeExecContainerResult{URL: url}, nil
}

// ServeAttachContainerConfig is the configuration for calling the ServeAttachContainer method.
type ServeAttachContainerConfig struct {
	// ID is the container identifier. A request for a container which the
	// server does not know fails with an error matching
	// [ErrContainerNotFound].
	ID string

	// Stdin indicates if stdin should be available or not.
	Stdin bool

	// Stdout indicates if stdout should be available or not.
	Stdout bool

	// Stderr indicates if stderr should be available or not.
	Stderr bool

	// StopAfterStdinEOF indicates if the container stdin should be closed
	// once the client closes its stdin (stdin once), which only SPDY and
	// v5.channel.k8s.io can signal, see ServeExecContainerConfig.Stdin. The
	// output is then still forwarded until the container closes it. A
	// terminal is never closed, it gets its end of file character instead
	// once the client closed its stdin, like from Ctrl-D. A client which
	// goes away without closing its stdin still closes the container stdin,
	// unless it is a terminal. Without StopAfterStdinEOF and without a
	// terminal, the end of the stdin of the client ends the session.
	StopAfterStdinEOF bool

	// CgroupManager can be used to select the cgroup manager.
	//
	// Deprecated: Attach does not run the runtime, so the value was never
	// sent and has no effect.
	CgroupManager CgroupManager

	// IdleTimeout fails a session which was idle for this long, like
	// [ServeExecContainerConfig.IdleTimeout], without killing anything.
	// Stdin which backs up behind a container which does not read it stops
	// counting. Zero disables the timeout. Servers without
	// [FeatureIdleTimeout] ignore it.
	IdleTimeout time.Duration
}

// ServeAttachContainerResult is the result for calling the ServeAttachContainer method.
type ServeAttachContainerResult struct {
	// URL specifies the returned URL.
	URL string
}

// ServeAttachContainer returns a URL on which the server attaches a client to
// a running container. Clients speak the protocols of ServeExecContainer.
//
// See the Streaming section of the package documentation for how long the
// URL is good for and how many may wait (ErrTooManyRequests), and usage.md
// in the repository for the protocols.
func (c *ConmonClient) ServeAttachContainer(
	ctx context.Context,
	cfg *ServeAttachContainerConfig,
) (_ *ServeAttachContainerResult, retErr error) {
	ctx, span := c.startSpan(ctx, "ServeAttachContainer")
	defer endSpan(span, &retErr)

	if cfg == nil {
		return nil, ErrMissingConfig
	}

	url, err := call(ctx, c, proto.Conmon.ServeAttachContainer,
		func(req proto.Conmon_ServeAttachContainerRequest) error {
			if err := req.SetId(cfg.ID); err != nil {
				return fmt.Errorf("set ID: %w", err)
			}

			req.SetStdin(cfg.Stdin)
			req.SetStdout(cfg.Stdout)
			req.SetStderr(cfg.Stderr)
			req.SetStopAfterStdinEof(cfg.StopAfterStdinEOF)
			req.SetIdleTimeoutSec(idleTimeoutSeconds(cfg.IdleTimeout))

			return nil
		},
		readURL,
	)
	if err != nil {
		return nil, err
	}

	return &ServeAttachContainerResult{URL: url}, nil
}

// ServePortForwardContainerConfig is the configuration for calling the
// ServePortForwardContainer method. It has no ports, the SPDY protocol names
// them per stream. The WebSocket channel protocol reads them from the port
// query parameters of the URL, which the kubelet does not pass on when it
// proxies a client. A caller which hands out the URL for that protocol
// appends the ports of the CRI PortForwardRequest as port query parameters,
// like ?port=80&port=443.
type ServePortForwardContainerConfig struct {
	// NetNsPath is the absolute path to the network namespace of the
	// container. Servers since v1.2.2 reject other ones.
	NetNsPath string

	// IdleTimeout closes a connection which was idle for this long, like
	// [ServeExecContainerConfig.IdleTimeout]. A stream which slowly takes
	// its data keeps it active. Zero disables the timeout.
	IdleTimeout time.Duration
}

// ServePortForwardContainerResult is the result for calling the ServePortForwardContainer method.
type ServePortForwardContainerResult struct {
	// URL specifies the returned URL.
	URL string
}

// ServePortForwardContainer returns a URL on which the server forwards ports
// into the network namespace. Clients connect with SPDY/3.1 and the Kubernetes
// portforward.k8s.io protocol like the kubelet, the same tunneled through a
// WebSocket like crictl, or with the older WebSocket channel protocol, whose
// ports the caller appends to the URL, see ServePortForwardContainerConfig.
// Servers without the FeaturePortForward feature return ErrUnsupported.
//
// See the Streaming section of the package documentation for how long the
// URL is good for and how many may wait (ErrTooManyRequests), and usage.md
// in the repository for the protocols.
func (c *ConmonClient) ServePortForwardContainer(
	ctx context.Context,
	cfg *ServePortForwardContainerConfig,
) (_ *ServePortForwardContainerResult, retErr error) {
	ctx, span := c.startSpan(ctx, "ServePortForwardContainer")
	defer endSpan(span, &retErr)

	// Both errors, a nil config always failed with the latter.
	if cfg == nil {
		return nil, fmt.Errorf("%w: %w", ErrMissingConfig, ErrMissingNetNsPath)
	}

	if cfg.NetNsPath == "" {
		return nil, ErrMissingNetNsPath
	}

	// The request goes to the checked server.
	rpcClient, err := c.requireServerFeature(ctx, FeaturePortForward, "port forwarding")
	if err != nil {
		return nil, err
	}
	defer rpcClient.Release()

	url, err := callOn(ctx, c, rpcClient, proto.Conmon.ServePortForwardContainer,
		func(req proto.Conmon_ServePortForwardContainerRequest) error {
			if setErr := req.SetNetNsPath(cfg.NetNsPath); setErr != nil {
				return fmt.Errorf("set network namespace path: %w", setErr)
			}

			req.SetIdleTimeoutSec(idleTimeoutSeconds(cfg.IdleTimeout))

			return nil
		},
		readURL,
	)
	if err != nil {
		return nil, err
	}

	return &ServePortForwardContainerResult{URL: url}, nil
}

// readURL reads the URL of the response of a Serve call.
func readURL[T interface{ Url() (string, error) }](response T) (string, error) {
	url, err := response.Url()
	if err != nil {
		return "", fmt.Errorf("get url: %w", err)
	}

	return url, nil
}

// idleTimeoutSeconds converts an idle timeout to seconds, rounded up so that a
// short timeout does not disable it.
func idleTimeoutSeconds(timeout time.Duration) uint64 {
	if timeout <= 0 {
		return 0
	}

	return uint64((timeout + time.Second - 1) / time.Second)
}
