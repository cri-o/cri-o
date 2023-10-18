/*
Copyright 2022 The Kubernetes Authors.

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

package osinfo

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	purl "github.com/package-url/packageurl-go"
	"github.com/sirupsen/logrus"
	apk "gitlab.alpinelinux.org/alpine/go/repository"
)

const apkDBPath = "lib/apk/db/installed"

// TODO: Move functions to its own implementation
type ContainerScanner struct{}

// ReadOSPackages reads a bunch of layers and extracts the os package
// information from them, it returns the OS package and the layer where
// they are defined. If the OS is not supported, we return a nil pointer.
func (ct *ContainerScanner) ReadOSPackages(layers []string) (
	layerNum int, packages *[]PackageDBEntry, err error,
) {
	loss := LayerScanner{}

	// First, let's try to determine which OS the container is based on
	osKind := ""
	for _, lp := range layers {
		osKind, err = loss.OSType(lp)
		if err != nil {
			return 0, nil, fmt.Errorf("reading os type from layer: %w", err)
		}
		if osKind != "" {
			break
		}
	}

	purlType := ""

	switch osKind {
	case OSDebian, OSUbuntu:
		layerNum, packages, err = ct.ReadDebianPackages(layers)
		purlType = purl.TypeDebian
	case OSAlpine, OSWolfi:
		layerNum, packages, err = ct.ReadApkPackages(layers)
		purlType = "apk"
	default:
		return 0, nil, nil
	}

	ct.setPurlData(purlType, osKind, packages)
	return layerNum, packages, err
}

// setPurlData stamps al found packages with the purl type and NS
func (ct *ContainerScanner) setPurlData(ptype, pnamespace string, packages *[]PackageDBEntry) {
	if packages == nil {
		return
	}
	for i := range *packages {
		(*packages)[i].Type = ptype
		(*packages)[i].Namespace = pnamespace
	}
}

// ReadDebianPackages scans through a set of container layers looking for the
// last update to the debian package database. If found, extracts it and
// sends it to parseDpkgDB to extract the package information from the file.
func (ct *ContainerScanner) ReadDebianPackages(layers []string) (layer int, pk *[]PackageDBEntry, err error) {
	// Cycle the layers in order, trying to extract the dpkg database
	dpkgDatabase := ""
	loss := LayerScanner{}
	for i, lp := range layers {
		dpkgDB, err := os.CreateTemp("", "dpkg-")
		if err != nil {
			return 0, pk, fmt.Errorf("opening temp dpkg file: %w", err)
		}
		dpkgPath := dpkgDB.Name()
		if err := loss.extractFileFromTar(lp, "var/lib/dpkg/status", dpkgPath); err != nil {
			os.Remove(dpkgDB.Name())
			if _, ok := err.(ErrFileNotFoundInTar); ok {
				continue
			}
			return 0, pk, fmt.Errorf("extracting dpkg database: %w", err)
		}
		logrus.Infof("Layer %d has a newer version of dpkg database", i)
		dpkgDatabase = dpkgPath
		layer = i
	}

	if dpkgDatabase == "" {
		logrus.Info("dbdata is blank")
		return layer, nil, nil
	}
	defer os.Remove(dpkgDatabase)
	pk, err = ct.parseDpkgDB(dpkgDatabase)
	return layer, pk, err
}

// ReadApkPackages reads the last known changed copy of the apk database
func (ct *ContainerScanner) ReadApkPackages(layers []string) (layer int, pk *[]PackageDBEntry, err error) {
	apkDatabase := ""
	loss := LayerScanner{}
	for i, lp := range layers {
		tmpDB, err := os.CreateTemp("", "apkdb-")
		if err != nil {
			return 0, pk, fmt.Errorf("opening temporary apkdb file: %w", err)
		}
		tmpDBPath := tmpDB.Name()
		if err := loss.extractFileFromTar(lp, apkDBPath, tmpDBPath); err != nil {
			os.Remove(tmpDBPath)
			if _, ok := err.(ErrFileNotFoundInTar); ok {
				continue
			}
			return 0, pk, fmt.Errorf("extracting apk database: %w", err)
		}
		logrus.Debugf("Layer %d has a newer version of apk database", i)
		apkDatabase = tmpDBPath
		layer = i
	}

	if apkDatabase == "" {
		logrus.Info("apk database data is empty")
		return layer, nil, nil
	}
	defer os.Remove(apkDatabase)

	pk, err = ct.parseApkDB(apkDatabase)
	if err != nil {
		return layer, nil, fmt.Errorf("parsing apk database: %w", err)
	}
	return layer, pk, err
}

type PackageDBEntry struct {
	Package         string
	Version         string
	Architecture    string
	Type            string // purl package type (ref: https://github.com/package-url/purl-spec/blob/master/PURL-TYPES.rst)
	Namespace       string // purl namespace
	MaintainerName  string
	MaintainerEmail string
	HomePage        string
	License         string // License expression
	Checksums       map[string]string
}

// PackageURL returns a purl representing the db entry. If the entry
// does not have enough data to generate the purl, it will return an
// empty string
func (e *PackageDBEntry) PackageURL() string {
	// We require type, package, namespace and version at the very
	// least to generate a purl
	if e.Package == "" || e.Version == "" || e.Namespace == "" || e.Type == "" {
		return ""
	}

	qualifiersMap := map[string]string{}

	// Add the architecture
	// TODO(puerco): Support adding the distro
	if e.Architecture != "" {
		qualifiersMap["arch"] = e.Architecture
	}
	return purl.NewPackageURL(
		e.Type, e.Namespace, e.Package,
		e.Version, purl.QualifiersFromMap(qualifiersMap), "",
	).ToString()
}

// parseDpkgDB reads a dpks database and populates a slice of PackageDBEntry
// with information from the packages found
func (ct *ContainerScanner) parseDpkgDB(dbPath string) (*[]PackageDBEntry, error) {
	file, err := os.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening for reading: %w", err)
	}
	defer file.Close()
	logrus.Infof("Scanning data from dpkg database in %s", dbPath)
	db := []PackageDBEntry{}
	scanner := bufio.NewScanner(file)
	var curPkg *PackageDBEntry
	for scanner.Scan() {
		parts := strings.SplitN(scanner.Text(), ":", 2)
		if len(parts) < 2 {
			continue
		}

		switch parts[0] {
		case "Package":
			if curPkg != nil {
				db = append(db, *curPkg)
			}
			curPkg = &PackageDBEntry{
				Package: strings.TrimSpace(parts[1]),
				Type:    purl.TypeDebian,
			}
		case "Architecture":
			if curPkg != nil {
				curPkg.Architecture = strings.TrimSpace(parts[1])
			}
		case "Version":
			if curPkg != nil {
				curPkg.Version = strings.TrimSpace(parts[1])
			}
		case "Homepage":
			if curPkg != nil {
				curPkg.HomePage = strings.TrimSpace(parts[1])
			}
		case "Maintainer":
			if curPkg != nil {
				mparts := strings.SplitN(parts[1], "<", 2)
				if len(mparts) == 2 {
					curPkg.MaintainerName = strings.TrimSpace(mparts[0])
					curPkg.MaintainerEmail = strings.TrimSuffix(strings.TrimSpace(mparts[1]), ">")
				}
			}
		}
	}

	logrus.Infof("Found %d packages", len(db))

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanning database file: %w", err)
	}

	return &db, err
}

func (ct *ContainerScanner) parseApkDB(dbPath string) (*[]PackageDBEntry, error) {
	f, err := os.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening apkdb: %w", err)
	}
	apks, err := apk.ParsePackageIndex(f)
	if err != nil {
		return nil, fmt.Errorf("parsing apk db: %w", err)
	}

	packages := []PackageDBEntry{}
	for _, p := range apks {
		cs := map[string]string{}
		if strings.HasPrefix(p.ChecksumString(), "Q1") {
			cs["SHA1"] = fmt.Sprintf("%x", p.Checksum)
		} else if p.ChecksumString() != "" {
			cs["MD5"] = fmt.Sprintf("%x", p.Checksum)
		}

		packages = append(packages, PackageDBEntry{
			Package:        p.Name,
			Version:        p.Version,
			Architecture:   p.Arch,
			Type:           "apk",
			MaintainerName: p.Maintainer,
			License:        p.License,
			Checksums:      cs,
		})
	}
	return &packages, nil
}
