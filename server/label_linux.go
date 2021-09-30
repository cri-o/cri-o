package server

import (
	"fmt"

	"github.com/opencontainers/selinux/go-selinux/label"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

func securityLabel(path, secLabel string, shared, maybeRelabel bool) error {
	if maybeRelabel {
		currentLabel, err := label.FileLabel(path)
		if err == nil && currentLabel == secLabel {
			logrus.Debugf(
				"Skipping relabel for %s, as TrySkipVolumeSELinuxRelabel is true and the label of the top level of the volume is already correct",
				path)
			return nil
		}
	}
	if err := label.Relabel(path, secLabel, shared); err != nil && !errors.Is(err, unix.ENOTSUP) {
		return fmt.Errorf("relabel failed %s: %v", path, err)
	}
	return nil
}
