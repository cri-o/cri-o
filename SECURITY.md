# CRI-O Security

CRI-O is used in production across many industries that rely on a stable and
secure container runtime for critical infrastructure. Security is taken
seriously and has high priority across all related projects to ensure users can
trust CRI-O for their systems. This means that not only vulnerabilities for this
project, but also for depending ones can be reported through our process, for
example if a vulnerability affects [conmon][conmon] or [conmon-rs][conmon-rs].

[conmon]: https://github.com/containers/conmon
[conmon-rs]: https://github.com/containers/conmon-rs

We're extremely grateful for security researchers and users that report
vulnerabilities to the CRI-O community. All reports are thoroughly investigated
by a set of community volunteers.

## Vulnerability Scope

CRI-O trusts kubelet and assumes every request to the Container Runtime
Interface (CRI) comes from kubelet. Access to the CRI socket is part of the
trusted node boundary. Kubelet requests can carry untrusted input from workloads.
Untrusted input also includes image manifests, layers, and signatures from
registries, as well as the root filesystems and logs of running containers.
Risks to the host or workload isolation are in scope when CRI-O processes
untrusted input through supported features.

- **Direct CRI access.** Issues that require an attacker to already have direct
  access to the CRI socket, such as issues exploitable only via `crictl`, are
  outside the vulnerability scope. Issues exploitable through kubelet remain
  in scope when `crictl` is used to reproduce them.
- **Privileged node access.** Issues that require an attacker to already have
  root access on the node, write access to CRI-O configuration or hook
  directories, or access to the NRI socket are outside the vulnerability scope.
- **Explicitly disabled security controls.** The expected loss of protection
  from a control that an administrator deliberately disables, such as
  certificate verification for a registry configured as insecure, is a
  hardening concern.
- **Container checkpoint and restore.** Container checkpoint remains supported
  and in scope. Container restore (requires
  `container_level_enabled = "checkpoint_restore"`) is unsupported, including
  restore from a checkpoint archive packaged as an OCI image. Issues whose
  exploitation is confined to container restore are outside the vulnerability
  scope.

## Report a Vulnerability

Submit a [private vulnerability report](https://github.com/cri-o/cri-o/security/advisories/new)
on GitHub. The `cncf-crio-security@lists.cncf.io` mailing list is being retired.
Include the security details and the information requested in the
[CRI-O bug report template](https://github.com/cri-o/cri-o/blob/main/.github/ISSUE_TEMPLATE/bug-report.yml).

We aim to send an initial response within 3 business days. It may take longer
depending on maintainer availability.

Discuss potential fixes in the private advisory. Treat fixes as embargoed
until they are merged into all affected branches. A project approver or reviewer
listed in [OWNERS](./OWNERS) will coordinate how the fixes are merged.

### When Should I Report a Vulnerability?

- You think you discovered a potential security vulnerability in CRI-O
- You are unsure how a vulnerability affects CRI-O
- You think you discovered a vulnerability in another project that CRI-O depends
  on (for projects with their own vulnerability reporting and disclosure
  process, please report it directly there)

### When Should I NOT Report a Vulnerability?

- Your issue requires preexisting direct CRI socket access, affects only
  container restore, or is otherwise outside the
  [vulnerability scope](#vulnerability-scope)
- You need help tuning CRI-O components for security
- You need help applying security related updates
- Your issue is not security related

## OpenVEX

CRI-O publishes [OpenVEX](https://openvex.dev) documents with each release to
communicate the exploitability of known vulnerabilities in our dependency tree.
These documents are generated using
[govulncheck](https://pkg.go.dev/golang.org/x/vuln/cmd/govulncheck) and indicate
which vulnerabilities are actually reachable in CRI-O's code paths versus those
that exist in dependencies but are not called.

The VEX report for each release is available as a signed artifact alongside the
release bundles. See the release notes for download and verification
instructions.

To generate a VEX report locally:

```console
make vex
```

This produces `build/cri-o.openvex.json`.
