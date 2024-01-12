# General Deprecating Process of CRI-O

## What are config options?

Config options are the settings that allow us to control various aspects of the behaviour and functionality of CRIO. They allow us to customise how CRI-O operates within a kubernetes cluster.

## General process of deprecating a config option

Config options undergo deprecation only during major and minor version changes in CRI-O. Removals do not occur abruptly within patch releases, ensuring a smoother transition for users adapting to evolving configurations.

A deprecated status is assigned to a configuration option for a minimum of one release cycle before it is actually removed. This gives users a warning and time to adjust their configurations to adapt to the upcoming changes.

The deprecation is communicated through multiple channels, including updates in the documentation, messages to indicate deprecated config option in the CRI-O CLI help text and a log entry within CRI-O itself.

Configuration options are typically removed if they are no longer considered necessary or if they have been replaced by alternatives, especially options present in the Kubelet.

Several CRI-O config options have been replaced by Kubelet as the management of runtime containers has shifted towards the Kubelet.

## Examples of crio.conf options getting replaced by Kubelet

- Runtime Management was previously done with the “runtime\_path” config option which was replaced with the “--container-runtime” flag in Kubelet.

- Pod PIDs Limit was previously done with the “pids\_limit” config option which was replaced with the “--pods\_pids-limit” flag in Kubelet

- Image Pull Timeout was previously done with the “image\_pull\_timeout” config option which was replaced with the “--image-pull-progress-deadline” flag in Kubelet.

The configuration files for CRI-O are written in TOML. TOML libraries used by CRIO usually ignore unknown or unrecognised configuration values. These unrecognised values are generally tolerated, but unknown flags in the CLI may cause CRI-O to fail.

In some cases, a CLI flag that has been marked for removal might be retained for an additional release, but it will be disabled. This grace period is provided to users so they have extra time to update their configurations and scripts before the flag is completely removed in subsequent releases.
