package server

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	libconfig "github.com/cri-o/cri-o/pkg/config"
)

func TestSplitPodCIDRS(t *testing.T) {
	t.Parallel()

	got := splitPodCIDRS("10.0.0.0/24, 2001:db8::/64")
	require.Equal(t, []string{"10.0.0.0/24", "2001:db8::/64"}, got)
}

func TestUpdateRuntimeConfig_InvalidCIDR(t *testing.T) {
	t.Parallel()

	s := &Server{
		config: *defaultConfigForUpdateRuntimeConfigTest(t),
	}

	_, err := s.UpdateRuntimeConfig(context.Background(), &types.UpdateRuntimeConfigRequest{
		RuntimeConfig: &types.RuntimeConfig{
			NetworkConfig: &types.NetworkConfig{
				PodCidr: "not-a-cidr",
			},
		},
	})
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	require.Equal(t, codes.InvalidArgument, st.Code())
}

func TestUpdateRuntimeConfig_TemplateWrite(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	tpl := filepath.Join(dir, "tpl.conflist.in")
	err := os.WriteFile(tpl, []byte(`podCIDR={{.PodCIDR}}`), 0o644)
	require.NoError(t, err)

	cfg := defaultConfigForUpdateRuntimeConfigTest(t)
	cfg.NetworkDir = dir
	cfg.CNIConfigWriteTemplate = tpl
	cfg.CNIConfigWriteFilename = "out.conflist"

	s := &Server{config: *cfg}

	_, err = s.UpdateRuntimeConfig(context.Background(), &types.UpdateRuntimeConfigRequest{
		RuntimeConfig: &types.RuntimeConfig{
			NetworkConfig: &types.NetworkConfig{
				PodCidr: "10.1.0.0/16",
			},
		},
	})
	require.NoError(t, err)

	b, err := os.ReadFile(filepath.Join(dir, "out.conflist"))
	require.NoError(t, err)
	require.Equal(t, "podCIDR=10.1.0.0/16", string(b))

	require.Equal(t, []string{"10.1.0.0/16"}, s.getPodCIDRRanges())
}

func TestUpdateRuntimeConfig_ClearsPodCIDR(t *testing.T) {
	t.Parallel()

	s := &Server{config: *defaultConfigForUpdateRuntimeConfigTest(t)}

	_, err := s.UpdateRuntimeConfig(context.Background(), &types.UpdateRuntimeConfigRequest{
		RuntimeConfig: &types.RuntimeConfig{
			NetworkConfig: &types.NetworkConfig{
				PodCidr: "10.1.0.0/16",
			},
		},
	})
	require.NoError(t, err)
	require.Len(t, s.getPodCIDRRanges(), 1)

	_, err = s.UpdateRuntimeConfig(context.Background(), &types.UpdateRuntimeConfigRequest{
		RuntimeConfig: &types.RuntimeConfig{
			NetworkConfig: &types.NetworkConfig{
				PodCidr: "",
			},
		},
	})
	require.NoError(t, err)
	require.Nil(t, s.getPodCIDRRanges())
}

func defaultConfigForUpdateRuntimeConfigTest(t *testing.T) *libconfig.Config {
	t.Helper()

	c, err := libconfig.DefaultConfig()
	require.NoError(t, err)

	return c
}
