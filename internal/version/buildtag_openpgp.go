//go:build containers_image_openpgp
// +build containers_image_openpgp

package version

// nolint: gochecknoinits
func init() {
	buildTags = append(buildTags, "containers_image_openpgp")
}
