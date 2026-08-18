package storage

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	cstorage "go.podman.io/storage"
	graphdriver "go.podman.io/storage/drivers"
)

type mockStore struct {
	cstorage.Store

	dedupCallCount atomic.Int32
	dedupDelay     time.Duration
}

func newMockStore(dedupDelay time.Duration) *mockStore {
	return &mockStore{
		dedupDelay: dedupDelay,
	}
}

func (m *mockStore) Dedup(args cstorage.DedupArgs) (graphdriver.DedupResult, error) {
	m.dedupCallCount.Add(1)

	if m.dedupDelay > 0 {
		time.Sleep(m.dedupDelay)
	}

	return graphdriver.DedupResult{Deduped: 1024 * 1024}, nil
}

func (m *mockStore) getCallCount() int32 {
	return m.dedupCallCount.Load()
}

func TestDedupSchedulerBatching(t *testing.T) {
	ctx := context.Background()

	store := newMockStore(200 * time.Millisecond)

	scheduler := NewDedupScheduler(store, 2)

	scheduler.Start(ctx)
	defer scheduler.Stop()

	for i := 1; i <= 5; i++ {
		scheduler.NotifyPullStarted(ctx)
	}

	var wg sync.WaitGroup
	for i := 1; i <= 5; i++ {
		wg.Add(1)

		go func(id int) {
			defer wg.Done()

			time.Sleep(10 * time.Millisecond)

			imageID := fmt.Sprintf("image%d", id)

			err := scheduler.ScheduleDedupAsync(ctx, imageID)
			if err != nil {
				t.Errorf("Request for %s failed: %v", imageID, err)
			}
		}(i)
	}

	time.Sleep(100 * time.Millisecond)
	wg.Wait()

	for i := 1; i <= 5; i++ {
		scheduler.NotifyPullCompleted(ctx)
	}

	time.Sleep(100 * time.Millisecond)

	callCount := store.getCallCount()
	if callCount != 1 {
		t.Errorf("Expected 1 Dedup call (batching all 5 images), got %d", callCount)
	}
}
