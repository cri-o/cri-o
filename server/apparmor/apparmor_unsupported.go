// +build !apparmor

package apparmor

const (
	// DefaultApparmorProfile is the name of default apparmor profile name.
	DefaultApparmorProfile = "ocid-default"

	// ContainerAnnotationKeyPrefix is the prefix to an annotation key specifying a container profile.
	ContainerAnnotationKeyPrefix = "container.apparmor.security.beta.kubernetes.io/"

	// ProfileRuntimeDefault is he profile specifying the runtime default.
	ProfileRuntimeDefault = "runtime/default"
	// ProfileNamePrefix is the prefix for specifying profiles loaded on the node.
	ProfileNamePrefix = "localhost/"
)

// IsEnabled returns false, when build without apparmor build tag.
func IsEnabled() bool {
	return false
}

// LoadDefaultAppArmorProfile dose nothing, when build without apparmor build tag.
func LoadDefaultAppArmorProfile() {
}

// GetProfileNameFromPodAnnotations dose nothing, when build without apparmor build tag.
func GetProfileNameFromPodAnnotations(annotations map[string]string, containerName string) string {
	return ""
}
