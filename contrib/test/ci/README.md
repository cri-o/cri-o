# Fedora and RHEL Test execution

This directory contains playbooks to set up and run, all the CRI-O CI tests
for both RHEL and Fedora hosts. The entry-point playbooks are labeled with `<test>-main.yml`
and they can be used with the following tags:

- `setup`: Run all tasks to set up the system for testing.
- `critest`: Run validation and benchmark tests.
- `e2e`: Build CRI-O from source and run Kubernetes end-to-end tests.
- `integration`: Build CRI-O from source and run the local integration suite twice.
  First normally, then again with user-namespace support enabled.

The playbooks assume the following things about your system:

- On RHEL, the repositories for EPEL, rhel-server,
  and extras repos are configured and functional.
- The system has been rebooted after installing/updating low-level packages,
  to ensure they're active.
- Ansible is installed, and functional with access to the 'root' user.
- The CRI-O repository is present in the desired state at
  `${GOPATH}/src/github.com/cri-o/cri-o`.
