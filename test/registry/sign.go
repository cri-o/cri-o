package main

import (
	"bytes"
	"crypto/mldsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sigstore/sigstore/pkg/cryptoutils"
	"github.com/sigstore/sigstore/pkg/signature"
	"github.com/urfave/cli/v2"
)

const (
	mediaTypeOCIManifest = "application/vnd.oci.image.manifest.v1+json"
	mediaTypeOCIConfig   = "application/vnd.oci.image.config.v1+json"
	// mediaTypeSimpleSigning and signatureAnnotation are the sigstore
	// signature layer media type and annotation read by containers/image.
	mediaTypeSimpleSigning = "application/vnd.dev.cosign.simplesigning.v1+json"
	signatureAnnotation    = "dev.cosignproject.cosign/signature"
)

// generateKey writes a new ML-DSA private key and its public key.
func generateKey(c *cli.Context) error {
	params, ok := mldsaParameters[c.String("algorithm")]
	if !ok {
		return fmt.Errorf("unsupported algorithm %q", c.String("algorithm"))
	}

	key, err := mldsa.GenerateKey(params)
	if err != nil {
		return fmt.Errorf("generate key: %w", err)
	}

	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return fmt.Errorf("marshal key: %w", err)
	}

	publicPEM, err := cryptoutils.MarshalPublicKeyToPEM(key.Public())
	if err != nil {
		return fmt.Errorf("marshal public key: %w", err)
	}

	prefix := c.String("prefix")

	return errors.Join(
		os.WriteFile(
			prefix+".key",
			pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}),
			0o600,
		),
		os.WriteFile(prefix+".pub", publicPEM, 0o644),
	)
}

// registryClient talks to the test registry over mTLS, using the files of a
// certs.d directory.
type registryClient struct {
	client  *http.Client
	baseURL string
}

func newRegistryClient(address, certsDir string) (*registryClient, error) {
	caPEM, err := os.ReadFile(filepath.Join(certsDir, "ca.crt"))
	if err != nil {
		return nil, fmt.Errorf("read CA: %w", err)
	}

	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(caPEM) {
		return nil, errors.New("no certificates found in CA")
	}

	clientCert, err := tls.LoadX509KeyPair(
		filepath.Join(certsDir, "client.cert"),
		filepath.Join(certsDir, "client.key"),
	)
	if err != nil {
		return nil, fmt.Errorf("load client certificate: %w", err)
	}

	return &registryClient{
		client: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					RootCAs:      roots,
					Certificates: []tls.Certificate{clientCert},
					MinVersion:   tls.VersionTLS13,
				},
			},
		},
		baseURL: "https://" + address + "/v2/",
	}, nil
}

func (r *registryClient) do(
	method, path, contentType string,
	body []byte,
	accept ...string,
) ([]byte, error) {
	req, err := http.NewRequest(method, r.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	for _, a := range accept {
		req.Header.Add("Accept", a)
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("%s %s: %s: %s", method, path, resp.Status, respBody)
	}

	return respBody, nil
}

type descriptor struct {
	MediaType   string            `json:"mediaType"`
	Digest      string            `json:"digest"`
	Size        int               `json:"size"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

// putBlob uploads data as a blob of repo and returns its descriptor.
func (r *registryClient) putBlob(
	repo, mediaType string,
	data []byte,
	annotations map[string]string,
) (*descriptor, error) {
	digest := sha256Digest(data)
	if _, err := r.do(
		http.MethodPost,
		repo+"/blobs/uploads/?digest="+digest,
		"application/octet-stream",
		data,
	); err != nil {
		return nil, err
	}

	return &descriptor{
		MediaType:   mediaType,
		Digest:      digest,
		Size:        len(data),
		Annotations: annotations,
	}, nil
}

func sha256Digest(data []byte) string {
	sum := sha256.Sum256(data)

	return "sha256:" + hex.EncodeToString(sum[:])
}

// sign signs an image in the test registry with an ML-DSA key and stores the
// signature as a sigstore attachment, as containers/image and cosign do.
func sign(c *cli.Context) error {
	address := c.String("address")

	repo, tag, ok := strings.Cut(c.String("image"), ":")
	if !ok {
		return fmt.Errorf("image %q has no tag", c.String("image"))
	}

	keyPEM, err := os.ReadFile(c.String("key"))
	if err != nil {
		return fmt.Errorf("read key: %w", err)
	}

	block, _ := pem.Decode(keyPEM)
	if block == nil {
		return errors.New("no PEM data found in key")
	}

	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("parse key: %w", err)
	}

	mldsaKey, ok := key.(*mldsa.PrivateKey)
	if !ok {
		return fmt.Errorf("key is %T, not ML-DSA", key)
	}

	signer, err := signature.LoadMLDSASigner(mldsaKey)
	if err != nil {
		return fmt.Errorf("load signer: %w", err)
	}

	client, err := newRegistryClient(address, c.String("certs-dir"))
	if err != nil {
		return err
	}

	manifest, err := client.do(http.MethodGet, repo+"/manifests/"+tag, "", nil,
		mediaTypeOCIManifest,
		"application/vnd.docker.distribution.manifest.v2+json",
	)
	if err != nil {
		return fmt.Errorf("get manifest: %w", err)
	}

	manifestDigest := sha256Digest(manifest)

	payload, err := json.Marshal(map[string]any{
		"critical": map[string]any{
			"type":     "cosign container image signature",
			"image":    map[string]string{"docker-manifest-digest": manifestDigest},
			"identity": map[string]string{"docker-reference": address + "/" + repo + ":" + tag},
		},
		"optional": map[string]any{
			"creator":   "cri-o test registry",
			"timestamp": time.Now().Unix(),
		},
	})
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	sig, err := signer.SignMessage(bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("sign payload: %w", err)
	}

	layer, err := client.putBlob(repo, mediaTypeSimpleSigning, payload, map[string]string{
		signatureAnnotation: base64.StdEncoding.EncodeToString(sig),
	})
	if err != nil {
		return fmt.Errorf("upload signature payload: %w", err)
	}

	configJSON, err := json.Marshal(map[string]any{
		"architecture": "",
		"os":           "",
		"rootfs":       map[string]any{"type": "layers", "diff_ids": []string{layer.Digest}},
	})
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	config, err := client.putBlob(repo, mediaTypeOCIConfig, configJSON, nil)
	if err != nil {
		return fmt.Errorf("upload config: %w", err)
	}

	attachment, err := json.Marshal(map[string]any{
		"schemaVersion": 2,
		"mediaType":     mediaTypeOCIManifest,
		"config":        config,
		"layers":        []*descriptor{layer},
	})
	if err != nil {
		return fmt.Errorf("marshal attachment manifest: %w", err)
	}

	attachmentTag := strings.Replace(manifestDigest, ":", "-", 1) + ".sig"
	if _, err := client.do(
		http.MethodPut,
		repo+"/manifests/"+attachmentTag,
		mediaTypeOCIManifest,
		attachment,
	); err != nil {
		return fmt.Errorf("put attachment manifest: %w", err)
	}

	fmt.Printf("Signed %s/%s:%s (%s) as %s\n", address, repo, tag, manifestDigest, attachmentTag)

	return nil
}
