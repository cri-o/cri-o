package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"

	"github.com/cri-o/crio-credential-provider/pkg/auth"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/opencontainers/go-digest"
	ispec "github.com/opencontainers/image-spec/specs-go/v1"
	"go.podman.io/image/v5/docker"
	"go.podman.io/image/v5/docker/reference"
	imagetypes "go.podman.io/image/v5/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/securityprofile"
	"github.com/cri-o/cri-o/pkg/config"
)

// The actual test suite.
var _ = t.Describe("SecurityProfile", func() {
	const ref = "registry.example.com/profile@sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

	// Prepare the sut
	BeforeEach(func() {
		beforeEach()
		setupSUT()
	})

	AfterEach(afterEach)

	t.Describe("PullSecurityProfile", func() {
		It("should fail without a reference", func() {
			// Given
			// When
			response, err := sut.PullSecurityProfile(context.Background(),
				&types.PullSecurityProfileRequest{
					ProfileKind: types.SecurityProfileKind_Seccomp,
				})

			// Then
			Expect(status.Code(err)).To(Equal(codes.InvalidArgument))
			Expect(status.Convert(err).Message()).To(HavePrefix("SecurityProfileInvalid: "))
			Expect(response).To(BeNil())
		})

		for _, kind := range []types.SecurityProfileKind{
			types.SecurityProfileKind_SecurityProfileKindUnspecified,
			types.SecurityProfileKind_AppArmor,
		} {
			It("should reject the "+kind.String()+" kind permanently", func() {
				// Given
				// When
				response, err := sut.PullSecurityProfile(context.Background(),
					&types.PullSecurityProfileRequest{
						Image:       &types.ImageSpec{Image: ref, UserSpecifiedImage: ref},
						ProfileKind: kind,
					})

				// Then
				Expect(status.Code(err)).To(Equal(codes.InvalidArgument))
				Expect(status.Convert(err).Message()).To(HavePrefix("SecurityProfileInvalid: "))
				Expect(response).To(BeNil())
			})
		}

		It("should fail with invalid credentials", func() {
			// Given
			// When
			response, err := sut.PullSecurityProfile(context.Background(),
				&types.PullSecurityProfileRequest{
					Image:       &types.ImageSpec{Image: ref, UserSpecifiedImage: ref},
					Auth:        &types.AuthConfig{Auth: "not base64"},
					ProfileKind: types.SecurityProfileKind_Seccomp,
				})

			// Then
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).NotTo(HavePrefix("SecurityProfileInvalid"))
			Expect(response).To(BeNil())
		})
	})

	t.Describe("RunPodSandbox", func() {
		It("should reject the OCI profile type for AppArmor", func() {
			// Given
			config := &types.PodSandboxConfig{Linux: &types.LinuxPodSandboxConfig{
				SecurityContext: &types.LinuxSandboxSecurityContext{
					Apparmor: &types.SecurityProfile{
						ProfileType: types.SecurityProfile_OCI,
						OciRef:      ref,
					},
				},
			}}

			// When
			response, err := sut.RunPodSandbox(context.Background(),
				&types.RunPodSandboxRequest{Config: config})

			// Then
			Expect(status.Code(err)).To(Equal(codes.InvalidArgument))
			Expect(response).To(BeNil())
		})

		It("should reject an OCI seccomp profile without a reference", func() {
			// Given
			config := &types.PodSandboxConfig{Linux: &types.LinuxPodSandboxConfig{
				SecurityContext: &types.LinuxSandboxSecurityContext{
					Seccomp: &types.SecurityProfile{ProfileType: types.SecurityProfile_OCI},
				},
			}}

			// When
			response, err := sut.RunPodSandbox(context.Background(),
				&types.RunPodSandboxRequest{Config: config})

			// Then
			Expect(status.Code(err)).To(Equal(codes.InvalidArgument))
			Expect(response).To(BeNil())
		})
	})

	t.Describe("CreateContainer", func() {
		It("should reject the OCI profile type for AppArmor", func() {
			// Given
			config := &types.ContainerConfig{
				Image: &types.ImageSpec{Image: "image"},
				Linux: &types.LinuxContainerConfig{
					SecurityContext: &types.LinuxContainerSecurityContext{
						Apparmor: &types.SecurityProfile{
							ProfileType: types.SecurityProfile_OCI,
							OciRef:      ref,
						},
					},
				},
			}

			// When
			response, err := sut.CreateContainer(context.Background(),
				&types.CreateContainerRequest{
					Config:        config,
					SandboxConfig: &types.PodSandboxConfig{Metadata: &types.PodSandboxMetadata{}},
				})

			// Then
			Expect(status.Code(err)).To(Equal(codes.InvalidArgument))
			Expect(response).To(BeNil())
		})
	})

	t.Describe("Status", func() {
		It("should report OCI security profiles if seccomp is enabled", func() {
			// Given
			Expect(serverConfig.ReloadSeccompProfile(serverConfig)).To(Succeed())

			// When
			response, err := sut.Status(context.Background(), &types.StatusRequest{})

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(response.GetFeatures().GetSeccompProfileOci()).To(
				Equal(!serverConfig.Seccomp().IsDisabled()),
			)
		})

		It("should not report OCI security profiles with an invalid baseline", func() {
			// Given
			profile := t.MustTempFile("seccomp")
			Expect(os.WriteFile(profile, []byte(`{"defaultAction": "SCMP_ACT_WRONG"}`), 0o644)).
				To(Succeed())

			newConfig, err := config.DefaultConfig()
			Expect(err).ToNot(HaveOccurred())

			newConfig.SeccompProfile = profile
			Expect(serverConfig.ReloadSeccompProfile(newConfig)).To(Succeed())

			// When
			response, err := sut.Status(context.Background(), &types.StatusRequest{})

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(response.GetFeatures().GetSeccompProfileOci()).To(BeFalse())
		})
	})
})

var _ = t.Describe("SecurityProfile pull", func() {
	const namespace = "team"

	var (
		registry   *fakeProfileRegistry
		policyDir  string
		authDir    string
		profileRef string
	)

	BeforeEach(func() {
		beforeEach()

		policyDir = t.MustTempDir("policies")
		authDir = t.MustTempDir("auth")
		acceptPolicy := filepath.Join(t.MustTempDir("policy"), "policy.json")
		Expect(
			os.WriteFile(
				acceptPolicy,
				[]byte(`{"default": [{"type": "insecureAcceptAnything"}]}`),
				0o644,
			),
		).
			To(Succeed())

		serverConfig.SignaturePolicyDir = policyDir
		serverConfig.NamespacedAuthDir = authDir
		serverConfig.SystemContext.SignaturePolicyPath = acceptPolicy

		setupSUT()

		registry = newFakeProfileRegistry()
		profileRef = registry.ref
		sut.SecurityProfiles().SetOpenSource(registry.open)
	})

	AfterEach(afterEach)

	pull := func(namespace string) (*types.PullSecurityProfileResponse, error) {
		return sut.PullSecurityProfile(context.Background(), &types.PullSecurityProfileRequest{
			Image: &types.ImageSpec{Image: profileRef, UserSpecifiedImage: profileRef},
			SandboxConfig: &types.PodSandboxConfig{
				Metadata: &types.PodSandboxMetadata{Namespace: namespace},
			},
			ProfileKind: types.SecurityProfileKind_Seccomp,
		})
	}

	writeAuthFile := func() string {
		named, err := reference.ParseNormalizedNamed(profileRef)
		Expect(err).ToNot(HaveOccurred())

		path, err := auth.FilePath(authDir, namespace, named.Name())
		Expect(err).ToNot(HaveOccurred())
		Expect(os.WriteFile(path, []byte(`{"auths": {}}`), 0o600)).To(Succeed())

		return path
	}

	It("should pull with the namespaced auth file and discard it when cached", func() {
		// Given
		authFile := writeAuthFile()

		// When
		response, err := pull(namespace)

		// Then
		Expect(err).ToNot(HaveOccurred())
		Expect(response.GetCached()).To(BeFalse())
		Expect(registry.authFiles).To(HaveLen(1))
		Expect(registry.authFiles[0]).To(ContainSubstring("in-use"))
		Expect(authFile).NotTo(BeAnExistingFile())

		// And when
		authFile = writeAuthFile()
		response, err = pull(namespace)

		// Then
		Expect(err).ToNot(HaveOccurred())
		Expect(response.GetCached()).To(BeTrue())
		Expect(registry.authFiles).To(HaveLen(1))
		Expect(authFile).NotTo(BeAnExistingFile())
	})

	It("should apply the signature policy of the namespace to a present profile", func() {
		// Given
		_, err := pull("")
		Expect(err).ToNot(HaveOccurred())

		Expect(os.WriteFile(
			filepath.Join(policyDir, namespace+".json"),
			[]byte(`{"default": [{"type": "reject"}]}`),
			0o644,
		)).To(Succeed())

		// When
		_, err = pull(namespace)

		// Then
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(HavePrefix("SignatureValidationFailed: "))
		Expect(registry.authFiles).To(HaveLen(2))

		// A container of the namespace cannot use it either.
		_, err = sut.SecurityProfiles().For(namespace, "ctr").MergeSeccomp(
			context.Background(), profileRef, nil, nil,
		)
		Expect(status.Code(err)).To(Equal(codes.FailedPrecondition))

		_, err = sut.SecurityProfiles().For("", "ctr").MergeSeccomp(
			context.Background(), profileRef, nil, nil,
		)
		Expect(err).ToNot(HaveOccurred())
	})

	It("should list and remove a pulled profile", func() {
		// Given
		_, err := pull("")
		Expect(err).ToNot(HaveOccurred())

		dgst := digest.FromBytes(registry.manifest).String()

		// When
		listed, err := sut.ListSecurityProfiles(
			context.Background(), &types.ListSecurityProfilesRequest{},
		)

		// Then
		Expect(err).ToNot(HaveOccurred())
		Expect(listed.GetProfiles()).To(HaveLen(1))
		Expect(listed.GetProfiles()[0].GetDigest()).To(Equal(dgst))
		Expect(listed.GetProfiles()[0].GetRefs()).To(Equal([]string{profileRef}))
		Expect(listed.GetProfiles()[0].GetSize()).To(BeNumerically(">", 0))

		// And when
		for range 2 {
			_, err = sut.RemoveSecurityProfile(
				context.Background(), &types.RemoveSecurityProfileRequest{Digest: dgst},
			)
			Expect(err).ToNot(HaveOccurred())
		}

		// Then
		listed, err = sut.ListSecurityProfiles(
			context.Background(), &types.ListSecurityProfilesRequest{},
		)
		Expect(err).ToNot(HaveOccurred())
		Expect(listed.GetProfiles()).To(BeEmpty())

		response, err := pull("")
		Expect(err).ToNot(HaveOccurred())
		Expect(response.GetCached()).To(BeFalse())
	})

	It("should reject the removal of an invalid digest", func() {
		// Given
		// When
		response, err := sut.RemoveSecurityProfile(
			context.Background(), &types.RemoveSecurityProfileRequest{Digest: profileRef},
		)

		// Then
		Expect(status.Code(err)).To(Equal(codes.InvalidArgument))
		Expect(response).To(BeNil())
	})
})

// fakeProfileRegistry serves one seccomp profile artifact.
type fakeProfileRegistry struct {
	ref       string
	manifest  []byte
	blobs     map[digest.Digest][]byte
	authFiles []string
}

func newFakeProfileRegistry() *fakeProfileRegistry {
	configData := []byte("{}")
	layer := []byte(`{"defaultAction": "SCMP_ACT_ALLOW"}`)
	manifest, err := json.Marshal(ispec.Manifest{
		SchemaVersion: 2,
		MediaType:     ispec.MediaTypeImageManifest,
		Config: ispec.Descriptor{
			MediaType: securityprofile.SeccompConfigMediaType,
			Digest:    digest.FromBytes(configData),
			Size:      int64(len(configData)),
		},
		Layers: []ispec.Descriptor{{
			MediaType: "application/json",
			Digest:    digest.FromBytes(layer),
			Size:      int64(len(layer)),
		}},
	})
	Expect(err).ToNot(HaveOccurred())

	return &fakeProfileRegistry{
		ref:      "registry.example.com/profile@" + digest.FromBytes(manifest).String(),
		manifest: manifest,
		blobs: map[digest.Digest][]byte{
			digest.FromBytes(configData): configData,
			digest.FromBytes(layer):      layer,
		},
	}
}

func (f *fakeProfileRegistry) open(
	_ context.Context,
	ref reference.Named,
	sys *imagetypes.SystemContext,
) (imagetypes.ImageSource, error) {
	f.authFiles = append(f.authFiles, sys.AuthFilePath)

	imageRef, err := docker.NewReference(ref)
	if err != nil {
		return nil, err
	}

	return &fakeProfileSource{registry: f, ref: imageRef}, nil
}

type fakeProfileSource struct {
	registry *fakeProfileRegistry
	ref      imagetypes.ImageReference
}

func (s *fakeProfileSource) Reference() imagetypes.ImageReference { return s.ref }
func (s *fakeProfileSource) Close() error                         { return nil }
func (s *fakeProfileSource) HasThreadSafeGetBlob() bool           { return false }

func (s *fakeProfileSource) GetManifest(
	context.Context, *digest.Digest,
) (raw []byte, mimeType string, err error) {
	return s.registry.manifest, ispec.MediaTypeImageManifest, nil
}

//nolint:gocritic // The signature is the one of types.ImageSource.
func (s *fakeProfileSource) GetBlob(
	_ context.Context,
	info imagetypes.BlobInfo,
	_ imagetypes.BlobInfoCache,
) (io.ReadCloser, int64, error) {
	data := s.registry.blobs[info.Digest]

	return io.NopCloser(bytes.NewReader(data)), int64(len(data)), nil
}

func (s *fakeProfileSource) GetSignatures(context.Context, *digest.Digest) ([][]byte, error) {
	return nil, nil
}

func (s *fakeProfileSource) LayerInfosForCopy(
	context.Context, *digest.Digest,
) ([]imagetypes.BlobInfo, error) {
	return nil, nil
}
