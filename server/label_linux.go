package server

import (
	"errors"
	"fmt"

	selinux "github.com/opencontainers/selinux/go-selinux"
	"github.com/opencontainers/selinux/go-selinux/label"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

func securityLabel(path, secLabel string, shared, maybeRelabel bool) error {
	if maybeRelabel {
		canonicalSecLabel, err := selinux.CanonicalizeContext(secLabel)
		if err != nil {
			logrus.Errorf("Canonicalize label failed %s: %v", secLabel, err)
		} else {
			currentLabel, err := selinux.FileLabel(path)
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
