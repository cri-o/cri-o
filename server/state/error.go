package state

import (
	"fmt"
)

// NoSuchSandboxError is an error occurring when requested sandbox does not exist
type NoSuchSandboxError struct {
	id    string
	name  string
	inner error
}

// Error produces an human-readable error message
func (e NoSuchSandboxError) Error() string {
	if e.id != "" {
		if e.inner == nil {
			return fmt.Sprintf("no sandbox found with ID %s", e.id)
		}

		return fmt.Sprintf("no sandbox found with ID %s: %v", e.id, e.inner)
	} else if e.name != "" {
		if e.inner == nil {
			return fmt.Sprintf("no sandbox found with name %s", e.name)
		}

		return fmt.Sprintf("no sandbox found with name %s: %v", e.name, e.inner)
	} else if e.inner != nil {
		return fmt.Sprintf("no such sandbox: %v", e.inner)
	}

	return "no such sandbox"
}

// NoSuchCtrError is an error occurring when requested container does not exist
type NoSuchCtrError struct {
	id      string
	name    string
	sandbox string
	inner   error
}

// Error produces a human-readable error message
func (e NoSuchCtrError) Error() string {
	if e.id != "" {
		if e.sandbox != "" {
			if e.inner == nil {
				return fmt.Sprintf("no container found with ID %s in sandbox %s", e.id, e.sandbox)
			}

			return fmt.Sprintf("no container found with ID %s in sandbox %s: %v", e.id, e.sandbox, e.inner)
		}
		if e.inner == nil {
			return fmt.Sprintf("no container found with ID %s", e.id)
		}

		return fmt.Sprintf("no container found with ID %s: %v", e.id, e.inner)
	} else if e.name != "" {
		if e.sandbox != "" {
			if e.inner == nil {
				return fmt.Sprintf("no container found with name %s in sandbox %s", e.name, e.sandbox)
			}

			return fmt.Sprintf("no container found with name %s in sandbox %s: %v", e.name, e.sandbox, e.inner)
		}
		if e.inner == nil {
			return fmt.Sprintf("no container found with name %s", e.name)
		}

		return fmt.Sprintf("no container found with name %s: %v", e.name, e.inner)
	} else if e.inner != nil {
		return fmt.Sprintf("no such container: %v", e.inner)
	}

	return "no such container"
}

// Functions for verifying errors

// IsSandboxNotExist checks if an error indicated that given sandbox does not exist
func IsSandboxNotExist(err error) bool {
	switch err.(type) {
	case *NoSuchSandboxError:
		return true
	default:
		return false
	}
}

// IsCtrNotExist checks if an error indicates that given container does not exist
func IsCtrNotExist(err error) bool {
	switch err.(type) {
	case *NoSuchCtrError:
		return true
	default:
		return false
	}
}
