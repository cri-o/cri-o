# CRI-O Automated Patch Releases

Automated Patch Releases provide an easy way of packaging and releasing a new version
of CRI-O
This involves the use of Golang Scripts and Github Actions

```mermaid
    flowchart TD
      A[Start: patch-release.yml Workflow] --> B[Create version bump intent prs]
      B --> C[Maintainers Review PRs]
      C -->|Decides Against Merges| E[Rebases on main branch] --> C
      C -->|Decides to Merge PRs| D[Run: tag-reconciler.yml Workflow]
      D --> F[Release Job: Build Packages, Create Release Notes, etc.]
      F --> G[End]
```

## Patch Release

The `.github/workflows/patch-release.yml` runs the golang script
`scripts/tag-reconciler/tag-reconciler.go`

```mermaid
    flowchart TD
        A[Start patch-release Github action] --> B[Run Release Script]
        --> C[Get latest Minor versions from release Branches]
        C --> D[Bump up version in specific release branch]
           D -->E[Create PR with new version]
        E--> F[End]
```

## Pushing New Version tags

To push the new version tags and cut the release, the
`.github/workflows/tag-reconciler.yml` runs the golang script
`scripts/tag-reconciler/tag-reconciler.go`

This will inturn run the `.github/workflows/test.yml` which will build and
package the latest versions for CRI-O

```mermaid
    flowchart TD
        A[Start tag-reconciler Github action] --> B[Run Tag-Reconciler Script]
        --> C[Check for latest minor versions Tags on remote]
        C --> D{Tag Exists?}
        D -->|Yes| E[No-op] --> H
        D -->|No| F[Tag and Push Tag to Remote]
        F--> G[Manually Trigger Test Workflow]
        G--> H[End]
```
