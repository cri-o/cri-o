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

1. Create the upstream `release-1.y` branch from the reviewed `1.y.0` commit on
   `main` if it does not exist. Arrange branch protection and release branch CI
   jobs with the admins.
2. Ensure packaging has OBS projects for the new minor. If missing, run its
   [add-version workflow](https://github.com/cri-o/packaging/blob/main/.github/workflows/add-version.yml),
   then review and merge the generated documentation PR:

   ```shell
   gh workflow run add-version.yml --repo cri-o/packaging --ref main -f version=v1.y
   ```

3. When ready to publish, add `1.y` to `ReleaseMinorVersions` on `main` and
   update `supported versions` in `dependencies.yaml` to match. Remove versions
   that are no longer supported under the
   [compatibility policy](README.md#compatibility-matrix-cri-o--kubernetes).
   Ensure every listed release branch exists and merge the PR after CI passes.
4. Wait for the tag reconciler or run it using the command above. If the minor
   was already listed, check whether `v1.y.0` has already been created.
5. After `v1.y.0` is published, bump `main` to the next development minor
   through a PR. Leave the release branch on `1.y.0` until its next patch
   release.

Until a tag exists for the new minor, the daily
[branch forwarding job](.github/workflows/release-branch-forward.yml) merges
`main` into the newest release branch. Wait until tagging is complete before
bumping `main` to the next minor. A minor already listed in
`ReleaseMinorVersions` becomes eligible for tagging as soon as its branch exists.

## Verify the release

```shell
RELEASE_TAG=v1.y.z
RELEASE_SHA=$(gh api -X GET "repos/cri-o/cri-o/commits/$RELEASE_TAG" --jq .sha)
gh run list --repo cri-o/cri-o --workflow test.yml \
  --commit "$RELEASE_SHA" --limit 5
gh release view "$RELEASE_TAG" --repo cri-o/cri-o
```

1. Confirm the tag points to the intended commit and its `test` workflow passes,
   including binary uploads. Review the GitHub Release notes.
2. Check that `https://storage.googleapis.com/cri-o/latest-1.y.txt` contains
   the new tag. Packaging uses this marker to discover stable releases.
3. Check the [packaging pipeline](https://github.com/cri-o/packaging/blob/main/docs/ci.md)
   completes and RPM/DEB packages and release bundles are available. Check the
   download links and follow the signature verification instructions in the
   release notes.

The GitHub Release can appear before binaries and packages are ready. Its job
depends only on release notes; packaging discovers updates on a separate daily
schedule.

## If something fails

Inspect the failed run in GitHub Actions, fix the cause, and rerun the failed
jobs. For missing PRs or tags, check the release workflow logs and that every
branch listed in `ReleaseMinorVersions` exists.

If the tag exists but its `test` workflow never started, dispatch it directly;
the tag reconciler skips existing tags:

```shell
gh workflow run test.yml --repo cri-o/cri-o --ref "$RELEASE_TAG"
```

For missing packages, check the version marker and packaging pipeline. Fix a
published code defect with a new patch release; do not move the published tag.
