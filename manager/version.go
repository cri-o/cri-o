package manager

// Version returns the runtime name and runtime version
func (m *Manager) Version() (*VersionResponse, error) {

	runtimeVersion, err := m.runtime.Version()
	if err != nil {
		return nil, err
	}

	runtimeName := m.runtime.Name()

	return &VersionResponse{
		RuntimeName:    runtimeName,
		RuntimeVersion: runtimeVersion,
	}, nil
}

// VersionResponse is returned from Version.
type VersionResponse struct {
	RuntimeVersion string
	RuntimeName    string
}
