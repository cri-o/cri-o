package storage

// IsCrioContainer returns whether a container coming from storage was created
// by CRI-O CRI-O sandboxes and containers differ from libpod container and
// pods because they require a PodName and PodID annotation
func IsCrioContainer(md *RuntimeContainerMetadata) bool {
	return md.PodName != "" && md.PodID != ""
}
