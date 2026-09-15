# Releasing CRI-O

Maintainers release CRI-O by merging version updates. Automation then creates
tags, builds binaries, and publishes release notes. Packaging runs separately.
See the [CI architecture](scripts/ci.md) for implementation details.

## Before starting

Agree on the release contents, merge the intended changes and backports, and
check the target branch's CI and release notes. The commands below require an
authenticated GitHub CLI account with permission to run the relevant workflows.
Replace `1.y.z` and `1.y` with the actual release and minor versions.

## Patch release

1. Find the `Bump version to 1.y.z` PR for the target `release-1.y` branch.
   The [patch release workflow](.github/workflows/patch-release.yml) prepares
   these on the first of each month. To prepare them sooner:

   ```shell
   gh workflow run patch-release.yml --repo cri-o/cri-o --ref main
   ```

   This processes every supported minor and rebases and force-pushes any existing
   `release-1.y.z` bump branch, even if its PR was closed. Delete the branch when
   closing an unmerged bump PR so the next run can open a new PR.

2. Review the version changes and required CI checks, then merge the PR when
   ready to publish.
3. The [tag reconciler](.github/workflows/tag-reconciler.yml) runs daily at
   00:00 UTC. It creates the missing version tag at the release branch's HEAD
   and starts the `test` workflow. To run it sooner:

   ```shell
   gh workflow run tag-reconciler.yml --repo cri-o/cri-o --ref main
   ```

Both workflows process **all minor versions listed in `ReleaseMinorVersions`
on `main`**. Merging a bump enables automatic tagging; changes merged before
the reconciler runs can also enter the release.

## Minor release

1. Create the upstream `release-1.y` branch from `main` when preparing the new
   minor, if it does not exist. Arrange release branch CI jobs with the admins.
   Keep the branch excluded from the release branch ruleset until `v1.y.0`
   exists: the forwarding job pushes merges directly, which the ruleset rejects.
2. Ensure packaging has OBS projects for the new minor. If missing, run its
   [add-version workflow](https://github.com/cri-o/packaging/blob/main/.github/workflows/add-version.yml),
   then review and merge the generated documentation PR:

   ```shell
   gh workflow run add-version.yml --repo cri-o/packaging --ref main -f version=v1.y
   ```

3. When ready to publish, dispatch the [branch forwarding job](.github/workflows/release-branch-forward.yml)
   and wait for it to finish. Review the resulting `release-1.y` contents and CI
   before enabling tagging:

   ```shell
   gh workflow run release-branch-forward.yml --repo cri-o/cri-o --ref main
   ```

4. Add `1.y` to `ReleaseMinorVersions` on `main` and update `supported versions`
   in `dependencies.yaml` to match. Remove versions no longer supported under the
   [compatibility policy](README.md#compatibility-matrix-cri-o--kubernetes).
   Never list a minor before its release branch exists: the release and tag
   scripts stop at the missing branch, skipping every later minor. Merge the PR
   after CI passes.
5. Dispatch the tag reconciler immediately after merging the PR and check that
   it creates `v1.y.0` at the reviewed release branch HEAD:

   ```shell
   gh workflow run tag-reconciler.yml --repo cri-o/cri-o --ref main
   ```

6. Once `v1.y.0` exists, add the branch to the release branch ruleset. After
   the release is published, update the `development version` in
   `dependencies.yaml` and its referenced files to the next minor.

The order matters:

- Bumping `main` to `1.(y+1).0` before `v1.y.0` exists lets the forwarding job
  merge that version into `release-1.y`. The reconciler would then tag
  `v1.(y+1).0` on the wrong branch.
- Creating `release-1.(y+1)` before `v1.y.0` exists makes the forwarding job
  switch to the newer branch and stop updating `release-1.y`.

## Verify the release

```shell
RELEASE_TAG=v1.y.z
gh run list --repo cri-o/cri-o --workflow test.yml \
  --branch "$RELEASE_TAG" --limit 5
gh release view "$RELEASE_TAG" --repo cri-o/cri-o
```

1. Confirm the tag points to the intended commit and its `test` workflow passes,
   including binary uploads. Review the GitHub Release notes.
2. Check that `https://storage.googleapis.com/cri-o/latest-1.y.txt` contains
   the new tag. Packaging uses this marker to discover stable releases.
   To start packaging before its daily schedule, dispatch the
   [OBS workflow](https://github.com/cri-o/packaging/blob/main/.github/workflows/obs.yml)
   after the marker updates:

   ```shell
   gh workflow run obs.yml --repo cri-o/packaging --ref main -f revision="$RELEASE_TAG"
   ```

3. Check the [packaging pipeline](https://github.com/cri-o/packaging/blob/main/docs/ci.md)
   completes and RPM/DEB packages and release bundles are available. Check the
   download links and follow the signature verification instructions in the
   release notes.

The GitHub Release can appear before binaries and packages are ready. Its job
depends only on release notes; packaging discovers updates on a separate daily
schedule.

## If something fails

Inspect the failed run in GitHub Actions, fix the cause, and rerun the failed
jobs. For missing bump PRs, check the `patch release` workflow; for missing
tags, check the `tag reconciler` workflow and that every branch listed in
`ReleaseMinorVersions` exists.

If the tag exists but its `test` workflow never started, dispatch it directly;
the tag reconciler skips existing tags:

```shell
gh workflow run test.yml --repo cri-o/cri-o --ref "$RELEASE_TAG"
```

For missing packages, check the version marker and packaging pipeline. Fix a
published code defect with a new patch release; do not move the published tag.
