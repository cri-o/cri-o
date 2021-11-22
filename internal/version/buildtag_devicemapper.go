//go:build exclude_graphdriver_devicemapper
// +build exclude_graphdriver_devicemapper

package version

// nolint: gochecknoinits
func init() {
	buildTags = append(buildTags, "exclude_graphdriver_devicemapper")
}
