package lib

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	// DefaultHooksDirPath Default directory containing hooks config files
	DefaultHooksDirPath = "/usr/share/containers/oci/hooks.d"
	// OverrideHooksDirPath Directory where admin can override the default configuration
	OverrideHooksDirPath = "/etc/containers/oci/hooks.d"
)

// HookParams is the structure returned from read the hooks configuration
type HookParams struct {
	Hook          string   `json:"hook"`
	Stage         []string `json:"stage"`
	Cmds          []string `json:"cmd"`
	Annotations   []string `json:"annotation"`
	HasBindMounts bool     `json:"hasbindmounts"`
	Arguments     []string `json:"arguments"`
}

var (
	// ErrNoJSONSuffix represents hook-add attempts where the filename
	// does not end in '.json'.
	ErrNoJSONSuffix = errors.New("hook filename does not end in '.json'")
)

// readHook reads hooks json files, verifies it and returns the json config
func readHook(hookPath string) (HookParams, error) {
	var hook HookParams
	if !strings.HasSuffix(hookPath, ".json") {
		return hook, ErrNoJSONSuffix
	}
	raw, err := ioutil.ReadFile(hookPath)
	if err != nil {
		return hook, errors.Wrapf(err, "error Reading hook %q", hookPath)
	}
	if err := json.Unmarshal(raw, &hook); err != nil {
		return hook, errors.Wrapf(err, "error Unmarshalling JSON for %q", hookPath)
	}
	if _, err := os.Stat(hook.Hook); err != nil {
		return hook, errors.Wrapf(err, "unable to stat hook %q in hook config %q", hook.Hook, hookPath)
	}
	validStage := map[string]bool{"prestart": true, "poststart": true, "poststop": true}
	for _, cmd := range hook.Cmds {
		if _, err = regexp.Compile(cmd); err != nil {
			return hook, errors.Wrapf(err, "invalid cmd regular expression %q defined in hook config %q", cmd, hookPath)
		}
	}
	for _, cmd := range hook.Annotations {
		if _, err = regexp.Compile(cmd); err != nil {
			return hook, errors.Wrapf(err, "invalid cmd regular expression %q defined in hook config %q", cmd, hookPath)
		}
	}
	if len(hook.Stage) == 0 {
		logrus.Warnf("No stage defined in hook config %q, hook will never run", hookPath)
	} else {
		for _, stage := range hook.Stage {
			if !validStage[stage] {
				return hook, errors.Wrapf(err, "unknown stage %q defined in hook config %q", stage, hookPath)
			}
		}
	}
	return hook, nil
}

// readHooks reads hooks json files in directory to setup OCI Hooks
// adding hooks to the passedin hooks map.
func readHooks(hooksPath string, hooks map[string]HookParams) error {
	if _, err := os.Stat(hooksPath); err != nil {
		if os.IsNotExist(err) {
			logrus.Warnf("hooks path: %q does not exist", hooksPath)
			return nil
		}
		return errors.Wrapf(err, "unable to stat hooks path %q", hooksPath)
	}

	files, err := ioutil.ReadDir(hooksPath)
	if err != nil {
		return err
	}

	for _, file := range files {
		hook, err := readHook(filepath.Join(hooksPath, file.Name()))
		if err != nil {
			if err == ErrNoJSONSuffix {
				continue
			}
			return err
		}
		hooks[file.Name()] = hook
	}
	return nil
}
