//go:build selinux
// +build selinux

package version

// nolint: gochecknoinits
func init() {
	buildTags = append(buildTags, "selinux")
}
