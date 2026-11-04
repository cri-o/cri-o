package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"time"

	"github.com/containers/conmon-rs/internal/proto"
)

// ServerVersion contains the version data of the server instance.
type ServerVersion struct {
	// The current version.
	Version string `json:"version"`

	// The tag of the build, see VersionResponse.Tag.
	Tag string `json:"tag"`

	// The git commit SHA of the build.
	Commit string `json:"commit"`

	// The build date string.
	BuildDate string `json:"build_date"` //nolint:tagliatelle // Rust's serde will use that format.

	// The target triple string.
	Target string `json:"target"`

	// The used Rust version.
	RustVersion string `json:"rust_version"` //nolint:tagliatelle // Rust's serde will use that format.

	// The used Cargo version.
	CargoVersion string `json:"cargo_version"` //nolint:tagliatelle // Rust's serde will use that format.

	// The cargo dependency tree. Always empty here, Version and
	// VersionContext do not ask for it. Use ConmonClient.Version with
	// Verbose.
	CargoTree string `json:"cargo_tree"` //nolint:tagliatelle // Rust's serde will use that format.

	// The optional features the server supports, see the Feature constants
	// like [FeatureSdNotify].
	Features []string `json:"features"`
}

// versionTimeout bounds running the binary for its version.
const versionTimeout = 10 * time.Second

// Version can be used to retrieve the server version without requiring a
// running Server. Running the binary may take up to 10 seconds.
//
// If binaryPath is empty, then the default binary will be used.
// If the server doesn't support it, then an ErrUnsupported error is returned.
func Version(binaryPath string) (res *ServerVersion, err error) {
	// A hanging binary must not block the caller forever.
	ctx, cancel := context.WithTimeout(context.Background(), versionTimeout)
	defer cancel()

	return VersionContext(ctx, binaryPath)
}

// VersionContext works like Version, the context bounds running the binary
// instead of the fixed timeout.
func VersionContext(ctx context.Context, binaryPath string) (res *ServerVersion, err error) {
	if binaryPath == "" {
		binaryPath = binaryName
	}

	const arg = "--version-json"

	cmd := exec.CommandContext(ctx, binaryPath, arg)
	// A child holding the pipes open must not keep the wait going after the
	// kill.
	cmd.WaitDelay = time.Second

	// Only stdout carries the JSON, the error output of a failed run is kept
	// by the error.
	data, err := cmd.Output()
	if err != nil {
		var stderr []byte

		if exitErr := (&exec.ExitError{}); errors.As(err, &exitErr) {
			stderr = exitErr.Stderr
		}

		if bytes.Contains(stderr, []byte("error: unexpected argument")) {
			return nil, ErrUnsupported
		}

		// Bounded like the server errors of New, the end has the error. A
		// binary which could not run at all wrote nothing.
		output := errorTail(string(stderr), false)

		if output != "" {
			return nil, fmt.Errorf("run `%s %s`: %w: %s", binaryPath, arg, err, output)
		}

		return nil, fmt.Errorf("run `%s %s`: %w", binaryPath, arg, err)
	}

	if err := json.Unmarshal(data, &res); err != nil {
		return nil, fmt.Errorf("unmarshal result: %w", err)
	}

	return res, nil
}

// VersionConfig is the configuration for calling the Version method.
type VersionConfig struct {
	// Verbose specifies verbose version output.
	Verbose bool
}

// VersionResponse is the response of the Version method.
type VersionResponse struct {
	// ProcessID is the PID of the server.
	ProcessID uint32

	// Version is the actual version string of the server.
	Version string

	// Tag is the BUILD_TAG of the build or else what
	// `git describe --tags --always --dirty` told, like
	// v1.2.1-44-gb172f5bd. It is empty for the static binaries and the
	// Copr RPMs, releases included, whose builds pass no tag. Use Version
	// and Commit to identify a build.
	Tag string

	// Commit is git commit SHA of the build.
	Commit string

	// BuildDate is the date of build.
	BuildDate string

	// Target is the build triple.
	Target string

	// RustVersion is the used Rust version.
	RustVersion string

	// CargoVersion is the used Cargo version.
	CargoVersion string

	// CargoTree is the used dependency tree.
	// Only set if request was in verbose mode.
	CargoTree string

	// Features are the optional features the server supports, see the
	// Feature constants like [FeatureSdNotify].
	Features []string
}

// FeatureSdNotify is reported by servers which relay sd_notify messages,
// see CreateContainerConfig.SdNotifySocket. Since v1.1.0.
const FeatureSdNotify = "sdnotify"

// FeaturePortForward is reported by servers which forward ports, see
// ServePortForwardContainer. Since v1.2.0.
const FeaturePortForward = "portforward"

// FeatureIdleTimeout is reported by servers which apply the IdleTimeout of
// ServeExecContainerConfig and ServeAttachContainerConfig. Others ignore it.
// Every server with FeaturePortForward applies the one of
// ServePortForwardContainerConfig. Since v1.2.0.
const FeatureIdleTimeout = "idletimeout"

// FeatureStreamingProtocols is reported by servers which serve exec and
// attach sessions over SPDY and the WebSocket protocols before
// v5.channel.k8s.io, see ServeExecContainer. Others only speak
// v5.channel.k8s.io. Since v1.2.0.
const FeatureStreamingProtocols = "streamingprotocols"

// FeatureJournaldFields is reported by servers which write the
// ContainerName, Tag and Labels of a ContainerLogDriver to the journal.
// Others ignore them. Since v1.2.1.
const FeatureJournaldFields = "journaldfields"

// SupportsFeature reports whether the server reports the feature, see the
// Feature constants. New records the features of the server it starts or
// connects to, which can be older than the binary on disk: a server keeps
// running across restarts of its callers and upgrades. A call which dials a
// new connection to the server, because the last one broke, records them
// again once the server told them. SupportsFeature does not ask the server
// itself, so it reports the features of a server which got replaced until a
// call reconnected, and keeps them if the new server did not tell its
// features within a few seconds. Methods which need a feature ask the server
// again then.
func (c *ConmonClient) SupportsFeature(feature string) bool {
	return c.serverFeatures()[feature]
}

// requireServerFeature fails with ErrUnsupported if the server does not
// report the feature. It replaces a broken connection first and only relies
// on features which got recorded on the connection it uses, otherwise it
// asks the server, also if the client which dialed the connection is still
// asking or failed to. It returns a reference of the RPC client of the
// checked connection, which the caller makes its call on and releases then,
// so that the call goes to the checked server: once that connection broke,
// the call fails rather than reaching another server.
func (c *ConmonClient) requireServerFeature(
	ctx context.Context, feature, usage string,
) (_ proto.Conmon, retErr error) {
	conn, rpcClient, err := c.clientConn(ctx)
	if err != nil {
		return proto.Conmon{}, fmt.Errorf("create RPC client: %w", err)
	}

	defer func() {
		if retErr != nil {
			rpcClient.Release()
		}
	}()

	features, current := c.featuresOn(conn)
	if !current {
		// Asked without holding the lock, a server which does not answer
		// must not block other callers beyond their own context.
		resp, err := c.version(ctx, rpcClient)
		if err != nil {
			return proto.Conmon{}, fmt.Errorf("get server version: %w", err)
		}

		features = c.recordFeatures(conn, resp.Features)
	}

	if !features[feature] {
		return proto.Conmon{}, fmt.Errorf(
			"%s needs the %q feature of the server: %w",
			usage,
			feature,
			ErrUnsupported,
		)
	}

	return rpcClient, nil
}

// serverFeatures returns the recorded features of the server, nil if they
// are unknown. They may belong to a server which got replaced, see
// featuresOn.
func (c *ConmonClient) serverFeatures() map[string]bool {
	c.featuresMu.Lock()
	defer c.featuresMu.Unlock()

	return c.features
}

// featuresOn returns the recorded features of the server, and whether they
// got recorded on the connection.
func (c *ConmonClient) featuresOn(conn *rpcConn) (map[string]bool, bool) {
	c.featuresMu.Lock()
	defer c.featuresMu.Unlock()

	return c.features, conn != nil && c.features != nil && c.featuresConn == conn
}

// setFeatures records the features the server reported on the connection in
// use. Only for New, which makes no other calls meanwhile.
func (c *ConmonClient) setFeatures(list []string) {
	c.connMu.Lock()
	defer c.connMu.Unlock()

	c.recordFeaturesLocked(c.conn, list)
}

// recordFeatures records the features the server reported on the
// connection, unless another one replaced it meanwhile, and returns them.
func (c *ConmonClient) recordFeatures(conn *rpcConn, list []string) map[string]bool {
	c.connMu.Lock()
	defer c.connMu.Unlock()

	if c.conn != conn {
		return featureSet(list)
	}

	return c.recordFeaturesLocked(conn, list)
}

// recordFeaturesLocked records the features the server reported on the
// connection and returns them. The caller holds connMu, which always gets
// taken before featuresMu.
func (c *ConmonClient) recordFeaturesLocked(conn *rpcConn, list []string) map[string]bool {
	features := featureSet(list)

	c.featuresMu.Lock()
	defer c.featuresMu.Unlock()

	c.features = features
	c.featuresConn = conn

	return features
}

// featureSet converts the features the server reported.
func featureSet(list []string) map[string]bool {
	features := make(map[string]bool, len(list))
	for _, feature := range list {
		features[feature] = true
	}

	return features
}

// Version can be used to retrieve all available version information. A nil
// config asks for the version without the verbose fields.
func (c *ConmonClient) Version(
	ctx context.Context, cfg *VersionConfig,
) (_ *VersionResponse, retErr error) {
	ctx, span := c.startSpan(ctx, "Version")
	defer endSpan(span, &retErr)

	return call(ctx, c, proto.Conmon.Version, versionRequest(cfg), readVersionResponse)
}

// version asks the server for its version on the RPC client, without the
// verbose fields.
func (c *ConmonClient) version(
	ctx context.Context,
	rpcClient proto.Conmon,
) (*VersionResponse, error) {
	return callOn(ctx, c, rpcClient, proto.Conmon.Version, versionRequest(nil), readVersionResponse)
}

// versionRequest fills the request of a Version call.
func versionRequest(cfg *VersionConfig) func(proto.Conmon_VersionRequest) error {
	return func(req proto.Conmon_VersionRequest) error {
		req.SetVerbose(cfg != nil && cfg.Verbose)

		return nil
	}
}

// readVersionResponse converts the response of a Version call.
func readVersionResponse(response proto.Conmon_VersionResponse) (*VersionResponse, error) {
	version, err := response.Version()
	if err != nil {
		return nil, fmt.Errorf("get version: %w", err)
	}

	tag, err := response.Tag()
	if err != nil {
		return nil, fmt.Errorf("get tag: %w", err)
	}

	commit, err := response.Commit()
	if err != nil {
		return nil, fmt.Errorf("get commit: %w", err)
	}

	buildDate, err := response.BuildDate()
	if err != nil {
		return nil, fmt.Errorf("get build date: %w", err)
	}

	target, err := response.Target()
	if err != nil {
		return nil, fmt.Errorf("get target: %w", err)
	}

	rustVersion, err := response.RustVersion()
	if err != nil {
		return nil, fmt.Errorf("get rust version: %w", err)
	}

	cargoVersion, err := response.CargoVersion()
	if err != nil {
		return nil, fmt.Errorf("get cargo version: %w", err)
	}

	cargoTree, err := response.CargoTree()
	if err != nil {
		return nil, fmt.Errorf("get cargo tree: %w", err)
	}

	featureList, err := response.Features()
	if err != nil {
		return nil, fmt.Errorf("get features: %w", err)
	}

	features := make([]string, 0, featureList.Len())

	for i := range featureList.Len() {
		feature, err := featureList.At(i)
		if err != nil {
			return nil, fmt.Errorf("get feature %d: %w", i, err)
		}

		features = append(features, feature)
	}

	return &VersionResponse{
		ProcessID:    response.ProcessId(),
		Version:      version,
		Tag:          tag,
		Commit:       commit,
		BuildDate:    buildDate,
		Target:       target,
		RustVersion:  rustVersion,
		CargoVersion: cargoVersion,
		CargoTree:    cargoTree,
		Features:     features,
	}, nil
}
