package storage

import "sync"

// layerProgress tracks the download progress of a single image layer (blob).
type layerProgress struct {
	Current int64 `json:"current"` // bytes downloaded so far
	Total   int64 `json:"total"`   // registry-reported total size; -1 if unknown
}

// ImagePullTracker tracks per-layer pull progress for a single image.
// It is safe for concurrent use.
type ImagePullTracker struct {
	mu     sync.Mutex
	Layers map[string]layerProgress
}

// NewImagePullTracker allocates a fresh tracker for one image pull.
func NewImagePullTracker() *ImagePullTracker {
	return &ImagePullTracker{
		Layers: make(map[string]layerProgress),
	}
}

// UpdateLayer records the latest byte counts for one layer.
//   - id      blob digest string (unique key per layer)
//   - current cumulative bytes received so far  (= p.Offset in progress events)
//   - total   registry-reported layer size       (= p.Artifact.Size; -1 if unknown)
func (t *ImagePullTracker) UpdateLayer(id string, current, total int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.Layers[id] = layerProgress{Current: current, Total: total}
}

// GetOverallPercentage returns the aggregate pull completion percentage [0, 100].
// Returns -1 when any layer's total size is unknown (registry did not report it),
// because an accurate percentage cannot be computed.
func (t *ImagePullTracker) GetOverallPercentage() float64 {
	t.mu.Lock()
	defer t.mu.Unlock()

	var totalBytes, currentBytes int64
	for _, layer := range t.Layers {
		if layer.Total <= 0 {
			return -1 // unknown size — cannot produce a meaningful percentage
		}
		totalBytes += layer.Total
		currentBytes += layer.Current
	}
	if totalBytes == 0 {
		return 0
	}
	pct := (float64(currentBytes) / float64(totalBytes)) * 100
	if pct > 100 {
		pct = 100
	}
	return pct
}

// ActivePullProgress is the process-wide registry of in-flight image pulls.
// Key:   image name string (full registry reference, e.g. "docker.io/library/nginx:latest")
// Value: *ImagePullTracker
//
// Placed in internal/storage (imported by both server/ and server/metrics/) to
// avoid the circular import that would arise if it lived in server/.
var ActivePullProgress sync.Map
