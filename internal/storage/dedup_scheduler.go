package storage

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	cstorage "go.podman.io/storage"

	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/server/metrics"
)

type pullEvent int

const (
	pullStarted pullEvent = iota
	pullCompleted
)

type pullNotification struct {
	event pullEvent
}

type dedupRequest struct {
	ctx      context.Context
	imageID  string
	resultCh chan dedupResult
}

type dedupResult struct {
	err error
}

type DedupScheduler struct {
	requestCh   chan dedupRequest
	pullCh      chan pullNotification
	store       cstorage.Store
	running     atomic.Bool
	maxDelayDur time.Duration
}

func NewDedupScheduler(store cstorage.Store, maxDelaySeconds int) *DedupScheduler {
	return &DedupScheduler{
		requestCh:   make(chan dedupRequest, 100),
		pullCh:      make(chan pullNotification, 100),
		store:       store,
		maxDelayDur: time.Duration(maxDelaySeconds) * time.Second,
	}
}

func (ds *DedupScheduler) NotifyPullStarted(ctx context.Context) {
	log.Infof(ctx, "NotifyPullStarted called")

	select {
	case ds.pullCh <- pullNotification{event: pullStarted}:
	default:
		log.Warnf(ctx, "NotifyPullStarted: channel full, notification dropped")
	}
}

func (ds *DedupScheduler) NotifyPullCompleted(ctx context.Context) {
	log.Infof(ctx, "NotifyPullCompleted called")

	select {
	case ds.pullCh <- pullNotification{event: pullCompleted}:
	default:
		log.Warnf(ctx, "NotifyPullCompleted: channel full, notification dropped")
	}
}

// Start begins the dedup scheduler consumer.
func (ds *DedupScheduler) Start(ctx context.Context) {
	if !ds.running.CompareAndSwap(false, true) {
		return
	}

	go ds.consumer(ctx)
}

func (ds *DedupScheduler) Stop() {
	if ds.running.CompareAndSwap(true, false) {
		close(ds.requestCh)
	}
}

// ScheduleDedupAsync schedules a dedup request (producer side) and waits for completion.
func (ds *DedupScheduler) ScheduleDedupAsync(ctx context.Context, imageID string) error {
	ctx, span := log.StartSpan(ctx)
	defer span.End()

	resultCh := make(chan dedupResult, 1)
	req := dedupRequest{
		ctx:      ctx,
		imageID:  imageID,
		resultCh: resultCh,
	}

	select {
	case ds.requestCh <- req:
	case <-ctx.Done():
		return ctx.Err()
	}

	select {
	case result := <-resultCh:
		return result.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// consumer is the single-threaded consumer that processes dedup requests.
func (ds *DedupScheduler) consumer(ctx context.Context) {
	var (
		activePulls   int
		pendingDedups []dedupRequest
		timer         *time.Timer
		timerCh       <-chan time.Time
	)

	for {
		select {
		case <-ctx.Done():
			if timer != nil {
				timer.Stop()
			}

			return

		case <-timerCh:
			if len(pendingDedups) > 0 {
				log.Infof(ctx, "Dedup timer expired: activePulls=%d, pendingDedups=%d", activePulls, len(pendingDedups))
				ds.processBatch(ctx, pendingDedups)
				pendingDedups = nil
			}

			timer = nil
			timerCh = nil

		case notification, ok := <-ds.pullCh:
			if !ok {
				if timer != nil {
					timer.Stop()
				}

				return
			}

			if notification.event == pullStarted {
				activePulls++
				log.Infof(ctx, "Pull started, active pulls now: %d", activePulls)
			} else {
				activePulls--
				log.Infof(ctx, "Pull completed, active pulls now: %d", activePulls)
			}

			if len(pendingDedups) > 0 && activePulls == 0 {
				log.Infof(ctx, "Triggering dedup: activePulls=0, pendingDedups=%d", len(pendingDedups))

				if timer != nil {
					timer.Stop()
					timer = nil
					timerCh = nil
				}

				ds.processBatch(ctx, pendingDedups)
				pendingDedups = nil
			}

		case req, ok := <-ds.requestCh:
			if !ok {
				if timer != nil {
					timer.Stop()
				}

				return
			}

			pendingDedups = append(pendingDedups, req)

			pending := len(ds.requestCh)
			for range pending {
				r, ok := <-ds.requestCh
				if !ok {
					break
				}

				pendingDedups = append(pendingDedups, r)
			}

			if activePulls == 0 {
				ds.processBatch(ctx, pendingDedups)
				pendingDedups = nil
			} else if ds.maxDelayDur > 0 && timer == nil {
				log.Infof(ctx, "Starting dedup delay timer: activePulls=%d, maxDelay=%v, pendingDedups=%d",
					activePulls, ds.maxDelayDur, len(pendingDedups))
				timer = time.NewTimer(ds.maxDelayDur)
				timerCh = timer.C
			}
		}
	}
}

// processBatch executes a single dedup operation for all batched requests.
func (ds *DedupScheduler) processBatch(ctx context.Context, batch []dedupRequest) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()

	log.Infof(ctx, "Processing batched dedup for %d images: %v", len(batch), extractImageIDs(batch))

	start := time.Now()

	// Execute single dedup operation for all images
	result, err := ds.store.Dedup(cstorage.DedupArgs{
		Options: cstorage.DedupOptions{
			HashMethod: cstorage.DedupHashSHA256,
		},
	})
	duration := time.Since(start)

	// Record metrics
	if err == nil {
		metrics.Instance().MetricImageLayerDedupDurationObserve(duration)
		metrics.Instance().MetricImageLayerDedupBytesSavedObserve(int64(result.Deduped))

		savedMB := float64(result.Deduped) / (1024 * 1024)
		log.Infof(ctx, "Batched deduplication complete for %d images: saved %.2f MB in %v",
			len(batch), savedMB, duration)
	} else {
		log.Errorf(ctx, "Batched deduplication failed: %v", err)
	}

	// Notify all waiting producers
	dedupErr := err
	if err != nil {
		dedupErr = fmt.Errorf("deduplication failed: %w", err)
	}

	for _, req := range batch {
		select {
		case req.resultCh <- dedupResult{err: dedupErr}:
		case <-req.ctx.Done():
			// Request context cancelled, skip notification
		}
	}
}

func extractImageIDs(batch []dedupRequest) []string {
	ids := make([]string, len(batch))
	for i, req := range batch {
		ids[i] = req.imageID
	}

	return ids
}
