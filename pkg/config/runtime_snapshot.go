package config

// RuntimeSnapshot is an immutable view of the runtime handler table and the
// default runtime, published as a single unit. Readers load one snapshot via
// Config.RuntimeSnapshot instead of reading the live RuntimeConfig fields, so
// a reload can never expose a torn combination of a new Runtimes table with
// an old (or no longer existing) DefaultRuntime, nor a partially applied
// reload.
type RuntimeSnapshot struct {
	// Runtimes is the runtime handler table of the snapshot.
	Runtimes Runtimes

	// DefaultRuntime is the name of the default runtime handler of the
	// snapshot.
	DefaultRuntime string
}

// RuntimeHandler returns the runtime handler of the snapshot for the
// provided name. If the name is empty, the default runtime handler of the
// snapshot is returned. It returns nil if the handler does not exist.
func (s *RuntimeSnapshot) RuntimeHandler(name string) *RuntimeHandler {
	if name == "" {
		name = s.DefaultRuntime
	}

	return s.Runtimes[name]
}

// publishRuntimeSnapshot atomically publishes the current runtime
// configuration as the active snapshot. It is called on startup by
// Config.Validate and on every successful runtime reload, so that readers
// only ever see fully validated runtime configurations.
func (c *Config) publishRuntimeSnapshot() {
	c.runtimeSnapshot.Store(&RuntimeSnapshot{
		Runtimes:       c.RuntimeConfig.Runtimes,
		DefaultRuntime: c.RuntimeConfig.DefaultRuntime,
	})
}

// PublishRuntimeSnapshot publishes the current runtime configuration as
// the active snapshot, see RuntimeSnapshot.
func (c *Config) PublishRuntimeSnapshot() {
	c.publishRuntimeSnapshot()
}

// RuntimeSnapshot returns the currently active runtime snapshot. The
// snapshot is published by Config.Validate on startup and refreshed
// atomically by Config.ReloadRuntimes, which means that readers never
// observe a runtime configuration in an intermediate state.
//
// If no snapshot has been published yet, which is only the case before
// Config.Validate ran (unit tests, `crio config` and similar offline code
// paths), a snapshot of the current runtime configuration is returned
// without publishing it.
func (c *Config) RuntimeSnapshot() *RuntimeSnapshot {
	if snapshot := c.runtimeSnapshot.Load(); snapshot != nil {
		return snapshot
	}

	return &RuntimeSnapshot{
		Runtimes:       c.RuntimeConfig.Runtimes,
		DefaultRuntime: c.RuntimeConfig.DefaultRuntime,
	}
}
