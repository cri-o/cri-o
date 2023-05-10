# Fedora and RHEL Test execution

This directory contains playbooks to set up and run, all the CRI-O CI tests
for both RHEL and Fedora hosts. Two entry-point playbooks exist:

- `main.yml`: sets up the machine and runs tests.
- `results.yml`: gathers test output to `/tmp/artifacts`.

When running the `main.yml` playbook, multiple tags are present:

- `setup`: Run all tasks to set up the system for testing.
- `e2e`: Build CRI-O from source and run Kubernetes end-to-end tests.
- `integration`: Build CRI-O from source and run the local integration suite twice.
  First usually, then again with user-namespace support enabled.

The playbooks assume the following things about your system:

- On RHEL, the repositories for EPEL, rhel-server,
  and extras repos are configured and functional.
- The system has been rebooted after installing/updating
  low-level packages, to ensure they're active.
- Ansible is installed, and functional with access to the 'root' user.
- The `$GOPATH` is set and present for all shells (*e.g.* written in `/etc/environment`).
- The CRI-O repository is present in the desired state at
  `${GOPATH}/src/github.com/cri-o/cri-o`.
