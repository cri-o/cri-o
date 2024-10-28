package storage_test

import (
	"context"
	"fmt"
	"os"

	"github.com/containers/image/v5/docker/reference"
	istorage "github.com/containers/image/v5/storage"
	"github.com/containers/image/v5/types"
	cs "github.com/containers/storage"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	digest "github.com/opencontainers/go-digest"
	"go.uber.org/mock/gomock"

	"github.com/cri-o/cri-o/internal/mockutils"
	"github.com/cri-o/cri-o/internal/storage"
	"github.com/cri-o/cri-o/internal/storage/references"
	"github.com/cri-o/cri-o/pkg/config"
	containerstoragemock "github.com/cri-o/cri-o/test/mocks/containerstorage"
	criostoragemock "github.com/cri-o/cri-o/test/mocks/criostorage"
)

// The actual test suite.
var _ = t.Describe("Image", func() {
	// Test constants
	const (
		testDockerRegistry                  = "docker.io"
		testQuayRegistry                    = "quay.io"
		testRedHatRegistry                  = "registry.access.redhat.com"
		testFedoraRegistry                  = "registry.fedoraproject.org"
		testImageName                       = "image"
		testImageAlias                      = "image-for-testing"
		testImageAliasResolved              = "registry.crio.test.com/repo"
		testNormalizedImageName             = "docker.io/library/image:latest" // Keep in sync with testImageName!
		testSHA256                          = "2a03a6059f21e150ae84b0973863609494aad70f0a80eaeb64bddd8d92465812"
		testImageWithTagAndDigest           = "image:latest@sha256:" + testSHA256
		testNormalizedImageWithTagAndDigest = "docker.io/library/image:latest@sha256:" + testSHA256
	)

	var (
		mockCtrl             *gomock.Controller
		storeMock            *containerstoragemock.MockStore
		storageTransportMock *criostoragemock.MockStorageTransport

		// The system under test
		sut storage.ImageServer

		// The empty system context
		ctx *types.SystemContext
	)

	// Prepare the system under test
	BeforeEach(func() {
		// Setup the mocks
		mockCtrl = gomock.NewController(GinkgoT())
		storeMock = containerstoragemock.NewMockStore(mockCtrl)
		storageTransportMock = criostoragemock.NewMockStorageTransport(mockCtrl)

		// Setup the SUT
		var err error
		ctx = &types.SystemContext{
			SystemRegistriesConfPath: t.MustTempFile("registries"),
		}
		config := &config.Config{
			SystemContext: &types.SystemContext{
				SystemRegistriesConfPath: t.MustTempFile("registries"),
			},
			ImageConfig: config.ImageConfig{
				DefaultTransport:   "docker://",
				InsecureRegistries: []string{},
			},
		}

		sut, err = storage.GetImageService(
			context.Background(), storeMock, storageTransportMock, config,
		)
		Expect(err).ToNot(HaveOccurred())
		Expect(sut).NotTo(BeNil())
	})
	AfterEach(func() {
		mockCtrl.Finish()
		Expect(os.Remove(ctx.SystemRegistriesConfPath)).To(Succeed())
	})

	t.Describe("GetImageService", func() {
		It("should succeed to retrieve an image service", func() {
			// Given
			// When
			imageService, err := storage.GetImageService(
				context.Background(), storeMock, storageTransportMock, &config.Config{},
			)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(imageService).NotTo(BeNil())
		})

		It("should succeed with custom registries.conf", func() {
			// Given
			// When
			config := &config.Config{
				SystemContext: &types.SystemContext{
					SystemRegistriesConfPath: "../../test/registries.conf",
				},
				ImageConfig: config.ImageConfig{
					DefaultTransport:   "",
					InsecureRegistries: []string{},
				},
			}
			imageService, err := storage.GetImageService(
				context.Background(),
				storeMock, storageTransportMock, config,
			)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(imageService).NotTo(BeNil())
		})
	})

	t.Describe("GetStore", func() {
		It("should succeed to retrieve the store", func() {
			// Given
			gomock.InOrder(
				storeMock.EXPECT().Delete(gomock.Any()).Return(nil),
			)

			// When
			store := sut.GetStore()

			// Then
			Expect(store).NotTo(BeNil())
			Expect(store.Delete("")).To(Succeed())
		})
	})

	t.Describe("HeuristicallyTryResolvingStringAsIDPrefix", func() {
		It("should not match an unrelated name of an existing image", func() {
			// Given
			gomock.InOrder(
				storeMock.EXPECT().Image(testImageName).
					Return(&cs.Image{ID: "id"}, nil),
			)

			// When
			id := sut.HeuristicallyTryResolvingStringAsIDPrefix(
				testImageName,
			)

			// Then
			Expect(id).To(BeNil())
		})

		It("should match a locally-not-matching image id", func() {
			// Given
			gomock.InOrder()

			// When
			id := sut.HeuristicallyTryResolvingStringAsIDPrefix(testSHA256)

			// Then
			Expect(id).NotTo(BeNil())
			Expect(id.IDStringForOutOfProcessConsumptionOnly()).To(Equal(testSHA256))
		})
	})

	t.Describe("CandidatesForPotentiallyShortImageName", func() {
		refsToNames := func(refs []storage.RegistryImageReference) []string {
			names := []string{}
			for _, ref := range refs {
				names = append(names, ref.StringForOutOfProcessConsumptionOnly())
			}
			return names
		}

		It("should succeed to resolve", func() {
			// Given
			gomock.InOrder()

			// When
			refs, err := sut.CandidatesForPotentiallyShortImageName(
				&types.SystemContext{
					SystemRegistriesConfPath: "../../test/registries.conf",
				},
				testImageName,
			)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(refsToNames(refs)).To(Equal([]string{
				testQuayRegistry + "/" + testImageName + ":latest",
				testRedHatRegistry + "/" + testImageName + ":latest",
				testFedoraRegistry + "/" + testImageName + ":latest",
				testDockerRegistry + "/library/" + testImageName + ":latest",
			}))
		})

		It("should succeed to resolve to a short-name alias", func() {
			// Given
			gomock.InOrder()

			// When
			refs, err := sut.CandidatesForPotentiallyShortImageName(
				&types.SystemContext{
					SystemRegistriesConfPath: "../../test/registries.conf",
				},
				testImageAlias,
			)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(refsToNames(refs)).To(Equal([]string{
				testImageAliasResolved + ":latest",
			}))
		})

		It("should succeed to resolve with full qualified image name", func() {
			// Given
			const imageName = "docker.io/library/busybox:latest"
			gomock.InOrder()

			// When
			refs, err := sut.CandidatesForPotentiallyShortImageName(ctx, imageName)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(refs).To(HaveLen(1))
			Expect(refs[0].StringForOutOfProcessConsumptionOnly()).To(Equal(imageName))
		})

		It("should succeed to resolve image name with tag and digest", func() {
			// Given
			gomock.InOrder()

			// When
			refs, err := sut.CandidatesForPotentiallyShortImageName(
				&types.SystemContext{
					SystemRegistriesConfPath: "../../test/registries.conf",
				},
				testImageWithTagAndDigest,
			)
			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(refsToNames(refs)).To(Equal([]string{
				testQuayRegistry + "/" + testImageName + "@sha256:" + testSHA256,
				testRedHatRegistry + "/" + testImageName + "@sha256:" + testSHA256,
				testFedoraRegistry + "/" + testImageName + "@sha256:" + testSHA256,
				testDockerRegistry + "/library/" + testImageName + "@sha256:" + testSHA256,
			}))
		})

		It("should succeed to resolve fully qualified image name with tag and digest", func() {
			// Given
			gomock.InOrder()

			// When
			refs, err := sut.CandidatesForPotentiallyShortImageName(ctx, testNormalizedImageWithTagAndDigest)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(refsToNames(refs)).To(Equal([]string{
				testDockerRegistry + "/library/" + testImageName + "@sha256:" + testSHA256,
			}))
		})

		It("should fail to resolve with invalid registry name", func() {
			// Given
			gomock.InOrder()

			// When
			refs, err := sut.CandidatesForPotentiallyShortImageName(ctx, "camelCaseName")

			// Then
			Expect(err).To(HaveOccurred())
			Expect(refs).To(BeNil())
		})

		It("should fail to resolve without configured registries", func() {
			// Given
			gomock.InOrder()
			config := &config.Config{
				SystemContext: ctx,
				ImageConfig: config.ImageConfig{
					DefaultTransport:   "",
					InsecureRegistries: []string{},
				},
			}
			// Create an empty file for the registries config path
			sut, err := storage.GetImageService(context.Background(), storeMock, storageTransportMock, config)
			Expect(err).ToNot(HaveOccurred())
			Expect(sut).NotTo(BeNil())

			// When
			refs, err := sut.CandidatesForPotentiallyShortImageName(
				&types.SystemContext{
					SystemRegistriesConfPath: "/dev/null",
				},
				testImageName,
			)

			// Then
			Expect(err).To(HaveOccurred())
			errString := fmt.Sprintf("short-name %q did not resolve to an alias and no unqualified-search registries are defined in %q", testImageName, "/dev/null")
			Expect(err.Error()).To(Equal(errString))
			Expect(refs).To(BeNil())
		})
	})

	t.Describe("UntagImage", func() {
		It("should succeed to untag an image", func() {
			// Given
			mockutils.InOrder(
				mockResolveReference(storeMock, storageTransportMock,
					testNormalizedImageName, "", testSHA256),
				storeMock.EXPECT().Image(testSHA256).
					Return(&cs.Image{ID: testSHA256}, nil),
				storeMock.EXPECT().DeleteImage(testSHA256, true).
					Return(nil, nil),
			)
			ref, err := references.ParseRegistryImageReferenceFromOutOfProcessData(testImageName)
			Expect(err).ToNot(HaveOccurred())

			// When
			err = sut.UntagImage(&types.SystemContext{}, ref)

			// Then
			Expect(err).ToNot(HaveOccurred())
		})

		It("should fail to untag an image that can't be found", func() {
			// Given
			mockutils.InOrder(
				mockResolveReference(storeMock, storageTransportMock,
					testNormalizedImageName, "", ""),
			)
			ref, err := references.ParseRegistryImageReferenceFromOutOfProcessData(testImageName)
			Expect(err).ToNot(HaveOccurred())

			// When
			err = sut.UntagImage(&types.SystemContext{}, ref)

			// Then
			Expect(err).To(HaveOccurred())
		})

		It("should fail to untag an image with multiple names", func() {
			// Given
			namedRef, err := reference.ParseNormalizedNamed(testImageName)
			Expect(err).ToNot(HaveOccurred())
			namedRef = reference.TagNameOnly(namedRef)
			expectedRef, err := istorage.Transport.NewStoreReference(storeMock, namedRef, "")
			Expect(err).ToNot(HaveOccurred())
			resolvedRef, err := istorage.Transport.NewStoreReference(storeMock, namedRef, testSHA256)
			Expect(err).ToNot(HaveOccurred())
			mockutils.InOrder(
				storageTransportMock.EXPECT().ResolveReference(expectedRef).
					Return(resolvedRef,
						&cs.Image{
							ID:    testSHA256,
							Names: []string{testNormalizedImageName, "localhost/b:latest", "localhost/c:latest"},
						},
						nil),

				storeMock.EXPECT().RemoveNames(testSHA256, []string{"docker.io/library/image:latest"}).
					Return(t.TestError),
			)
			ref, err := references.ParseRegistryImageReferenceFromOutOfProcessData(testImageName)
			Expect(err).ToNot(HaveOccurred())

			// When
			err = sut.UntagImage(&types.SystemContext{}, ref)

			// Then
			Expect(err).To(HaveOccurred())
		})
	})

	t.Describe("ImageStatusByName", func() {
		It("should succeed to get the image status with digest", func() {
			namedRef, err := reference.ParseNormalizedNamed(testImageName)
			Expect(err).ToNot(HaveOccurred())
			namedRef = reference.TagNameOnly(namedRef)
			expectedRef, err := istorage.Transport.NewStoreReference(storeMock, namedRef, "")
			Expect(err).ToNot(HaveOccurred())
			resolvedRef, err := istorage.Transport.NewStoreReference(storeMock, namedRef, testSHA256)
			Expect(err).ToNot(HaveOccurred())
			// Given
			mockutils.InOrder(
				storageTransportMock.EXPECT().ResolveReference(expectedRef).
					Return(resolvedRef,
						&cs.Image{
							ID: testSHA256,
							Names: []string{
								testNormalizedImageName,
								"localhost/a@sha256:" + testSHA256,
								"localhost/b@sha256:" + testSHA256,
								"localhost/c:latest",
							},
						}, nil),
				// buildImageCacheItem
				mockNewImage(storeMock, namedRef.String(), testSHA256, testSHA256),
				storeMock.EXPECT().Image(testSHA256).
					Return(&cs.Image{
						ID: testSHA256,
						Names: []string{
							testNormalizedImageName,
							"localhost/a@sha256:" + testSHA256,
							"localhost/b@sha256:" + testSHA256,
							"localhost/c:latest",
						},
					}, nil),
				storeMock.EXPECT().ImageBigData(testSHA256, gomock.Any()).
					Return(nil, nil),
				// makeRepoDigests
				storeMock.EXPECT().ImageBigDataDigest(testSHA256, gomock.Any()).
					Return(digest.Digest("a:"+testSHA256), nil),
				storeMock.EXPECT().Layer(gomock.Any()).Return(&cs.Layer{}, nil),
			)
			ref, err := references.ParseRegistryImageReferenceFromOutOfProcessData(testImageName)
			Expect(err).ToNot(HaveOccurred())

			// When
			res, err := sut.ImageStatusByName(&types.SystemContext{}, ref)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(res).NotTo(BeNil())
		})

		It("should fail to get on missing store image", func() {
			// Given
			mockutils.InOrder(
				mockResolveReference(storeMock, storageTransportMock,
					testNormalizedImageName, "", ""),
			)
			ref, err := references.ParseRegistryImageReferenceFromOutOfProcessData(testImageName)
			Expect(err).ToNot(HaveOccurred())

			// When
			res, err := sut.ImageStatusByName(&types.SystemContext{}, ref)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(res).To(BeNil())
		})

		It("should fail to get on corrupt image", func() {
			// Given
			mockutils.InOrder(
				mockResolveReference(storeMock, storageTransportMock,
					testNormalizedImageName, "", testSHA256),
				// In buildImageCacheItem, storageReference.NewImage fails reading the manifest:
				mockResolveImage(storeMock, testNormalizedImageName, testSHA256, testSHA256),
				storeMock.EXPECT().ImageBigData(testSHA256, gomock.Any()).
					Return(nil, t.TestError),
			)
			ref, err := references.ParseRegistryImageReferenceFromOutOfProcessData(testImageName)
			Expect(err).ToNot(HaveOccurred())

			// When
			res, err := sut.ImageStatusByName(&types.SystemContext{}, ref)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(res).To(BeNil())
		})
	})

	t.Describe("ListImages", func() {
		It("should succeed to list images without filter", func() {
			// Given
			gomock.InOrder(
				storeMock.EXPECT().Images().Return([]cs.Image{}, nil),
			)

			// When
			res, err := sut.ListImages(&types.SystemContext{})

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(res).To(BeEmpty())
		})

		It("should succeed to list multiple images without filter", func() {
			// Given
			mockLoop := func() mockutils.MockSequence {
				return mockutils.InOrder(
					// buildImageCacheItem:
					mockNewImage(storeMock, "", testSHA256, testSHA256),
					storeMock.EXPECT().Image(gomock.Any()).
						Return(&cs.Image{
							ID: testSHA256,
							Names: []string{
								"localhost/c:latest",
							},
						}, nil),
					storeMock.EXPECT().ImageBigData(testSHA256, gomock.Any()).
						Return(nil, nil),
					// makeRepoDigests:
					storeMock.EXPECT().ImageBigDataDigest(testSHA256, gomock.Any()).
						Return(digest.Digest(""), nil),
					storeMock.EXPECT().Layer(gomock.Any()).Return(&cs.Layer{}, nil),
				)
			}
			mockutils.InOrder(
				storeMock.EXPECT().Images().Return(
					[]cs.Image{
						{ID: testSHA256, Names: []string{"a:latest", "b:notlatest", "c@sha256:" + testSHA256}},
						{ID: testSHA256},
					},
					nil),
				mockLoop(),
				mockLoop(),
			)

			// When
			res, err := sut.ListImages(&types.SystemContext{})

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(res).To(HaveLen(2))
		})

		It("should fail to list images without a filter on failing store", func() {
			// Given
			gomock.InOrder(
				storeMock.EXPECT().Images().Return(nil, t.TestError),
			)

			// When
			res, err := sut.ListImages(&types.SystemContext{})

			// Then
			Expect(err).To(HaveOccurred())
			Expect(res).To(BeNil())
		})

		It("should fail to list multiple images without filter on invalid image ID in results", func() {
			// Given
			gomock.InOrder(
				storeMock.EXPECT().Images().Return(
					[]cs.Image{{ID: ""}}, nil),
			)

			// When
			res, err := sut.ListImages(&types.SystemContext{})

			// Then
			Expect(err).To(HaveOccurred())
			Expect(res).To(BeNil())
		})

		It("should fail to list multiple images without filter on append", func() {
			// Given
			mockutils.InOrder(
				storeMock.EXPECT().Images().Return(
					[]cs.Image{{ID: testSHA256}}, nil),
				storeMock.EXPECT().Image(gomock.Any()).
					Return(nil, t.TestError),
			)

			// When
			res, err := sut.ListImages(&types.SystemContext{})

			// Then
			Expect(err).To(HaveOccurred())
			Expect(res).To(BeNil())
		})
	})

	t.Describe("PullImage", func() {
		It("should fail on invalid policy path", func() {
			// Given
			imageRef, err := references.ParseRegistryImageReferenceFromOutOfProcessData("localhost/busybox:latest")
			Expect(err).ToNot(HaveOccurred())

			// When
			res, err := sut.PullImage(context.Background(), imageRef, &storage.ImageCopyOptions{
				SourceCtx: &types.SystemContext{SignaturePolicyPath: "/not-existing"},
			})

			// Then
			Expect(err).To(HaveOccurred())
			Expect(res).To(Equal(storage.RegistryImageReference{}))
		})

		It("should fail on copy image", func() {
			// Given
			imageRef, err := references.ParseRegistryImageReferenceFromOutOfProcessData("localhost/busybox:latest")
			Expect(err).ToNot(HaveOccurred())

			// When
			res, err := sut.PullImage(context.Background(), imageRef, &storage.ImageCopyOptions{
				SourceCtx: &types.SystemContext{SignaturePolicyPath: "../../test/policy.json"},
			})

			// Then
			Expect(err).To(HaveOccurred())
			Expect(res).To(Equal(storage.RegistryImageReference{}))
		})

		It("should fail on canonical copy image", func() {
			// Given
			imageRef, err := references.ParseRegistryImageReferenceFromOutOfProcessData("localhost/busybox@sha256:" + testSHA256)
			Expect(err).ToNot(HaveOccurred())

			// When
			res, err := sut.PullImage(context.Background(), imageRef, &storage.ImageCopyOptions{
				SourceCtx: &types.SystemContext{SignaturePolicyPath: "../../test/policy.json"},
			})

			// Then
			Expect(err).To(HaveOccurred())
			Expect(res).To(Equal(storage.RegistryImageReference{}))
		})

		It("should fail on cancelled context", func() {
			// Given
			imageRef, err := references.ParseRegistryImageReferenceFromOutOfProcessData("localhost/busybox:latest")
			Expect(err).ToNot(HaveOccurred())

			// When
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			res, err := sut.PullImage(ctx, imageRef, &storage.ImageCopyOptions{
				SourceCtx: &types.SystemContext{SignaturePolicyPath: "../../test/policy.json"},
			})

			// Then
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("context canceled"))
			Expect(res).To(Equal(storage.RegistryImageReference{}))
		})

		It("should fail on timed out context", func() {
			// Given
			imageRef, err := references.ParseRegistryImageReferenceFromOutOfProcessData("localhost/busybox:latest")
			Expect(err).ToNot(HaveOccurred())

			// When
			ctx, cancel := context.WithTimeout(context.Background(), 0)
			defer cancel()
			res, err := sut.PullImage(ctx, imageRef, &storage.ImageCopyOptions{
				SourceCtx: &types.SystemContext{SignaturePolicyPath: "../../test/policy.json"},
			})

			// Then
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("context deadline exceeded"))
			Expect(res).To(Equal(storage.RegistryImageReference{}))
		})
	})

	t.Describe("CompileRegexpsForPinnedImages", func() {
		It("should return regexps for exact patterns", func() {
			patterns := []string{"quay.io/crio/pause:latest", "docker.io/crio/sandbox:latest", "registry.k8s.io/pause:3.10"}
			regexps := storage.CompileRegexpsForPinnedImages(patterns)
			Expect(regexps).To(HaveLen(len(patterns)))
			Expect(regexps[0].MatchString("quay.io/crio/pause:latest")).To(BeTrue())
			Expect(regexps[1].MatchString("docker.io/crio/sandbox:latest")).To(BeTrue())
			Expect(regexps[2].MatchString("registry.k8s.io/pause:3.10")).To(BeTrue())
		})

		It("should return regexps for keyword patterns", func() {
			patterns := []string{"*Fedora*"}
			regexps := storage.CompileRegexpsForPinnedImages(patterns)
			Expect(regexps).To(HaveLen(len(patterns)))
			Expect(regexps[0].MatchString("quay.io/crio/Fedora34:latest")).To(BeTrue())
		})

		It("should return regexps for glob patterns", func() {
			patterns := []string{"quay.io/*", "*Fedora*", "docker.io/*"}
			regexps := storage.CompileRegexpsForPinnedImages(patterns)
			Expect(regexps).To(HaveLen(len(patterns)))
			Expect(regexps[0].MatchString("quay.io/test/image")).To(BeTrue())
			Expect(regexps[1].MatchString("gcr.io/CRIO-Fedora34")).To(BeTrue())
			Expect(regexps[2].MatchString("docker.io/test/image")).To(BeTrue())
		})

		It("should panic for invalid pattern", func() {
			patterns := []string{"*"}
			Expect(func() { storage.CompileRegexpsForPinnedImages(patterns) }).To(Panic())
		})
	})
})
