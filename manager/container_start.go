package manager

import "fmt"

// StartContainer starts the container.
func (m *Manager) StartContainer(cID string) error {
	c, err := m.getContainerWithPartialID(cID)
	if err != nil {
		return err
	}

	if err := m.runtime.StartContainer(c); err != nil {
		return fmt.Errorf("failed to start container %s: %v", c.ID(), err)
	}

	return nil
}
