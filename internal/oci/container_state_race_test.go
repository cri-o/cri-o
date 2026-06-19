package oci_test

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"strings"
	"sync"
	"testing"
	"time"

	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/oci"
)

// TestContainerStateToDiskRace reproduces the exact race pattern from
// https://github.com/cri-o/cri-o/issues/9419. The race is between
// ContainerStateToDisk (container_server.go:640-658) and
// runtimeOCI.UpdateContainerStatus (runtime_oci.go:1167-1270):
//
//   Goroutine A (ContainerStateToDisk):
//     1. UpdateContainerStatus completes (lock released)
//     2. state := ctr.State()        // gets raw pointer, releases RLock
//     3. json.Encode(state)          // iterates Annotations map — NO LOCK
//
//   Goroutine B (UpdateContainerStatus, called from handleExit):
//     1. opLock.Lock()
//     2. state := *c.state           // shallow copy — shares Annotations map
//     3. json.Decode(&state)         // writes to the SHARED Annotations map
//     4. *c.state = *state           // copies back
//     5. opLock.Unlock()
//
// The race: step A.3 iterates the Annotations map while step B.3
// writes to the same map through the shallow copy. The opLock does not
// protect step A.3 because State() already released it.
//
// Run with: go test -race -tags test -run TestContainerStateToDiskRace ./internal/oci/
func TestContainerStateToDiskRace(t *testing.T) {
	ctr, err := oci.NewContainer("test-id", "test-name", "", "",
		make(map[string]string), make(map[string]string),
		make(map[string]string), "", nil, nil, "",
		&types.ContainerMetadata{}, "test-sandbox", false,
		false, false, "", "", time.Now(), "")
	if err != nil {
		t.Fatal(err)
	}

	state := ctr.StateNoLock()
	state.Annotations = map[string]string{
		"io.kubernetes.cri-o.Name": "test",
	}

	// OCI runtime state JSON that UpdateContainerStatus would decode.
	// Contains annotations that json.Decode will merge into the
	// existing (shared) Annotations map.
	ociState := `{
		"ociVersion": "1.0.0",
		"id": "test-id",
		"status": "running",
		"pid": 12345,
		"bundle": "/run/containers/test-id",
		"annotations": {
			"io.kubernetes.cri-o.Name": "test",
			"io.kubernetes.cri-o.SandboxID": "test-sandbox"
		}
	}`

	var wg sync.WaitGroup

	// Goroutine A: ContainerStateToDisk's encode step
	// (container_server.go:655-657)
	//
	//   enc := json.NewEncoder(jsonSource)
	//   return enc.Encode(ctr.State())
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			json.NewEncoder(io.Discard).Encode(ctr.State())
		}
	}()

	// Goroutine B: UpdateContainerStatus's shallow-copy + decode step
	// (runtime_oci.go:1214-1215)
	//
	//   state := *c.state
	//   json.NewDecoder(strings.NewReader(out)).Decode(&state)
	//
	// The shallow copy shares the Annotations map with c.state.
	// json.Decode writes new entries into this shared map while
	// goroutine A iterates it.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			stateCopy := *ctr.StateNoLock()
			stateCopy.Annotations = maps.Clone(stateCopy.Annotations)
			json.NewDecoder(strings.NewReader(ociState)).Decode(&stateCopy)
		}
	}()

	wg.Wait()
}

// TestContainerStateStructCopyRace demonstrates that the struct
// assignment *c.state = *state in UpdateContainerStatus
// (runtime_oci.go:1232, 1252) races with concurrent readers.
// The struct copy is not atomic — a reader can observe a
// partially-written struct with fields from both old and new states.
//
// Run with: go test -race -tags test -run TestContainerStateStructCopyRace ./internal/oci/
func TestContainerStateStructCopyRace(t *testing.T) {
	ctr, err := oci.NewContainer("test-id", "test-name", "", "",
		make(map[string]string), make(map[string]string),
		make(map[string]string), "", nil, nil, "",
		&types.ContainerMetadata{}, "test-sandbox", false,
		false, false, "", "", time.Now(), "")
	if err != nil {
		t.Fatal(err)
	}

	state := ctr.StateNoLock()
	state.Annotations = map[string]string{
		"io.kubernetes.cri-o.Name": "test",
	}

	var wg sync.WaitGroup

	// Reader: ContainerStateToDisk encoding the state
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			json.NewEncoder(io.Discard).Encode(ctr.State())
		}
	}()

	// Writer: UpdateContainerStatus doing *c.state = *newState
	// (runtime_oci.go:1232)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			newState := oci.ContainerState{}
			newState.Annotations = map[string]string{
				fmt.Sprintf("key-%d", i): fmt.Sprintf("value-%d", i),
			}
			// This is what runtime_oci.go:1232 does: *c.state = *state
			*ctr.StateNoLock() = newState
		}
	}()

	wg.Wait()
}
