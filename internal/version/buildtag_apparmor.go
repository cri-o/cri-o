// +build apparmor

package version

// nolint: gochecknoinits
func init() {
	buildTags = append(buildTags, "apparmor")
}
