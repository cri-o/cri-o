# CRI-O GOVERNANCE.md

CRI-O, being a project whose purpose is linked to fulfilling the needs of Kubernetes, has a governance structure that mirrors many SIGs and sub-projects of Kubernetes. This document highlights important aspects of, and serves as a summary of the [Kubernetes' community documentation](https://www.kubernetes.dev/docs).

# CRI-O contributor tiers

* CRI-O has four tiers of code contributors: approvers, reviewers, emeritus_approvers and community members.
	* Anyone can (and is encouraged to) be a community member, whose responsibilities are up to the contributor (submitting PRs, reviewing PRs).
	* Reviewers are a set of people responsible for reviewing PRs and triaging issues. In most cases, they're also responsible for fixing issues in the code base.
	* Approvers are the set of people that have final say over the direction of the project. For any PR to merge, an approver must approve it.
	* Emeritus Approvers can be considered "retired" maintainers--while they don't have approval rights, they are listed in the OWNERS file to indicate their expertise in the area.
* The current list of reviewers and approvers can be found in the [CRI-O OWNERS file](github.com/cri-o/cri-o/blob/main/OWNERS).

## Community Member

* Anyone can and is encouraged to be a community member! The CRI-O community eagerly accept PRs, issues, new users and questions.

## Reviewer

### How to become a reviewer

* If a community member would like to take on a more active role in the community, and one day become an approver, becoming a reviewer is a good first step.
* Becoming a reviewer allows someone to build trust with the community and deepen their knowledge of the code base.
* To become a reviewer, one should file an issue with [this template](https://github.com/cri-o/cri-o/issues/new?assignees=&labels=area%2Fgithub-membership&template=membership.yml&title=REQUEST%3A+New+membership+for+%3Cyour-GH-handle%3E).
* To find a sponsor, a community member is encouraged to reach out to the approvers. The approver team is happy to foster growth within the CRI-O community.

### Reviewer responsibilities

* Once a community member is a reviewer, the CRI-O community will expect them to:
* Start contributing PRs and taking issues under the guidance of the current approvers.
* Monitor our channel [#crio](https://kubernetes.slack.com/archives/crio) on the Kubernetes Slack instance and answer questions where possible.
* Triage GitHub issues and perform pull request reviews for other maintainers and the community. The areas of specialization listed in [OWNERS.md](OWNERS.md) can be used to help with routing an issue/question to the right person.
* Triage CI issues - file issues for known flaky CI runs or bugs, and either fix or find someone to fix any breakages.
* During GitHub issue triage, apply all applicable [labels](https://github.com/cri-o/cri-o/labels) to each new issue. Labels are extremely useful for future issue follow up. A few of the most important labels that are not self explanatory are:
  * **good first issue**: Mark any issue that can reasonably be accomplished by a new contributor with this label.
  * **help wanted**: Unless it is immediately obvious that someone is going to work on an issue (and if so assign it), mark it help wanted.
  * **CRI change**: If sufficiently fixing the issue involves a change to the CRI, this label is given. Often, maintainers won't have time to make such a change, but the change could be seen as good and as such the issue isn't closed.
* Make sure that ongoing PRs are moving forward at the right pace or closing them.

## Approver
### How to become an approver

* The barrier to become an approver is naturally higher than that of becoming a reviewer.
* There isn't an explicitly spelled out set of criteria that is required, but generally being an approver means one will be spending significant time maintaining and reviewing CRI-O code.
* To become an approver, a simple majority vote among the current approvers should be performed. See [conflict resolution](#conflict-resolution-and-voting) for more information on how to handle disputes.

### Approver responsibilities

* Approvers have all of the responsibilities of the reviewers, as well as:
* The expectation of reviews is higher. Approvers should be experts of their knowledge domain, and should serve as code owners of those pieces.
* Participate when called upon in the [security release process](github.com/cri-o/cri-o/blob/main/SECURITY.md). Note that although this should be a rare occurrence, if a serious vulnerability is found, the process may take up to several full days of work to implement. This reality should be taken into account when discussing time commitment obligations with employers.

## Emeritus Approver

* All good things must come to an end, and an approver's tenure is no different.
* The CRI-O community has included the emeritus approver title for approvers who have completed their service to the community, and have moved on to other things.
* While the emeritus approver has no more power within the project, they do have wisdom and can be called upon for guidance.
* If an approver has been inactive in the community (no pull requests or reviews) for more than 2 years, then they may be asked by the approvers to move themselves to an emeritus approver. This is to open up slots for new approvers to fill their slots.
* To become an emeritus approver, one simply has to move themselves from the approvers field to the emeritus approvers field in the OWNERS file.

## Demotion
* In order to maintain an active reviewers and approvers list, the CRI-O team has a right to demote an approver or reviewer if they appear inactive for one year.
* For any kind of demotion, "inactive" means having had no PRs opened or comments on any PR.
* If a reviewer is demoted, they become a community member. If an approver is demoted, they become an emeritus approver.
* The demotion process works by:
	* Someone opening a PR on the person's behalf making the appropriate OWNERS file adjustment.
	* They also give justification by showing contribution and review history.
	* The person is given a week to respond, giving justification for or against their demotion.
	* If they fail to do so, or are in favor of the demotion, then the PR can be merged as any other would be.

# Cutting a release

* CRI-O cuts releases in line with Kubernetes minor releases (1.x bump). The CRI-O community attempts to release a minor version of CRI-O *within three days of the corresponding Kubernetes release*.
* For Patch releases (1.1.z bump), releases are cut intermittantly, when there are sufficient bug fixes backported to the branch. End-users can request releases be cut, and maintainers can choose to accept that request at their discretion.
* Release notes are compiled by our [release-notes generator](https://github.com/cri-o/cri-o/blob/main/scripts/release-notes/release_notes.go).
* A release can be cut with our [release script](https://github.com/cri-o/cri-o/blob/main/scripts/release/release.go).
* Finally, create a [tagged release](https://github.com/cri-o/cri-o/releases). The release should  start with "v" and be followed by the version number. E.g., "v1.18.0". *This must match the corresponding Kubernetes release number.*

# Feature addition policy

* Features are added by simply adding a PR.
* The maintainers handle this the same as a bug fix PR, discussing whether the feature will be added in the PR page.
* Typically, though not always, new features are not backported to release branches.
	* However, the distinction is sometimes muddy between a bug fix and a feature, so discretion lies with the approvers.

# Community meetings

* A weekly meeting is held to discuss CRI-O development. It is open to everyone.
* It is generally run by an approver.
* The details to join the meeting are on the [wiki](https://github.com/cri-o/cri-o/wiki/CRI-O-Weekly-Meeting).

# Github Aliases

* There is an alias on github that one can use to ping all of the CRI-O reviewers and approvers: "@cri-o/cri-o-maintainers".

# Conflict resolution and voting

* In general, we prefer that technical issues and maintainer membership are amicably worked out between the persons involved.
* If a dispute cannot be decided independently, the maintainers can be called in to decide an issue.
* If the maintainers themselves cannot decide an issue, the issue will be resolved by voting:
	* The voting process is a simple majority in which each maintainer receives one vote.
	* Votes not received within the span of one week are treated as an abstention.
	* In case of a tie, the issue will be considered to not have passed.

# Adding new projects to the CRI-O GitHub organization

* New projects will be added to the [CRI-O organization](github.com/cri-o) via GitHub issue discussion in one of the existing projects in the organization.
* Once sufficient discussion has taken place (~3-5 business days but depending on the volume of conversation), the maintainers will decide whether the new project should be added.
* See the section above on voting if the maintainers cannot easily decide.

# Credits

* This document was partially based off of [Envoy's GOVERNANCE.md](https://github.com/envoyproxy/envoy/blob/main/GOVERNANCE.md) file. Some credit goes to the maintainers of the Envoy project.
* As mentioned above, credit is also due to the Kubernetes community documentation, which also served as a foundation.
