//go:build containers_image_ostree_stub
// +build containers_image_ostree_stub

package version

// nolint: gochecknoinits
func init() {
	buildTags = append(buildTags, "containers_image_ostree_stub")
}
