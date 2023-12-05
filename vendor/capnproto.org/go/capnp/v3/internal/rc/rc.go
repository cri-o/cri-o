package rc

import "sync/atomic"

type Releaser struct {
	release  func()
	refcount int32
}

func NewReleaser(init int32, release func()) *Releaser {
	return &Releaser{
		release:  release,
		refcount: init,
	}
}

func (rc *Releaser) Decr() {
	newCount := atomic.AddInt32(&rc.refcount, -1)
	if newCount == 0 {
		rc.release()
		rc.release = nil
	} else if newCount < 0 {
		panic("Decremented an already-zero refcount")
	}
}

func (rc *Releaser) Incr() {
	newCount := atomic.AddInt32(&rc.refcount, 1)
	if newCount == 1 {
		panic("Incremented an already-zero refcount")
	}
}
