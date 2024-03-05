package container

type secLabel struct {
	impl Impl
}

var slabel *secLabel = &secLabel{}

func SecurityLabel(path, secLabel string, shared, maybeRelabel bool) error {
	return slabel.securityLabel(path, secLabel, shared, maybeRelabel)
}
