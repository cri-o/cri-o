# Project Governance

<!-- toc -->
- [Values](#values)
- [Contribution Tiers](#contribution-tiers)
  - [Community Member](#community-member)
  - [CRI-O Organization Member](#cri-o-organization-member)
    - [How to be come an org member](#how-to-be-come-an-org-member)
    - [Organization member responsibilities](#organization-member-responsibilities)
  - [Reviewer](#reviewer)
    - [How to become a reviewer](#how-to-become-a-reviewer)
    - [Reviewer responsibilities](#reviewer-responsibilities)
  - [Approver](#approver)
    - [How to become an approver](#how-to-become-an-approver)
    - [Approver responsibilities](#approver-responsibilities)
    - [Distinction between top-level approvers and sub-tree approvers](#distinction-between-top-level-approvers-and-sub-tree-approvers)
  - [Emeritus Approver](#emeritus-approver)
  - [Demotion](#demotion)
- [Meetings](#meetings)
- [CNCF Resources](#cncf-resources)
- [Code of Conduct](#code-of-conduct)
- [Security](#security)
- [Conflict resolution and voting](#conflict-resolution-and-voting)
- [Miscellaneous Administrivia](#miscellaneous-administrivia)
  - [Cutting a release](#cutting-a-release)
  - [Github Aliases](#github-aliases)
  - [Adding new projects to the CRI-O GitHub organization](#adding-new-projects-to-the-cri-o-github-organization)
  - [Feature addition policy](#feature-addition-policy)
  - [Feature request policy](#feature-request-policy)
- [Credits](#credits)
<!-- /toc -->

## Values

The CRI-O project and its leadership embrace the following values:

- Openness: Communication and decision-making happens in the open and is
  discoverable for future reference. As much as possible, all discussions
  and work should take place in public forums and open repositories.
- Fairness: All stakeholders have the opportunity to provide feedback and
  submit contributions, which will be considered
  on their merits and not from source.
- Community over Product or Company: Sustaining and growing our community
  takes priority over shipping code or sponsors' organizational goals. Each
  contributor participates in the project as an individual.
- Prefer Stable Evolution: A willingness to constantly improve and reevaluate
  old decisions will keep the project and community relevant and growing.
  However, this constant growth cannot be done at the expense of stability of
  the project. New features should be introduced liberally, but must be
  configured and opted-into until there is proven stability.
- Inclusivity: We innovate through different perspectives and skill sets, which
  can only be accomplished in a welcoming and respectful environment.
- Participation: Responsibilities within the project are earned through
  participation, and there is a clear path up the contributor ladder into
  leadership positions.

## Contribution Tiers

CRI-O has five tiers of code contributors: community members, org members,
reviewers, approvers and emeritus approvers.

- Anyone can (and is encouraged to) be a community member, whose
  responsibilities are up to the contributor (submitting PRs, reviewing PRs).
- An org member is someone added to the [cri-o](https://github.com/cri-o)
  organization, and can be considered to be a community member whose
  contributions have demonstrated they can be trusted with access to the CRI-O CI.
- Reviewers are a set of people responsible for reviewing PRs and triaging
  issues. In most cases, they're also responsible for fixing issues in the code base.
- Approvers are the set of people that have final say over the direction of the
  project. For any PR to merge, an approver must approve it.
- Emeritus Approvers can be considered "retired" approver--while they don't have
  approval rights, they are listed in the OWNERS file to
  indicate their expertise in the area.
The current list of reviewers and approvers can be found in the [CRI-O OWNERS file](https://github.com/cri-o/cri-o/blob/main/OWNERS).
Further, some more detailed information can be found in the [CRI-O Maintainers file](https://github.com/cri-o/cri-o/blob/main/MAINTAINERS.md).

### Community Member

- Anyone can and is encouraged to be a community member! The CRI-O community
  eagerly accepts PRs, issues, new users and questions.
- All the CRI-O community asks of a community member is they follow the
  [Code of Conduct](./code-of-conduct.md).
- While technical contributions are always appreciated, the CRI-O community
  welcomes contributions of any kind. Some examples of contributions that don't
  involve coding are:
  - Improving documentation
  - Updating the website
  - Write blogs
  - Give conference presentations
  - Testing out new releases

### CRI-O Organization Member

#### How to be come an org member

- Organization members are contributors whose contributions automatically have
  CI run on them.
- As such, a proven track-record of good faith contributions is required
  for org membership.
- To become an org member, one should file an issue with [this template](https://github.com/cri-o/cri-o/issues/new?template=org_member.yml&title=REQUEST%3A+New+organization+membership+for+%3Cyour-GH-handle%3E).

#### Organization member responsibilities

- Organization members are expected to fulfill the responsibilities of a community
  member, with a layer of added trust that the CRI-O CI will not be used in a
  malicious or wasteful manner.

### Reviewer

#### How to become a reviewer

- If an org member would like to take on a more active role in the community,
  and one day become an approver, becoming a reviewer is a good first step.
- Becoming a reviewer allows someone to build trust with the community and
  deepen their knowledge of the code base.
- To become a reviewer, one should file an issue with [this template](https://github.com/cri-o/cri-o/issues/new?template=reviewer.yml&title=REQUEST%3A+New+reviewer+status+for+%3Cyour-GH-handle%3E).
- To find a sponsor, a community member is encouraged to reach out to the approvers.
  The approver team is happy to foster growth within the CRI-O community.

#### Reviewer responsibilities

- Once a community member is a reviewer, the CRI-O community will expect them to:
- Start contributing PRs and taking issues under the guidance of the current approvers.
- Monitor our channel [#crio](https://kubernetes.slack.com/archives/crio) on the
  Kubernetes Slack instance and answer questions where possible.
- Triage GitHub issues and review pull requests. The areas of specialization
  listed in [OWNERS.md](OWNERS.md) can be used to help with routing an issue/question
  to the right person.
- Triage CI issues - file issues for known flaky CI runs or bugs, and either fix
  or find someone to fix any breakages.
- During GitHub issue triage, apply all applicable [labels](https://github.com/cri-o/cri-o/labels)
  to each new issue. Labels are extremely useful for future issue follow up.
  A few of the most important labels that are not self explanatory are:
  - **good first issue**: Mark any issue that can reasonably be accomplished by
  a new contributor with this label.
  - **help wanted**: Unless it is immediately obvious that someone is going to
  work on an issue (and if so assign it), mark it as help wanted.
  - **CRI change**: If sufficiently fixing the issue involves a change to the CRI,
  this label is given. These changes require consensus from the Kubernetes
  SIG-Node community, and thus may be more complex than an internal CRI-O change.
- Make sure that ongoing PRs are moving forward at the right pace or closing them.
- Reviewers are granted approver rights to dependency bumps and are expected to
  focus their attention on them.

### Approver

#### How to become an approver

- Becoming an approver is considered both a privilege and a responsibility.
  - The barrier to become an approver is naturally higher than that of becoming
  a reviewer, and thus further encompasses all of the requirements of a reviewer.
  - Approvers should serve as a reviewer for at least three months, though this
  alone does not necessarily qualify a reviewer to be an approver.
  - Approvers are community members with a vested interest (which need not be
  professional) in the future direction of CRI-O.
  - An approver is not just someone who can make changes, but someone who has
  demonstrated their ability to collaborate with the team, get the most knowledgeable
  people to review code and docs, contribute high-quality code, and follow through
  to fix issues (in code or tests).
  - They are expected to have a deep understanding of various pieces of CRI-O and
  how they work together.
  - They are expected to be able to participate in the management of the project.
  Joining in the discussion of technical questions and aiding in resolving
  technical challenges.
- The person in question for approvership can nominate themselves, or be nominated
  by someone else.
  - A contributor is nominated by someone submitting a PR to add their github
  handle to the approvers section of the [OWNERS](OWNERS.md) file.
- To become an approver, a simple majority vote among the current approvers
  should be performed.
  - See [conflict resolution](#conflict-resolution-and-voting) for more information
  on how to handle disputes.

#### Approver responsibilities

Approvers have all of the responsibilities of the reviewers, as well as:

- The expectation of reviews is higher. Approvers should be experts of their
  knowledge domain, and should serve as code owners of those pieces.
- Participate when called upon in the [security release process](https://github.com/cri-o/cri-o/blob/main/SECURITY.md).
  Note that although this should be a rare occurrence, if a serious vulnerability
  is found, the process may take up to several full days of work to implement.
  This reality should be taken into account when discussing time commitment
  obligations with employers.

#### Distinction between top-level approvers and sub-tree approvers

Through the mechanism of the OWNERS file, it’s possible to define sub-tree approvers
for areas of specialty. For the purposes of this document, when “approvers” is
mentioned, sub-tree approvers are not necessarily included, though they may be.

### Emeritus Approver

- All good things must come to an end, and an approver's tenure is no different.
- The CRI-O community has included the emeritus approver title for approvers who
  have completed their service to the community, and have moved on to other things.
- While the emeritus approver has no more power within the project, they do have
  wisdom and can be called upon for guidance.
- If an approver has been inactive in the community (no pull requests or reviews)
  for more than 2 years, then they may be asked by the approvers to move themselves
  to an emeritus approver. This is to open up slots for new approvers
  to fill their slots.
- To become an emeritus approver, one simply has to move themselves from the
  approvers field to the emeritus approvers field in the OWNERS file.

### Demotion

- In order to maintain an active reviewers and approvers list, the CRI-O team has
  a right to demote an approver or reviewer if they appear inactive for one year.
- For any kind of demotion, "inactive" means having had no PRs opened or comments
  on any PR.
- If a reviewer is demoted, they become a community member. If an approver is demoted,
  they become an emeritus approver.
- The demotion process works by:
  - Someone opening a PR on the person's behalf making the appropriate
  OWNERS file adjustment.
  - They also give justification by showing contribution and reviewing history.
  - The person is given a week to respond, giving justification for or against
  their demotion.
  - If they fail to do so, or are in favor of the demotion, then the PR can be
  merged as any other would be.

## Meetings

Time zones permitting, approvers and reviewers are expected to participate
in the public community meeting, the details of which are described on the
[CRI-O wiki](https://github.com/cri-o/cri-o/wiki/CRI-O-Weekly-Meeting).

Approvers will also have closed meetings in order to discuss security reports or
Code of Conduct violations.  Such meetings should be scheduled by any approver on
receipt of a security issue or CoC report.  All current approvers must be invited
to such closed meetings, except for any approver who is accused of a CoC violation.

## CNCF Resources

Any Maintainer may suggest a request for CNCF resources, either by opening an issue,
or during a meeting. The request is approved through the
[standard voting mechanism](#conflict-resolution-and-voting).
The approvers may also choose to delegate working with the CNCF to community members.

## Code of Conduct

[Code of Conduct](./code-of-conduct.md) violations by community members will be
discussed and resolved privately in a slack conversation on the
Kubernetes slack instance with the approvers.  If the reported CoC violator is an
approver, the approvers will instead designate two other approvers to work
with CNCF staff in resolving the report.

## Security

Documentation on CRI-O's security policy is spelled out in the
[security](./SECURITY.md) document.

## Conflict resolution and voting

In general, the CRI-O community prefers technical issues are amicably worked out
between the people involved. Those, and most other decisions are conducted by "lazy
consensus". If a dispute cannot be decided independently, the approvers can be
called in to decide an issue.

If the approvers cannot reach unanimous consensus on an issue, the issue will be
resolved by voting on an issue or PR on Github, in which each approver gets one
vote. Votes not received within the span of one week are treated as an abstention.
In the case of abstention, an approver will not be included in the voting approvers
group when deciding the majority. In the case of a tie, the issue will not be considered
to have passed.

Most votes require a simple majority of all voting approvers to succeed.
Approvers can be removed by a 2/3 majority vote of all voting approvers,
and changes to this Governance require a 2/3 vote of all approvers. The
approvers will make every reasonable effort to make sure that any removal
or amendment to the Governance be affirmed with votes by employees of
more than one organization.

## Miscellaneous Administrivia

### Cutting a release

- CRI-O cuts releases in line with Kubernetes minor releases (1.x bump). The CRI-O
  community attempts to release a minor version of CRI-O
  *within three days ofthe corresponding Kubernetes release*.
- For Patch releases (1.1.z bump), releases are cut intermittently, when there
  are sufficient bug fixes backported to the branch. End-users can request
  releases be cut, and approvers can choose to accept that request at their discretion.
- Release notes are compiled by our [release-notes generator](https://github.com/cri-o/cri-o/blob/main/scripts/release-notes/release_notes.go).
- A release can be cut with our [release script](https://github.com/cri-o/cri-o/blob/main/scripts/release/release.go).
- Finally, create a [tagged release](https://github.com/cri-o/cri-o/releases).
  The release should  start with "v" and be followed by the version number.
  E.g., "v1.18.0". *This must match the corresponding Kubernetes release number.*

### Github Aliases

There is an alias on Github that one can use to ping all of the CRI-O reviewers
and approvers: "@cri-o/cri-o-maintainers".

### Adding new projects to the CRI-O GitHub organization

New projects will be added to the [CRI-O organization](https://github.com/cri-o)
via GitHub issue discussion in one of the existing projects in the organization.
Once sufficient discussion has taken place (~3-5 business days but depending on
the volume of conversation), the approvers will decide whether the new project
should be added by a simple majority described in [#Conflict resolution and voting](#conflict-resolution-and-voting)

### Feature addition policy

Features are added by submitting a PR. The reviewers and approvers handle this
the same as a bug fix PR, discussing whether the feature will be added in the
PR page. Typically, though not always, new features are not backported to release
branches. However, the distinction is sometimes muddy between a bug fix and a feature,
so discretion lies with the approvers.

### Feature request policy

Features are requested by adopters by opening a [blank issue](https://github.com/cri-o/cri-o/issues/new)
and populating it with the description. Decision to add the feature is ultimately
up to the approvers, though partially depends on developer bandwidth.
More concretely: submitting a feature request does not guarantee the CRI-O developers
will have time to implement it. In many cases, a feature will be accepted faster
if someone takes responsibility for it and drives it end-to-end, and the CRI-O community
hopes end-users have the desire and capacity to do so.

## Credits

Thanks to the [Jaeger Project](https://github.com/jaegertracing/jaeger/blob/main/GOVERNANCE.md),
the [Envoy project](https://github.com/envoyproxy/envoy/blob/main/GOVERNANCE.md),
the [Kubernetes community documentation](https://kubernetes.io/community/values/),
and the [CNCF](https://github.com/cncf/project-template/blob/main/GOVERNANCE-maintainer.md)
for portions and structure of this document.
