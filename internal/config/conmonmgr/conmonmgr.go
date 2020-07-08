package conmonmgr

import (
	"bytes"
	"os/exec"
	"regexp"
	"strconv"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	majorVersionSupportsSync = 2
	minorVersionSupportsSync = 0
	patchVersionSupportsSync = 19
)

type ConmonManager struct {
	majorVersion int
	minorVersion int
	patchVersion int

	supportsSync bool
}

// this function is heavily based on github.com/containers/common#probeConmon
func New(conmonPath string) (*ConmonManager, error) {
	c := &ConmonManager{}
	var out bytes.Buffer

	cmd := exec.Command(conmonPath, "--version")
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return nil, err
	}
	r := regexp.MustCompile(`^conmon version (?P<Major>\d+).(?P<Minor>\d+).(?P<Patch>\d+)`)

	matches := r.FindStringSubmatch(out.String())
	if len(matches) != 4 {
		return nil, err
	}

	if c.majorVersion, err = strconv.Atoi(matches[1]); err != nil {
		return nil, errors.Wrapf(err, "failed to parse major version of conmon")
	}
	if c.minorVersion, err = strconv.Atoi(matches[2]); err != nil {
		return nil, errors.Wrapf(err, "failed to parse minor version of conmon")
	}
	if c.patchVersion, err = strconv.Atoi(matches[3]); err != nil {
		return nil, errors.Wrapf(err, "failed to parse patch version of conmon")
	}

	c.initializeSupportsSync()
	return c, nil
}

func (c *ConmonManager) initializeSupportsSync() {
	defer func() {
		verb := "does not"
		if c.supportsSync {
			verb = "does"
		}

		logrus.Infof("conmon %s support the --sync option", verb)
	}()

	if c.majorVersion < majorVersionSupportsSync {
		return
	}
	if c.majorVersion > majorVersionSupportsSync {
		c.supportsSync = true
		return
	}
	if c.minorVersion < minorVersionSupportsSync {
		return
	}
	if c.minorVersion > minorVersionSupportsSync {
		c.supportsSync = true
		return
	}
	if c.patchVersion < patchVersionSupportsSync {
		return
	}
	if c.patchVersion > patchVersionSupportsSync {
		c.supportsSync = true
		return
	}
	// version exactly matches
	c.supportsSync = true
}

func (c *ConmonManager) SupportsSync() bool {
	return c.supportsSync
}
