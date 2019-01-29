package namespaces

import (
	"strings"
)

// UsernsMode represents userns mode in the container.
type UsernsMode string

// IsHost indicates whether the container uses the host's userns.
func (n UsernsMode) IsHost() bool {
	return n == "host"
}

// IsPrivate indicates whether the container uses the a private userns.
func (n UsernsMode) IsPrivate() bool {
	return !(n.IsHost())
}

// Valid indicates whether the userns is valid.
func (n UsernsMode) Valid() bool {
	parts := strings.Split(string(n), ":")
	switch mode := parts[0]; mode {
	case "", "host":
	default:
		return false
	}
	return true
}

// IsContainer indicates whether container uses a container userns.
func (n UsernsMode) IsContainer() bool {
	return false
}

// Container is the id of the container which network this container is connected to.
func (n UsernsMode) Container() string {
	return ""
}

// UTSMode represents the UTS namespace of the container.
type UTSMode string

// IsPrivate indicates whether the container uses its private UTS namespace.
func (n UTSMode) IsPrivate() bool {
	return !(n.IsHost())
}

// IsHost indicates whether the container uses the host's UTS namespace.
func (n UTSMode) IsHost() bool {
	return n == "host"
}

// IsContainer indicates whether the container uses a container's UTS namespace.
func (n UTSMode) IsContainer() bool {
	parts := strings.SplitN(string(n), ":", 2)
	return len(parts) > 1 && parts[0] == "container"
}

// Container returns the name of the container whose uts namespace is going to be used.
func (n UTSMode) Container() string {
	parts := strings.SplitN(string(n), ":", 2)
	if len(parts) > 1 {
		return parts[1]
	}
	return ""
}

// Valid indicates whether the UTS namespace is valid.
func (n UTSMode) Valid() bool {
	parts := strings.Split(string(n), ":")
	switch mode := parts[0]; mode {
	case "", "host":
	case "container":
		if len(parts) != 2 || parts[1] == "" {
			return false
		}
	default:
		return false
	}
	return true
}

// IpcMode represents the container ipc stack.
type IpcMode string

// IsPrivate indicates whether the container uses its own private ipc namespace which cannot be shared.
func (n IpcMode) IsPrivate() bool {
	return n == "private"
}

// IsHost indicates whether the container shares the host's ipc namespace.
func (n IpcMode) IsHost() bool {
	return n == "host"
}

// IsShareable indicates whether the container's ipc namespace can be shared with another container.
func (n IpcMode) IsShareable() bool {
	return n == "shareable"
}

// IsContainer indicates whether the container uses another container's ipc namespace.
func (n IpcMode) IsContainer() bool {
	parts := strings.SplitN(string(n), ":", 2)
	return len(parts) > 1 && parts[0] == "container"
}

// IsNone indicates whether container IpcMode is set to "none".
func (n IpcMode) IsNone() bool {
	return n == "none"
}

// IsEmpty indicates whether container IpcMode is empty
func (n IpcMode) IsEmpty() bool {
	return n == ""
}

// Valid indicates whether the ipc mode is valid.
func (n IpcMode) Valid() bool {
	return n.IsEmpty() || n.IsNone() || n.IsPrivate() || n.IsHost() || n.IsShareable() || n.IsContainer()
}

// Container returns the name of the container ipc stack is going to be used.
func (n IpcMode) Container() string {
	parts := strings.SplitN(string(n), ":", 2)
	if len(parts) > 1 && parts[0] == "container" {
		return parts[1]
	}
	return ""
}

// PidMode represents the pid namespace of the container.
type PidMode string

// IsPrivate indicates whether the container uses its own new pid namespace.
func (n PidMode) IsPrivate() bool {
	return !(n.IsHost() || n.IsContainer())
}

// IsHost indicates whether the container uses the host's pid namespace.
func (n PidMode) IsHost() bool {
	return n == "host"
}

// IsContainer indicates whether the container uses a container's pid namespace.
func (n PidMode) IsContainer() bool {
	parts := strings.SplitN(string(n), ":", 2)
	return len(parts) > 1 && parts[0] == "container"
}

// Valid indicates whether the pid namespace is valid.
func (n PidMode) Valid() bool {
	parts := strings.Split(string(n), ":")
	switch mode := parts[0]; mode {
	case "", "host":
	case "container":
		if len(parts) != 2 || parts[1] == "" {
			return false
		}
	default:
		return false
	}
	return true
}

// Container returns the name of the container whose pid namespace is going to be used.
func (n PidMode) Container() string {
	parts := strings.SplitN(string(n), ":", 2)
	if len(parts) > 1 {
		return parts[1]
	}
	return ""
}

// NetworkMode represents the container network stack.
type NetworkMode string

// IsNone indicates whether container isn't using a network stack.
func (n NetworkMode) IsNone() bool {
	return n == "none"
}

// IsHost indicates whether the container uses the host's network stack.
func (n NetworkMode) IsHost() bool {
	return n == "host"
}

// IsDefault indicates whether container uses the default network stack.
func (n NetworkMode) IsDefault() bool {
	return n == "default"
}

// IsPrivate indicates whether container uses its private network stack.
func (n NetworkMode) IsPrivate() bool {
	return !(n.IsHost() || n.IsContainer())
}

// IsContainer indicates whether container uses a container network stack.
func (n NetworkMode) IsContainer() bool {
	parts := strings.SplitN(string(n), ":", 2)
	return len(parts) > 1 && parts[0] == "container"
}

// Container is the id of the container which network this container is connected to.
func (n NetworkMode) Container() string {
	parts := strings.SplitN(string(n), ":", 2)
	if len(parts) > 1 {
		return parts[1]
	}
	return ""
}

//UserDefined indicates user-created network
func (n NetworkMode) UserDefined() string {
	if n.IsUserDefined() {
		return string(n)
	}
	return ""
}

// IsBridge indicates whether container uses the bridge network stack
func (n NetworkMode) IsBridge() bool {
	return n == "bridge"
}

// IsSlirp4netns indicates if we are running a rootless network stack
func (n NetworkMode) IsSlirp4netns() bool {
	return n == "slirp4netns"
}

// IsUserDefined indicates user-created network
func (n NetworkMode) IsUserDefined() bool {
	return !n.IsDefault() && !n.IsBridge() && !n.IsHost() && !n.IsNone() && !n.IsContainer() && !n.IsSlirp4netns()
}
