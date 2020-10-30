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

package release

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"k8s.io/release/pkg/command"
	"k8s.io/release/pkg/gcp"
	"k8s.io/release/pkg/git"
	"k8s.io/release/pkg/github"
	"k8s.io/release/pkg/release/regex"
	"k8s.io/release/pkg/testgrid"
	"k8s.io/release/pkg/util"
)

const (
	jobPrefix      = "ci-kubernetes-"
	jobLimit       = 100
	jenkinsLogRoot = "gs://kubernetes-jenkins/logs/"
)

//counterfeiter:generate . githubClient
type githubClient interface {
	GetCommitDate(string) (time.Time, error)
}

type defaultGithubClient struct{}

func (*defaultGithubClient) GetCommitDate(sha string) (date time.Time, err error) {
	commit, _, err := github.New().Client().GetRepoCommit(
		context.Background(),
		git.DefaultGithubOrg,
		git.DefaultGithubRepo,
		sha,
	)
	if err != nil {
		return date, err
	}
	return commit.GetCommit().GetAuthor().GetDate(), err
}

//counterfeiter:generate . jobCacheClient
type jobCacheClient interface {
	GetJobCache(string, bool) (*JobCache, error)
}

type defaultJobCacheClient struct{}

func (*defaultJobCacheClient) GetJobCache(job string, dedup bool) (*JobCache, error) {
	return NewJobCacheClient().GetJobCache(job, dedup)
}

//counterfeiter:generate . testGridClient
type testGridClient interface {
	BlockingTests(string) ([]string, error)
}

type defaultTestGridClient struct{}

func (*defaultTestGridClient) BlockingTests(branch string) (tests []string, err error) {
	return testgrid.New().BlockingTests(branch)
}

type BuildVersionClient struct {
	jobCacheClient jobCacheClient
	githubClient   githubClient
	testGridClient testGridClient
}

// NewBuildVersionClient creates a new build version client
func NewBuildVersionClient() *BuildVersionClient {
	return &BuildVersionClient{
		jobCacheClient: &defaultJobCacheClient{},
		githubClient:   &defaultGithubClient{},
		testGridClient: &defaultTestGridClient{},
	}
}

// SetGithubClient can be used to set the github client
func (b *BuildVersionClient) SetGithubClient(client githubClient) {
	b.githubClient = client
}

// SetJobCacheClient can be used to set the job cache client
func (b *BuildVersionClient) SetJobCacheClient(client jobCacheClient) {
	b.jobCacheClient = client
}

// SetJobCacheClient can be used to set the job cache client
func (b *BuildVersionClient) SetTestGridClient(client testGridClient) {
	b.testGridClient = client
}

// SetBuildVersion returns the build version for a branch
// against a set of blocking CI jobs
//
// branch - The branch name.
// jobPath - A local directory to store the copied cache entries.
// exclude_suites - A list of (greedy) patterns to exclude CI jobs from
// 					checking against the primary job.
func (b *BuildVersionClient) SetBuildVersion(
	branch, jobPath string,
	excludeSuites []string,
) (foundVersion string, err error) {
	logrus.Infof("Setting build version for branch %q", branch)

	if branch == git.Master {
		branch = "release-master"
		logrus.Infof("Changing %s branch to %q", git.Master, branch)
	}

	allJobs, err := b.testGridClient.BlockingTests(branch)
	if err != nil {
		return "", errors.Wrap(err, "getting all test jobs")
	}
	logrus.Infof("Got testgrid jobs for branch %q: %v", branch, allJobs)

	if len(allJobs) == 0 {
		return "", errors.Errorf(
			"No sig-%s-blocking list found in the testgrid config.yaml", branch,
		)
	}

	// Filter out excluded suites
	secondaryJobs := []string{}
	for i, job := range allJobs {
		if i == 0 {
			continue
		}

		excluded := false
		for _, pattern := range excludeSuites {
			matched, err := regexp.MatchString(pattern, job)
			if err != nil {
				return "", errors.Wrapf(err,
					"regex compile failed: %s", pattern,
				)
			}
			excluded = matched
		}

		if !excluded {
			secondaryJobs = append(secondaryJobs, job)
		}
	}

	// Update main cache
	// We dedup the mainJob's list of successful runs and just run through
	// that unique list. We then leave the full state of secondaries below so
	// we have finer granularity at the Jenkin's job level to determine if a
	// build is ok.
	mainJob := allJobs[0]

	mainJobCache, err := b.jobCacheClient.GetJobCache(mainJob, true)
	if err != nil {
		return "", errors.Wrap(err, "building job cache for main job")
	}
	if mainJobCache == nil {
		return "", errors.Errorf("main job cache for job %q is nil", mainJob)
	}

	// Update secondary caches limited by main cache last build number
	secondaryJobCaches := []*JobCache{}
	for _, job := range secondaryJobs {
		cache, err := b.jobCacheClient.GetJobCache(job, true)
		if err != nil {
			return "", errors.Wrapf(err, "building job cache for job: %s", job)
		}
		if cache != nil {
			secondaryJobCaches = append(secondaryJobCaches, cache)
		}
	}

	for i, version := range mainJobCache.Versions {
		sb := strings.Builder{}
		tw := tabwriter.NewWriter(&sb, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "Job\tRun\tBuild\tDate/Status")

		buildVersion := mainJobCache.BuildNumbers[i]

		logrus.Infof("Trying version %q for build %q", version, buildVersion)

		if i > jobLimit {
			return "", errors.Errorf("job count limit of %d exceeded", jobLimit)
		}

		matches := regex.ReleaseAndBuildRegex.FindStringSubmatch(version)
		if matches == nil || len(matches) < 8 {
			return "", errors.Errorf("invalid build version: %v", version)
		}

		buildRun := matches[6]
		buildSHA := matches[7]

		date, err := b.githubClient.GetCommitDate(buildSHA)
		if err != nil {
			return "", errors.Wrapf(err, "retrieve repository commit %s", buildSHA)
		}

		fmt.Fprint(&sb, "(*) Primary job (-) Secondary jobs\n\n")
		fmt.Fprintf(tw,
			"%s\t%s\t%s\t%s\t\n",
			"* "+strings.TrimPrefix(mainJob, jobPrefix),
			buildVersion, buildRun, date,
		)

		type BuildStatus string
		const (
			BuildStatusNotExisting BuildStatus = "NOT EXISTING"
			BuildStatusFailed      BuildStatus = "FAILED"
			BuildStatusSucceeded   BuildStatus = "SUCCEEDED"
		)
		// Check secondaries to ensure that build number is green across all
		success := true
		foundBuildNumber := ""
		for _, secondaryJob := range secondaryJobs {
			status := BuildStatusNotExisting
			for _, secondaryJobCache := range secondaryJobCaches {
				if secondaryJobCache.Name == secondaryJob {
					status = BuildStatusFailed

					for j, secVersion := range secondaryJobCache.Versions {
						matches := regex.ReleaseAndBuildRegex.FindStringSubmatch(secVersion)
						if matches == nil || len(matches) < 8 {
							logrus.Errorf(
								"Invalid build version %s for job %s",
								secVersion, secondaryJob,
							)
							continue
						}

						// Verify that we have the same build number
						if buildRun == matches[6] {
							status = BuildStatusSucceeded
							foundBuildNumber = secondaryJobCache.BuildNumbers[j]
						}
					}
					if status == BuildStatusSucceeded {
						break
					}
				}
			}

			fmt.Fprintf(tw,
				"%s\t%s\t%s\t%s\t\n",
				"- "+strings.TrimPrefix(secondaryJob, jobPrefix),
				foundBuildNumber, buildRun, status,
			)

			if status == BuildStatusFailed {
				success = false
			}
		}

		tw.Flush()
		fmt.Println(sb.String())

		if success {
			return version, nil
		}
	}

	return "", errors.New("unable to find successful build version")
}

type JobCacheClient struct {
	gcpClient gcpClient
}

// NewJobCacheClient creates a new job cache retrieval client
func NewJobCacheClient() *JobCacheClient {
	return &JobCacheClient{
		gcpClient: &defaultGcpClient{},
	}
}

func (j *JobCacheClient) SetClient(client gcpClient) {
	j.gcpClient = client
}

//counterfeiter:generate . gcpClient
type gcpClient interface {
	CopyJobCache(string) string
}

type defaultGcpClient struct{}

func (g *defaultGcpClient) CopyJobCache(job string) (jsonPath string) {
	jsonPath = filepath.Join(os.TempDir(), fmt.Sprintf("job-cache-%s", job))

	remotePath := jenkinsLogRoot + filepath.Join(job, "jobResultsCache.json")
	if err := gcp.GSUtil("-qm", "cp", remotePath, jsonPath); err != nil {
		logrus.Warnf("Skipping unavailable remote path %s", remotePath)
		return ""
	}
	return jsonPath
}

// JobCache is a map of build numbers (key) and their versions (value)
type JobCache struct {
	Name         string
	BuildNumbers []string
	Versions     []string
}

// GetJobCache pulls Jenkins server job cache from GS and resutns a `JobCache`
//
// job - The Jenkins job name.
// dedup -  dedup git's monotonically increasing (describe) build numbers.
func (j *JobCacheClient) GetJobCache(job string, dedup bool) (*JobCache, error) {
	logrus.Infof("Getting %s build results from GCS", job)

	tempJSON := j.gcpClient.CopyJobCache(job)
	if tempJSON == "" {
		return nil, nil
	}

	if !util.Exists(tempJSON) {
		// If there's no file up on job doesn't exist: Skip it.
		logrus.Infof("Skipping non existing job: %s", job)
		return nil, nil
	}
	defer os.RemoveAll(tempJSON)

	// Additional select on .version is because we have so many empty versions
	// for now 2 passes. First pass sorts by buildnumber, second builds the
	// dictionary.
	out, err := command.New("jq", "-r",
		`.[] | `+
			`select(.result == "SUCCESS") | `+
			`select(.version != null) | `+
			`[.version,.buildnumber] | "\(.[0]|rtrimstr("\n")) \(.[1])"`,
		tempJSON,
	).Pipe("sort", "-rn", "-k2,2").RunSilentSuccessOutput()
	if err != nil {
		return nil, errors.Wrap(err, "filtering job cache")
	}

	lastVersion := ""
	res := &JobCache{Name: job}
	scanner := bufio.NewScanner(strings.NewReader(out.OutputTrimNL()))
	for scanner.Scan() {
		split := strings.Split(scanner.Text(), " ")
		if len(split) != 2 {
			return nil, errors.Wrapf(err,
				"unexpected string in job results cache %s: %s",
				tempJSON, scanner.Text(),
			)
		}

		version := split[0]
		buildNumber := split[1]

		if dedup && version == lastVersion {
			continue
		}
		lastVersion = version

		if buildNumber != "" && version != "" {
			res.BuildNumbers = append(res.BuildNumbers, buildNumber)
			res.Versions = append(res.Versions, version)
		}
	}

	return res, nil
}
