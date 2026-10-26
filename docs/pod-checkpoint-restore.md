# Pod checkpoint and restore

The alpha CRI `CheckpointPod` and `RestorePod` RPCs require durable operation
identity and confirmed backend cancellation. CRI-O currently keeps
`pod_checkpoint_restore_cancellation` disabled and rejects these RPCs before
admission or resource creation. The OCI/conmon backend does not yet implement
the abort-and-wait handshake required to advertise this capability.

The preparation and recovery code described below remains experimental. Enabling
`container_level_enabled = "checkpoint_restore"` alone does not enable the new
Pod RPCs. Container-level checkpoint and restore use their existing interfaces.

## Requirements

- Linux with CRIU installed.
- runc with checkpoint/restore support. The integration tests do not cover
  Pod checkpoint and restore with crun.
- A kubelet built with the Pod-level checkpoint/restore CRI API.
- Pods created after checkpoint/restore support was enabled. CRI-O persists
  immutable CRI creation metadata for those pods and uses it to validate a
  later restore.

VM and pod runtime handlers are rejected. Runtime-specific checkpoint and
restore options are not currently supported.

## Checkpoint behavior

`CheckpointPod` accepts an existing, empty, absolute output directory and exact
IDs for the containers selected by kubelet. CRI-O freezes the pod cgroup,
pauses every selected container, thaws the pod cgroup, and captures each
container while all selected containers remain paused. It then resumes the
containers before returning. If pausing a container or capturing its state
fails, CRI-O resumes the selected containers and removes the unfinished output.

The output contains a versioned CRI-O manifest, the checkpoint-time sandbox
configuration, and one archive per container. The format is private to CRI-O
and is not interchangeable with containerd checkpoint data.

CRI-O records an operation marker before freezing the pod. Startup does not
recover an interrupted checkpoint; `CancelPodOperation` recovers it once the
operation's runtime calls have ended. Recovery thaws the pod, resumes selected
containers, and removes unfinished files from the original output directory
while preserving a published manifest and its archives. If the output path no
longer refers to the original directory, recovery leaves it unchanged.

If CRI-O stops while a runtime call of the operation is still running, CRI-O
cannot confirm that the call has ended. The operation then remains uncertain,
and `CancelPodOperation` fails, until the node restarts.

## Restore behavior

`RestorePod` validates the checkpoint, restore-time sandbox configuration,
container names, security settings, runtime handler, archive checksums, and
immutable base image IDs. It then:

1. Requires the base container images to be locally available and match the
   captured image IDs. A tag and a digest referring to the same image are accepted.
2. Creates a normal pod sandbox.
3. Creates every requested container in the `CREATED` state.
4. Copies the corresponding archive into CRI-O-owned storage and records a
   durable pending restore.
5. Durably commits the whole Pod and returns its sandbox and container IDs to
   kubelet.

The restored processes do not execute until kubelet calls `StartContainer`.
`StartContainer` rejects containers until the whole Pod restore has committed.
Pending restore state survives a CRI-O restart between `RestorePod` and
`StartContainer`. Failures before commit roll back the containers and sandbox.
Startup does not remove an interrupted transaction; it keeps its operation
record until `CancelPodOperation` establishes backend completion and removes
the resources.

CRI-O retains the committed container mapping until the sandbox is removed.
An admitted operation ID cannot be replayed, even for an identical request.
Another operation targeting the same sandbox metadata is rejected while the
original resources or cleanup still own that identity. A fresh operation ID may
reuse the metadata only after cleanup has stopped all old work and readers,
removed the resources, and durably fenced the old operation.

Normal successful RPC teardown does not abandon prepared or started containers.
Cancellation before successful handoff schedules runtime-owned cleanup. A client
error does not acknowledge cleanup: `CancelPodOperation` must join later starts
and any surviving backend work before resources or metadata can be released.
Uncertain work and failed cleanup retain their recovery records. Sandbox removal
uses the same fence and cleanup owner. Normal removal of one container fences
that container and records its completed backend cleanup without canceling healthy
siblings or releasing the Pod metadata. A failed container removal retains
uncertainty even if a later deletion succeeds; further starts require whole-Pod
settlement to prove the earlier work has stopped.

Destination hostname, command, arguments, and environment may differ from the
saved CRI configuration, including values supplied through the Downward API.
The restored processes retain their captured arguments and environment. Working
directory, terminal settings, security contexts, sysctls, and port mappings must
remain compatible.

Before committing a restore, CRI-O also compares the captured and destination
OCI process credentials, capabilities, resource limits, device assignments,
ordered device-cgroup rules, `no_new_privs`, AppArmor profile, SELinux label and
resolved seccomp policy. It refuses mismatches, including a
changed seccomp file at the same profile path. A newly allocated SELinux MCS
category can therefore prevent restore. Matching these settings does not
authenticate arbitrary CRIU process images; checkpoint artifacts must come
from a trusted source.

The CRI restore request has no registry credentials. Pull every required image
before calling `RestorePod`; CRI-O does not pull images during restore.

## Storage and security

Checkpoint archives contain process memory, environment variables, filesystem
changes, and potentially credentials. Restrict the checkpoint directory to
the node administrator and the CRI-O process. CRI-O writes checkpoint files
with mode `0600`, rejects symlink output directories and unsafe manifest
paths, opens metadata and archives without following symlinks, bounds metadata
to 4 MiB and Pod requests to 1024 containers, and verifies archives again while
copying them into owned storage. Restore does not modify the supplied checkpoint.

Archived external bind mounts must name a destination in the newly generated
OCI configuration. Deleted-file paths remain confined to the destination rootfs,
including when it contains symlinks, and writable-layer extraction runs in a
chroot beneath that rootfs.

## Limitations

- Restore is node-local. The checkpoint does not move Kubernetes volumes.
- CRI-O creates a fresh pod sandbox and CNI network. Existing external network
  connections are not guaranteed to survive.
- Completed non-restartable init containers and ephemeral containers are not
  selected by kubelet's Pod checkpoint workflow.
- The source and destination must use the same resolved CRI-O runtime handler.

## Checkpoint storage limits

`pod_checkpoint_storage_isolation` means all bulk capture data stays on the
caller's output filesystem. Pod capture uses a private `.checkpoint-work`
directory for CRIU images, work logs, writable-layer diffs, container metadata,
and copied application logs. Container-level checkpointing retains its existing
paths. The Pod's container reservations cover the scoped path override through
capture, export, and cleanup; normal container directories are unchanged.

The caller provisions a dedicated filesystem with enforced byte and inode
limits. Kubelet requires administrator storage policy, available headroom, and
the runtime capability before suspending probes. There is no fallback to the
runtime root or host temporary directory when output storage fills. Recovery
resumes tasks before cleaning scratch, removes scratch even after publication,
and can remove partial output when rewriting the resumed marker fails.

Control journals remain outside bulk storage. The daemon retains at most 1024
operation identities, including completed and canceled replay fences, and writes
records no larger than its existing 1 MiB recovery limit. Reaching the journal
limit rejects new operation IDs; existing recovery and cancellation still work.
Existing journals above the limit remain readable. Fences are never evicted to
make space because delayed requests must not replay. Safe retirement of old
identities needs a separate lifecycle design.
