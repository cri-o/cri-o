# kubensmnt

[![Integration Test](https://github.com/containers/kubensmnt/actions/workflows/integration-test.yml/badge.svg)](https://github.com/containers/kubensmnt/actions/workflows/integration-test.yml)
[![ShellCheck](https://github.com/containers/kubensmnt/actions/workflows/shellcheck.yml/badge.svg)](https://github.com/containers/kubensmnt/actions/workflows/shellcheck.yml)

A small library to enable go programs to join a new mount namespace, designed
for helping get the Kubernetes control plane (kubelet and the container
runtime) into a separate mountpoint.

## Rationale

There are benefits to hiding all of Kubernetes' mount points from the host OS,
including cleanliness, safety from inspection, and reducing the load on system
processes like systemd that may need to interact with all mount in the default
namespace.

# How to use

Include this library in your main.go:

```go
import "github.com/containers/kubensmnt"
```

This will cause a C constructor function to run before the Go runtime fully
initializes which will do the following:
- If `$KUBENSMNT` is not set in the environment, do nothing.
- If `$KUBENSMNT` is set in the environment, and it points at a valid path that
  is a bind-mount to a mount namespace, join that mount namespace.
  - If there is an error finding the bindmount path or joining the namespace,
    the error is recorded and can be retrieved via the `Status` call.

Inside the Go code, you can then check what happened during init and take
actions accordingly:

```go
func main() {
    path, err := kubensmnt.Status()
    if err != nil {
        panic(err)
    }
    if path == "" {
        fmt.Println("No mount namespace was configured; no action was taken")
    } else {
        fmt.Printf("Successfully joined the namespace bound to %q\n", path)
    }
    // Go on to do more important things...
}
```

# Running in a separate mount namespace

See the explanation and systemd examples in [utils/systemd](utils/systemd/)

# Testing

See [test/README.md](test/README.md)
