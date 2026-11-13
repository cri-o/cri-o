//go:build linux && test

package server

// CheckpointPodForTest exposes native preparation to the external test suite.
// The public RPC remains disabled until the backend supports cancellation.
var CheckpointPodForTest = (*Server).checkpointPod
