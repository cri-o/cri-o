package oci

import (
	"context"
	"net"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/containerd/ttrpc"
	"github.com/containerd/typeurl"
	"google.golang.org/protobuf/types/known/anypb"
)

// The helpers below implement the client side of the containerd shim-v2
// protocol, matching containerd v1.7's runtime/v2/shim package.

const (
	// shimNamespace is the containerd namespace passed to shims.
	shimNamespace = "default"

	// shimNamespaceTTRPCHeader is the ttrpc metadata key shims read the
	// namespace from.
	shimNamespaceTTRPCHeader = "containerd-namespace-ttrpc"

	// shimConnectTimeout is the timeout for connecting to the shim socket.
	shimConnectTimeout = 100 * time.Second
)

// shimBootstrapParams is the JSON payload a shim writes to stdout on "start".
type shimBootstrapParams struct {
	// Version is the version of shim parameters (expected 2 for shim v2).
	Version int `json:"version"`
	// Address is the address to use to connect to the shim.
	Address string `json:"address"`
	// Protocol is either TTRPC or GRPC.
	Protocol string `json:"protocol"`
}

// withShimNamespace sets the containerd namespace on the outgoing ttrpc
// metadata of ctx.
func withShimNamespace(ctx context.Context) context.Context {
	md, ok := ttrpc.GetMetadata(ctx)
	if ok {
		md = md.Clone()
	} else {
		md = ttrpc.MD{}
	}

	md.Set(shimNamespaceTTRPCHeader, shimNamespace)

	return ttrpc.WithMetadata(ctx, md)
}

// shimCommand returns the command to run a shim-v2 binary with the given
// arguments in the bundle directory.
func shimCommand(ctx context.Context, runtime, bundle string, args []string) (*exec.Cmd, error) {
	self, err := os.Executable()
	if err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(ctx, runtime, append([]string{
		"-namespace", shimNamespace,
		"-address", "",
		"-publish-binary", self,
	}, args...)...)
	cmd.Dir = bundle
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	cmd.Env = append(
		os.Environ(),
		"GOMAXPROCS=2",
		"MAX_SHIM_VERSION=2",
		"TTRPC_ADDRESS=",
		"GRPC_ADDRESS=",
		"NAMESPACE="+shimNamespace,
	)

	return cmd, nil
}

// shimConnect connects to a shim socket address. Addresses without the
// unix:// prefix refer to abstract sockets.
func shimConnect(address string) (net.Conn, error) {
	path, ok := strings.CutPrefix(address, "unix://")
	if !ok {
		path = "\x00" + address
	}

	return net.DialTimeout("unix", path, shimConnectTimeout)
}

// toProtoAny converts a typeurl.Any into its protobuf representation.
func toProtoAny(from typeurl.Any) *anypb.Any {
	if from == nil {
		return nil
	}

	if pbany, ok := from.(*anypb.Any); ok {
		return pbany
	}

	return &anypb.Any{
		TypeUrl: from.GetTypeUrl(),
		Value:   from.GetValue(),
	}
}
