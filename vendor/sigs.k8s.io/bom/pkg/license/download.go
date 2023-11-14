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
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/sirupsen/logrus"

	"sigs.k8s.io/release-utils/http"
	"sigs.k8s.io/release-utils/util"
)

// ListURL is the json list of all spdx licenses
const (
	LicenseDataURL      = "https://spdx.org/licenses/"
	LicenseListFilename = "licenses.json"
	BaseReleaseURL      = "https://github.com/spdx/license-list-data/archive/refs/tags/"
	LatestReleaseURL    = "https://api.github.com/repos/spdx/license-list-data/releases/latest"
	EmbeddedDataDir     = "pkg/license/data/"
)

//go:embed data
var f embed.FS

// NewDownloader returns a downloader with the default options
func NewDownloader() (*Downloader, error) {
	return NewDownloaderWithOptions(DefaultDownloaderOpts)
}

// NewDownloaderWithOptions returns a downloader with specific options
func NewDownloaderWithOptions(opts *DownloaderOptions) (*Downloader, error) {
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("validating downloader options: %w", err)
	}
	impl := DefaultDownloaderImpl{}
	impl.SetOptions(opts)

	d := &Downloader{}
	d.SetImplementation(&impl)

	return d, nil
}

// DownloaderOptions is a set of options for the license downloader
type DownloaderOptions struct {
	EnableCache       bool   // Should we use the cache or not
	CacheDir          string // Directory where data will be cached, defaults to temporary dir
	parallelDownloads int    // Number of license downloads we'll do at once
	Version           string // Version of the licenses to download  (eg v3.19) or blank for latest
}

// Validate Checks the downloader options
func (do *DownloaderOptions) Validate() error {
	// If we are using a cache
	if do.EnableCache {
		// and no cache dir was specified
		if do.CacheDir == "" {
			// use a temporary dir
			dir, err := os.MkdirTemp(os.TempDir(), "license-cache-")
			do.CacheDir = dir
			if err != nil {
				return fmt.Errorf("creating temporary directory: %w", err)
			}
		} else if !util.Exists(do.CacheDir) {
			if err := os.MkdirAll(do.CacheDir, os.FileMode(0o755)); err != nil {
				return fmt.Errorf("creating license downloader cache: %w", err)
			}
		}

		// Is we have a cache dir, check if it exists
		if !util.Exists(do.CacheDir) {
			return errors.New("the specified cache directory does not exist: " + do.CacheDir)
		}
	}
	return nil
}

// Downloader handles downloading f license data
type Downloader struct {
	impl DownloaderImplementation
}

// SetImplementation sets the implementation that will drive the downloader
func (d *Downloader) SetImplementation(di DownloaderImplementation) {
	d.impl = di
}

// GetLicenses is the mina function of the downloader. Returns a license list
// or an error if could get them
func (d *Downloader) GetLicenses() (*List, error) {
	tag := d.impl.Version()
	var err error
	if tag == "" {
		tag, err = d.impl.GetLatestTag()
		if err != nil {
			return nil, fmt.Errorf("getting latest license list tag: %w", err)
		}
	}

	return d.impl.GetLicenses(tag)
}

//counterfeiter:generate . DownloaderImplementation

// DownloaderImplementation has only one method
type DownloaderImplementation interface {
	GetLicenses(versionTag string) (*List, error)
	SetOptions(*DownloaderOptions)
	GetLatestTag() (string, error)
	Version() string
	DownloadLicenseArchive(tag string) (zipData []byte, err error)
}

// DefaultDownloaderOpts set of options for the license downloader
var DefaultDownloaderOpts = &DownloaderOptions{
	EnableCache:       true,
	CacheDir:          "",
	parallelDownloads: 5,
}

// DefaultDownloaderImpl is the default implementation that gets licenses
type DefaultDownloaderImpl struct {
	Options *DownloaderOptions
}

// Version returns the version from the options
func (ddi *DefaultDownloaderImpl) Version() string {
	return ddi.Options.Version
}

// SetOptions sets the implementation options
func (ddi *DefaultDownloaderImpl) SetOptions(opts *DownloaderOptions) {
	ddi.Options = opts
}

// GetLatestTag gets the latest version of the license list from github
func (ddi *DefaultDownloaderImpl) GetLatestTag() (string, error) {
	var data []byte
	var err error
	if ddi.Options.EnableCache {
		data, err = ddi.getCachedData(LatestReleaseURL)
		if err != nil {
			return "", fmt.Errorf("getting latest version from cache: %w", err)
		}
	}

	if data == nil {
		data, err = http.NewAgent().Get(LatestReleaseURL)
		if err != nil {
			return "", err
		}

		if err := ddi.cacheData(LatestReleaseURL, data); err != nil {
			return "", fmt.Errorf("caching latest version: %w", err)
		}
	}
	type GHReleaseResp struct {
		TagName string `json:"tag_name"`
	}
	resp := GHReleaseResp{}
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", err
	}
	return resp.TagName, nil
}

// readLicenseDirectory Reads the license data from a filsystem. It supports a
// subpath if the license tree is located in a different directory.
func (ddi *DefaultDownloaderImpl) readLicenseDirectory(licensefs fs.FS, subpath string) (licenses *List, err error) {
	licenses = &List{}
	licensesJSON, err := fs.ReadFile(licensefs, filepath.Join(subpath, "json/licenses.json"))
	if err != nil {
		return nil, fmt.Errorf("reading license catalog: %w", err)
	}

	if err := json.Unmarshal(licensesJSON, &licenses); err != nil {
		return nil, fmt.Errorf("parsing SPDX licence list: %w", err)
	}

	err = fs.WalkDir(licensefs, filepath.Join(subpath, "json/details"), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		data, err := fs.ReadFile(licensefs, path)
		if err != nil {
			return fmt.Errorf("reading license file%s: %w", path, err)
		}
		license, err := ParseLicense(data)
		if err != nil {
			return fmt.Errorf("parsing license data: %w", err)
		}
		licenses.Add(license)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walking license filesystem: %w", err)
	}
	return licenses, nil
}

// DownloadLicenseListToFile downloads the license list from github and
// stores it in a file
func (d *Downloader) DownloadLicenseListToFile(tag, path string) (err error) {
	if tag == "" {
		tag, err = d.impl.GetLatestTag()
		if err != nil {
			return fmt.Errorf("getting latest license list")
		}
	}
	data, err := d.impl.DownloadLicenseArchive(tag)
	if err != nil {
		return fmt.Errorf("downloading archive: %w", err)
	}
	if err := os.WriteFile(path, data, os.FileMode(0o644)); err != nil {
		return fmt.Errorf("writing archive data: %w", err)
	}
	return nil
}

func (ddi *DefaultDownloaderImpl) DownloadLicenseArchive(tag string) (zipData []byte, err error) {
	if tag == DefaultCatalogOpts.Version {
		logrus.Infof("Using embedded %s license list", DefaultCatalogOpts.Version)
		return f.ReadFile(fmt.Sprintf("data/license-list-%s.zip", tag))
	}

	link := BaseReleaseURL + tag + ".zip"
	if ddi.Options.EnableCache {
		zipData, err = ddi.getCachedData(link)
		if err != nil {
			return nil, fmt.Errorf("getting cached data: %w", err)
		}
	}

	// No cached data available
	if zipData == nil {
		zipData, err = http.NewAgent().WithTimeout(time.Hour).Get(link)
		if err != nil {
			return nil, fmt.Errorf("downloading license tarball: %w", err)
		}
		if err := ddi.cacheData(link, zipData); err != nil {
			return nil, fmt.Errorf("caching license list: %w", err)
		}
	}
	return zipData, nil
}

// GetLicenses downloads the main json file listing all SPDX supported licenses
func (ddi *DefaultDownloaderImpl) GetLicenses(tag string) (licenses *List, err error) {
	zipData, err := ddi.DownloadLicenseArchive(tag)
	if err != nil {
		return nil, fmt.Errorf("downloading licenses: %w", err)
	}

	reader, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return nil, fmt.Errorf("creating zip reader: %w", err)
	}

	licenses, err = ddi.readLicenseDirectory(reader, fmt.Sprintf("license-list-data-%s", tag[1:]))
	if err != nil {
		return nil, fmt.Errorf("reading license filesystem: %w", err)
	}

	return licenses, nil
}

// cacheFileName return the cache filename for an URL
func (ddi *DefaultDownloaderImpl) cacheFileName(url string) string {
	return filepath.Join(
		ddi.Options.CacheDir, fmt.Sprintf("%x.json", sha256.New().Sum([]byte(url))),
	)
}

// cacheData writes data to a cache file
func (ddi *DefaultDownloaderImpl) cacheData(url string, data []byte) error {
	cacheFileName := ddi.cacheFileName(url)
	_, err := os.Stat(filepath.Dir(cacheFileName))
	if err != nil && os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(cacheFileName), os.FileMode(0o755)); err != nil {
			return fmt.Errorf("creating cache directory: %w", err)
		}
	}
	if err = os.WriteFile(cacheFileName, data, os.FileMode(0o644)); err != nil {
		return fmt.Errorf("writing cache file: %w", err)
	}
	logrus.Debugf("Cached %s to %s", url, cacheFileName)
	return nil
}

// getCachedData returns cached data for an URL if we have it
func (ddi *DefaultDownloaderImpl) getCachedData(url string) ([]byte, error) {
	cacheFileName := ddi.cacheFileName(url)
	finfo, err := os.Stat(cacheFileName)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("checking if cached data exists: %w", err)
	}

	if err != nil {
		logrus.Debugf("No cached data for %s", url)
		return nil, nil
	}

	if finfo.Size() == 0 {
		logrus.Warnf("Cached file %s is empty, removing", cacheFileName)
		return nil, fmt.Errorf("removing corrupt cached file: %w", os.Remove(cacheFileName))
	}
	cachedData, err := os.ReadFile(cacheFileName)
	if err != nil {
		return nil, fmt.Errorf("reading cached data file: %w", err)
	}
	logrus.Debugf("Reusing cached data from %s", url)
	return cachedData, nil
}

// GetLatestTag returns the last version of the SPDX LIcense list found on GitHub
func (d *Downloader) GetLatestTag() (string, error) {
	return d.impl.GetLatestTag()
}
