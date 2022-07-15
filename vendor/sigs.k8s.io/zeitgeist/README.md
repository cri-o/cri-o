# Zeitgeist

([/ˈzaɪtɡaɪst/](https://en.wikipedia.org/wiki/Help:IPA/English)) is a language-agnostic dependency checker that keeps track of external dependencies across your project and ensure they're up-to-date.

[![Go Report Card](https://goreportcard.com/badge/sigs.k8s.io/zeitgeist)](https://goreportcard.com/report/sigs.k8s.io/zeitgeist)
[![GoDoc](https://godoc.org/sigs.k8s.io/zeitgeist?status.svg)](https://godoc.org/sigs.k8s.io/zeitgeist)

- [Rationale](#rationale)
- [What is Zeitgeist](#what-is-zeitgeist)
- [Installation](#installation)
- [Supported upstreams](#supported-upstreams)
- [Supported version schemes](#supported-version-schemes)
- [When is Zeitgeist _not_ suggested](#when-is-zeitgeist-not-suggested)
- [Naming](#naming)
- [Releasing](#releasing)
- [Credit](#credit)
- [To do](#to-do)
- [Community, discussion, contribution, and support](#community-discussion-contribution-and-support)
  - [Code of conduct](#code-of-conduct)

## Rationale

More and more projects nowadays have external dependencies, and the best way to ensure stability and reproducibility is to pin these dependencies to a specific version.

However, this leads to a new problem: the world changes around us, and new versions of these dependencies are released _all the time_.

For a simple project with a couple of dependencies, a team can usually keep up to speed by following mailing lists or Slack channels, but for larger projects this becomes a daunting task.

This problem is pretty much solved by package managers in specific programming languages (see [_When is Zeitgeist _not_ suggested_](#when-is-zeitgeist-not-suggested) below), but it remains a big issue when:

- Your project relies on packages outside your programming language of choice
- You declare infrastructure-as-code, where the "build step" is usually bespoke and dependencies are managed manually
- Dependencies do not belong in a classical "package manager" (e.g. AMI images)

## What is Zeitgeist

Zeitgeist is a tool that takes a configuration file with a list of dependencies, and ensures that:

- These dependencies versions are consistent within your project
- These dependencies are up-to-date

A Zeitgeist configuration file (usually `dependencies.yaml`) is a list of _dependencies_, referenced in files, which may or may not have an _upstream_:

```yaml
dependencies:
- name: terraform
  version: 0.12.3
  upstream:
    flavour: github
    url: hashicorp/terraform
  refPaths:
  - path: helper-image/Dockerfile
    match: TERRAFORM_VERSION
  - path: .github/actions/run.yaml
    match: terraform
- name: aws-eks-ami
  version: ami-09bbefc07310f7914
  scheme: random
  upstream:
    flavour: ami
    owner: amazon
    name: "amazon-eks-node-1.21-*"
  refPaths:
  - path: clusters.yaml
    match: workers_ami
```

Use `zeitgeist validate` to verify that the dependency version is correct in all files referenced in _`refPaths`_, and whether any newer version is available `upstream`:

![zeigeist validate](/docs/validate.png)

## Installation

You will need to build Zeitgeist from source (for now at least!).

Clone the repository and run `go build` will give you the `zeitgeist` binary:
```bash
git clone https://github.com/kubernetes-sigs/zeitgeist.git
cd zeitgeist/
go build
```

## Supported upstreams

**Github**

The [Github upstream](upstream/github.go) looks at [releases](https://docs.github.com/en/github/administering-a-repository/releasing-projects-on-github/about-releases) from a Github repository.

Example:

```yaml
dependencies:
- name: terraform
  version: 0.15.3
  upstream:
    flavour: github
    url: hashicorp/terraform
  refPaths:
  - path: testdata/zeitgeist-example/a-config-file.yaml
    match: terraform_version
```

For API access, you will need to set the following env var:

```console
export GITHUB_TOKEN=<YOUR_GITHUB_TOKEN>
```

**Helm**

The [Helm upstream](upstream/helm.go) looks at [chart versions](https://helm.sh/docs/topics/charts/) from a Helm repository.

Example:

```yaml
dependencies:
- name: linkerd
  version: 2.10.0
  upstream:
    flavour: helm
    repo: https://helm.linkerd.io/stable
    chart: linkerd2
  refPaths:
  - path: testdata/zeitgeist-example/a-config-file.yaml
    match: linkerd-
```

**Gitlab**

The [Gitlab upstream](upstream/gitlab.go) looks at [releases](https://docs.gitlab.com/ee/user/project/releases/) from a Gitlab repository.

Example:
```yaml
dependencies:
- name: gitlab-agent
  version: v14.0.1
  upstream:
    flavour: gitlab
    url: gitlab-org/cluster-integration/gitlab-agent
  refPaths:
  - path: testdata/zeitgeist-example/a-config-file.yaml
    match: GL_VERSION
```

The Gitlab API requires authentication, so you will need to set an [Access Token](https://docs.gitlab.com/ee/user/profile/personal_access_tokens.html).

When using the public `GitLab` instance at https://gitlab.com/ :

```console
export GITLAB_TOKEN=<YOUR_GITLAB_TOKEN>
```

When using a self-hosted `GitLab` instance, ie. https://my-gitlab.company.com/ :

```console
export GITLAB_PRIVATE_TOKEN=<YOUR_GITLAB_PRIVATE_TOKEN>
```

You can use in the `dependencies.yaml` both public and private GitLab instances. The only limitation today is that you can only use one private GitLab at the moment.

**AMI**

The [AMI upstream](upstream/ami.go) looks at [Amazon Machine Images](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/AMIs.html) from AWS.

Example:
```yaml
dependencies:
- name: aws-eks-ami
  version: ami-09bbefc07310f7914
  scheme: random
  upstream:
    flavour: ami
    owner: amazon
    name: "amazon-eks-node-1.21-*"
  refPaths:
  - path: testdata/zeitgeist-example/a-config-file.yaml
    match: zeitgeist:aws-eks-ami
```

It uses the standard [go AWS SDK authentication methods](https://docs.aws.amazon.com/sdk-for-go/v1/developer-guide/configuring-sdk.html) for authentication and authorization, so it can be used for both public & private AMIs.

**Container**

The [container upstream](upstream/container.go) talks to [OCI container registries](https://github.com/opencontainers/distribution-spec), such as Docker registries.

Example:
```yaml
dependencies:
- name: docker-in-docker
  version: 19.03.15
  upstream:
    flavour: container
    registry: hub.docker.io/docker
  refPaths:
  - path: testdata/zeitgeist-example/a-config-file.yaml
    match: docker-dind
```

If you're connecting to a private registry, you will need to set the following env vars:

```console
export REGISTRY_USERNAME=<YOUR_REGISTRY_USERNAME>
export REGISTRY_USER_PASSWORD=<YOUR_REGISTRY_TOKEN_PASSWORD>
```

**EKS**

The [EKS](upstream/eks.go) checks for updates to [Elastic Kubernetes Service](https://aws.amazon.com/eks/), Amazon's managed Kubernetes offering.

Example:
```yaml
dependencies:
- name: eks
  version: 1.13.0
  upstream:
    flavour: eks
  refPaths:
  - path: testdata/zeitgeist-example/a-config-file.yaml
    match: eks
```

## Supported version schemes

Zeitgeist supports several version schemes:
- `semver`: [SemVer](https://semver.org/) v2, default
- `alpha`: alphanumeric ordering. A newer version is considered an update if it's alphanumerically higher, e.g. "release-d" is higher "release-c" but "release-b-update-1" wouldn't be higher than "release-c".
- `random`: any newer version is considered an update. Useful for UUID or hash-based versioning.

See the [full documentation](https://godoc.org/sigs.k8s.io/zeitgeist/dependencies#Dependency) to see configuration options.

## When is Zeitgeist _not_ suggested

While Zeitgeist aims to be a great cross-language solution for tracking external dependencies, it won't be as well integrated as native package managers.

If your project is mainly written in one single language with a well-known and supported package manager (e.g. [`npm`](https://www.npmjs.com/), [`maven`](https://maven.apache.org/), [`rubygems`](https://rubygems.org/), [`pip`](https://pypi.org/project/pip/), [`cargo`](https://crates.io/)...), you definitely should use your package manager rather than Zeitgeist.

## Naming

[Zeitgeist](https://en.wikipedia.org/wiki/Zeitgeist), a German compound word, can be translated as "spirit of the times" and refers to _a schema of fashions or fads which prescribes what is considered to be acceptable or tasteful for an era_.

## Releasing

Releases are generated with [goreleaser](https://goreleaser.com/).

```bash
git tag v0.0.0 # Use the correct version here
git push --tags
export GPG_TTY=$(tty)
goreleaser release --rm-dist
```

## Credit

Zeitgeist is inspired by [Kubernetes' script to manage external dependencies](https://groups.google.com/forum/?pli=1#!topic/kubernetes-dev/cTaYyb1a18I) and extended to include checking with upstream sources to ensure dependencies are up-to-date.

## To do

- [x] Find a good name for the project
- [x] Support `helm` upstream
- [x] Support `eks` upstream
- [x] Support `ami` upstream
- [x] support `docker` upstream
- [x] Cleanly separate various upstreams to make it easy to add new upstreams
- [x] Implement non-semver support (e.g. for AMI, but also for classic releases)
- [x] Write good docs :)
- [x] Write good tests!
- [x] Externalise the project into its own repo
- [x] Generate releases
- [x] Automate release generation from a tag

## Community, discussion, contribution, and support

Learn how to engage with the Kubernetes community on the [community page](http://kubernetes.io/community/).

You can reach the maintainers of this project at:

- [`#release-management`](https://kubernetes.slack.com/archives/CJH2GBF7Y) channel on [Kubernetes Slack](http://slack.k8s.io/)
- [Mailing List](https://groups.google.com/forum/#!forum/kubernetes-sig-release)

### Code of conduct

Participation in the Kubernetes community is governed by the [Kubernetes Code of Conduct](code-of-conduct.md).
