# Pod checkpoint and restore

CRI-O implements the alpha CRI `CheckpointPod` and `RestorePod` RPCs when
`container_level_enabled = "checkpoint_restore"` is set in the
`[crio.checkpoint_restore]` configuration table. The API checkpoints a selected
set of running containers as one consistency group and restores them into a
newly created pod sandbox.

## Requirements

- Linux with CRIU installed.
- An OCI runtime with checkpoint/restore support, such as a compatible runc or
  crun release.
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
containers before returning.

The output contains a versioned CRI-O manifest, the checkpoint-time sandbox
configuration, and one archive per container. The format is private to CRI-O
and is not interchangeable with containerd checkpoint data.

CRI-O records an operation marker before freezing the pod. If the daemon
restarts during checkpointing, startup recovery thaws the pod and resumes any
selected containers before CRI starts serving requests. Recovery removes
unfinished checkpoint files from the original output directory, while preserving
a published manifest and its archives. A retry also recovers a retained marker
before starting another checkpoint.

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
`StartContainer`. Failures before commit roll back the containers and sandbox;
startup also removes resources belonging to an interrupted transaction.

CRI-O retains the committed container mapping until the sandbox is removed.
Retrying an identical request returns the same IDs, including after a daemon
restart or container start. A different checkpoint or request for the same
sandbox identity and attempt is rejected.

Destination hostname, command, arguments, and environment may differ from the
saved CRI configuration, including values supplied through the Downward API.
The restored processes retain their captured arguments and environment. Working
directory, terminal settings, security contexts, sysctls, and port mappings must
remain compatible.

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

## Limitations

- Restore is node-local. The checkpoint does not move Kubernetes volumes.
- CRI-O creates a fresh pod sandbox and CNI network. Existing external network
  connections are not guaranteed to survive.
- Completed non-restartable init containers and ephemeral containers are not
  selected by kubelet's Pod checkpoint workflow.
- The source and destination must use the same resolved CRI-O runtime handler.
