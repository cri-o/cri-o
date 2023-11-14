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

package license

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"

	"sigs.k8s.io/release-utils/util"
)

// CatalogOptions are the spdx settings
type CatalogOptions struct {
	CacheDir string // Directrory to catch the license we download from SPDX.org
	Version  string // Version of the licenses to download  (eg v3.19) or blank for latest
}

// DefaultCatalogOpts are the predetermined settings. License and cache directories
// are in the temporary OS directory and are created if the do not exist.
//
// The version included here is hardcoded and is intended to be the latest. The
// magefile in the project takes care of replacing this value when updating the
// license zip file.
//
//	DO NOT RENAME OR MOVE THIS OPTION WITHOUT MODIFYING THE MAGEFILE
var DefaultCatalogOpts = CatalogOptions{
	Version: "v3.20",
}

// NewCatalogWithOptions returns a SPDX object with the specified options
func NewCatalogWithOptions(opts CatalogOptions) (catalog *Catalog, err error) {
	// Create the license downloader
	doptions := DefaultDownloaderOpts
	doptions.Version = opts.Version
	doptions.CacheDir = opts.CacheDir
	downloader, err := NewDownloaderWithOptions(doptions)
	if err != nil {
		return nil, fmt.Errorf("creating downloader: %w", err)
	}
	catalog = &Catalog{
		Downloader: downloader,
		opts:       opts,
	}

	return catalog, nil
}

// Options returns  a pointer to the catlog options
func (catalog *Catalog) Options() CatalogOptions {
	return catalog.opts
}

// LoadLicenses reads the license data from the downloader
func (catalog *Catalog) LoadLicenses() error {
	logrus.Info("Loading license data from downloader")
	licenses, err := catalog.Downloader.GetLicenses()
	if err != nil {
		return fmt.Errorf("getting licenses from downloader: %w", err)
	}
	catalog.List = licenses
	logrus.Infof("Got %d licenses from downloader", len(licenses.Licenses))
	return nil
}

// Catalog is an objec to interact with licenses and manifest creation
type Catalog struct {
	Downloader *Downloader    // License Downloader
	List       *List          // List of licenses
	opts       CatalogOptions // SPDX Options
}

// WriteLicensesAsText writes the SPDX license collection to text files
func (catalog *Catalog) WriteLicensesAsText(targetDir string) error {
	logrus.Infof("Writing %d SPDX licenses to %s", len(catalog.List.Licenses), targetDir)
	if catalog.List.Licenses == nil {
		return errors.New("unable to write licenses, they have not been loaded yet")
	}
	if !util.Exists(targetDir) {
		if err := os.MkdirAll(targetDir, os.FileMode(0o755)); err != nil {
			return fmt.Errorf("creating license data dir: %w", err)
		}
	}

	var wg errgroup.Group
	for _, l := range catalog.List.Licenses {
		l := l
		wg.Go(func() error {
			if l.IsDeprecatedLicenseID {
				return nil
			}
			licPath := filepath.Join(targetDir, "assets", l.LicenseID)
			if !util.Exists(licPath) {
				if err := os.MkdirAll(licPath, 0o755); err != nil {
					return fmt.Errorf("creating license directory: %w", err)
				}
			}
			if err := l.WriteText(filepath.Join(licPath, "license.txt")); err != nil {
				return fmt.Errorf("wriiting license text: %w", err)
			}
			return nil
		})
	}
	if err := wg.Wait(); err != nil {
		return fmt.Errorf("while writing license files: %w", err)
	}
	return nil
}

// GetLicense returns a license struct from its SPDX ID label
func (catalog *Catalog) GetLicense(label string) *License {
	if lic, ok := catalog.List.Licenses[label]; ok {
		return lic
	}
	logrus.Warnf("Label %s is not an identifier of a known license ", label)
	return nil
}
