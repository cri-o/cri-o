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

package spdx

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/nozzle/throttler"
	purl "github.com/package-url/packageurl-go"
	"github.com/sirupsen/logrus"
	"golang.org/x/mod/modfile"
	"golang.org/x/tools/go/vcs"

	"sigs.k8s.io/bom/pkg/license"
	"sigs.k8s.io/release-utils/command"
	"sigs.k8s.io/release-utils/util"
)

const (
	downloadDir   = spdxTempDir + "/gomod-scanner"
	GoModFileName = "go.mod"
	GoSumFileName = "go.sum"
	goModRevPtn   = `v\d+\.\d+\.\d+-[0-9.]+-([a-f0-9]+)` // Match revisions in go modules
)

var goModRevRe *regexp.Regexp

// NewGoModule returns a new go module from the specified path
func NewGoModuleFromPath(path string) (*GoModule, error) {
	mod := NewGoModule()
	mod.opts.Path = path
	return mod, nil
}

func NewGoModule() *GoModule {
	return &GoModule{
		opts: &GoModuleOptions{},
		impl: &GoModDefaultImpl{},
	}
}

// GoModule abstracts the go module data of a project
type GoModule struct {
	impl     GoModImplementation
	GoMod    *modfile.File
	opts     *GoModuleOptions // Options
	Packages []*GoPackage     // maps of package download locations
}

type GoModuleOptions struct {
	Path           string // Path to the dir where go.mod resides
	OnlyDirectDeps bool   // Only include direct dependencies from go.mod
	ScanLicenses   bool   // Scan licenses from everypossible place unless false
}

// Options returns a pointer to the module options set
func (mod *GoModule) Options() *GoModuleOptions {
	return mod.opts
}

// GoPackage basic pkg data we need
type GoPackage struct {
	TmpDir        bool
	ImportPath    string
	Revision      string
	LocalDir      string
	LocalInstall  string
	LicenseID     string
	CopyrightText string
}

// SPDXPackage builds a spdx package from the go package data
func (pkg *GoPackage) ToSPDXPackage() (*Package, error) {
	repo, err := vcs.RepoRootForImportPath(pkg.ImportPath, true)
	if err != nil {
		return nil, fmt.Errorf("building repository from package import path: %w", err)
	}
	spdxPackage := NewPackage()
	spdxPackage.Options().Prefix = "gomod"
	spdxPackage.Name = pkg.ImportPath

	spdxPackage.BuildID(pkg.ImportPath, pkg.Revision)
	if strings.Contains(pkg.Revision, "+incompatible") {
		spdxPackage.DownloadLocation = repo.VCS.Scheme[0] + "+" + repo.Repo
	} else {
		spdxPackage.DownloadLocation = fmt.Sprintf(
			"https://proxy.golang.org/%s/@v/%s.zip", pkg.ImportPath,
			strings.TrimSuffix(pkg.Revision, "+incompatible"),
		)
	}
	spdxPackage.LicenseConcluded = pkg.LicenseID
	spdxPackage.Version = strings.TrimSuffix(pkg.Revision, "+incompatible")
	spdxPackage.CopyrightText = pkg.CopyrightText
	if packageurl := pkg.PackageURL(); packageurl != "" {
		spdxPackage.ExternalRefs = append(spdxPackage.ExternalRefs, ExternalRef{
			Category: CatPackageManager,
			Type:     "purl",
			Locator:  packageurl,
		})
	}
	return spdxPackage, nil
}

func nsAndNameFromImportPath(importPath string) (namespace, packageName string) {
	lastSlashIndex := strings.LastIndex(importPath, "/")
	if lastSlashIndex == -1 {
		return "", ""
	}
	namespace = importPath[0:lastSlashIndex]
	packageName = importPath[lastSlashIndex+1:]
	return namespace, packageName
}

// PackageURL returns a purl if the go package has enough data to generate
// one. If data is missing, it will return an empty string
func (pkg *GoPackage) PackageURL() string {
	namespace, pname := nsAndNameFromImportPath(pkg.ImportPath)
	// We require type, package, namespace and version at the very
	// least to generate a purl
	if pname == "" || pkg.Revision == "" || namespace == "" {
		return ""
	}

	return purl.NewPackageURL(
		purl.TypeGolang, namespace, pname,
		strings.TrimSuffix(pkg.Revision, "+incompatible"), nil, "",
	).ToString()
}

type GoModImplementation interface {
	OpenModule(*GoModuleOptions) (*modfile.File, error)
	BuildPackageList(*modfile.File) ([]*GoPackage, error)
	DownloadPackage(*GoPackage, *GoModuleOptions, bool) error
	RemoveDownloads([]*GoPackage) error
	LicenseReader() (*license.Reader, error)
	ScanPackageLicense(*GoPackage, *license.Reader, *GoModuleOptions) error
}

// Initializes a go module from the specified path
func (mod *GoModule) Open() error {
	gomod, err := mod.impl.OpenModule(mod.opts)
	if err != nil {
		return fmt.Errorf("opening module: %w", err)
	}
	mod.GoMod = gomod

	// Build the package list
	var pkgs []*GoPackage
	if mod.Options().OnlyDirectDeps {
		pkgs, err = mod.impl.BuildPackageList(mod.GoMod)
	} else {
		pkgs, err = mod.BuildFullPackageList(mod.GoMod)
	}
	if err != nil {
		return fmt.Errorf("building module package list: %w", err)
	}
	mod.Packages = pkgs
	return nil
}

// RemoveDownloads cleans all downloads
func (mod *GoModule) RemoveDownloads() error {
	return mod.impl.RemoveDownloads(mod.Packages)
}

// DownloadPackages downloads all the module's packages to the local disk
func (mod *GoModule) DownloadPackages() error {
	logrus.Infof("Downloading source code for %d packages", len(mod.Packages))
	if mod.Packages == nil {
		return errors.New("unable to download packages, package list is nil")
	}

	for _, pkg := range mod.Packages {
		if err := mod.impl.DownloadPackage(pkg, mod.opts, true); err != nil {
			return err
		}
	}
	return nil
}

// ScanLicenses scans the licenses and populats the fields
func (mod *GoModule) ScanLicenses() error {
	if mod.Packages == nil {
		return errors.New("unable to scan lincese files, package list is nil")
	}

	reader, err := mod.impl.LicenseReader()
	if err != nil {
		return fmt.Errorf("creating license scanner: %w", err)
	}

	logrus.Infof("Scanning licenses for %d go packages", len(mod.Packages))

	// Create a new Throttler that will get parallelDownloads urls at a time
	t := throttler.New(10, len(mod.Packages))
	// Do a quick re-check for missing downloads
	// todo: paralelize this. urgently.
	for _, pkg := range mod.Packages {
		// Launch a goroutine to fetch the package contents
		go func(curPkg *GoPackage) {
			logrus.WithField(
				"package", curPkg.ImportPath).Debugf(
				"Downloading package (%d total)", len(mod.Packages),
			)
			defer t.Done(err)
			if curPkg.LocalInstall == "" {
				// Call download with no force in case local data is missing
				if err2 := mod.impl.DownloadPackage(curPkg, mod.opts, false); err2 != nil {
					// If we're unable to download the module we dont treat it as
					// fatal, package will remain without license info but we go
					// on scanning the rest of the packages.
					logrus.WithField("package", curPkg.ImportPath).Error(err2)
					return
				}
			} else {
				logrus.WithField("package", curPkg.ImportPath).Debugf(
					"There is a local copy of %s@%s", curPkg.ImportPath, curPkg.Revision,
				)
			}

			if err = mod.impl.ScanPackageLicense(curPkg, reader, mod.opts); err != nil {
				logrus.WithField("package", curPkg.ImportPath).Errorf(
					"scanning package %s for licensing info", curPkg.ImportPath,
				)
			}
		}(pkg)
		t.Throttle()
	}

	if t.Err() != nil {
		return t.Err()
	}

	return nil
}

// BuildFullPackageList return the complete of packages imported into
// the module, instead of reading go.mod, this functions calls
// go list and works from there
func (mod *GoModule) BuildFullPackageList(_ *modfile.File) (packageList []*GoPackage, err error) {
	packageList = []*GoPackage{}

	// If no go.sum is found, then there are no deps
	if !util.Exists(filepath.Join(mod.opts.Path, GoSumFileName)) {
		return packageList, nil
	}

	gobin, err := exec.LookPath("go")
	if err != nil {
		return nil, errors.New("unable to get full list of packages, go executbale not found ")
	}

	gorun := command.NewWithWorkDir(mod.opts.Path, gobin, "list", "-deps", "-e", "-json", "./...")
	output, err := gorun.RunSilentSuccessOutput()
	if err != nil {
		return nil, fmt.Errorf("while calling go to get full list of deps: %w", err)
	}

	type ModEntry struct {
		DepOnly bool `json:"DepOnly,omitempty"`
		Main    bool `json:"Main,omitempty"`
		Module  struct {
			Path     string `json:"Path,omitempty"`    // Path is theImportPath
			Main     bool   `json:"Main,omitempty"`    // true if its the main module (eg k/release)
			Dir      string `json:"Dir,omitempty"`     // The source can be found here
			GoMod    string `json:"GoMod,omitempty"`   // Or cached here
			Version  string `json:"Version,omitempty"` // PAckage version
			Indirect bool   `json:"Indirect,omitempty"`
			Replace  *struct {
				Dir string `json:"Dir,omitempty"`
			} `json:"Replace,omitempty"`
		} `json:"Module,omitempty"`
	}

	dec := json.NewDecoder(strings.NewReader(output.Output()))
	list := map[string]map[string]*ModEntry{}
	for dec.More() {
		m := &ModEntry{}
		// Decode the json stream as we get "Module" blocks from go:
		if err := dec.Decode(m); err != nil {
			return nil, fmt.Errorf("decoding module list: %w", err)
		}
		if m.Module.Path != "" {
			// If this is the main package (ie the module itself) skip
			if m.Module.Main {
				continue
			}

			if _, ok := list[m.Module.Path]; !ok {
				list[m.Module.Path] = map[string]*ModEntry{}
			}

			// Go list will return modules with a specific version
			// and sometime duplicate entries, generic for the module
			// witjout version. We try to handle both cases here:
			if m.Module.Version == "" {
				// If we got a generic module entry, add it to the list
				// but only if we do not have a more specific (versioned)
				// entry
				if len(list[m.Module.Path]) == 0 {
					list[m.Module.Path][m.Module.Version] = m
				}
			} else {
				// If we got a specific version, but previously had a
				// generic entry for the module, delete it
				list[m.Module.Path][m.Module.Version] = m
				delete(list[m.Module.Path], "")
			}
		}
	}
	logrus.Info("Adding full list of dependencies:")
	for _, versions := range list {
		for _, fmod := range versions {
			dep := &GoPackage{
				ImportPath:   fmod.Module.Path,
				Revision:     fmod.Module.Version,
				LocalDir:     "",
				LocalInstall: "",
			}
			status := ""
			if fmod.Module.Dir != "" && util.Exists(fmod.Module.Dir) {
				dep.LocalInstall = fmod.Module.Dir
				status = "(available locally)"
			}

			// Check if we have a local replacement
			if fmod.Module.Replace != nil &&
				fmod.Module.Replace.Dir != "" &&
				// If the local directory exists:
				util.Exists(fmod.Module.Replace.Dir) {
				logrus.Infof(
					"Package %s has local replacement in %s",
					dep.ImportPath, fmod.Module.Replace.Dir,
				)
				dep.LocalInstall = fmod.Module.Replace.Dir
				status = "(has a local replacement)"
			}

			logrus.Infof(" > %s@%s %s", dep.ImportPath, dep.Revision, status)
			packageList = append(packageList, dep)
		}
	}
	logrus.Infof("Found %d packages from full dependency tree", len(packageList))
	return packageList, nil
}

type GoModDefaultImpl struct {
	licenseReader *license.Reader
}

// OpenModule opens the go,mod file for the module and parses it
func (di *GoModDefaultImpl) OpenModule(opts *GoModuleOptions) (*modfile.File, error) {
	modData, err := os.ReadFile(filepath.Join(opts.Path, GoModFileName))
	if err != nil {
		return nil, fmt.Errorf("reading module's go.mod file: %w", err)
	}
	gomod, err := modfile.ParseLax("file", modData, nil)
	if err != nil {
		return nil, fmt.Errorf("reading go.mod: %w", err)
	}
	logrus.Infof(
		"Parsed go.mod file for %s, found %d direct dependencies",
		gomod.Module.Mod.Path, len(gomod.Require),
	)
	return gomod, nil
}

// BuildPackageList builds a slice of packages to assign to the module
func (di *GoModDefaultImpl) BuildPackageList(gomod *modfile.File) ([]*GoPackage, error) {
	pkgs := []*GoPackage{}
	for _, req := range gomod.Require {
		pkgs = append(pkgs, &GoPackage{
			ImportPath: req.Mod.Path,
			Revision:   req.Mod.Version,
		})
	}
	return pkgs, nil
}

// DownloadPackage takes a pkg, downloads it from its src and sets
//
//	the download dir in the LocalDir field
func (di *GoModDefaultImpl) DownloadPackage(pkg *GoPackage, _ *GoModuleOptions, force bool) error {
	if pkg.LocalDir != "" && util.Exists(pkg.LocalDir) && !force {
		logrus.WithField("package", pkg.ImportPath).Infof("Not downloading %s as it already has local data", pkg.ImportPath)
		return nil
	}

	logrus.WithField("package", pkg.ImportPath).Debugf("Downloading package %s@%s", pkg.ImportPath, pkg.Revision)
	repo, err := vcs.RepoRootForImportPath(pkg.ImportPath, true)
	if err != nil {
		repoName := "[unknown repo]"
		if repo != nil {
			repoName = repo.Repo
		}
		return fmt.Errorf("fetching package %s from %s: %w", pkg.ImportPath, repoName, err)
	}

	if !util.Exists(filepath.Join(os.TempDir(), downloadDir)) {
		if err := os.MkdirAll(
			filepath.Join(os.TempDir(), downloadDir), os.FileMode(0o755),
		); err != nil {
			return fmt.Errorf("creating parent tmpdir: %w", err)
		}
	}

	// Create tempdir
	tmpDir, err := os.MkdirTemp(filepath.Join(os.TempDir(), downloadDir), "package-download-")
	if err != nil {
		return fmt.Errorf("creating temporary dir: %w", err)
	}
	// Create a clone of the module repo at the revision
	rev := strings.TrimSuffix(pkg.Revision, "+incompatible")

	// Strip the revision from the whole string part
	if goModRevRe == nil {
		goModRevRe = regexp.MustCompile(goModRevPtn)
	}
	m := goModRevRe.FindStringSubmatch(pkg.Revision)
	if len(m) > 1 {
		rev = m[1]
		logrus.WithField("package", pkg.ImportPath).Infof("Using commit %s as revision for download", rev)
	}
	if rev == "" {
		if err := repo.VCS.Create(tmpDir, repo.Repo); err != nil {
			return fmt.Errorf("creating local clone of %s: %w", repo.Repo, err)
		}
	} else {
		if err := repo.VCS.CreateAtRev(tmpDir, repo.Repo, rev); err != nil {
			return fmt.Errorf("creating local clone of %s: %w", repo.Repo, err)
		}
	}

	logrus.WithField("package", pkg.ImportPath).Infof("Go Package %s (rev %s) downloaded to %s", pkg.ImportPath, pkg.Revision, tmpDir)
	pkg.LocalDir = tmpDir
	pkg.TmpDir = true
	return nil
}

// RemoveDownloads takes a list of packages and remove its downloads
func (di *GoModDefaultImpl) RemoveDownloads(packageList []*GoPackage) error {
	for _, pkg := range packageList {
		if pkg.ImportPath != "" && util.Exists(pkg.LocalDir) && pkg.TmpDir {
			if err := os.RemoveAll(pkg.LocalDir); err != nil {
				return fmt.Errorf("removing package data: %w", err)
			}
		}
	}
	return nil
}

// LicenseReader returns a license reader
func (di *GoModDefaultImpl) LicenseReader() (*license.Reader, error) {
	if di.licenseReader == nil {
		opts := license.DefaultReaderOptions
		opts.CacheDir = filepath.Join(os.TempDir(), spdxLicenseDlCache)
		opts.LicenseDir = filepath.Join(os.TempDir(), spdxLicenseData)
		if !util.Exists(opts.CacheDir) {
			if err := os.MkdirAll(opts.CacheDir, os.FileMode(0o755)); err != nil {
				return nil, fmt.Errorf("creating dir: %w", err)
			}
		}
		reader, err := license.NewReaderWithOptions(opts)
		if err != nil {
			return nil, fmt.Errorf("creating reader: %w", err)
		}

		di.licenseReader = reader
	}
	return di.licenseReader, nil
}

// ScanPackageLicense scans a package for licensing info
func (di *GoModDefaultImpl) ScanPackageLicense(
	pkg *GoPackage, reader *license.Reader, _ *GoModuleOptions,
) error {
	dir := pkg.LocalDir
	if dir == "" && pkg.LocalInstall != "" {
		dir = pkg.LocalInstall
	}
	licenseResult, err := reader.ReadTopLicense(dir)
	if err != nil {
		return fmt.Errorf("scanning package %s for licensing information: %w", pkg.ImportPath, err)
	}

	if licenseResult != nil {
		logrus.Debugf(
			"Package %s license is %s", pkg.ImportPath,
			licenseResult.License.LicenseID,
		)
		pkg.LicenseID = licenseResult.License.LicenseID
		pkg.CopyrightText = licenseResult.Text
	} else {
		logrus.Warnf("Could not find licensing information for package %s", pkg.ImportPath)
	}
	return nil
}
