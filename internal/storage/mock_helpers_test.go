package storage_test

import (
	"fmt"
	"strings"

	cstorage "github.com/containers/storage"
	containerstoragemock "github.com/cri-o/cri-o/test/mocks/containerstorage"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
)

type mockSequence struct {
	first, last *gomock.Call // may be both nil (= the default value of mockSequence) to mean empty sequence
}

// like gomock.inOrder, but can be nested
func inOrder(calls ...interface{}) mockSequence {
	var first, last *gomock.Call
	// This implementation does a few more assignments and checks than strictly necessary, but it is O(N) and reasonably easy to read, so, whatever.
	for i := 0; i < len(calls); i++ {
		var elem mockSequence
		switch e := calls[i].(type) {
		case mockSequence:
			elem = e
		case *gomock.Call:
			elem = mockSequence{e, e}
		default:
			Fail(fmt.Sprintf("Invalid inOrder parameter %#v", e))
		}

		if elem.first == nil {
			continue
		}
		if first == nil {
			first = elem.first
		} else if last != nil {
			elem.first.After(last)
		}
		last = elem.last
	}
	return mockSequence{first, last}
}

// containers/image/storage.storageReference.StringWithinTransport
func mockStorageReferenceStringWithinTransport(storeMock *containerstoragemock.MockStore) mockSequence {
	return inOrder(
		storeMock.EXPECT().GraphOptions().Return([]string{}),
		storeMock.EXPECT().GraphDriverName().Return(""),
		storeMock.EXPECT().GraphRoot().Return(""),
		storeMock.EXPECT().RunRoot().Return(""),
	)
}

// containers/image/storage.Transport.ParseStoreReference
func mockParseStoreReference(storeMock *containerstoragemock.MockStore, expectedImageName string) mockSequence {
	// ParseStoreReference calls store.Image() to check whether short strings are possible prefixes of IDs of existing images
	// (either using the unambiguous "@idPrefix" syntax, or the ambiguous "idPrefix" syntax).
	// None of our tests use ID prefixes (only full IDs), so we can safely and correctly return ErrImageUnknown in all such cases;
	// it only matters that we include, or not, the mock expectation.
	//
	// This hard-codes a heuristic in ParseStoreReference for whether to consider expectedImageName a possible ID prefix.
	// The "@" check also happens to exclude full @digest values, which can happen but would not trigger a store.Image() lookup.
	var c1 *gomock.Call
	if len(expectedImageName) >= 3 && !strings.ContainsAny(expectedImageName, "@:") {
		c1 = storeMock.EXPECT().Image(expectedImageName).Return(nil, cstorage.ErrImageUnknown)
	}
	return inOrder(
		c1,
		mockStorageReferenceStringWithinTransport(storeMock),
	)
}

// containers/image/storage.Transport.GetStoreImage
// expectedImageName must be in the fully normalized format (reference.Named.String())!
// resolvedImageID may be "" to simulate a missing image
func mockGetStoreImage(storeMock *containerstoragemock.MockStore, expectedImageName, resolvedImageID string) mockSequence {
	if resolvedImageID == "" {
		return inOrder(
			storeMock.EXPECT().Image(expectedImageName).Return(nil, cstorage.ErrImageUnknown),
			mockResolveImage(storeMock, expectedImageName, ""),
		)
	}
	return inOrder(
		storeMock.EXPECT().Image(expectedImageName).
			Return(&cstorage.Image{ID: resolvedImageID, Names: []string{expectedImageName}}, nil),
	)
}

// containers/image/storage.storageReference.resolveImage
// expectedImageNameOrID, if a name, must be in the fully normalized format (reference.Named.String())!
// resolvedImageID may be "" to simulate a missing image
func mockResolveImage(storeMock *containerstoragemock.MockStore, expectedImageNameOrID, resolvedImageID string) mockSequence {
	if resolvedImageID == "" {
		return inOrder(
			storeMock.EXPECT().Image(expectedImageNameOrID).Return(nil, cstorage.ErrImageUnknown),
			// Assuming expectedImageNameOrID does not have a digest, so resolveName does not call ImagesByDigest
			mockStorageReferenceStringWithinTransport(storeMock),
			mockStorageReferenceStringWithinTransport(storeMock),
		)
	}
	return inOrder(
		storeMock.EXPECT().Image(expectedImageNameOrID).
			Return(&cstorage.Image{ID: resolvedImageID, Names: []string{expectedImageNameOrID}}, nil),
	)
}

// containers/image/storage.storageImageSource.getSize
func mockStorageImageSourceGetSize(storeMock *containerstoragemock.MockStore) mockSequence {
	return inOrder(
		storeMock.EXPECT().ListImageBigData(gomock.Any()).
			Return([]string{""}, nil), // A single entry
		storeMock.EXPECT().ImageBigDataSize(gomock.Any(), gomock.Any()).
			Return(int64(0), nil),
		// FIXME: This should also walk through the layer list and call store.Layer() on each, but we would have to mock the whole layer list.
	)
}

// containers/image/storage.storageReference.newImage
func mockNewImage(storeMock *containerstoragemock.MockStore, expectedImageName, resolvedImageID string) mockSequence {
	return inOrder(
		mockResolveImage(storeMock, expectedImageName, resolvedImageID),
		storeMock.EXPECT().ImageBigData(gomock.Any(), gomock.Any()).
			Return(testManifest, nil),
		mockStorageImageSourceGetSize(storeMock),
	)
}
