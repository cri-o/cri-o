# General Deprecation Process of CRI-O

## Understanding Configuration Options

Configuration options encompass settings that provide control over various
aspects of CRIO's behavior and functionality. These options allow for the
customization of operational parameters, shaping how CRI-O operates within a
Kubernetes cluster.

## The General Process of Deprecating a Configuration Option

In CRI-O, configuration options undergo deprecation exclusively during major
and minor version changes. Removals are not implemented abruptly within patch
releases, ensuring a seamless transition for users adapting to evolving
configurations.

A configuration option is labeled as deprecated for a minimum of one release
cycle before its actual removal. This period serves as a warning, offering
users an opportunity to adjust their configurations in preparation for upcoming
changes.

The deprecation is communicated through various channels, including
documentation revisions, notifications indicating the deprecation of
configuration options in the CRI-O CLI help text, and a corresponding log entry
within CRI-O itself.

In the domain of system configurations, options are typically excluded when they
are no longer considered essential or have been superseded by alternatives,
particularly those within the Kubelet.

The management of runtime containers has shifted towards the Kubelet, leading
to the replacement of several CRI-O configuration options by the Kubelet.

## Examples of CRI-O Configuration Options Replaced by Kubelet

- Runtime Management, previously handled with the "runtime_path" configuration
  option, has been replaced with the "--container-runtime" flag in Kubelet.

- Pod PIDs Limit, formerly set with the "pids_limit" configuration option, has
  been replaced with the "--pods_pids-limit" flag in Kubelet.

- Image Pull Timeout, initially defined by the "image_pull_timeout"
  configuration option, has been replaced with the
  "--image-pull-progress-deadline" flag in Kubelet.

CRI-O configuration files are composed in TOML. Typically, TOML libraries used
by CRI-O ignore unfamiliar or unacknowledged configuration parameters. While
these unacknowledged values are generally accepted, any unfamiliar flags in the
Command Line Interface (CLI) might result in a failure in CRI-O.

In specific cases, a Command Line Interface (CLI) flag designated for removal
may be retained for an additional release; however, it will be deactivated
during this period. This extension is provided to users, granting them
additional time to update their configurations and scripts before the flag is
ultimately eliminated in subsequent releases.
