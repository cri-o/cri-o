# Roadmap

## Overview

The initial roadmap of CRI-O was lightweight and followed the main
Kubernetes Container Runtime Interface (CRI) development lifecycle.
This is partially because additional features on top of that are either integrated
into a CRI-O release as soon as they’re ready, or are tracked through the
Milestone mechanism in GitHub.
Another reason is that feature availability is mostly tied to Kubernetes releases,
and thus most of its long-term goals are already tracked in [SIG-Node](https://github.com/kubernetes/community/blob/master/sig-node/README.md)
through the Kubernetes Enhancement Proposal (KEP) process.
Finally, CRI-O’s long-term roadmap outside of features being added by SIG-Node
is in part described by its mission:
to be a secure, performant and stable implementation of the Container Runtime Interface.
However, all of these together do construct a roadmap,
and this document will describe how.

## Milestones

CRI-O’s primary internal planning mechanism is the
Milestone feature in GitHub, along with Issues.
Since CRI-O releases in lock-step with Kubernetes minor releases,
where the CRI-O community aims to have a x.y.0 release released within three days
after the corresponding Kubernetes x.y.0 release, there is a well established
deadline that must be met.
For PRs and Issues that are targeted at a particular x.y.0 release can be added
to the x.y Milestone and they will be considered for priority in leading up
to the actual release.
However, since CRI-O’s releases are time bound and partially out of the CRI-O
communities’ control, tagging a PR or issue with a Milestone does not guarantee
it will be included.
Users or contributors who don’t have permissions to add the Milestone can
request an Approver to do so.
If there is disagreement, the standard [Conflict resolution and voting](https://github.com/cri-o/cri-o/blob/main/GOVERNANCE.md#conflict-resolution-and-voting)
mechanism will be used.
Pull requests targeting patch release-x.y branches are not part of any milestone.
The release branches are decoupled from the [Kubernetes patch release cycle](https://k8s.io/releases/patch-releases)
and fixes can be merged ad-hoc.
The support for patch release branches follows the yearly Kubernetes period and
can be longer based on contributor bandwidth.

## SIG-Node

CRI-O’s primary purpose is to be a CRI compliant runtime for Kubernetes, and thus
most of the features that CRI-O adds are added to remain conformant to the CRI.
Often, though not always, CRI-O will attempt to support new features in Kubernetes
while they’re in the Alpha stage, though sometimes this target is missed and
support is added while the feature is in Beta.
To track the features that may be added to CRI-O from upstream, one can watch
[SIG-Node’s KEPs](https://github.com/kubernetes/enhancements/pulls?q=is%3Apr+is%3Aopen+label%3Asig%2Fnode)
for a given release.
If a particular feature interests you, the CRI-O community recommends you open
an issue in CRI-O so it can be included in the Milestone for that given release.
CRI-O maintainers are involved in SIG-Node in driving various upstream initiatives
that can be tracked in the [SIG-Node planning document](https://docs.google.com/document/d/1U10J0WwgWXkdYrqWGGvO8iH2HKeerQAlygnqgDgWv4E/edit?usp=sharing).

## Long-term, CRI-O Specific Features

There still exist features that CRI-O will add that exist outside of the purview
of SIG-Node, and span multiple releases.
These features are usually implemented to fulfill CRI-O’s purpose of being secure,
performant, and stable.
As all of these are aspirational and seek to improve CRI-O structurally, as opposed
to fixing a bug or clearly adding a feature, it’s less appropriate to have
an issue for them.
As such, updates to this document will be made once per release cycle.
Finally, it is worth noting that the integration of these features will be deliberate,
slow, and strictly opted into.
CRI-O does aim to constantly improve, but also aims to never compromise its stability
in the process.
Some of these features can be seen below:

### Initiatives on CRI-O’s Roadmap

- Improve upstream documentation
- Automate release process
- Improved seccomp notification support
- Increase pod density on nodes:
  - Reduce overhead of relisting pods and containers (alongside [KEP 3386](https://github.com/kubernetes/enhancements/blob/master/keps/sig-node/3386-kubelet-evented-pleg/README.md))
  - Reduce overhead of metrics collection (alongside [KEP 2371](https://github.com/kubernetes/enhancements/blob/master/keps/sig-node/2371-cri-pod-container-stats/README.md))
  - Reduce process overhead of multiple conmons per pod (through [conmon-rs](github.com/containers/conmon-rs))
  - Improve maintainability by ensuring the code is easy to understand and follow
  - Improve observability and tracing internally
- Evaluate rust reimplementation of different pieces of the codebase.

## Known Risks

- Relying on different SIGs for CRI-O features:
  - We have a need to discuss our enhancements with different SIGs to get all
    required information and drive the change. This can lead into helpful, but maybe
    not expected input and delay the deliverable.

- Some features require initial research:
  - We're not completely aware of all technical aspects for the changes. This means
    that there is a risk of delaying because of investing more time in pre-research.
