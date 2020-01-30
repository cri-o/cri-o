# release-tool

[![Build Status](https://travis-ci.org/containerd/release-tool.svg?branch=master)](https://travis-ci.org/containerd/release-tool)

A release tool for generating detailed release notes

The `release-tool` utility is maintained as a separate repository
to reduce duplication across the various release branches of the main
`containerd/containerd` repository where it is used to cut release
notes and project data to aid release engineers.

The release tool is designed to be project agnostic and useful for
any project.

## How to use

Run `release-tool` from the project root directory with the release commit
checked out. Prepare and provide a template file to generate the release notes,
it is recommended that each release have its own file containing the release
notes.

### Command line

Use the following command to generate release notes for v1.0.0 using the
recommended location for the template. The release notes will be outputted
to stdout.

```
$ release-tool -l -d -n -t v1.0.0 ./releases/v1.0.0.toml
```

This command uses the `-n`, or dry run mode, option to generate the release notes
to stdout rather than create the release tag.

Also `-l` converts the changelog commits to markdown style links to Github.

To create the tag, use `git tag` with the output from the previous command

```
git tag --cleanup=whitespace -s v1.0.0 -F /tmp/release-tool-ouput
```

Use `--cleanup=whitespace` to prevent the `git tag` command from treating
`#` generated for the markdown as comments.

NOTE: It is recommended to use dry run mode, review the output, then create
the tag in git. Currently the tool does not support creating the tag, so
`-n` is required.

### Template

The template file uses TOML, here is a basic example

```
# commit to be tagged for new release
commit = "HEAD"

# project_name is used to refer to the project in the notes
project_name = "release tool"

# github_repo is the github project, only github is currently supported
github_repo = "containerd/release-tool"

# match_deps is a pattern to determine which dependencies should be included
# as part of this release. The changelog will also include changes for these
# dependencies based on the change in the dependency's version.
match_deps = "^github.com/(containerd/[a-zA-Z0-9-]+)$"

# previous release of this project for determining changes
previous = "v0.9.0"

# pre_release is whether to include a disclaimer about being a pre-release
pre_release = false

# preface is the description of the release which precedes the author list
# and changelog. This description could include highlights as well as any
# description of changes. Use markdown formatting.
preface = """\
This is the first release"""
```

## Project details

release-tool is a containerd sub-project, licensed under the [Apache 2.0 license](./LICENSE).
As a containerd sub-project, you will find the:
 * [Project governance](https://github.com/containerd/project/blob/master/GOVERNANCE.md),
 * [Maintainers](https://github.com/containerd/project/blob/master/MAINTAINERS),
 * and [Contributing guidelines](https://github.com/containerd/project/blob/master/CONTRIBUTING.md)

information in our [`containerd/project`](https://github.com/containerd/project) repository.
