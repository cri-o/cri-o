package rpc

import "capnproto.org/go/capnp/v3"

type releaseList []capnp.ReleaseFunc

func (rl *releaseList) Release() {
	funcs := *rl
	for i, r := range funcs {
		if r != nil {
			r()
			funcs[i] = nil
		}
	}
}

func (rl *releaseList) Add(r capnp.ReleaseFunc) {
	*rl = append(*rl, r)
}
