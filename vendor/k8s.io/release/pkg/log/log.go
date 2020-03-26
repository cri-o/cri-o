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
	"io/ioutil"
	"strings"

	"github.com/sirupsen/logrus"
)

const (
	logTraceKey = "trace"
	logTraceSep = "."
)

func SetupGlobalLogger(level string) error {
	logrus.SetFormatter(&logrus.TextFormatter{DisableTimestamp: true})
	lvl, err := logrus.ParseLevel(level)
	if err != nil {
		return err
	}
	logrus.SetLevel(lvl)
	logrus.AddHook(NewFilenameHook())
	logrus.Debugf("Using log level %q", lvl)
	return nil
}

// AddTracePath adds a path element to the logrus entry's field 'trace'. This
// is meant to be done everytime you hand off a logger/entry to a different
// component to have a clear trace how we ended up here. When logs are emitted
// by this logger entry, the field might look something like:
//   trace=patch-announce.announcer.release-noter
func AddTracePath(l *logrus.Entry, newPathElement string) *logrus.Entry {
	if newPathElement == "" {
		// get a copy with the same data, err, context, ...
		return l.WithFields(l.Data)
	}

	newPath := ""

	curPathInt, ok := l.Data[logTraceKey]
	if !ok {
		newPath = newPathElement
	} else {
		curPath, ok := curPathInt.(string)
		if !ok {
			newPath = "<unkn>" + logTraceSep + newPathElement
		} else {
			newPath = curPath + logTraceSep + newPathElement
		}
	}

	return l.WithField(logTraceKey, newPath)
}

func NullLogger() *logrus.Entry {
	logger := logrus.New()
	logger.SetOutput(ioutil.Discard)
	logger.SetLevel(logrus.PanicLevel)
	return logrus.NewEntry(logger)
}

// Logger can be embedded in other struct to enable logging and keep the
// zero-value of the struct useful.
// Examples of the usage can be found in k8s.io/release/pkg/patch/...
type Mixin struct {
	logger *logrus.Entry
}

func (l *Mixin) Logger() *logrus.Entry {
	if l.logger == nil {
		l.logger = NullLogger()
	}
	return l.logger
}

func (l *Mixin) SetLogger(logger *logrus.Entry, tracePaths ...string) {
	p := strings.Join(tracePaths, logTraceSep)
	l.logger = AddTracePath(logger, p)
}
