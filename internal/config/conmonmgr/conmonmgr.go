package conmonmgr

import (
	"bytes"
	"fmt"
	"path"
	"strings"

	"github.com/blang/semver/v4"
	"github.com/cri-o/cri-o/utils/cmdrunner"
	"github.com/sirupsen/logrus"
)

var (
	versionSupportsSync             = semver.MustParse("2.0.19")
	versionSupportsLogGlobalSizeMax = semver.MustParse("2.1.2")
)

type ConmonManager struct {
	conmonVersion            *semver.Version
	supportsSync             bool
	supportsLogGlobalSizeMax bool
}

// this function is heavily based on github.com/containers/common#probeConmon
func New(conmonPath string) (*ConmonManager, error) {
	if !path.IsAbs(conmonPath) {
		return nil, fmt.Errorf("conmon path is not absolute: %s", conmonPath)
	}
	out, err := cmdrunner.CombinedOutput(conmonPath, "--version")
	if err != nil {
		return nil, fmt.Errorf("get conmon version: %w", err)
	}
	fields := strings.Fields(string(out))
	if len(fields) < 3 {
		return nil, fmt.Errorf("conmon version output too short: expected three fields, got %d in %s", len(fields), out)
	}

	c := new(ConmonManager)
	if err := c.parseConmonVersion(fields[2]); err != nil {
		return nil, fmt.Errorf("get conmon version: %w", err)
	}

	c.initializeSupportsSync()
	c.initializeSupportsLogGlobalSizeMax(conmonPath)
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

func (c *ConmonManager) initializeSupportsLogGlobalSizeMax(conmonPath string) {
	c.supportsLogGlobalSizeMax = c.conmonVersion.GTE(versionSupportsLogGlobalSizeMax)
	if !c.supportsLogGlobalSizeMax {
		// Read help output as a fallback in case the feature was backported to conmon,
		// but the version wasn't bumped.
		helpOutput, err := cmdrunner.CombinedOutput(conmonPath, "--help")
		c.supportsLogGlobalSizeMax = err == nil && bytes.Contains(helpOutput, []byte("--log-global-size-max"))
	}
	verb := "does not"
	if c.supportsLogGlobalSizeMax {
		verb = "does"
	}

	logrus.Infof("Conmon %s support the --log-global-size-max option", verb)
}

func (c *ConmonManager) SupportsLogGlobalSizeMax() bool {
	return c.supportsLogGlobalSizeMax
}

func (c *ConmonManager) initializeSupportsSync() {
	c.supportsSync = c.conmonVersion.GTE(versionSupportsSync)
	verb := "does not"
	if c.supportsSync {
		verb = "does"
	}

	logrus.Infof("Conmon %s support the --sync option", verb)
}

func (c *ConmonManager) SupportsSync() bool {
	return c.supportsSync
}
