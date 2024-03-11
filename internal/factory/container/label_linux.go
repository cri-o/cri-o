//go:build linux
// +build linux

package container

import (
	"errors"
	"fmt"
	"strings"

	"github.com/cri-o/cri-o/utils"
	selinux "github.com/opencontainers/selinux/go-selinux"
	"github.com/opencontainers/selinux/go-selinux/label"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

func (slabel *secLabelImpl) SecurityLabel(path, secLabel string, shared, maybeRelabel bool) error {
	if maybeRelabel {
		canonicalSecLabel, err := selinux.CanonicalizeContext(secLabel)
		if err != nil {
			logrus.Errorf("Canonicalize label failed %s: %v", secLabel, err)
		} else {
			currentLabel, err := label.FileLabel(path)
			if err == nil && currentLabel == canonicalSecLabel {
				logrus.Debugf(
					"Skipping relabel for %s, as TrySkipVolumeSELinuxLabel is true and the label of the top level of the volume is already correct",
					path)
				return nil
			}
		}
	}
	if err := label.Relabel(path, secLabel, shared); err != nil && !errors.Is(err, unix.ENOTSUP) {
		return fmt.Errorf("relabel failed %s: %w", path, err)
	}
	return nil
}

// SelinuxLabel returns the container's SelinuxLabel
// it takes the sandbox's label, which it falls back upon
func (c *container) SelinuxLabel(sboxLabel string) ([]string, error) {
	selinuxConfig := c.config.Linux.SecurityContext.SelinuxOptions

	labels := map[string]string{}

	labelOptions, err := label.DupSecOpt(sboxLabel)
	if err != nil {
		return nil, err
	}
	for _, r := range labelOptions {
		k := strings.Split(r, ":")[0]
		labels[k] = r
	}

	if selinuxConfig != nil {
		for _, r := range utils.GetLabelOptions(selinuxConfig) {
			k := strings.Split(r, ":")[0]
			labels[k] = r
		}
	}
	ret := []string{}
	for _, v := range labels {
		ret = append(ret, v)
	}
	return ret, nil
}
