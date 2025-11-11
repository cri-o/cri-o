# CRI-O Annotation Migration Guide

This document describes the migration of CRI-O annotations to follow
Kubernetes-recommended naming conventions.

## Overview

CRI-O is migrating from the legacy annotation format
`io.kubernetes.cri-o.*` to the Kubernetes-recommended format `*.crio.io`.
This migration implements a deprecation phase where both old and new
annotations are supported, with the new format taking precedence.

## Annotation Format

### Old Format (Deprecated)

```text
io.kubernetes.cri-o.<feature-name>
```

### New Format (Recommended)

```text
<feature-name>.crio.io
```

## Migration Status

All CRI-O annotations have been migrated to the v2 package following the
Kubernetes-recommended naming conventions.

### Annotations with V1 Equivalents (Migrated)

The following annotations had old V1 formats that are now deprecated:

| Old Annotation (Deprecated)                      | New Annotation (V2)                     | Status   |
| ------------------------------------------------ | --------------------------------------- | -------- |
| `io.kubernetes.cri-o.Devices`                    | `devices.crio.io`                       | Migrated |
| `io.kubernetes.cri-o.DisableFIPS`                | `disable-fips.crio.io`                  | Migrated |
| `io.kubernetes.cri-o.LinkLogs`                   | `link-logs.crio.io`                     | Migrated |
| `io.kubernetes.cri-o.PlatformRuntimePath`        | `platform-runtime-path.crio.io`         | Migrated |
| `io.kubernetes.cri-o.PodLinuxOverhead`           | `pod-linux-overhead.crio.io`            | Migrated |
| `io.kubernetes.cri-o.PodLinuxResources`          | `pod-linux-resources.crio.io`           | Migrated |
| `io.kubernetes.cri-o.ShmSize`                    | `shm-size.crio.io`                      | Migrated |
| `io.kubernetes.cri-o.Spoofed`                    | `spoofed.crio.io`                       | Migrated |
| `io.kubernetes.cri-o.TrySkipVolumeSELinuxLabel`  | `try-skip-volume-selinux-label.crio.io` | Migrated |
| `io.kubernetes.cri-o.UnifiedCgroup`              | `unified-cgroup.crio.io`                | Migrated |
| `io.kubernetes.cri-o.cgroup2-mount-hierarchy-rw` | `cgroup2-mount-hierarchy-rw.crio.io`    | Migrated |
| `io.kubernetes.cri-o.seccompNotifierAction`      | `seccomp-notifier-action.crio.io`       | Migrated |
| `io.kubernetes.cri-o.umask`                      | `umask.crio.io`                         | Migrated |
| `io.kubernetes.cri-o.userns-mode`                | `userns-mode.crio.io`                   | Migrated |
| `seccomp-profile.kubernetes.cri-o.io`            | `seccomp-profile.crio.io`               | Migrated |

### Annotations Already Following Convention (Now in V2 Package)

The following annotations were already following the recommended format.
They have been moved to the v2 package for consistency, making it the
complete successor to pkg/annotations:

- `cpu-c-states.crio.io`
- `cpu-freq-governor.crio.io`
- `cpu-load-balancing.crio.io`
- `cpu-quota.crio.io`
- `cpu-shared.crio.io`
- `irq-load-balancing.crio.io`

### External Annotations (Now in V2 Package)

The following external annotations have also been included in the v2 package:

- `io.containers.trace-syscall` - Used by OCI seccomp BPF hook
- `org.opencontainers.image.stopSignal` - Defined in OCI image-spec

## Backwards Compatibility

### For Users

Both old and new annotation formats are supported during the deprecation
phase. When both formats are present, the new V2 format takes precedence.

**Example:**

```yaml
apiVersion: v1
kind: Pod
metadata:
  annotations:
    # New format (recommended)
    userns-mode.crio.io: "auto"

    # Old format (still works, but deprecated)
    io.kubernetes.cri-o.userns-mode: "host"
```

In this example, CRI-O will use `"auto"` because the V2 annotation takes
precedence.

### For Developers

CRI-O provides helper functions for accessing annotations with automatic
fallback. The new V2 annotations are available in a separate package:

```go
import (
    v2 "github.com/cri-o/cri-o/pkg/annotations/v2"
)

// Using the V2 key - automatically checks V2 first, then V1 fallback
value, ok := v2.GetAnnotationValue(podAnnotations, v2.UsernsMode)
```

The `v2.GetAnnotationValue()` function:

- Prefers the V2 annotation if present
- Falls back to the V1 annotation if V2 is not present
- Returns `("", false)` if neither is present
- Works with both base annotations and container-specific annotations
  (e.g., `unified-cgroup.crio.io/<container-name>` or `seccomp-profile.crio.io/<container-name>`)

**Note:** A deprecated wrapper function `annotations.GetAnnotationValue()`
exists in the `pkg/annotations` package for backwards compatibility, but
new code should use `v2.GetAnnotationValue()` directly.

The V2 annotations are defined in `pkg/annotations/v2` package to avoid
confusion and maintain consistency with how metrics are organized.

## Migration Timeline

### Phase 1: Deprecation and Adoption (Current)

- ✅ New V2 annotations added
- ✅ Old V1 annotations marked as deprecated
- ✅ Both formats supported with V2 taking precedence
- ✅ Helper functions added for backwards compatibility
- ✅ Codebase migrated to use `v2.GetAnnotationValue()` directly
- ✅ Documentation updated to recommend V2 format
- ✅ Examples and tutorials updated to use V2 annotations

### Phase 2: Removal (Future)

- Remove support for V1 annotations
  (TBD - after sufficient adoption period)

## Usage Examples

### User Namespace Mode

```yaml
# Old (deprecated)
metadata:
  annotations:
    io.kubernetes.cri-o.userns-mode: "auto"

# New (recommended)
metadata:
  annotations:
    userns-mode.crio.io: "auto"
```

### Shared Memory Size

```yaml
# Old (deprecated)
metadata:
  annotations:
    io.kubernetes.cri-o.ShmSize: "128Mi"

# New (recommended)
metadata:
  annotations:
    shm-size.crio.io: "128Mi"
```

### Device Access

```yaml
# Old (deprecated)
metadata:
  annotations:
    io.kubernetes.cri-o.Devices: "/dev/fuse"

# New (recommended)
metadata:
  annotations:
    devices.crio.io: "/dev/fuse"
```

### Seccomp Profile

```yaml
# Old (deprecated) - for a specific container
metadata:
  annotations:
    seccomp-profile.kubernetes.cri-o.io/my-container:
      "runtime/default"

# New (recommended) - for a specific container
metadata:
  annotations:
    seccomp-profile.crio.io/my-container: "runtime/default"

# For the whole pod
metadata:
  annotations:
    seccomp-profile.crio.io/POD: "runtime/default"
```

## Testing

Comprehensive tests have been added to verify:

- V2 annotations work correctly
- V1 annotations still work (backwards compatibility)
- V2 annotations take precedence when both are present
- Helper functions correctly handle all cases

Run the tests:

```bash
go test ./pkg/annotations/... -v
```

## References

- [Kubernetes Annotation Conventions][k8s-annotations]
- [GitHub Issue #7781](https://github.com/cri-o/cri-o/issues/7781)

[k8s-annotations]: https://kubernetes.io/docs/concepts/overview/working-with-objects/annotations/
