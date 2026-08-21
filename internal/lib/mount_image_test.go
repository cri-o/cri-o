package lib

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	containerstoragemock "github.com/cri-o/cri-o/test/mocks/containerstorage"
)

// waitForMountInProgress polls the dedup map until an entry for imageID appears,
// then waits briefly for remaining goroutines to reach wg.Wait().
func waitForMountInProgress(t *testing.T, sut *ContainerServer, imageID string) {
	t.Helper()

	deadline := time.After(5 * time.Second)

	for {
		sut.mountOperationsLock.Lock()
		_, exists := sut.mountOperationsInProgress[imageID]
		sut.mountOperationsLock.Unlock()

		if exists {
			time.Sleep(10 * time.Millisecond)

			return
		}

		select {
		case <-deadline:
			t.Fatal("timed out waiting for mount operation to start")
		default:
			time.Sleep(time.Millisecond)
		}
	}
}

func TestMountImageByIDDeduplicates(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	storeMock := containerstoragemock.NewMockStore(ctrl)

	sut := NewContainerServerForTest(storeMock)

	const (
		imageID    = "abc123"
		mountPoint = "/var/lib/containers/storage/overlay/abc123/merged"
		concurrent = 50
	)

	var callCount atomic.Int32

	unblock := make(chan struct{})

	storeMock.EXPECT().
		MountImage(imageID, []string{"ro", "noexec", "nosuid", "nodev"}, "").
		DoAndReturn(func(string, []string, string) (string, error) {
			<-unblock
			callCount.Add(1)

			return mountPoint, nil
		}).
		Times(1)

	var wg sync.WaitGroup

	for range concurrent {
		wg.Go(func() {
			mp, err := sut.MountImageByID(t.Context(), imageID)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if mp != mountPoint {
				t.Errorf("expected mount point %q, got %q", mountPoint, mp)
			}
		})
	}

	waitForMountInProgress(t, sut, imageID)

	close(unblock)
	wg.Wait()

	if c := callCount.Load(); c != 1 {
		t.Errorf("expected exactly 1 MountImage call, got %d", c)
	}
}

func TestMountImageByIDDistinctImages(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	storeMock := containerstoragemock.NewMockStore(ctrl)

	sut := NewContainerServerForTest(storeMock)

	storeMock.EXPECT().
		MountImage(gomock.Any(), []string{"ro", "noexec", "nosuid", "nodev"}, "").
		DoAndReturn(func(id string, _ []string, _ string) (string, error) {
			return "/mnt/" + id, nil
		}).
		Times(2)

	mp1, err := sut.MountImageByID(t.Context(), "image-a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mp1 != "/mnt/image-a" {
		t.Errorf("expected /mnt/image-a, got %s", mp1)
	}

	mp2, err := sut.MountImageByID(t.Context(), "image-b")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mp2 != "/mnt/image-b" {
		t.Errorf("expected /mnt/image-b, got %s", mp2)
	}
}

func TestMountImageByIDErrorPropagation(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	storeMock := containerstoragemock.NewMockStore(ctrl)

	sut := NewContainerServerForTest(storeMock)

	const (
		imageID    = "abc123"
		concurrent = 20
	)

	mountErr := errors.New("storage locked")
	unblock := make(chan struct{})

	storeMock.EXPECT().
		MountImage(imageID, []string{"ro", "noexec", "nosuid", "nodev"}, "").
		DoAndReturn(func(string, []string, string) (string, error) {
			<-unblock

			return "", mountErr
		}).
		Times(1)

	var wg sync.WaitGroup

	for range concurrent {
		wg.Go(func() {
			mp, err := sut.MountImageByID(t.Context(), imageID)
			if !errors.Is(err, mountErr) {
				t.Errorf("expected %v, got %v", mountErr, err)
			}

			if mp != "" {
				t.Errorf("expected empty mount point on error, got %q", mp)
			}
		})
	}

	waitForMountInProgress(t, sut, imageID)

	close(unblock)
	wg.Wait()
}
