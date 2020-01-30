/*
   Copyright The containerd Authors.

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

package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

const (
	vendorConf = "vendor.conf"
	modulesTxt = "vendor/modules.txt"
	goMod      = "go.mod"
)

var (
	errUnknownFormat = errors.New("unknown file format")
	errEndOfSection  = errors.New("End of directive section")
)

func loadRelease(path string) (*release, error) {
	var r release
	if _, err := toml.DecodeFile(path, &r); err != nil {
		if os.IsNotExist(err) {
			return nil, errors.New("please specify the release file as the first argument")
		}
		return nil, err
	}
	return &r, nil
}

func parseTag(path string) string {
	return strings.TrimSuffix(filepath.Base(path), ".toml")
}

func parseDependencies(commit string) ([]dependency, error) {
	rd, err := fileFromRev(commit, vendorConf)
	if err == nil {
		return parseVendorConfDependencies(rd)
	}
	rd, err = fileFromRev(commit, modulesTxt)
	if err == nil {
		return parseModulesTxtDependencies(rd)
	}
	rd, err = fileFromRev(commit, goMod)
	if err == nil {
		return parseGoModDependencies(rd)
	}
	return nil, errors.Errorf("finding dependency file failed: %v", err)
}

func parseModulesTxtDependencies(r io.Reader) ([]dependency, error) {
	var dependencies []dependency
	s := bufio.NewScanner(r)
	for s.Scan() {
		ln := strings.TrimSpace(s.Text())
		if ln == "" {
			continue
		}
		parts := strings.Fields(ln)
		if parts[0] != "#" {
			continue
		}
		var commitOrVersionPart string
		if len(parts) == 3 {
			commitOrVersionPart = parts[2]
		} else if len(parts) == 6 && parts[3] == "=>" {
			commitOrVersionPart = parts[5]
		} else {
			return nil, errors.Wrapf(errUnknownFormat, "%s", ln)
		}
		commitOrVersion := getCommitOrVersion(commitOrVersionPart)
		if commitOrVersion == "" {
			return nil, errors.Wrapf(errUnknownFormat, "poorly formatted version in replace section %s", parts[2])
		}

		dependencies = append(dependencies, formatDependency(parts[1], commitOrVersion))
	}
	return dependencies, nil
}

func parseGoModDependencies(r io.Reader) ([]dependency, error) {
	var err error

	depMap := make(map[string]*dependency)
	replaceMap := make(map[string]string)
	s := bufio.NewScanner(r)
	for s.Scan() {
		ln := sanitizeLine(s.Text(), "//")
		if ln == "" {
			continue
		}
		parts := strings.Fields(ln)

		// scan the file until we find `$DIRECTIVE (`
		if parts[0] == "require" {
			if len(parts) < 2 {
				return nil, errors.Wrapf(errUnknownFormat, "%s", ln)
			}
			if parts[1] == "(" {
				depMap, err = processRequireSection(s, depMap)
				if err != nil {
					return nil, err
				}
			} else {
				dep, err := processRequireLine(parts[1:])
				if err != nil {
					return nil, err
				}
				depMap[dep.Name] = dep
			}
		}
		if parts[0] == "replace" {
			if len(parts) < 2 {
				return nil, errors.Wrapf(errUnknownFormat, "%s", ln)
			}
			if parts[1] == "(" {
				replaceMap, err = processReplaceSection(s, replaceMap)
				if err != nil {
					return nil, err
				}
			} else {
				name, commitOrVersion, err := processReplaceLine(parts[1:])
				if err != nil {
					return nil, err
				}
				replaceMap[name] = commitOrVersion
			}
		}
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	for depName, version := range replaceMap {
		if oldDep, ok := depMap[depName]; ok {
			oldDep.Commit = version
		} else {
			logrus.Debugf("dependency %s found in replace section, but doesn't exist in requires section. Skipping", depName)
			continue
		}
	}
	var deps []dependency
	for _, dep := range depMap {
		deps = append(deps, *dep)
	}

	return deps, nil
}

func processRequireSection(s *bufio.Scanner, depMap map[string]*dependency) (map[string]*dependency, error) {
	for s.Scan() {
		ln := sanitizeLine(s.Text(), "//")
		if ln == "" {
			continue
		}
		dep, err := processRequireLine(strings.Fields(ln))
		if err != nil {
			if errors.Cause(err) == errEndOfSection {
				break
			}
			return nil, err
		}
		depMap[dep.Name] = dep
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	return depMap, nil
}

func processRequireLine(parts []string) (*dependency, error) {
	numParts := len(parts)

	if numParts != 2 {
		if numParts == 1 && parts[0] == ")" {
			return nil, errEndOfSection
		}
		return nil, errors.Wrapf(errUnknownFormat, "%v", parts)
	}

	commitOrVersion := getCommitOrVersion(parts[1])
	if commitOrVersion == "" {
		return nil, errors.Wrapf(errUnknownFormat, "poorly formatted version in replace section %s", parts[2])
	}

	dep := formatDependency(parts[0], commitOrVersion)
	return &dep, nil
}

func processReplaceSection(s *bufio.Scanner, replaceMap map[string]string) (map[string]string, error) {
	for s.Scan() {
		ln := sanitizeLine(s.Text(), "//")
		if ln == "" {
			continue
		}

		name, commitOrVersion, err := processReplaceLine(strings.Fields(ln))
		if err != nil {
			if errors.Cause(err) == errEndOfSection {
				break
			}
			return nil, err
		}

		replaceMap[name] = commitOrVersion
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	return replaceMap, nil
}

func processReplaceLine(parts []string) (string, string, error) {
	numParts := len(parts)
	if numParts != 4 {
		if numParts == 1 && parts[0] == ")" {
			// this is the end of the requires section, break out to process the others
			return "", "", errEndOfSection
		}
		return "", "", errors.Wrapf(errUnknownFormat, "%v", parts)
	}

	commitOrVersion := getCommitOrVersion(parts[3])
	if commitOrVersion == "" {
		return "", "", errors.Wrapf(errUnknownFormat, "poorly formatted version in replace section %s", parts[2])
	}
	return parts[0], commitOrVersion, nil
}

func sanitizeLine(line, commentDelim string) string {
	ln := strings.TrimSpace(line)
	if ln == "" {
		return ""
	}
	cidx := strings.Index(ln, commentDelim)
	// whole line is commented
	if cidx == 0 {
		return ""
	}
	if cidx > 0 {
		ln = ln[:cidx]
	}

	return strings.TrimSpace(ln)
}

func getCommitOrVersion(cov string) string {
	// parse the commit or version. It'll either be of the form
	// v0.0.0 or v0.0.0-date-commitID. Split by '-' to check
	dashFields := strings.FieldsFunc(cov, func(c rune) bool { return c == '-' })
	fieldsLen := len(dashFields)

	if fieldsLen > 3 {
		// empty string signifies error to caller
		return ""
	}

	// if dashFields has one or two fields, it is likely a version (possibly with a -rc1).
	// Thus, it should be used as is.
	// the only case we meddle is when there are three fields, so we can strip the commitID
	if len(dashFields) == 3 {
		// If there are three fields, use the last (the commit)
		// as often the version found in the first field is just a placeholder
		cov = dashFields[2]
	}

	// despite it being idiomatic to go modules, the +incompatible is a bit
	// unsightly in release notes. Let's cut it out of the version if it
	// exists
	if incpIdx := strings.Index(cov, "+incompatible"); incpIdx > 0 {
		return cov[:incpIdx]
	}
	return cov
}

func formatDependency(name, commitOrVersion string) dependency {
	return dependency{
		Name:     name,
		Commit:   commitOrVersion,
		CloneURL: "git://" + name,
	}
}

func parseVendorConfDependencies(r io.Reader) ([]dependency, error) {
	var deps []dependency
	re, err := regexp.Compile("[0-9a-f]{40}")
	if err != nil {
		return nil, err
	}

	s := bufio.NewScanner(r)
	for s.Scan() {
		ln := sanitizeLine(s.Text(), "#")
		if ln == "" {
			continue
		}
		parts := strings.Fields(ln)
		if len(parts) != 2 && len(parts) != 3 {
			return nil, fmt.Errorf("invalid config format: %s", ln)
		}

		var cloneURL string
		if len(parts) == 3 {
			cloneURL = parts[2]
		} else {
			cloneURL = "git://" + parts[0]
		}

		// trim the commit to 12 characters to match go mod length
		commitOrVersion := parts[1]
		if matched := re.Match([]byte(commitOrVersion)); matched {
			commitOrVersion = commitOrVersion[:12]
		}

		deps = append(deps, dependency{
			Name:     parts[0],
			Commit:   commitOrVersion,
			CloneURL: cloneURL,
		})
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	return deps, nil
}

func changelog(previous, commit string) ([]change, error) {
	raw, err := getChangelog(previous, commit)
	if err != nil {
		return nil, err
	}
	return parseChangelog(raw)
}

func gitChangeDiff(previous, commit string) string {
	if previous != "" {
		return fmt.Sprintf("%s..%s", previous, commit)
	}
	return commit
}

func getChangelog(previous, commit string) ([]byte, error) {
	return git("log", "--oneline", gitChangeDiff(previous, commit))
}

func linkifyChanges(c []change, commit, msg func(change) (string, error)) error {
	for i := range c {
		commitLink, err := commit(c[i])
		if err != nil {
			return err
		}

		description, err := msg(c[i])
		if err != nil {
			return err
		}

		c[i].Commit = fmt.Sprintf("[`%s`](%s)", c[i].Commit, commitLink)
		c[i].Description = description

	}

	return nil
}

func parseChangelog(changelog []byte) ([]change, error) {
	var (
		changes []change
		s       = bufio.NewScanner(bytes.NewReader(changelog))
	)
	for s.Scan() {
		fields := strings.Fields(s.Text())
		changes = append(changes, change{
			Commit:      fields[0],
			Description: strings.Join(fields[1:], " "),
		})
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	return changes, nil
}

func fileFromRev(rev, file string) (io.Reader, error) {
	p, err := git("show", fmt.Sprintf("%s:%s", rev, file))
	if err != nil {
		return nil, err
	}

	return bytes.NewReader(p), nil
}

var gitConfigs = map[string]string{}

func git(args ...string) ([]byte, error) {
	var gitArgs []string
	for k, v := range gitConfigs {
		gitArgs = append(gitArgs, "-c", fmt.Sprintf("%s=%s", k, v))
	}
	gitArgs = append(gitArgs, args...)
	o, err := exec.Command("git", gitArgs...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%s: %s", err, o)
	}
	return o, nil
}

func renameDependencies(deps []dependency, renames map[string]projectRename) {
	if len(renames) == 0 {
		return
	}
	type dep struct {
		shortname string
		name      string
	}
	renameMap := map[string]dep{}
	for shortname, rename := range renames {
		renameMap[rename.Old] = dep{
			shortname: shortname,
			name:      rename.New,
		}
	}
	for i := range deps {
		if updated, ok := renameMap[deps[i].Name]; ok {
			logrus.Debugf("Renamed %s from %s to %s", updated.shortname, deps[i].Name, updated.name)
			deps[i].Name = updated.name
		}
	}
}

func updatedDeps(previous, deps []dependency) []dependency {
	var updated []dependency
	pm, cm := toDepMap(previous), toDepMap(deps)
	for name, c := range cm {
		d, ok := pm[name]
		if !ok {
			// it is a new dep and should be noted
			updated = append(updated, c)
			continue
		}
		// it exists, see if its updated
		if d.Commit != c.Commit {
			// set the previous commit
			c.Previous = d.Commit
			updated = append(updated, c)
		}
	}
	return updated
}

func toDepMap(deps []dependency) map[string]dependency {
	out := make(map[string]dependency)
	for _, d := range deps {
		out[d.Name] = d
	}
	return out
}

type contributor struct {
	name  string
	email string
}

func addContributors(previous, commit string, contributors map[contributor]int) error {
	raw, err := git("log", `--format=%aE %aN`, gitChangeDiff(previous, commit))
	if err != nil {
		return err
	}
	s := bufio.NewScanner(bytes.NewReader(raw))
	for s.Scan() {
		p := strings.SplitN(s.Text(), " ", 2)
		if len(p) != 2 {
			return errors.Errorf("invalid author line: %q", s.Text())
		}
		c := contributor{
			name:  p[1],
			email: p[0],
		}
		contributors[c] = contributors[c] + 1
	}
	return s.Err()
}

func orderContributors(contributors map[contributor]int) []string {
	type contribstat struct {
		name  string
		email string
		count int
	}
	all := make([]contribstat, 0, len(contributors))
	for c, count := range contributors {
		all = append(all, contribstat{
			name:  c.name,
			email: c.email,
			count: count,
		})
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].count == all[j].count {
			return all[i].name < all[j].name
		}
		return all[i].count > all[j].count
	})
	names := make([]string, len(all))
	for i := range names {
		logrus.Debugf("Contributor: %s <%s> with %d commits", all[i].name, all[i].email, all[i].count)
		names[i] = all[i].name
	}

	return names
}

// getTemplate will use a builtin template if the template is not specified on the cli
func getTemplate(context *cli.Context) (string, error) {
	path := context.GlobalString("template")
	f, err := os.Open(path)
	if err != nil {
		// if the template file does not exist and the path is for the default template then
		// return the compiled in template
		if os.IsNotExist(err) && path == defaultTemplateFile {
			return releaseNotes, nil
		}
		return "", err
	}
	defer f.Close()
	data, err := ioutil.ReadAll(f)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func githubCommitLink(repo string) func(change) (string, error) {
	return func(c change) (string, error) {
		full, err := git("rev-parse", c.Commit)
		if err != nil {
			return "", err
		}
		commit := strings.TrimSpace(string(full))

		return fmt.Sprintf("https://github.com/%s/commit/%s", repo, commit), nil
	}
}

func githubPRLink(repo string) func(change) (string, error) {
	r := regexp.MustCompile("^Merge pull request #[0-9]+")
	return func(c change) (string, error) {
		var err error
		message := r.ReplaceAllStringFunc(c.Description, func(m string) string {
			idx := strings.Index(m, "#")
			pr := m[idx+1:]

			// TODO: Validate links using github API
			// TODO: Validate PR merged as commit hash
			link := fmt.Sprintf("https://github.com/%s/pull/%s", repo, pr)

			return fmt.Sprintf("%s [#%s](%s)", m[:idx], pr, link)
		})
		if err != nil {
			return "", err
		}
		return message, nil
	}
}
