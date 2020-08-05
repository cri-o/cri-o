package conmonmgr

import (
	"path"
	"strings"

	"github.com/blang/semver"
	"github.com/cri-o/cri-o/utils/cmdrunner"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

var versionSupportsSync = semver.MustParse("2.0.19")

type ConmonManager struct {
	conmonVersion *semver.Version
	supportsSync  bool
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
		return nil, errors.Wrapf(err, "get conmon version")
	}
	fields := strings.Fields(string(out))
	if len(fields) < 3 {
		return nil, errors.Errorf("conmon version output too short: expected three fields, got %d in %s", len(fields), out)
	}

	c := new(ConmonManager)
	if err := c.parseConmonVersion(fields[2]); err != nil {
		return nil, errors.Wrapf(err, "get conmon version")
	}

	c.initializeSupportsSync()
	return c, nil
}

func (c *ConmonManager) parseConmonVersion(versionString string) error {
	parsedVersion, err := semver.New(versionString)
	if err != nil {
		return err
	}
	c.conmonVersion = parsedVersion
	return nil
}

func (c *ConmonManager) initializeSupportsSync() {
	c.supportsSync = c.conmonVersion.GTE(versionSupportsSync)
	verb := "does not"
	if c.supportsSync {
		verb = "does"
	}

	logrus.Infof("conmon %s support the --sync option", verb)
}

func (c *ConmonManager) SupportsSync() bool {
	return c.supportsSync
}
