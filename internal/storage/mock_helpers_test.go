package storage_test

import (
	"github.com/containers/image/v5/docker/reference"
	istorage "github.com/containers/image/v5/storage"
	cstorage "github.com/containers/storage"
	"github.com/cri-o/cri-o/internal/mockutils"
	containerstoragemock "github.com/cri-o/cri-o/test/mocks/containerstorage"
	criostoragemock "github.com/cri-o/cri-o/test/mocks/criostorage"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/gomega"
)

// containers/image/storage.storageReference.StringWithinTransport
func mockStorageReferenceStringWithinTransport(storeMock *containerstoragemock.MockStore) mockutils.MockSequence {
	return mockutils.InOrder(
		storeMock.EXPECT().GraphOptions().Return([]string{}),
		storeMock.EXPECT().GraphDriverName().Return(""),
		storeMock.EXPECT().GraphRoot().Return(""),
		storeMock.EXPECT().RunRoot().Return(""),
	)
}

// containers/image/storage.ResolveReference
// expectedImageName must be in the fully normalized format (reference.Named.String())!
// resolvedImageID may be "" to simulate a missing image
func mockResolveReference(storeMock *containerstoragemock.MockStore, storageTransportMock *criostoragemock.MockStorageTransport, expectedImageName, expectedImageID, resolvedImageID string) mockutils.MockSequence { //nolint:unparam
	var namedRef reference.Named
	if expectedImageName != "" {
		nr, err := reference.ParseNormalizedNamed(expectedImageName)
		Expect(err).ToNot(HaveOccurred())
		namedRef = nr
	}
	expectedRef, err := istorage.Transport.NewStoreReference(storeMock, namedRef, expectedImageID)
	Expect(err).ToNot(HaveOccurred())
	if resolvedImageID == "" {
		return mockutils.InOrder(
			storageTransportMock.EXPECT().ResolveReference(expectedRef).
				Return(nil, nil, istorage.ErrNoSuchImage),
		)
	}
	resolvedRef, err := istorage.Transport.NewStoreReference(storeMock, namedRef, resolvedImageID)
	Expect(err).ToNot(HaveOccurred())
	return mockutils.InOrder(
		storageTransportMock.EXPECT().ResolveReference(expectedRef).
			Return(resolvedRef,
				&cstorage.Image{ID: resolvedImageID, Names: []string{expectedImageName}},
				nil),
	)
}

// containers/image/storage.storageReference.resolveImage
// expectedImageName, if not "", must be in the fully normalized format (reference.Named.String())!
// expectedImageID may be ""
// resolvedImageID may be "" to simulate a missing image
func mockResolveImage(storeMock *containerstoragemock.MockStore, expectedImageName, expectedImageID, resolvedImageID string) mockutils.MockSequence {
	lookupKey := expectedImageID
	if lookupKey == "" {
		lookupKey = expectedImageName
	}
	if resolvedImageID == "" {
		return mockutils.InOrder(
			storeMock.EXPECT().Image(lookupKey).Return(nil, cstorage.ErrImageUnknown),
			// Assuming lookupKey does not have a digest, so resolveName does not call ImagesByDigest
			mockStorageReferenceStringWithinTransport(storeMock),
			mockStorageReferenceStringWithinTransport(storeMock),
		)
	}
	return mockutils.InOrder(
		storeMock.EXPECT().Image(lookupKey).
			Return(&cstorage.Image{ID: resolvedImageID, Names: []string{expectedImageName}}, nil),
	)
}

// containers/image/storage.storageImageSource.getSize
func mockStorageImageSourceGetSize(storeMock *containerstoragemock.MockStore) mockutils.MockSequence {
	return mockutils.InOrder(
		storeMock.EXPECT().ListImageBigData(gomock.Any()).
			Return([]string{""}, nil), // A single entry
		storeMock.EXPECT().ImageBigDataSize(gomock.Any(), gomock.Any()).
			Return(int64(0), nil),
		// FIXME: This should also walk through the layer list and call store.Layer() on each, but we would have to mock the whole layer list.
	)
}

// containers/image/storage.storageReference.newImage
func mockNewImage(storeMock *containerstoragemock.MockStore, expectedImageName, expectedImageID, resolvedImageID string) mockutils.MockSequence {
	return mockutils.InOrder(
		mockResolveImage(storeMock, expectedImageName, expectedImageID, resolvedImageID),
		storeMock.EXPECT().ImageBigData(gomock.Any(), gomock.Any()).
			Return(testManifest, nil),
		mockStorageImageSourceGetSize(storeMock),
	)
}
