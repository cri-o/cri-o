package roles

import (
	"strconv"
	"strings"
)

var TopLevelRoles = map[string]struct{}{
	"root":      {},
	"targets":   {},
	"snapshot":  {},
	"timestamp": {},
}

func IsTopLevelRole(name string) bool {
	_, ok := TopLevelRoles[name]
	return ok
}

func IsDelegatedTargetsRole(name string) bool {
	return !IsTopLevelRole(name)
}

func IsTopLevelManifest(name string) bool {
	return IsTopLevelRole(strings.TrimSuffix(name, ".json"))
}

func IsDelegatedTargetsManifest(name string) bool {
	return !IsTopLevelManifest(name)
}

func IsVersionedManifest(name string) bool {
	parts := strings.Split(name, ".")
	// Versioned manifests have the form "x.role.json"
	if len(parts) < 3 {
		return false
	}

	_, err := strconv.Atoi(parts[0])
	return err == nil
}
