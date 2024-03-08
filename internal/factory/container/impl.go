package container

// Impl is the main implementation interface of this package.
type Impl interface {
	SecurityLabel(path, secLabel string, shared, maybeRelabel bool) error
}
