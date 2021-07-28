// +build seccomp

package version

// nolint: gochecknoinits
func init() {
	buildTags = append(buildTags, "seccomp")
}
