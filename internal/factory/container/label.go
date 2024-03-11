package container

type SecLabel struct {
	impl Impl
}

type secLabelImpl struct{}

func newSecLabel() *SecLabel {
	return &SecLabel{
		impl: &secLabelImpl{},
	}
}

func SecurityLabel(path, secLabel string, shared, maybeRelabel bool) error {
	return newSecLabel().impl.SecurityLabel(path, secLabel, shared, maybeRelabel)
}
