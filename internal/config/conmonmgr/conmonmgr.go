package conmonmgr

import (
	"path"
	"regexp"
	"strconv"

	"github.com/cri-o/cri-o/utils/cmdrunner"
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
	return newWithCommandRunner(conmonPath, &cmdrunner.RealCommandRunner{})
}

func newWithCommandRunner(conmonPath string, runner cmdrunner.CommandRunner) (*ConmonManager, error) {
	if !path.IsAbs(conmonPath) {
		return nil, errors.Errorf("conmon path is not absolute: %s", conmonPath)
	}
	out, err := runner.CombinedOutput(conmonPath, "--version")
	if err != nil {
		return nil, err
	}
	r := regexp.MustCompile(`^conmon version (?P<Major>\d+).(?P<Minor>\d+).(?P<Patch>\d+)`)

	matches := r.FindStringSubmatch(string(out))
	if len(matches) != 4 {
		return nil, errors.Errorf("conmon version returned unexpected output %s", string(out))
	}

	c := new(ConmonManager)
	if err := c.parseConmonVersion(matches[1], matches[2], matches[3]); err != nil {
		return nil, err
	}

	c.initializeSupportsSync()
	return c, nil
}

func (c *ConmonManager) parseConmonVersion(major, minor, patch string) error {
	var err error
	if c.majorVersion, err = strconv.Atoi(major); err != nil {
		return errors.Wrapf(err, "failed to parse major version of conmon")
	}
	if c.minorVersion, err = strconv.Atoi(minor); err != nil {
		return errors.Wrapf(err, "failed to parse minor version of conmon")
	}
	if c.patchVersion, err = strconv.Atoi(patch); err != nil {
		return errors.Wrapf(err, "failed to parse patch version of conmon")
	}
	return nil
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
