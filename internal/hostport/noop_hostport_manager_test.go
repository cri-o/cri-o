package hostport

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// To make CodeCov happy
func TestNoopHostportManager(t *testing.T) {
	manager := NewNoopHostportManager()
	assert.NotNil(t, manager)

	err := manager.Add("id", nil, "")
	assert.NoError(t, err)

	err = manager.Remove("id", nil)
	assert.NoError(t, err)
}
