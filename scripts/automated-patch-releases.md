# CRI-O Automated Patch Releases

Automated Patch Releases provide an easy way of packaging and releasing a new version
of CRI-O. This involves the use of Golang scripts and Github Actions and follows
the outlined flow:

```mermaid
    flowchart TD
      A[Start: patch-release.yml Workflow] --> B[Create version bump PRs]
      B --> C[Maintainers Review PRs] -->
|Decides Against Merges| E[Rebases on main branch on the first day of every month]
      --> C
      C -->|Decides to Merge PRs| D[Run: tag-reconciler.yml Workflow everyday]
      D --> F[Release Job: Build Packages, Create Release Notes, etc.]
      F --> G[End]
      click A href "https://github.com/cri-o/cri-o/actions/workflows/patch-release.yml"
      click D href "https://github.com/cri-o/cri-o/actions/workflows/tag-reconciler.yml"
```

## Patch Release

The
[patch-release.yml](https://github.com/cri-o/cri-o/actions/workflows/patch-release.yml)
Workflow runs the golang script
[release.go](https://github.com/cri-o/cri-o/blob/main/scripts/release/release.go)

```mermaid
    flowchart TD
        A[Start patch-release.yml Workflow] --> B[Run release.go Script]
        --> C[Get latest Minor versions from release Branches]
        C --> D[Bump up version in specific release branch]
           D -->E[Create PR with new version]
        E--> F[End]
        click A href "https://github.com/cri-o/cri-o/actions/workflows/patch-release.yml"
        click B href "https://github.com/cri-o/cri-o/blob/main/scripts/release/release.go"
```

## Pushing New Version tags

To push the new version tags and cut the release, the
[tag-reconciler.yml](https://github.com/cri-o/cri-o/actions/workflows/tag-reconciler.yml)
Workflow runs the golang script
[tag-reconciler.go](https://github.com/cri-o/cri-o/blob/main/scripts/tag-reconciler/tag-reconciler.go)

This will inturn run the
[test.yml](https://github.com/cri-o/cri-o/actions/workflows/test.yml)
which will build and package the latest versions for CRI-O

```mermaid
    flowchart TD
        A[Start tag-reconciler.yml Workflow]
    --> B[Run tag-reconciler.go Script]
        --> C[Check for latest minor versions tags on remote]
        C --> D{Tag Exists?}
        D -->|Yes| E[No-op] --> H
        D -->|No| F[Tag and Push Tag to Remote]
        F--> G[Manually Trigger Test Workflow]
        G--> H[End]
        click A href "https://github.com/cri-o/cri-o/actions/workflows/tag-reconciler.yml"
        click B href "https://github.com/cri-o/cri-o/blob/main/scripts/tag-reconciler/tag-reconciler.go"
        click G href "https://github.com/cri-o/cri-o/actions/workflows/test.yml"
```
