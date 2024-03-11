//go:build !linux
// +build !linux

package container

func (slabel *secLabelImp) SecurityLabel(path string, seclabel string, shared, maybeRelabel bool) error {
	return nil
}

// SelinuxLabel returns the container's SelinuxLabel
// it takes the sandbox's label, which it falls back upon
func (c *container) SelinuxLabel(sboxLabel string) ([]string, error) {
	return []string{}, nil
}
