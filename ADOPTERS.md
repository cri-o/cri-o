# CRI-O Adopters

Below is a non-exhaustive list of CRI-O adopters in production environments:

- [Red Hat's Openshift Container Platform](https://www.openshift.com/) uses
  CRI-O as the only supported CRI implementation starting with the release of
  OpenShift 4. CRI-O was chosen because it provides a lean, stable, simple and
  complete container runtime that moves in lock-step with Kubernetes, while also
  simplifying the support and configuration of clusters.
- [Oracle Linux Cloud Native Environment](https://www.oracle.com/it-infrastructure/software.html)
  has used CRI-O since release due to its tight focus on the Kubernetes CRI and
  its ability to manage both the [runc](https://opencontainers.org/) and
  [Kata Containers](https://katacontainers.io/) runtime engines.
- [SUSE CaaS Platform](https://www.suse.com/products/caas-platform) uses CRI-O
  since version 3. It has been initially supported as technology preview in
  version 3 and moved to the default Kubernetes container runtime since version
  4 as a replacement for Docker.
- [openSUSE Kubic](https://kubic.opensuse.org) is a Certified Kubernetes
  distribution based on openSUSE MicroOS. It uses CRI-O as a supported container
  runtime for Kubernetes.
- [Digital Science](https://www.digital-science.com/) is using CRI-O as the
  runtime in data processing clusters behind [Dimensions](https://www.dimension.ai)
  due to it being just enough runtime for Kubernetes, and the flexibility to
  use more than runc.
- [HERE Technologies](https://here.com) uses CRI-O as the runtime for our home
  grown Kubernetes clusters. We like that it is purpose built for Kubernetes and
  has a strong community backing it.
- [Particule](https://particule.io/en) uses CRI-O as part of our bare metal
  solution [Symplegma](https://github.com/particuleio/symplegma) to deploy
  Kubernetes with Ansible. We aim to be as vanilla and up to date with community
  standards as possible.
- [Nestybox](https://www.nestybox.com) distributes CRI-O together with the
  [Sysbox](https://github.com/nestybox/sysbox) container runtime, to enable
  running secure "VM-like" pods on Kubernetes clusters (voiding the need for
  privileged pods in many scenarios). CRI-O was chosen as it's the only
  container manager that supports creating pods that are strongly isolated using
  the Linux user-namespace (as of Jan 2022).
- [Lyft](https://www.lyft.com/) has used CRI-O since 2017, alongside our
  [CNIIPvlan networking](https://github.com/lyft/cni-ipvlan-vpc-k8s) in AWS.
  All of Lyft runs on top of our Kubernetes stack.
- [Reddit](https://www.reddit.com) has been using CRI-O as the runtime for all
  self-managed Kubernetes clusters since 2021. CRI-O was chosen for its clean and
  easy to reason about codebase, its good observability, and its ability to allow
  for transparently rewriting image registries for mirroring.
- [Adobe](https://www.adobe.com/) uses CRI-O as the primary container runtime for
  our Kubernetes clusters. We chose CRI-O because it's stable at scale and has
  fantastic support from its maintainers and community.
- [PITS Global Data Recovery Services](https://www.pitsdatarecovery.net/) CRI-O is used on K8s and was chosen for its clean and easy-to-manage interface. 
