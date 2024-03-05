package container

// Impl is the main implementation interface of this package.
type Impl interface {
	securityLabel(path, secLabel string, shared, maybeRelabel bool) error
}
