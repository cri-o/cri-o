package hostport

import "github.com/sirupsen/logrus"

// This interface implements, when hostport mapping is disabled in CRI-O.
type noopHostportManager struct{}

// NewNoopHostportManager creates a new HostPortManager.
func NewNoopHostportManager() HostPortManager {
	logrus.Info("HostPort Mapping is Disabled in CRI-O")

	return &noopHostportManager{}
}

func (mh *noopHostportManager) Add(id string, podPortMapping *PodPortMapping) error {
	logrus.Debug("HostPort Mapping is Disabled in CRI-O")

	return nil
}

func (mh *noopHostportManager) Remove(id string, podPortMapping *PodPortMapping) error {
	logrus.Debug("HostPort Mapping is Disabled in CRI-O")

	return nil
}
