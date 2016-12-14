package manager

import pb "k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/runtime"

// Status returns the status of the runtime
func (m *Manager) Status() (*pb.RuntimeStatus, error) {

	// Deal with Runtime conditions
	runtimeReady, err := m.runtime.RuntimeReady()
	if err != nil {
		return nil, err
	}
	networkReady, err := m.runtime.NetworkReady()
	if err != nil {
		return nil, err
	}

	// Use vendored strings
	runtimeReadyConditionString := pb.RuntimeReady
	networkReadyConditionString := pb.NetworkReady

	status := &pb.RuntimeStatus{
		Conditions: []*pb.RuntimeCondition{
			&pb.RuntimeCondition{
				Type:   &runtimeReadyConditionString,
				Status: &runtimeReady,
			},
			&pb.RuntimeCondition{
				Type:   &networkReadyConditionString,
				Status: &networkReady,
			},
		},
	}

	return status, nil
}
