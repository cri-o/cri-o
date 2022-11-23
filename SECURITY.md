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

## Report a Vulnerability

To make a report, email the vulnerability to the private
[cncf-crio-security@lists.cncf.io](mailto:cncf-crio-security@lists.cncf.io) list
with the security details and the details expected for [all CRI-O bug
reports](https://github.com/cri-o/cri-o/blob/main/.github/ISSUE_TEMPLATE/bug-report.yml).

You can expect an initial response to the report within 3 business days.
Possible fixes for vulnerabilities will be then discussed via the mail thread
and can be considered as automatically embargoed until they got merged into all
related branches. A project approver or reviewer (as defined in the
[OWNERS](./OWNERS) file) will coordinate how the pull requests and patches are
being incorporated into the repository without breaking the embargo.

### When Should I Report a Vulnerability?

- You think you discovered a potential security vulnerability in CRI-O
- You are unsure how a vulnerability affects CRI-O
- You think you discovered a vulnerability in another project that CRI-O depends
  on (for projects with their own vulnerability reporting and disclosure
  process, please report it directly there)

### When Should I NOT Report a Vulnerability?

- You need help tuning CRI-O components for security
- You need help applying security related updates
- Your issue is not security related
