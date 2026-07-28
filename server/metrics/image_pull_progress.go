package metrics

import (
	"encoding/json"
	"net/http"

	"github.com/cri-o/cri-o/internal/storage"
)

// handleImageProgressQuery serves GET /image/progress.
//
// Returns a JSON object whose keys are full image registry references and
// whose values contain the pull percentage.
//
// Example response while pulling:
//
//	{
//	  "docker.io/library/nginx:latest": {"percentage": 54.38},
//	}
//
// Returns {} when no image pull is currently in flight.
// "percentage" is -1 when the registry did not report layer sizes.
func handleImageProgressQuery(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	responsePayload := make(map[string]map[string]interface{})

	storage.ActivePullProgress.Range(func(key, value interface{}) bool {
		imgName := key.(string)
		tracker := value.(*storage.ImagePullTracker)

		responsePayload[imgName] = map[string]interface{}{
			"percentage": tracker.GetOverallPercentage(),
		}
		return true
	})

	_ = json.NewEncoder(w).Encode(responsePayload)
}
