# Fedora and RHEL Integration and End-to-End Tests

This directory contains playbooks to set up for and run the integration and
end-to-end tests for CRI-O on RHEL and Fedora hosts.  The expected entry-point
is the ``main.yml`` Ansible playbook.

##Definitions:

    Control-host:  The system from which the ``ansible-playbook`` or
                   ``venv-ansible-playbook.sh`` command is executed.

    Subject-host(s): The target systems, on which actual playbook tasks are
                     being carried out.

##Topology:

The control-host:

 - May be the subject.
 - Is based on either RHEL/CentOS 6 (or later), or Fedora 24 (or later).
 - Runs ``main.yml`` from within the cri-o repository already in the
   desired state for testing.

The subject-host(s):

 - May be the control-host.
 - May be executing the ``main.yml`` playbook against itself.
 - If RHEL-like, has the ``server``, ``extras``, and ``EPEL`` repositories available
   and enabled.
 - Has remote password-less ssh configured for access by the control-host.
 - When ssh-access is for a regular user, that user has password-less
   sudo access to root.

##Runtime Requirements:

Execution of the ``main.yml`` playbook:

 - Should occur through the ``cri-o/contrib/test/venv-ansible-playbook.sh`` wrapper.
 - Execution may target localhost, or one or more subjects via standard Ansible
   inventory arguments.
 - Should use a combination (including none) of the following tags:

     - ``setup``: Run all tasks to set up the system for testing. Final state must
                  be self-contained and independent from other tags (i.e. support
                  stage-caching).
     - ``integration``: Assumes 'setup' previously completed successfully.
                        May be executed from cached-state of ``setup``.
                        Not required to execute coincident with other tags.
                        Must build CRI-O from source and run the
                        integration test suite.
     - ``e2e``: Assumes 'setup' previously completed successfully.  May be executed
                from cached-state of ``setup``. Not required to execute coincident with
                other tags.  Must build CRI-O from source and run Kubernetes node
                E2E tests.

``cri-o/contrib/test/venv-ansible-playbook.sh`` Wrapper:

 - May be executed on the control-host to both hide and version-lock playbook
   execution dependencies, ansible and otherwise.
 - Must accept all of the valid Ansible command-line options.
 - Must sandbox dependencies under a python virtual environment ``.cri-o_venv``
   with packages as specified in ``requirements.txt``.
 - Requires the control-host has the following fundamental dependencies installed
   (or equivalent): ``python2-virtualenv gcc openssl-devel
   redhat-rpm-config libffi-devel python-devel libselinux-python rsync
   yum-utils python3-pycurl python-simplejson``.

For example:

Given a populated '/path/to/inventory' file, a control-host could run:

./venv-ansible-playbook.sh -i /path/to/inventory ./integration/main.yml

-or-

From a subject-host without an inventory:

./venv-ansible-playbook.sh -i localhost, ./integration/main.yml
