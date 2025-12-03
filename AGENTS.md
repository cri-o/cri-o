# CRI-O Project Context for AI Agents

## Project Overview

CRI-O is an OCI-based implementation of the Kubernetes Container Runtime
Interface (CRI). It provides a lightweight, production-ready container runtime
specifically designed for Kubernetes.

**Key Facts:**

- Primary Language: Go (see `go.mod` for version requirement)
- License: Apache 2.0
- Repository: <https://github.com/cri-o/cri-o>
- Release Cycle: Follows Kubernetes release cycles (n-2 version skew policy)
- Main Branch: `main`
- Current Version: See `internal/version/version.go`
- Supported Versions: See `dependencies.yaml` and
  `internal/version/version.go`

**Project Scope:**

- ✅ Implements Kubernetes CRI using OCI runtimes
- ✅ Manages container images and storage
- ✅ Handles container lifecycle and networking
- ✅ Provides monitoring/logging for Kubernetes
- ❌ Does NOT build, sign, or push images
- ❌ Does NOT provide stable end-user CLI tools

## Critical Workflow Rules (from user's preferences)

**Git Workflow:**

- Commit with `-s` (signed-off-by) - all commits are DCO compliant
- Prefer single commit per branch for simple features/fixes - amend instead of
  new commits: `git commit --amend -s`
- For complex features, use multiple logical commits when it aids review
- Force push after amending: `git push --force-with-lease`
- Update commit message to reflect ALL changes when amending
- Keep docs, commit messages, and PR descriptions synchronized
- DON'T link issues/PRs in commit messages or comment on GitHub unless asked

**GitHub Templates:**

- Use issue templates (`.github/ISSUE_TEMPLATE/`: bug-report, failing-test,
  org_member, reviewer)
- Use PR template (`.github/PULL_REQUEST_TEMPLATE.md`: /kind label, what/why,
  fixes, release notes)

**Documentation:**

- Keep in sync with code; edit `.md` source files (not generated)
- Balance clarity with brevity - focus on "why" not "what"
- Update `dependencies.yaml` when changing tool versions; verify with
  `make verify-dependencies`
- When adding/changing features, update related repositories:
  - Website: <https://cri-o.io> (source: <https://github.com/cri-o/cri-o.io>)
  - Packaging: <https://github.com/cri-o/packaging>

## Repository Structure

```text
/cmd/crio/          - Main daemon entry point
/internal/          - Internal packages (~28 packages)
  ├── config/       - Configuration (seccomp, nri, apparmor, rdt, etc.)
  ├── lib/          - Core container/sandbox management
  ├── oci/          - OCI runtime interface
  ├── storage/      - Image and storage layer
  └── ...           - See ls internal/ for complete list
/pkg/               - Public API packages (annotations, config, types)
/server/            - CRI gRPC server implementation
/test/              - Integration tests (BATS framework)
/docs/              - Documentation and man pages
/hack/              - Build and development scripts
/contrib/           - Systemd units, CNI configs, metrics exporter
/pinns/             - Pin namespace utility (C program)
```

**Key Files:**

- `go.mod` - Go version and dependencies
- `Makefile` - Build system (see `make help`)
- `dependencies.yaml` - **Critical** version tracking for all tools
- `internal/version/version.go` - Version constants

## Build System

```bash
# Build (avoid pkg-config dependency with openpgp tag)
make BUILDTAGS="containers_image_openpgp containers_image_ostree_stub" \
  all test-binaries

# Test
make testunit                          # Unit tests (Ginkgo)
sudo make localintegration             # Integration tests (BATS)
sudo -E ./test/test_runner.sh version.bats  # Single test
sudo -E ./test/test_runner.sh ctr.bats -f 'pattern'  # Filter
```

**Key Makefile Targets:** `make help` for full list

- `make` / `make all` - Build binaries and docs
- `make testunit` / `make localintegration` - Run tests
- `make lint` - Run golangci-lint (required before PR)
- `make prettier` / `make verify-prettier` - Format/verify markdown, YAML,
  JSON (required before PR)
- `make verify-mdtoc` - Verify TOC in markdown files
- `make mockgen` - Regenerate mocks
- `make verify-dependencies` - Verify dependencies.yaml

**Build Variables:** `BUILDTAGS`, `DEBUG=1`, `PREFIX` (default: /usr/local)

## Testing

**Unit Tests (Ginkgo):** `*_test.go` files, run with `make testunit`, coverage
in `build/coverage/`

**Integration Tests (BATS):** `test/*.bats` files, run with
`sudo -E ./test/test_runner.sh [file.bats] [-f 'pattern']`

**Mocks:** Regenerate with `make mockgen`, located in `test/mocks/*/`,
committed to git

## Configuration

**Files:** `/etc/crio/crio.conf` (main), `/etc/crio/crio.conf.d/*.conf`
(drop-ins), `/etc/containers/{registries.conf,policy.json,storage.conf}`

**Generate/Validate:** `./bin/crio config [--validate]`

**Paths:** Socket `/var/run/crio/crio.sock`, Storage
`/var/lib/containers/storage`, CNI `/etc/cni/net.d/`, Hooks
`/usr/share/containers/oci/hooks.d`

## Development Patterns

**Code Style:** Interface-based design, dependency injection, context.Context
propagation, fmt.Errorf with %w, logrus with fields, comment on "why" not
"what"

**File Names:** `*_{linux,freebsd}.go` (platform), `*_test.go` (unit),
`*.bats` (integration), `*.md` (docs/man pages)

**Adding a Feature:**

1. Check if needs Kubernetes KEP; create issue (use template)
2. Implement + include tests (unit & integration); update docs/man pages
3. Update `dependencies.yaml` if changing tools
4. Run linting: `make lint && make prettier && make verify-mdtoc`
5. Use PR template: specify `/kind`, what/why, fixes, release notes
6. Keep commit message and PR description synchronized as code evolves

## Debugging

**Logs:** Set `log_level = "debug"` in config (levels: fatal, panic, error,
warn, info, debug, trace)

**CLI:** `crio [config|status info|status containers|version|wipe|check]`

**Signals:** SIGINT/TERM (shutdown), SIGUSR1 (goroutine dump), SIGUSR2 (GC),
SIGHUP (reload hooks)

**HTTP API:**
`curl --unix-socket /var/run/crio/crio.sock \
  http://localhost/{info,config,containers/:id}`

## Common Pitfalls

- **pkg-config missing**: Use
  `BUILDTAGS="containers_image_openpgp containers_image_ostree_stub"`
- **Test paths**: Use relative paths (e.g., `version.bats` not
  `test/version.bats`)
- **Integration tests**: Must run with `sudo -E ./test/test_runner.sh`
- **Unsigned commits**: Always `git commit -s`
- **Man pages**: Edit `.md` source, not generated files
- **Dependencies**: Run `make verify-dependencies` after updates
- **CI lint failures**: Run
  `make lint && make prettier && make verify-mdtoc` before pushing

## Architecture

**Lifecycle:** Parse config → Create socket (0660) → Init tracing →
gRPC+HTTP servers (cmux) → Register CRI services → Write version files → GC
storage → Start monitors/hooks → Serve → Graceful shutdown

**Stack:** OCI runtimes (runc/crun/kata), container-libs (storage/image), CNI,
conmon/conmon-rs, gRPC/HTTP/D-Bus, OpenTelemetry

## CI/CD

- **GitHub Actions**: `.github/workflows/`
- **OpenShift CI (Prow)**: Main CI platform
  - Job definitions: <https://github.com/openshift/release>
  - Presubmits: `ci-operator/jobs/cri-o/cri-o/cri-o-cri-o-main-presubmits.yaml`
  - Periodics: `ci-operator/jobs/cri-o/cri-o/cri-o-cri-o-main-periodics.yaml`

## Resources

- Installation: `install.md`
- Tutorial: `tutorial.md`
- CRI Edge Cases: `cri.md` (important for image handling)
- Contributing: `CONTRIBUTING.md`
- Governance: `GOVERNANCE.md`
- Roadmap: `roadmap.md`
- Man Pages: `docs/*.md`

## Special Notes for AI Assistants

1. **Never** commit without `-s` flag
2. **Prefer** single commit per branch for simple changes - amend instead of
   creating new commits; use multiple logical commits for complex features when
   it aids review
3. **Always** force push after amending: `git push --force-with-lease`
4. **Always** keep docs/commit messages/PR descriptions synchronized when
   making changes
5. **Always** update commit message to reflect ALL changes when amending
6. **Always** use issue templates (`.github/ISSUE_TEMPLATE/`) when creating
   issues
7. **Always** use PR template (`.github/PULL_REQUEST_TEMPLATE.md`) when
   creating PRs
8. **Always** update dependencies.yaml when changing tool versions
9. Use `make verify-dependencies` after updating versions
10. Man pages: Edit `.md` source files, not generated output
11. Build tags critical: Use `containers_image_openpgp` to avoid pkg-config
12. Integration tests: Must use `sudo -E ./test/test_runner.sh`
13. **Always run linting before pushing**:
    `make lint && make prettier && make verify-mdtoc`
14. Check `dependencies.yaml` for current versions (not this file)
15. When in doubt about versions, check the source files (`go.mod`, `Makefile`,
    `dependencies.yaml`)
16. **Don't over-document** - keep docs, comments, and descriptions clear but
    concise
