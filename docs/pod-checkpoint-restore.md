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
selected containers before CRI starts serving requests.

## Restore behavior

`RestorePod` validates the checkpoint, restore-time sandbox configuration,
container names, process configuration, runtime handler, and archive
checksums. It then:

1. Ensures the base container images are locally available.
2. Creates a normal pod sandbox.
3. Creates every requested container in the `CREATED` state.
4. Copies the corresponding archive into CRI-O-owned storage and records a
   durable pending restore.
5. Returns the sandbox and container IDs to kubelet.

The restored processes do not execute until kubelet calls `StartContainer`.
Pending restore state survives a CRI-O restart between `RestorePod` and
`StartContainer`. Any failure before `RestorePod` returns rolls back all
containers and the sandbox. Startup also removes resources belonging to an
interrupted restore transaction.

The CRI restore request has no registry credentials. Images from private
registries must therefore already be present in CRI-O's image store.

## Storage and security

Checkpoint archives contain process memory, environment variables, filesystem
changes, and potentially credentials. Restrict the checkpoint directory to
the node administrator and the CRI-O process. CRI-O writes checkpoint files
with mode `0600`, rejects symlink output directories and unsafe manifest
paths, and does not modify the caller-owned checkpoint during restore.

## Limitations

- Restore is node-local. The checkpoint does not move Kubernetes volumes.
- CRI-O creates a fresh pod sandbox and CNI network. Existing external network
  connections are not guaranteed to survive.
- Completed non-restartable init containers and ephemeral containers are not
  selected by kubelet's Pod checkpoint workflow.
- The source and destination must use the same resolved CRI-O runtime handler.
