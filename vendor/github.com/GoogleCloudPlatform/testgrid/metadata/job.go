/*
Copyright 2019 The Kubernetes Authors.

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

package metadata

import (
	"strings"
)

// Started holds the started.json values of the build.
type Started struct {
	// Timestamp is UTC epoch seconds when the job started.
	Timestamp int64 `json:"timestamp"` // epoch seconds
	// Node holds the name of the machine that ran the job.
	Node string `json:"node,omitempty"`

	// Consider whether to keep the following:

	// Pull holds the PR number the primary repo is testing
	Pull string `json:"pull,omitempty"`
	// Repos holds the RepoVersion of all commits checked out.
	Repos      map[string]string `json:"repos,omitempty"` // {repo: branch_or_pull} map
	RepoCommit string            `json:"repo-commit,omitempty"`

	// Deprecated fields:

	// Metadata is deprecated, add to finished.json
	Metadata Metadata `json:"metadata,omitempty"` // TODO(fejta): remove

	// Use RepoCommit
	DeprecatedJobVersion  string `json:"job-version,omitempty"`  // TODO(fejta): remove
	DeprecatedRepoVersion string `json:"repo-version,omitempty"` // TODO(fejta): remove

}

const (
	// JobVersion is the metadata key that overrides repo-commit in Started when set.
	JobVersion = "job-version"
)

// Finished holds the finished.json values of the build
type Finished struct {
	// Timestamp is UTC epoch seconds when the job finished.
	// An empty value indicates an incomplete job.
	Timestamp *int64 `json:"timestamp,omitempty"`
	// Passed is true when the job completes successfully.
	Passed *bool `json:"passed"`
	// Metadata holds data computed by the job at runtime.
	// For example, the version of a binary downloaded at runtime
	// The JobVersion key overrides the auto-version set in Started.
	Metadata Metadata `json:"metadata,omitempty"`

	// Consider whether to keep the following:

	// Deprecated fields:

	// Result is deprecated, use Passed.
	Result string `json:"result,omitempty"` // TODO(fejta): remove

	// Use Metadata[JobVersion] or Started.RepoCommit
	DeprecatedJobVersion  string `json:"job-version,omitempty"`  // TODO(fejta): remove
	DeprecatedRevision    string `json:"revision,omitempty"`     // TODO(fejta): remove
	DeprecatedRepoVersion string `json:"repo-version,omitempty"` // TODO(fejta): remove
}

// Metadata holds the finished.json values in the metadata key.
//
// Metadata values can either be string or string map of strings
//
// TODO(fejta): figure out which of these we want and document them
// Special values: infra-commit, repos, repo, repo-commit, links, others
type Metadata map[string]interface{}

// String returns the name key if its value is a string, and true if the key is present.
func (m Metadata) String(name string) (*string, bool) {
	if v, ok := m[name]; !ok {
		return nil, false
	} else if t, good := v.(string); !good {
		return nil, true
	} else {
		return &t, true
	}
}

// Meta returns the name key if its value is a child object, and true if they key is present.
func (m Metadata) Meta(name string) (*Metadata, bool) {
	if v, ok := m[name]; !ok {
		return nil, false
	} else if t, good := v.(Metadata); good {
		return &t, true
	} else if t, good := v.(map[string]interface{}); good {
		child := Metadata(t)
		return &child, true
	}
	return nil, true
}

// Keys returns an array of the keys of all valid Metadata values.
func (m Metadata) Keys() []string {
	ka := make([]string, 0, len(m))
	for k := range m {
		if _, ok := m.Meta(k); ok {
			ka = append(ka, k)
		}
	}
	return ka
}

// Strings returns the submap of values in the map that are strings.
func (m Metadata) Strings() map[string]string {
	bm := map[string]string{}
	for k, v := range m {
		if s, ok := v.(string); ok {
			bm[k] = s
		}
		// TODO(fejta): handle sub items
	}
	return bm
}

// firstFilled returns the first non-empty option or else def.
func firstFilled(def string, options ...string) string {
	for _, o := range options {
		if o != "" {
			return o
		}
	}
	return def
}

const missing = "missing"

// Version extracts the job's custom version or else the checked out repo commit.
func Version(started Started, finished Finished) string {
	// TODO(fejta): started.RepoCommit, finished.Metadata.String(JobVersion)
	meta := func(key string) string {
		if finished.Metadata == nil {
			return ""
		}
		v, ok := finished.Metadata.String(key)
		if !ok {
			return ""
		}
		return *v
	}

	val := firstFilled(
		missing,
		finished.DeprecatedJobVersion, started.DeprecatedJobVersion,
		started.DeprecatedRepoVersion, finished.DeprecatedRepoVersion,
		meta("revision"), meta("repo-commit"),
		meta(JobVersion), started.RepoCommit, // TODO(fejta): remove others
	)
	parts := strings.SplitN(val, "+", 2)
	val = parts[len(parts)-1]
	if n := len(val); n > 9 {
		return val[:9]
	}
	return val
}

// SetVersion ensures that the repoCommit and jobVersion are set appropriately.
func SetVersion(started *Started, finished *Finished, repoCommit, jobVersion string) {
	if started != nil && repoCommit != "" {
		started.DeprecatedRepoVersion = repoCommit // TODO(fejta): pump this
		started.RepoCommit = repoCommit
	}
	if finished != nil && jobVersion != "" {
		if finished.Metadata == nil {
			finished.Metadata = Metadata{}
		}
		finished.Metadata["job-version"] = jobVersion
		finished.DeprecatedJobVersion = jobVersion
	}
}
