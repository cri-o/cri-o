package container

type secLabel struct {
	impl Impl
}

type secLabelImpl struct{}

func newSecLabel() *secLabel {
	return &secLabel{
		impl: &secLabelImpl{},
	}
}

func SecurityLabel(path, secLabel string, shared, maybeRelabel bool) error {
	return newSecLabel().impl.securityLabel(path, secLabel, shared, maybeRelabel)
}
