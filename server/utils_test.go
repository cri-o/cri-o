package server

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"os"
	"testing"

	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func TestMergeEnvs(t *testing.T) {
	configImage := &v1.Image{
		Config: v1.ImageConfig{
			Env: []string{"VAR1=1", "VAR2=2"},
		},
	}

	configKube := []*types.KeyValue{
		{
			Key:   "VAR2",
			Value: "3",
		},
		{
			Key:   "VAR3",
			Value: "3",
		},
	}

	mergedEnvs := mergeEnvs(configImage, configKube)

	if len(mergedEnvs) != 3 {
		t.Fatalf("Expected 3 env var, VAR1=1, VAR2=3 and VAR3=3, found %d", len(mergedEnvs))
	}
	for _, env := range mergedEnvs {
		if env != "VAR1=1" && env != "VAR2=3" && env != "VAR3=3" {
			t.Fatalf("Expected VAR1=1 or VAR2=3 or VAR3=3, found %s", env)
		}
	}
}

func TestGetDecryptionKeys(t *testing.T) {
	keysDir, err := os.MkdirTemp("", "temp-keys-1")
	if err != nil {
		t.Fatalf("Unable to create a temporary directory %v", err)
	}
	defer os.RemoveAll(keysDir)

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
