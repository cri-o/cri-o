package server

import (
	"io/ioutil"
	"os"
	"testing"

	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"

	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

const (
	defaultDNSPath = "/etc/resolv.conf"
	testDNSPath    = "fixtures/resolv_test.conf"
	dnsPath        = "fixtures/resolv.conf"
)

func TestParseDNSOptions(t *testing.T) {
	testCases := []struct {
		Servers, Searches, Options []string
		Path                       string
		Want                       string
	}{
		{
			[]string{},
			[]string{},
			[]string{},
			testDNSPath, defaultDNSPath,
		},
		{
			[]string{"cri-o.io", "github.com"},
			[]string{"192.30.253.113", "192.30.252.153"},
			[]string{"timeout:5", "attempts:3"},
			testDNSPath, dnsPath,
		},
	}

	for _, c := range testCases {
		if err := parseDNSOptions(c.Servers, c.Searches,
			c.Options, c.Path); err != nil {
			t.Error(err)
		}

		expect, _ := ioutil.ReadFile(c.Want) // nolint: errcheck
		result, _ := ioutil.ReadFile(c.Path) // nolint: errcheck
		if string(expect) != string(result) {
			t.Errorf("expect %v: \n but got : %v", string(expect), string(result))
		}
		os.Remove(c.Path)
	}
}

func TestMergeEnvs(t *testing.T) {
	configImage := &v1.Image{
		Config: v1.ImageConfig{
			Env: []string{"VAR1=1", "VAR2=2"},
		},
	}

	configKube := []*pb.KeyValue{
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
	keysDir, err := ioutil.TempDir("", "temp-keys-1")
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

	err = ioutil.WriteFile(keysDir+"/private.key", privateKeyBytes, 0644)
	if err != nil {
		t.Fatalf("Unable to write a private key %v", err)
	}

	cc, err := getDecryptionKeys(keysDir)

	if err != nil && cc.DecryptConfig != nil {
		t.Fatalf("Unable to find the expected keys")
	}
}
