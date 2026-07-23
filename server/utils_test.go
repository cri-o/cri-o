package server

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"os"
	"testing"

	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"go.podman.io/storage/pkg/mount"

	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func TestMergeEnvs(t *testing.T) {
	cases := []struct {
		name        string
		imageConfig *v1.Image
		kubeEnvs    []*types.KeyValue
		expected    []string
	}{
		{
			name: "both provided, kubeEnvs overrides",
			imageConfig: &v1.Image{
				Config: v1.ImageConfig{
					Env: []string{"VAR1=1", "VAR2=2"},
				},
			},
			kubeEnvs: []*types.KeyValue{
				{Key: "VAR2", Value: []byte("3")},
				{Key: "VAR3", Value: []byte("3")},
			},
			expected: []string{"VAR2=3", "VAR3=3", "VAR1=1"},
		},
		{
			name: "kubeEnvs is nil",
			imageConfig: &v1.Image{
				Config: v1.ImageConfig{
					Env: []string{"VAR1=1", "VAR2=2"},
				},
			},
			kubeEnvs: nil,
			expected: []string{"VAR1=1", "VAR2=2"},
		},
		{
			name:        "imageConfig is nil",
			imageConfig: nil,
			kubeEnvs: []*types.KeyValue{
				{Key: "VAR1", Value: []byte("1")},
			},
			expected: []string{"VAR1=1"},
		},
		{
			name: "kubeEnvs with empty key skipped",
			imageConfig: &v1.Image{
				Config: v1.ImageConfig{
					Env: []string{"VAR1=1"},
				},
			},
			kubeEnvs: []*types.KeyValue{
				{Key: "", Value: []byte("3")},
				{Key: "VAR2", Value: []byte("2")},
			},
			expected: []string{"VAR2=2", "VAR1=1"},
		},
		{
			name: "imageConfig with invalid env skipped",
			imageConfig: &v1.Image{
				Config: v1.ImageConfig{
					Env: []string{"INVALID_NO_EQUALS", "=NO_KEY", "VAR1=1"},
				},
			},
			kubeEnvs: []*types.KeyValue{
				{Key: "VAR2", Value: []byte("2")},
			},
			expected: []string{"VAR2=2", "VAR1=1"},
		},
		{
			name: "valid envs with empty values",
			imageConfig: &v1.Image{
				Config: v1.ImageConfig{
					Env: []string{"VAR1=", "VAR2=2"},
				},
			},
			kubeEnvs: []*types.KeyValue{
				{Key: "VAR3", Value: []byte("")},
			},
			expected: []string{"VAR3=", "VAR1=", "VAR2=2"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mergedEnvs := mergeEnvs(tc.imageConfig, tc.kubeEnvs)

			if len(mergedEnvs) != len(tc.expected) {
				t.Fatalf("Expected %d env vars, found %d", len(tc.expected), len(mergedEnvs))
			}

			expectedMap := make(map[string]bool)
			for _, e := range tc.expected {
				expectedMap[e] = true
			}

			for _, env := range mergedEnvs {
				if !expectedMap[env] {
					t.Fatalf("Unexpected env var found: %s", env)
				}
			}
		})
	}
}

func TestGetDecryptionKeys(t *testing.T) {
	keysDir := t.TempDir()

	// Create a RSA private key
	privateKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		t.Fatalf("Unable to generate a private key %v", err)
	}

	privateKeyBytes := x509.MarshalPKCS1PrivateKey(privateKey)

	err = os.WriteFile(keysDir+"/private.key", privateKeyBytes, 0o644)
	if err != nil {
		t.Fatalf("Unable to write a private key %v", err)
	}

	cc, err := getDecryptionKeys(keysDir)
	if err != nil && cc != nil {
		t.Fatalf("Unable to find the expected keys")
	}
}

func TestGetSourceMount(t *testing.T) {
	mountinfo := []*mount.Info{
		{Mountpoint: "/"},
		{Mountpoint: "/sys"},
		{Mountpoint: "/sys/fs/cgroup"},
		{Mountpoint: "/other/dir"},
	}

	cases := []struct {
		in, out string
		err     bool
	}{
		{in: "/sys/fs/cgroup/aaa/bbb", out: "/sys/fs/cgroup"},
		{in: "/some/weird/dir", out: "/"},
		{in: "/sys/fs/foo/bar", out: "/sys"},
		{in: "/other/dir/yeah", out: "/other/dir"},
		{in: "/other/dir", out: "/other/dir"},
		{in: "/other", out: "/"},
		{in: "/", out: "/"},
		{in: "bad/path", err: true},
		{in: "", err: true},
	}

	for _, tc := range cases {
		out, _, err := getSourceMount(tc.in, mountinfo)
		if tc.err {
			if err == nil {
				t.Errorf("input: %q, expected error, got nil", tc.in)
			}

			continue
		}

		if err != nil {
			t.Errorf("input: %q, expected no error, got %v", tc.in, err)

			continue
		}

		if out != tc.out {
			t.Errorf("input: %q, expected %q, got %q", tc.in, tc.out, out)
		}
	}
}
