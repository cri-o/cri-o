/*
Copyright 2020 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package log

import (
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"k8s.io/release/pkg/command"
)

func SetupGlobalLogger(level string) error {
	logrus.SetFormatter(&logrus.TextFormatter{DisableTimestamp: true})
	lvl, err := logrus.ParseLevel(level)
	if err != nil {
		return errors.Wrapf(err, "setting log level to %s", level)
	}
	logrus.SetLevel(lvl)
	if lvl >= logrus.DebugLevel {
		logrus.Debug("Setting commands globally into verbose mode")
		command.SetGlobalVerbose(true)
	}
	logrus.AddHook(NewFilenameHook())
	logrus.Debugf("Using log level %q", lvl)
	return nil
}
