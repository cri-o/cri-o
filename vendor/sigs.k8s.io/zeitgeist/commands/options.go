/*
Copyright 2021 The Kubernetes Authors.

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

package commands

import (
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"
)

type options struct {
	// configuration options
	localOnly bool
	remote    bool

	// path options
	basePath   string
	configFile string

	// command options
	logLevel string
}

// setAndValidate sets some default options and verifies if options are valid
func (o *options) setAndValidate() error {
	logrus.Debug("Validating zeitgeist options...")

	if o.basePath != "" {
		if _, err := os.Stat(o.basePath); os.IsNotExist(err) {
			return err
		}
	} else {
		dir, err := filepath.Abs(filepath.Dir(os.Args[0]))
		if err != nil {
			return err
		}
		o.basePath = dir
	}

	return nil
}
