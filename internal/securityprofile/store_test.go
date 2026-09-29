package securityprofile_test

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/opencontainers/go-digest"
	ispec "github.com/opencontainers/image-spec/specs-go/v1"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
	"go.podman.io/image/v5/docker"
	"go.podman.io/image/v5/docker/reference"
	imagetypes "go.podman.io/image/v5/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/ociartifact"
	"github.com/cri-o/cri-o/internal/securityprofile"
)

const (
	denyChmod = `{
		"defaultAction": "SCMP_ACT_ALLOW",
		"syscalls": [{"names": ["chmod"], "action": "SCMP_ACT_ERRNO", "errnoRet": 1}]
	}`
	repository = "registry.example.com/profiles/deny-chmod"
	maxSize    = 4096

	acceptPolicy = `{"default": [{"type": "insecureAcceptAnything"}]}`
	rejectPolicy = `{"default": [{"type": "reject"}]}`
	otherPolicy  = `{
		"default": [{"type": "insecureAcceptAnything"}],
		"transports": {"docker": {"other.example.com": [{"type": "reject"}]}}
	}`
)

// artifact describes the content of a profile artifact.
type artifact struct {
	configMediaType string
	configData      []byte
	artifactType    string
	layerMediaType  string
	layers          [][]byte
	urls            []string
	annotations     map[string]string
}

func seccompArtifact(profile string) *artifact {
	return &artifact{
		configMediaType: securityprofile.SeccompConfigMediaType,
		configData:      []byte("{}"),
		artifactType:    securityprofile.SeccompConfigMediaType,
		layerMediaType:  "application/json",
		layers:          [][]byte{[]byte(profile)},
	}
}

func descriptor(mediaType string, data []byte) ispec.Descriptor {
	return ispec.Descriptor{
		MediaType: mediaType,
		Digest:    digest.FromBytes(data),
		Size:      int64(len(data)),
	}
}

func (a *artifact) manifest() []byte {
	m := ispec.Manifest{
		SchemaVersion: 2,
		MediaType:     ispec.MediaTypeImageManifest,
		ArtifactType:  a.artifactType,
		Config:        descriptor(a.configMediaType, a.configData),
		Annotations:   a.annotations,
	}

	for _, layer := range a.layers {
		desc := descriptor(a.layerMediaType, layer)
		desc.URLs = a.urls
		m.Layers = append(m.Layers, desc)
	}

	raw, err := json.Marshal(m)
	Expect(err).NotTo(HaveOccurred())

	return raw
}

func (a *artifact) digest() digest.Digest {
	return digest.FromBytes(a.manifest())
}

func (a *artifact) ref() string {
	return repository + "@" + a.digest().String()
}

func (a *artifact) blobs() map[digest.Digest][]byte {
	blobs := map[digest.Digest][]byte{digest.FromBytes(a.configData): a.configData}
	for _, layer := range a.layers {
		blobs[digest.FromBytes(layer)] = layer
	}

	return blobs
}

// storeLayout adds the artifact to the OCI layout at root, as a tool filling
// an additional store would.
func (a *artifact) storeLayout(root string) {
	raw := a.manifest()

	for _, data := range append([][]byte{a.configData, raw}, a.layers...) {
		dgst := digest.FromBytes(data)
		dir := filepath.Join(root, "blobs", dgst.Algorithm().String())
		Expect(os.MkdirAll(dir, 0o755)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(dir, dgst.Encoded()), data, 0o644)).To(Succeed())
	}

	indexPath := filepath.Join(root, "index.json")
	indexData, err := os.ReadFile(indexPath)
	Expect(err).NotTo(HaveOccurred())

	var index ispec.Index
	Expect(json.Unmarshal(indexData, &index)).To(Succeed())

	desc := descriptor(ispec.MediaTypeImageManifest, raw)
	desc.Annotations = map[string]string{
		ispec.AnnotationRefName: "other.example.com/profile:latest",
	}
	index.Manifests = append(index.Manifests, desc)

	indexData, err = json.Marshal(index)
	Expect(err).NotTo(HaveOccurred())
	Expect(os.WriteFile(indexPath+".tmp", indexData, 0o644)).To(Succeed())
	Expect(os.Rename(indexPath+".tmp", indexPath)).To(Succeed())
}

// fakeRegistry serves artifacts by the digest of their manifest. It returns
// errors rather than failing the test, as it runs on the goroutine of a
// shared pull.
type fakeRegistry struct {
	mu        sync.Mutex
	artifacts map[digest.Digest]*artifact
	opens     []string
	sys       *imagetypes.SystemContext
	openErr   error
	blobErr   error

	// blobs replace the content of blobs by their digests.
	blobs map[digest.Digest][]byte

	// release blocks opening until it is closed, if set.
	release chan struct{}

	// onBlob runs before a blob is served, if set.
	onBlob func()
}

func (f *fakeRegistry) add(a *artifact) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.artifacts[a.digest()] = a
}

func (f *fakeRegistry) open(
	ctx context.Context,
	ref reference.Named,
	sys *imagetypes.SystemContext,
) (imagetypes.ImageSource, error) {
	f.mu.Lock()
	f.opens = append(f.opens, ref.String())
	f.sys = sys
	release := f.release
	f.mu.Unlock()

	if release != nil {
		select {
		case <-release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	if f.openErr != nil {
		return nil, f.openErr
	}

	imageRef, err := docker.NewReference(ref)
	if err != nil {
		return nil, err
	}

	digested, ok := ref.(reference.Digested)
	if !ok {
		return nil, errors.New("reference without digest")
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	// Serve by the SHA-256 digest, or by any other digest of a manifest.
	a := f.artifacts[digested.Digest()]
	if a == nil {
		for _, candidate := range f.artifacts {
			if digested.Digest().Algorithm().FromBytes(candidate.manifest()) == digested.Digest() {
				a = candidate
			}
		}
	}

	if a == nil {
		// The unparsed image rejects a manifest with another digest.
		for _, candidate := range f.artifacts {
			a = candidate
		}
	}

	if a == nil {
		return nil, errors.New("manifest unknown")
	}

	return &fakeSource{registry: f, ref: imageRef, artifact: a}, nil
}

func (f *fakeRegistry) openCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return len(f.opens)
}

type fakeSource struct {
	registry *fakeRegistry
	ref      imagetypes.ImageReference
	artifact *artifact
}

func (s *fakeSource) Reference() imagetypes.ImageReference { return s.ref }
func (s *fakeSource) Close() error                         { return nil }
func (s *fakeSource) HasThreadSafeGetBlob() bool           { return false }

func (s *fakeSource) GetManifest(
	context.Context, *digest.Digest,
) (raw []byte, mimeType string, err error) {
	return s.artifact.manifest(), ispec.MediaTypeImageManifest, nil
}

//nolint:gocritic // The signature is the one of types.ImageSource.
func (s *fakeSource) GetBlob(
	_ context.Context,
	info imagetypes.BlobInfo,
	_ imagetypes.BlobInfoCache,
) (io.ReadCloser, int64, error) {
	if s.registry.onBlob != nil {
		s.registry.onBlob()
	}

	if s.registry.blobErr != nil {
		return nil, 0, s.registry.blobErr
	}

	data, ok := s.artifact.blobs()[info.Digest]
	if !ok {
		return nil, 0, errors.New("blob unknown")
	}

	if replaced, ok := s.registry.blobs[info.Digest]; ok {
		data = replaced
	}

	return io.NopCloser(bytes.NewReader(data)), int64(len(data)), nil
}

func (s *fakeSource) GetSignatures(context.Context, *digest.Digest) ([][]byte, error) {
	return nil, nil
}

func (s *fakeSource) LayerInfosForCopy(
	context.Context, *digest.Digest,
) ([]imagetypes.BlobInfo, error) {
	return nil, nil
}

func expectInvalid(err error, substring string) {
	GinkgoHelper()
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(HavePrefix("SecurityProfileInvalid: "))
	Expect(err.Error()).To(ContainSubstring(substring))
}

func tarball(data []byte) []byte {
	var buf bytes.Buffer

	w := tar.NewWriter(&buf)
	Expect(
		w.WriteHeader(&tar.Header{Name: "profile.json", Mode: 0o644, Size: int64(len(data))}),
	).To(Succeed())
	_, err := w.Write(data)
	Expect(err).NotTo(HaveOccurred())
	Expect(w.Close()).To(Succeed())

	return buf.Bytes()
}

func writePolicy(content string) string {
	path := filepath.Join(GinkgoT().TempDir(), "policy.json")
	Expect(os.WriteFile(path, []byte(content), 0o644)).To(Succeed())

	return path
}

// The actual test suite.
var _ = t.Describe("Store", func() {
	var (
		sut        *securityprofile.Store
		dir        string
		registry   *fakeRegistry
		additional *ociartifact.Store
		addRoot    string
		sys        *imagetypes.SystemContext
		policies   map[string]string
		profile    *artifact
		logs       *bytes.Buffer
		ctx        context.Context
	)

	open := func() *securityprofile.Store {
		GinkgoHelper()

		store := securityprofile.New(ctx, &securityprofile.Options{
			Dir:        dir,
			MaxSize:    maxSize,
			Additional: additional,
			SystemContext: func(namespace string) (*imagetypes.SystemContext, error) {
				path, ok := policies[namespace]
				if !ok {
					return nil, errors.New("unknown namespace")
				}

				return &imagetypes.SystemContext{SignaturePolicyPath: path}, nil
			},
		})
		Expect(store.Available()).To(BeTrue())

		store.SetOpenSource(registry.open)

		return store
	}

	pull := func(ref string) (bool, error) {
		return sut.Pull(ctx, ref, types.SecurityProfileKind_Seccomp, sys, nil)
	}

	blobPath := func(data []byte) string {
		dgst := digest.FromBytes(data)

		return filepath.Join(dir, "blobs", dgst.Algorithm().String(), dgst.Encoded())
	}

	BeforeEach(func() {
		ctx = context.Background()
		logs = &bytes.Buffer{}
		logrus.SetOutput(logs)

		dir = t.MustTempDir("profiles")
		root := t.MustTempDir("artifacts")
		addPath := t.MustTempDir("additional")
		addRoot = filepath.Join(addPath, "artifacts")

		var err error

		additional, err = ociartifact.NewStore(
			root, []string{addPath}, &imagetypes.SystemContext{}, nil,
		)
		Expect(err).NotTo(HaveOccurred())

		profile = seccompArtifact(denyChmod)
		registry = &fakeRegistry{artifacts: map[digest.Digest]*artifact{}}
		registry.add(profile)

		sys = &imagetypes.SystemContext{
			SignaturePolicyPath: writePolicy(acceptPolicy),
			DockerAuthConfig:    &imagetypes.DockerAuthConfig{Username: "user", Password: "pass"},
		}
		policies = map[string]string{"default": sys.SignaturePolicyPath}

		sut = open()
	})

	t.Describe("Pull", func() {
		It("should pull a missing profile", func() {
			// Given
			// When
			cached, err := pull(profile.ref())

			// Then
			Expect(err).NotTo(HaveOccurred())
			Expect(cached).To(BeFalse())
			Expect(registry.opens).To(Equal([]string{profile.ref()}))
			Expect(registry.sys.DockerAuthConfig).To(Equal(sys.DockerAuthConfig))
			Expect(blobPath(profile.layers[0])).To(BeAnExistingFile())
		})

		It("should not contact the registry for a present profile", func() {
			// Given
			_, err := pull(profile.ref())
			Expect(err).NotTo(HaveOccurred())

			// When
			cached, err := pull(profile.ref())

			// Then
			Expect(err).NotTo(HaveOccurred())
			Expect(cached).To(BeTrue())
			Expect(registry.openCount()).To(Equal(1))
		})

		It("should keep the profiles across restarts", func() {
			// Given
			_, err := pull(profile.ref())
			Expect(err).NotTo(HaveOccurred())

			sut = open()

			// When
			cached, err := pull(profile.ref())

			// Then
			Expect(err).NotTo(HaveOccurred())
			Expect(cached).To(BeTrue())
			Expect(registry.openCount()).To(Equal(1))
		})

		for name, content := range map[string]string{
			"corrupt": `{"profiles": [`,
			"null":    `null`,
		} {
			It("should start over with a "+name+" index", func() {
				// Given
				_, err := pull(profile.ref())
				Expect(err).NotTo(HaveOccurred())
				Expect(os.WriteFile(filepath.Join(dir, "profiles.json"), []byte(content), 0o600)).
					To(Succeed())

				sut = open()

				// When
				cached, err := pull(profile.ref())

				// Then
				Expect(err).NotTo(HaveOccurred())
				Expect(cached).To(BeFalse())
				Expect(registry.openCount()).To(Equal(2))
			})
		}

		It("should pull a profile again whose blob went missing", func() {
			// Given
			_, err := pull(profile.ref())
			Expect(err).NotTo(HaveOccurred())
			Expect(os.Remove(blobPath(profile.layers[0]))).To(Succeed())

			// When
			cached, err := pull(profile.ref())

			// Then
			Expect(err).NotTo(HaveOccurred())
			Expect(cached).To(BeFalse())
			Expect(registry.openCount()).To(Equal(2))
		})

		It("should pull a profile again whose blob got corrupted", func() {
			// Given
			_, err := pull(profile.ref())
			Expect(err).NotTo(HaveOccurred())
			Expect(
				os.WriteFile(blobPath(profile.layers[0]), bytes.ToUpper(profile.layers[0]), 0o600),
			).
				To(Succeed())

			sut = open()

			// When
			cached, err := pull(profile.ref())

			// Then
			Expect(err).NotTo(HaveOccurred())
			Expect(cached).To(BeFalse())
			Expect(registry.openCount()).To(Equal(2))
		})

		It("should pull a present profile again for another signature policy", func() {
			// Given
			_, err := pull(profile.ref())
			Expect(err).NotTo(HaveOccurred())

			sys.SignaturePolicyPath = writePolicy(otherPolicy)

			// When
			cached, err := pull(profile.ref())

			// Then
			Expect(err).NotTo(HaveOccurred())
			Expect(cached).To(BeFalse())
			Expect(registry.openCount()).To(Equal(2))
		})

		It("should share verified profiles between identical signature policies", func() {
			// Given
			_, err := pull(profile.ref())
			Expect(err).NotTo(HaveOccurred())

			sys.SignaturePolicyPath = writePolicy(acceptPolicy)

			// When
			cached, err := pull(profile.ref())

			// Then
			Expect(err).NotTo(HaveOccurred())
			Expect(cached).To(BeTrue())
			Expect(registry.openCount()).To(Equal(1))
		})

		It("should pull a present profile again for another repository", func() {
			// Given
			_, err := pull(profile.ref())
			Expect(err).NotTo(HaveOccurred())

			mirror := "mirror.example.com/other@" + profile.digest().String()

			// When
			cached, err := pull(mirror)

			// Then
			Expect(err).NotTo(HaveOccurred())
			Expect(cached).To(BeFalse())
			Expect(registry.opens).To(Equal([]string{profile.ref(), mirror}))
		})

		It("should pull a present profile again once a key of the policy rotates", func() {
			// Given
			keyPath := filepath.Join(GinkgoT().TempDir(), "key.pub")
			Expect(os.WriteFile(keyPath, []byte("old"), 0o644)).To(Succeed())

			// The policy accepts the profile, but references the key.
			sys.SignaturePolicyPath = writePolicy(`{
				"default": [{"type": "insecureAcceptAnything"}],
				"transports": {"docker": {"other.example.com": [
					{"type": "signedBy", "keyType": "GPGKeys", "keyPath": "` + keyPath + `"}
				]}}
			}`)

			_, err := pull(profile.ref())
			Expect(err).NotTo(HaveOccurred())

			Expect(os.WriteFile(keyPath, []byte("newer"), 0o644)).To(Succeed())

			// When
			cached, err := pull(profile.ref())

			// Then
			Expect(err).NotTo(HaveOccurred())
			Expect(cached).To(BeFalse())
			Expect(registry.openCount()).To(Equal(2))
		})

		It("should fail and not record a profile if the policy changes during the pull", func() {
			// Given
			policyPath := sys.SignaturePolicyPath
			registry.onBlob = func() {
				//nolint:errcheck // The next pull fails if this does.
				os.WriteFile(policyPath, []byte(otherPolicy), 0o644)
			}

			_, err := pull(profile.ref())
			Expect(err).To(MatchError(ContainSubstring("signature policy changed")))
			Expect(err.Error()).NotTo(HavePrefix("SecurityProfileInvalid"))

			registry.onBlob = nil

			// When
			cached, err := pull(profile.ref())

			// Then
			Expect(err).NotTo(HaveOccurred())
			Expect(cached).To(BeFalse())
			Expect(registry.openCount()).To(Equal(2))
		})

		It("should reject a profile the signature policy rejects", func() {
			// Given
			sys.SignaturePolicyPath = writePolicy(rejectPolicy)

			// When
			_, err := pull(profile.ref())

			// Then
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(HavePrefix("SignatureValidationFailed: "))
			Expect(blobPath(profile.layers[0])).NotTo(BeAnExistingFile())
		})

		It("should fail with a broken signature policy", func() {
			// Given
			sys.SignaturePolicyPath = writePolicy("{")

			// When
			_, err := pull(profile.ref())

			// Then
			Expect(err).To(MatchError(ContainSubstring("load signature policy")))
			Expect(registry.opens).To(BeEmpty())
		})

		It("should prepare credentials only for a pull and discard them otherwise", func() {
			// Given
			prepared, cleaned, discarded := 0, 0, 0
			auth := &securityprofile.Auth{
				Scope: "namespace",
				Prepare: func(sys *imagetypes.SystemContext) (func(), error) {
					prepared++
					sys.AuthFilePath = "/tmp/auth.json"

					return func() { cleaned++ }, nil
				},
				Discard: func() { discarded++ },
			}

			// When
			_, err := sut.Pull(ctx, profile.ref(), types.SecurityProfileKind_Seccomp, sys, auth)
			Expect(err).NotTo(HaveOccurred())

			cached, err := sut.Pull(
				ctx,
				profile.ref(),
				types.SecurityProfileKind_Seccomp,
				sys,
				auth,
			)

			// Then
			Expect(err).NotTo(HaveOccurred())
			Expect(cached).To(BeTrue())
			Expect(prepared).To(Equal(1))
			Expect(cleaned).To(Equal(1))
			Expect(discarded).To(Equal(1))
			Expect(registry.sys.AuthFilePath).To(Equal("/tmp/auth.json"))
		})

		It("should not fail waiting callers if the first one gives up", func() {
			// Given
			registry.release = make(chan struct{})

			var joined sync.WaitGroup

			joined.Add(2)
			sut.SetJoinedHook(joined.Done)

			firstCtx, cancel := context.WithCancel(ctx)
			firstErr := make(chan error, 1)
			secondErr := make(chan error, 1)

			go func() {
				_, err := sut.Pull(
					firstCtx,
					profile.ref(),
					types.SecurityProfileKind_Seccomp,
					sys,
					nil,
				)
				firstErr <- err
			}()

			go func() {
				_, err := sut.Pull(ctx, profile.ref(), types.SecurityProfileKind_Seccomp, sys, nil)
				secondErr <- err
			}()

			joined.Wait()

			// When
			cancel()
			Eventually(firstErr).Should(Receive(MatchError(context.Canceled)))
			close(registry.release)

			// Then
			Eventually(secondErr).Should(Receive(BeNil()))
			Expect(registry.openCount()).To(Equal(1))
		})

		It("should record a pull whose callers all gave up", func() {
			// Given
			registry.release = make(chan struct{})

			var joined sync.WaitGroup

			joined.Add(1)
			sut.SetJoinedHook(joined.Done)

			pullCtx, cancel := context.WithCancel(ctx)
			pullErr := make(chan error, 1)

			go func() {
				_, err := sut.Pull(
					pullCtx,
					profile.ref(),
					types.SecurityProfileKind_Seccomp,
					sys,
					nil,
				)
				pullErr <- err
			}()

			joined.Wait()
			sut.SetJoinedHook(nil)
			cancel()
			Eventually(pullErr).Should(Receive(MatchError(context.Canceled)))

			// When
			close(registry.release)

			// Then
			Eventually(func() bool {
				_, err := os.Stat(blobPath(profile.manifest()))

				return err == nil
			}).Should(BeTrue())
			Eventually(func() (bool, error) {
				return pull(profile.ref())
			}).Should(BeTrue())
			Expect(registry.openCount()).To(Equal(1))
		})

		for name, kind := range map[string]types.SecurityProfileKind{
			"unspecified": types.SecurityProfileKind_SecurityProfileKindUnspecified,
			"AppArmor":    types.SecurityProfileKind_AppArmor,
			"unknown":     types.SecurityProfileKind(42),
		} {
			It("should reject the "+name+" kind", func() {
				// Given
				// When
				_, err := sut.Pull(ctx, profile.ref(), kind, sys, nil)

				// Then
				Expect(status.Code(err)).To(Equal(codes.InvalidArgument))
				Expect(status.Convert(err).Message()).To(HavePrefix("SecurityProfileInvalid: "))
				Expect(registry.opens).To(BeEmpty())
			})
		}

		for name, ref := range map[string]string{
			"tag":        repository + ":latest",
			"short name": "deny-chmod@" + digest.FromString("").String(),
			"invalid":    "registry.example.com/UPPER@sha256:abc",
			"empty":      "",
		} {
			It("should reject a reference with a "+name, func() {
				// Given
				// When
				_, err := pull(ref)

				// Then
				Expect(status.Code(err)).To(Equal(codes.InvalidArgument))
				Expect(status.Convert(err).Message()).To(HavePrefix("SecurityProfileInvalid: "))
				Expect(registry.opens).To(BeEmpty())
			})
		}

		for name, tc := range map[string]struct {
			modify    func(*artifact)
			substring string
		}{
			"AppArmor config": {
				func(a *artifact) {
					a.configMediaType = securityprofile.AppArmorConfigMediaType
					a.artifactType = ""
				},
				"does not match the profile kind",
			},
			"generic config": {
				func(a *artifact) {
					a.configMediaType = "application/vnd.unknown.config.v1+json"
					a.artifactType = ""
				},
				"does not match the profile kind",
			},
			"empty config without artifact type": {
				func(a *artifact) {
					a.configMediaType = ispec.MediaTypeEmptyJSON
					a.artifactType = ""
				},
				"does not match the profile kind",
			},
			"conflicting artifact type": {
				func(a *artifact) { a.artifactType = securityprofile.AppArmorConfigMediaType },
				"does not match config media type",
			},
			"second layer": {
				func(a *artifact) { a.layers = append(a.layers, []byte("{}")) },
				"expected exactly one",
			},
			"missing layer": {
				func(a *artifact) { a.layers = nil },
				"expected exactly one",
			},
			"oversized layer": {
				func(a *artifact) { a.layers = [][]byte{bytes.Repeat([]byte(" "), maxSize+1)} },
				"the maximum is",
			},
			"oversized config": {
				func(a *artifact) { a.configData = bytes.Repeat([]byte(" "), maxSize+1) },
				"the maximum is",
			},
			"foreign layer": {
				func(a *artifact) { a.urls = []string{"https://example.com/profile.json"} },
				"external URLs",
			},
		} {
			It("should reject an artifact with a "+name+" before downloading it", func() {
				// Given
				tc.modify(profile)
				registry.add(profile)
				registry.blobErr = errors.New("must not be downloaded")

				// When
				_, err := pull(profile.ref())

				// Then
				expectInvalid(err, tc.substring)
			})
		}

		It("should accept an empty config with an artifact type", func() {
			// Given
			profile.configMediaType = ispec.MediaTypeEmptyJSON
			profile.layerMediaType = ispec.MediaTypeImageLayer
			registry.add(profile)

			// When
			_, err := pull(profile.ref())

			// Then
			Expect(err).NotTo(HaveOccurred())
		})

		It("should accept a layer of the maximum size", func() {
			// Given
			profile.layers = [][]byte{append(
				[]byte(denyChmod), bytes.Repeat([]byte(" "), maxSize-len(denyChmod))...,
			)}
			registry.add(profile)

			// When
			_, err := pull(profile.ref())

			// Then
			Expect(err).NotTo(HaveOccurred())
		})

		for name, blob := range map[string][]byte{
			"larger than stated":        bytes.Repeat([]byte(" "), maxSize),
			"shorter than stated":       []byte("{}"),
			"different from its digest": bytes.ToUpper([]byte(denyChmod)),
		} {
			It("should report a registry serving a layer "+name+" as unavailable", func() {
				// Given
				registry.blobs = map[digest.Digest][]byte{digest.FromBytes(profile.layers[0]): blob}

				// When
				_, err := pull(profile.ref())

				// Then
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(HavePrefix("RegistryUnavailable: "))
				Expect(blobPath(profile.manifest())).NotTo(BeAnExistingFile())
			})
		}

		It("should not fetch the config", func() {
			// Given
			registry.blobs = map[digest.Digest][]byte{
				digest.FromBytes(profile.configData): []byte("must not be fetched"),
			}

			// When
			_, err := pull(profile.ref())

			// Then
			Expect(err).NotTo(HaveOccurred())
			Expect(blobPath(profile.configData)).NotTo(BeAnExistingFile())
		})

		It("should fail if the registry serves a manifest of another digest", func() {
			// Given
			other := seccompArtifact(`{"defaultAction": "SCMP_ACT_ERRNO"}`)
			registry.artifacts = map[digest.Digest]*artifact{digest.FromString("unknown"): other}

			// When
			_, err := pull(profile.ref())

			// Then
			Expect(err).To(MatchError(ContainSubstring("does not match")))
			Expect(err.Error()).NotTo(HavePrefix("SecurityProfileInvalid"))
			Expect(blobPath(other.manifest())).NotTo(BeAnExistingFile())
			Expect(blobPath(other.layers[0])).NotTo(BeAnExistingFile())
		})

		It("should reject a manifest over 64 KiB", func() {
			// Given
			profile.annotations = map[string]string{"padding": strings.Repeat("x", 64*1024)}
			registry.add(profile)

			// When
			_, err := pull(profile.ref())

			// Then
			expectInvalid(err, "manifest of")
		})

		for name, tc := range map[string]struct {
			profile   []byte
			substring string
		}{
			"tar archive":      {tarball([]byte(denyChmod)), "is an archive"},
			"gzip stream":      {[]byte{0x1f, 0x8b, 0x08, 0x00}, "is an archive"},
			"unknown member":   {[]byte(`{"defaultAction": "SCMP_ACT_ERRNO", "unknown": true}`), "decode seccomp profile"},
			"repeated member":  {[]byte(`{"defaultAction": "SCMP_ACT_ALLOW", "defaultAction": "SCMP_ACT_ERRNO"}`), "decode seccomp profile"},
			"trailing data":    {[]byte(`{"defaultAction": "SCMP_ACT_ERRNO"} {}`), "decode seccomp profile"},
			"notify action":    {[]byte(`{"defaultAction": "SCMP_ACT_ERRNO", "syscalls": [{"names": ["mount"], "action": "SCMP_ACT_NOTIFY"}]}`), "validate seccomp profile"},
			"listener path":    {[]byte(`{"defaultAction": "SCMP_ACT_ERRNO", "listenerPath": "/run/listener.sock"}`), "validate seccomp profile"},
			"errno over 4095":  {[]byte(`{"defaultAction": "SCMP_ACT_ERRNO", "defaultErrnoRet": 4096}`), "validate seccomp profile"},
			"not a JSON value": {[]byte(`null`), "decode seccomp profile"},
		} {
			It("should reject a pulled profile with a "+name+" without pulling it again", func() {
				// Given
				profile.layers = [][]byte{tc.profile}
				registry.add(profile)

				cached, err := pull(profile.ref())
				expectInvalid(err, tc.substring)
				Expect(cached).To(BeFalse())

				// Only the reason is kept, across restarts.
				Expect(blobPath(tc.profile)).NotTo(BeAnExistingFile())
				Expect(blobPath(profile.manifest())).NotTo(BeAnExistingFile())

				sut = open()

				// When
				cached, err = pull(profile.ref())

				// Then
				expectInvalid(err, tc.substring)
				Expect(cached).To(BeTrue())
				Expect(registry.openCount()).To(Equal(1))
			})
		}

		It("should reject too many entries for one syscall", func() {
			// Given
			entries := make([]string, 0, 129)
			for range 129 {
				entries = append(entries, `{"names": ["read"], "action": "SCMP_ACT_ALLOW"}`)
			}

			// The layer exceeds maxSize, so the store needs a larger limit.
			sut = securityprofile.New(
				ctx,
				&securityprofile.Options{Dir: dir, MaxSize: 1024 * 1024},
			)
			sut.SetOpenSource(registry.open)

			profile.layers = [][]byte{
				[]byte(
					`{"defaultAction": "SCMP_ACT_ERRNO", "syscalls": [` + strings.Join(
						entries,
						",",
					) + `]}`,
				),
			}
			registry.add(profile)

			// When
			_, err := pull(profile.ref())

			// Then
			expectInvalid(err, "validate seccomp profile")
		})

		It("should find a pulled profile by a SHA-512 digest", func() {
			// Given
			_, err := pull(profile.ref())
			Expect(err).NotTo(HaveOccurred())

			sut = open()
			ref := repository + "@" + digest.SHA512.FromBytes(profile.manifest()).String()

			// When
			cached, err := pull(ref)

			// Then
			Expect(err).NotTo(HaveOccurred())
			Expect(cached).To(BeTrue())
			Expect(registry.openCount()).To(Equal(1))
		})

		It("should not pull by a SHA-512 digest", func() {
			// Given
			_, err := pull(profile.ref())
			Expect(err).NotTo(HaveOccurred())

			ref := repository + "@" + digest.SHA512.FromString("other").String()

			// When
			_, err = pull(ref)

			// Then
			Expect(status.Code(err)).To(Equal(codes.InvalidArgument))
			Expect(status.Convert(err).Message()).To(HavePrefix("SecurityProfileInvalid: "))
			Expect(err.Error()).To(ContainSubstring("only sha256 references can be pulled"))
			Expect(registry.openCount()).To(Equal(1))
		})

		for name, tc := range map[string]struct {
			openErr error
			prefix  string
		}{
			"refused connection": {
				&net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED},
				"RegistryUnavailable: ",
			},
			"unknown host": {
				&net.DNSError{Err: "no such host", Name: "registry.example.com", IsNotFound: true},
				"RegistryUnavailable: ",
			},
			"server error": {
				docker.UnexpectedHTTPStatusError{StatusCode: 503},
				"RegistryUnavailable: ",
			},
			"missing credentials": {
				docker.UnexpectedHTTPStatusError{StatusCode: 401},
				"open ",
			},
			"certificate problem": {
				errors.New("tls: failed to verify certificate: x509: certificate signed by unknown authority"),
				"open ",
			},
		} {
			It("should classify a "+name, func() {
				// Given
				registry.openErr = tc.openErr

				// When
				_, err := pull(profile.ref())

				// Then
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(HavePrefix(tc.prefix))
			})
		}
	})

	t.Describe("unavailable store", func() {
		It("should fail calls without keeping CRI-O from starting", func() {
			// Given
			file := filepath.Join(t.MustTempDir("file"), "profiles")
			Expect(os.WriteFile(file, nil, 0o600)).To(Succeed())

			// When
			store := securityprofile.New(ctx, &securityprofile.Options{Dir: file, MaxSize: maxSize})

			// Then
			Expect(store.Available()).To(BeFalse())

			_, err := store.Pull(ctx, profile.ref(), types.SecurityProfileKind_Seccomp, sys, nil)
			Expect(err).To(MatchError(ContainSubstring("unavailable")))
			Expect(err.Error()).NotTo(HavePrefix("SecurityProfileInvalid"))

			_, err = store.For("default", "ctr").MergeSeccomp(ctx, profile.ref(), nil, nil)
			Expect(err).To(MatchError(ContainSubstring("unavailable")))

			_, err = store.List()
			Expect(err).To(MatchError(ContainSubstring("unavailable")))

			err = store.Remove(ctx, profile.digest().String())
			Expect(err).To(MatchError(ContainSubstring("unavailable")))
		})

		It("should not write the index when opening an unchanged store", func() {
			// Given
			_, err := pull(profile.ref())
			Expect(err).NotTo(HaveOccurred())

			index := filepath.Join(dir, "profiles.json")
			before, err := os.Stat(index)
			Expect(err).NotTo(HaveOccurred())

			// When
			sut = open()

			// Then
			after, err := os.Stat(index)
			Expect(err).NotTo(HaveOccurred())
			Expect(after.ModTime()).To(Equal(before.ModTime()))
		})
	})

	t.Describe("additional stores", func() {
		It("should trust a profile of an additional store", func() {
			// Given
			profile.storeLayout(addRoot)

			sys.SignaturePolicyPath = writePolicy(rejectPolicy)

			// When
			cached, err := pull(profile.ref())

			// Then
			Expect(err).NotTo(HaveOccurred())
			Expect(cached).To(BeTrue())
			Expect(registry.opens).To(BeEmpty())
		})

		It("should validate a profile of an additional store", func() {
			// Given
			invalid := seccompArtifact(`{"defaultAction": "SCMP_ACT_NOTIFY"}`)
			invalid.storeLayout(addRoot)

			// When
			cached, err := pull(invalid.ref())

			// Then
			expectInvalid(err, "validate seccomp profile")
			Expect(cached).To(BeTrue())
			Expect(registry.opens).To(BeEmpty())
		})

		It("should skip a profile of an additional store that cannot be read", func() {
			// Given
			profile.storeLayout(addRoot)

			layer := digest.FromBytes(profile.layers[0])
			Expect(os.WriteFile(
				filepath.Join(addRoot, "blobs", "sha256", layer.Encoded()),
				bytes.ToUpper(profile.layers[0]),
				0o644,
			)).To(Succeed())

			// When
			cached, err := pull(profile.ref())

			// Then
			Expect(err).NotTo(HaveOccurred())
			Expect(cached).To(BeFalse())
			Expect(registry.openCount()).To(Equal(1))
		})

		It("should notice a profile removed from an additional store", func() {
			// Given
			profile.storeLayout(addRoot)

			cached, err := pull(profile.ref())
			Expect(err).NotTo(HaveOccurred())
			Expect(cached).To(BeTrue())

			Expect(
				os.Remove(filepath.Join(addRoot, "blobs", "sha256", profile.digest().Encoded())),
			).
				To(Succeed())

			// When
			cached, err = pull(profile.ref())

			// Then
			Expect(err).NotTo(HaveOccurred())
			Expect(cached).To(BeFalse())
			Expect(registry.openCount()).To(Equal(1))
		})

		It("should not block on a blob of an additional store that is a FIFO", func() {
			// Given
			profile.storeLayout(addRoot)

			layer := filepath.Join(
				addRoot,
				"blobs",
				"sha256",
				digest.FromBytes(profile.layers[0]).Encoded(),
			)
			Expect(os.Remove(layer)).To(Succeed())
			Expect(syscall.Mkfifo(layer, 0o600)).To(Succeed())

			// When
			cached, err := pull(profile.ref())

			// Then
			Expect(err).NotTo(HaveOccurred())
			Expect(cached).To(BeFalse())
			Expect(registry.openCount()).To(Equal(1))
		})

		It("should find a profile of an additional store by a SHA-512 digest", func() {
			// Given
			profile.storeLayout(addRoot)
			ref := repository + "@" + digest.SHA512.FromBytes(profile.manifest()).String()

			// When
			cached, err := pull(ref)

			// Then
			Expect(err).NotTo(HaveOccurred())
			Expect(cached).To(BeTrue())
			Expect(registry.opens).To(BeEmpty())
		})
	})

	t.Describe("List", func() {
		It("should list a pulled profile with its reference and size", func() {
			// Given
			_, err := pull(profile.ref())
			Expect(err).NotTo(HaveOccurred())

			// When
			profiles, err := sut.List()

			// Then
			Expect(err).NotTo(HaveOccurred())
			Expect(profiles).To(HaveLen(1))
			Expect(profiles[0].GetDigest()).To(Equal(profile.digest().String()))
			Expect(profiles[0].GetRefs()).To(Equal([]string{profile.ref()}))
			Expect(profiles[0].GetSize()).To(
				BeEquivalentTo(len(profile.manifest()) + len(profile.layers[0])),
			)
		})

		It("should list a profile once with every reference it was pulled with", func() {
			// Given
			_, err := pull(profile.ref())
			Expect(err).NotTo(HaveOccurred())

			mirror := "mirror.example.com/other@" + profile.digest().String()
			_, err = pull(mirror)
			Expect(err).NotTo(HaveOccurred())

			// A present profile is not recorded again.
			_, err = pull(profile.ref())
			Expect(err).NotTo(HaveOccurred())

			sut = open()

			// When
			profiles, err := sut.List()

			// Then
			Expect(err).NotTo(HaveOccurred())
			Expect(profiles).To(HaveLen(1))
			Expect(profiles[0].GetRefs()).To(Equal([]string{profile.ref(), mirror}))
		})

		It("should list a reference as it was requested", func() {
			// Given
			tagged := repository + ":v1@" + profile.digest().String()

			_, err := pull(tagged)
			Expect(err).NotTo(HaveOccurred())

			// When
			profiles, err := sut.List()

			// Then
			Expect(err).NotTo(HaveOccurred())
			Expect(profiles).To(HaveLen(1))
			Expect(profiles[0].GetRefs()).To(Equal([]string{tagged}))
		})

		It("should record a new reference of a present profile", func() {
			// Given
			_, err := pull(profile.ref())
			Expect(err).NotTo(HaveOccurred())

			tagged := repository + ":v1@" + profile.digest().String()

			// When
			cached, err := pull(tagged)

			// Then
			Expect(err).NotTo(HaveOccurred())
			Expect(cached).To(BeTrue())
			Expect(registry.openCount()).To(Equal(1))

			profiles, err := sut.List()
			Expect(err).NotTo(HaveOccurred())
			Expect(profiles).To(HaveLen(1))
			Expect(profiles[0].GetRefs()).To(Equal([]string{profile.ref(), tagged}))
		})

		It("should record a SHA-512 reference of a present profile", func() {
			// Given
			_, err := pull(profile.ref())
			Expect(err).NotTo(HaveOccurred())

			ref := repository + "@" + digest.SHA512.FromBytes(profile.manifest()).String()

			// When
			cached, err := pull(ref)

			// Then
			Expect(err).NotTo(HaveOccurred())
			Expect(cached).To(BeTrue())

			sut = open()

			profiles, err := sut.List()
			Expect(err).NotTo(HaveOccurred())
			Expect(profiles).To(HaveLen(1))
			Expect(profiles[0].GetDigest()).To(Equal(profile.digest().String()))
			Expect(profiles[0].GetRefs()).To(Equal([]string{profile.ref(), ref}))
		})

		It("should not write the index for a known reference", func() {
			// Given
			_, err := pull(profile.ref())
			Expect(err).NotTo(HaveOccurred())

			before, err := os.Stat(filepath.Join(dir, "profiles.json"))
			Expect(err).NotTo(HaveOccurred())

			// When
			cached, err := pull(profile.ref())

			// Then
			Expect(err).NotTo(HaveOccurred())
			Expect(cached).To(BeTrue())

			after, err := os.Stat(filepath.Join(dir, "profiles.json"))
			Expect(err).NotTo(HaveOccurred())
			Expect(os.SameFile(before, after)).To(BeTrue())
		})

		It("should not add a profile removed before its reference is recorded", func() {
			// Given
			_, err := pull(profile.ref())
			Expect(err).NotTo(HaveOccurred())

			sut.SetBeforeRecordHook(func() {
				Expect(sut.Remove(ctx, profile.digest().String())).To(Succeed())
			})

			tagged := repository + ":v1@" + profile.digest().String()

			// When
			_, err = pull(tagged)

			// Then
			Expect(err).NotTo(HaveOccurred())

			profiles, err := sut.List()
			Expect(err).NotTo(HaveOccurred())
			Expect(profiles).To(BeEmpty())
			Expect(blobPath(profile.layers[0])).NotTo(BeAnExistingFile())

			sut = open()

			profiles, err = sut.List()
			Expect(err).NotTo(HaveOccurred())
			Expect(profiles).To(BeEmpty())
		})

		It("should list an invalid profile, so that it can be removed", func() {
			// Given
			profile.layers = [][]byte{[]byte(`{"defaultAction": "SCMP_ACT_NOTIFY"}`)}
			registry.add(profile)

			_, err := pull(profile.ref())
			expectInvalid(err, "validate seccomp profile")

			// When
			profiles, err := sut.List()

			// Then
			Expect(err).NotTo(HaveOccurred())
			Expect(profiles).To(HaveLen(1))
			Expect(profiles[0].GetDigest()).To(Equal(profile.digest().String()))
			Expect(profiles[0].GetRefs()).To(Equal([]string{profile.ref()}))
			Expect(profiles[0].GetSize()).To(BeZero())

			// Pulls that find it record their references as well.
			ref := repository + "@" + digest.SHA512.FromBytes(profile.manifest()).String()
			_, err = pull(ref)
			expectInvalid(err, "validate seccomp profile")

			profiles, err = sut.List()
			Expect(err).NotTo(HaveOccurred())
			Expect(profiles[0].GetRefs()).To(Equal([]string{profile.ref(), ref}))
		})

		It("should not list profiles of additional stores", func() {
			// Given
			profile.storeLayout(addRoot)

			cached, err := pull(profile.ref())
			Expect(err).NotTo(HaveOccurred())
			Expect(cached).To(BeTrue())

			// When
			profiles, err := sut.List()

			// Then
			Expect(err).NotTo(HaveOccurred())
			Expect(profiles).To(BeEmpty())
		})
	})

	t.Describe("Remove", func() {
		BeforeEach(func() {
			_, err := pull(profile.ref())
			Expect(err).NotTo(HaveOccurred())
		})

		It("should remove a pulled profile", func() {
			// Given
			// When
			err := sut.Remove(ctx, profile.digest().String())

			// Then
			Expect(err).NotTo(HaveOccurred())
			Expect(blobPath(profile.layers[0])).NotTo(BeAnExistingFile())
			Expect(blobPath(profile.manifest())).NotTo(BeAnExistingFile())

			profiles, err := sut.List()
			Expect(err).NotTo(HaveOccurred())
			Expect(profiles).To(BeEmpty())

			sut = open()

			profiles, err = sut.List()
			Expect(err).NotTo(HaveOccurred())
			Expect(profiles).To(BeEmpty())

			cached, err := pull(profile.ref())
			Expect(err).NotTo(HaveOccurred())
			Expect(cached).To(BeFalse())
			Expect(registry.openCount()).To(Equal(2))
		})

		It("should not fail for a profile that is not present", func() {
			// Given
			Expect(sut.Remove(ctx, profile.digest().String())).To(Succeed())

			// When
			err := sut.Remove(ctx, profile.digest().String())

			// Then
			Expect(err).NotTo(HaveOccurred())
		})

		It("should remove a profile by a SHA-512 digest", func() {
			// Given
			dgst := digest.SHA512.FromBytes(profile.manifest())

			// When
			err := sut.Remove(ctx, dgst.String())

			// Then
			Expect(err).NotTo(HaveOccurred())
			Expect(blobPath(profile.layers[0])).NotTo(BeAnExistingFile())
		})

		It("should keep the other profiles", func() {
			// Given
			other := seccompArtifact(`{"defaultAction": "SCMP_ACT_ALLOW"}`)
			registry.add(other)

			_, err := pull(other.ref())
			Expect(err).NotTo(HaveOccurred())

			// When
			err = sut.Remove(ctx, profile.digest().String())

			// Then
			Expect(err).NotTo(HaveOccurred())

			profiles, err := sut.List()
			Expect(err).NotTo(HaveOccurred())
			Expect(profiles).To(HaveLen(1))
			Expect(profiles[0].GetDigest()).To(Equal(other.digest().String()))
			Expect(blobPath(other.layers[0])).To(BeAnExistingFile())
		})

		It("should not remove a profile of an additional store", func() {
			// Given
			Expect(sut.Remove(ctx, profile.digest().String())).To(Succeed())
			profile.storeLayout(addRoot)

			// When
			err := sut.Remove(ctx, profile.digest().String())

			// Then
			Expect(err).NotTo(HaveOccurred())

			cached, err := pull(profile.ref())
			Expect(err).NotTo(HaveOccurred())
			Expect(cached).To(BeTrue())
		})

		for name, dgst := range map[string]string{
			"an empty digest":      "",
			"a reference":          "registry.example.com/profile@sha256:" + strings.Repeat("0", 64),
			"a short digest":       "sha256:1234",
			"an unknown algorithm": "md5:" + strings.Repeat("0", 32),
		} {
			It("should reject "+name, func() {
				// Given
				// When
				err := sut.Remove(ctx, dgst)

				// Then
				Expect(status.Code(err)).To(Equal(codes.InvalidArgument))
				Expect(blobPath(profile.layers[0])).To(BeAnExistingFile())
			})
		}

		It("should not remove the blobs of a pull that did not commit yet", func() {
			// Given
			sut.SetBeforeCommitHook(func() {
				Expect(sut.Remove(ctx, profile.digest().String())).To(Succeed())
			})

			// A pull for another policy writes the same blobs again.
			sys.SignaturePolicyPath = writePolicy(otherPolicy)

			// When
			cached, err := pull(profile.ref())

			// Then
			Expect(err).NotTo(HaveOccurred())
			Expect(cached).To(BeFalse())
			Expect(blobPath(profile.layers[0])).To(BeAnExistingFile())
			Expect(blobPath(profile.manifest())).To(BeAnExistingFile())

			sut.SetBeforeCommitHook(nil)

			cached, err = pull(profile.ref())
			Expect(err).NotTo(HaveOccurred())
			Expect(cached).To(BeTrue())
			Expect(registry.openCount()).To(Equal(2))
		})

		It("should make new containers wait for the next pull", func() {
			// Given
			profiles := sut.For("default", "ctr")

			// When
			Expect(sut.Remove(ctx, profile.digest().String())).To(Succeed())

			// Then
			_, err := profiles.MergeSeccomp(ctx, profile.ref(), nil, nil)
			Expect(err).To(MatchError(ContainSubstring("has not been pulled")))

			_, err = pull(profile.ref())
			Expect(err).NotTo(HaveOccurred())

			_, err = profiles.MergeSeccomp(ctx, profile.ref(), nil, nil)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should stay consistent with concurrent pulls and merges", func() {
			// Given
			var wg sync.WaitGroup

			// When
			for range 20 {
				wg.Go(func() {
					defer GinkgoRecover()

					// A pull may find its profile removed again.
					if _, err := pull(profile.ref()); err != nil {
						Expect(err).To(MatchError(ContainSubstring("is not in the store")))
					}
				})
				wg.Go(func() {
					defer GinkgoRecover()

					Expect(sut.Remove(ctx, profile.digest().String())).To(Succeed())
				})
				wg.Go(func() {
					defer GinkgoRecover()

					_, err := sut.For("default", "ctr").MergeSeccomp(ctx, profile.ref(), nil, nil)
					if err != nil {
						Expect(err).To(MatchError(ContainSubstring("has not been pulled")))
					}
				})
			}

			wg.Wait()

			// Then
			profiles, err := sut.List()
			Expect(err).NotTo(HaveOccurred())

			if len(profiles) > 0 {
				Expect(blobPath(profile.layers[0])).To(BeAnExistingFile())
				Expect(blobPath(profile.manifest())).To(BeAnExistingFile())
			}

			sut = open()

			reopened, err := sut.List()
			Expect(err).NotTo(HaveOccurred())
			Expect(reopened).To(HaveLen(len(profiles)))

			_, err = pull(profile.ref())
			Expect(err).NotTo(HaveOccurred())

			_, err = sut.For("default", "ctr").MergeSeccomp(ctx, profile.ref(), nil, nil)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	t.Describe("MergeSeccomp", func() {
		baseline := &rspec.LinuxSeccomp{
			DefaultAction: rspec.ActErrno,
			Syscalls: []rspec.LinuxSyscall{
				{Names: []string{"chmod", "read", "write"}, Action: rspec.ActAllow},
			},
		}

		action := func(profile *rspec.LinuxSeccomp, name string) rspec.LinuxSeccompAction {
			for _, syscall := range profile.Syscalls {
				for _, n := range syscall.Names {
					if n == name && len(syscall.Args) == 0 {
						return syscall.Action
					}
				}
			}

			return profile.DefaultAction
		}

		merge := func(ref string, baseline, base *rspec.LinuxSeccomp) (*rspec.LinuxSeccomp, error) {
			return sut.For("default", "ns/pod/ctr").MergeSeccomp(ctx, ref, baseline, base)
		}

		BeforeEach(func() {
			_, err := pull(profile.ref())
			Expect(err).NotTo(HaveOccurred())
		})

		It("should intersect the pulled profile with the baseline", func() {
			// Given
			// When
			res, err := merge(profile.ref(), baseline, nil)

			// Then
			Expect(err).NotTo(HaveOccurred())
			Expect(res.DefaultAction).To(Equal(rspec.ActErrno))
			Expect(action(res, "read")).To(Equal(rspec.ActAllow))
			Expect(action(res, "chmod")).To(Equal(rspec.ActErrno))
			Expect(action(res, "mount")).To(Equal(rspec.ActErrno))
			Expect(logs.String()).To(ContainSubstring(securityprofile.MergeConstrainedEvent))
			Expect(logs.String()).To(ContainSubstring("container=ns/pod/ctr"))
			Expect(logs.String()).To(ContainSubstring("baseline="))
		})

		It("should name the base profile that constrained the pulled profile", func() {
			// Given
			base := &rspec.LinuxSeccomp{
				DefaultAction: rspec.ActAllow,
				Syscalls: []rspec.LinuxSyscall{
					{Names: []string{"write"}, Action: rspec.ActErrno},
				},
			}

			// When
			res, err := merge(profile.ref(), baseline, base)

			// Then
			Expect(err).NotTo(HaveOccurred())
			Expect(action(res, "read")).To(Equal(rspec.ActAllow))
			Expect(action(res, "write")).To(Equal(rspec.ActErrno))
			Expect(logs.String()).To(ContainSubstring("baseProfile="))
			Expect(logs.String()).To(ContainSubstring("baseline="))
		})

		It("should attribute the constraints of a base profile without a baseline", func() {
			// Given
			base := &rspec.LinuxSeccomp{
				DefaultAction: rspec.ActAllow,
				Syscalls: []rspec.LinuxSyscall{
					{Names: []string{"write"}, Action: rspec.ActErrno},
				},
			}

			// When
			res, err := merge(profile.ref(), nil, base)

			// Then
			Expect(err).NotTo(HaveOccurred())
			Expect(action(res, "write")).To(Equal(rspec.ActErrno))
			Expect(action(res, "chmod")).To(Equal(rspec.ActErrno))
			Expect(logs.String()).To(ContainSubstring("baseProfile="))
			Expect(logs.String()).NotTo(ContainSubstring("baseline="))
		})

		It("should not report the normalization of the pulled profile alone", func() {
			// Given
			// When
			res, err := merge(profile.ref(), nil, nil)

			// Then
			Expect(err).NotTo(HaveOccurred())
			Expect(action(res, "chmod")).To(Equal(rspec.ActErrno))
			Expect(logs.String()).NotTo(ContainSubstring(securityprofile.MergeConstrainedEvent))
		})

		It("should not log a profile the baseline does not constrain", func() {
			// Given
			strict := seccompArtifact(`{
				"defaultAction": "SCMP_ACT_ERRNO",
				"syscalls": [{"names": ["read"], "action": "SCMP_ACT_ALLOW"}]
			}`)
			registry.add(strict)
			_, err := pull(strict.ref())
			Expect(err).NotTo(HaveOccurred())

			// When
			res, err := merge(strict.ref(), baseline, nil)

			// Then
			Expect(err).NotTo(HaveOccurred())
			Expect(action(res, "read")).To(Equal(rspec.ActAllow))
			Expect(action(res, "write")).To(Equal(rspec.ActErrno))
			Expect(logs.String()).NotTo(ContainSubstring(securityprofile.MergeConstrainedEvent))
		})

		It("should report an invalid baseline", func() {
			// Given
			invalid := &rspec.LinuxSeccomp{DefaultAction: "SCMP_ACT_WRONG"}

			// When
			_, err := merge(profile.ref(), invalid, nil)

			// Then
			Expect(err).To(MatchError(ContainSubstring("invalid baseline")))
		})

		It("should report an invalid base profile without a baseline", func() {
			// Given
			invalid := &rspec.LinuxSeccomp{DefaultAction: "SCMP_ACT_WRONG"}

			// When
			_, err := merge(profile.ref(), nil, invalid)

			// Then
			Expect(err).To(MatchError(ContainSubstring("invalid base profile")))
		})

		It("should fail for a profile that was not pulled", func() {
			// Given
			other := seccompArtifact(`{"defaultAction": "SCMP_ACT_ERRNO"}`)

			// When
			_, err := merge(other.ref(), baseline, nil)

			// Then
			Expect(err).To(MatchError(ContainSubstring("has not been pulled")))
			Expect(registry.openCount()).To(Equal(1))
		})

		It("should fail for a profile not verified under the policy of the namespace", func() {
			// Given
			policies["strict"] = writePolicy(rejectPolicy)

			// When
			_, err := sut.For("strict", "strict/pod/ctr").
				MergeSeccomp(ctx, profile.ref(), baseline, nil)

			// Then
			Expect(status.Code(err)).To(Equal(codes.FailedPrecondition))
			Expect(err.Error()).NotTo(ContainSubstring("SecurityProfileInvalid"))
		})

		It("should trust a profile of an additional store in every namespace", func() {
			// Given
			other := seccompArtifact(`{"defaultAction": "SCMP_ACT_ALLOW"}`)
			other.storeLayout(addRoot)

			policies["strict"] = writePolicy(rejectPolicy)

			// When
			res, err := sut.For("strict", "strict/pod/ctr").
				MergeSeccomp(ctx, other.ref(), baseline, nil)

			// Then
			Expect(err).NotTo(HaveOccurred())
			Expect(action(res, "read")).To(Equal(rspec.ActAllow))
		})
	})
})
