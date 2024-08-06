// Package bufferpool supports object pooling for byte buffers.
package bufferpool

import (
	"sync"

	"github.com/colega/zeropool"
)

const (
	defaultMinSize     = 1024
	defaultBucketCount = 20
)

// A default global pool.
var Default Pool

// Pool maintains a list of BucketCount buckets that contain buffers
// of exponentially-increasing capacity, 1 << 0 to 1 << BucketCount.
//
// The MinAlloc field specifies the minimum capacity of new buffers
// allocated by Pool, which improves reuse of small buffers. For the
// avoidance of doubt:  calls to Get() with size < MinAlloc return a
// buffer of len(buf) = size and cap(buf) >= MinAlloc. MinAlloc MUST
// NOT exceed 1 << BucketCount, or method calls to Pool will panic.
//
// The zero-value Pool is ready to use, defaulting to BucketCount=20
// and MinAlloc=1024 (max size = ~1MiB).  Most applications will not
// benefit from tuning these parameters.
//
// As a general rule, increasing MinAlloc reduces GC latency at the
// expense of increased memory usage.  Increasing BucketCount can
// reduce GC latency in applications that frequently allocate large
// buffers.
type Pool struct {
	once                  sync.Once
	MinAlloc, BucketCount int
	buckets               bucketSlice
}

// Get a buffer of len(buf) == size and cap >= size.
func (p *Pool) Get(size int) []byte {
	p.init()

	if buf := p.buckets.Get(size); buf != nil {
		return buf[:size]
	}

	return make([]byte, size)
}

// Put returns the buffer to the pool.  The first len(buf) bytes
// of the buffer are zeroed.
func (p *Pool) Put(buf []byte) {
	p.init()

	for i := range buf {
		buf[i] = 0
	}

	p.buckets.Put(buf[:cap(buf)])
}

func (p *Pool) init() {
	p.once.Do(func() {
		if p.MinAlloc <= 0 {
			p.MinAlloc = defaultMinSize
		}

		if p.BucketCount <= 0 {
			p.BucketCount = defaultBucketCount
		}

		if p.MinAlloc > (1 << p.BucketCount) {
			panic("MinAlloc greater than largest bucket")
		}

		// Get the index of the bucket responsible for MinAlloc.
		var idx int
		for idx = range p.buckets {
			if 1<<idx >= p.MinAlloc {
				break
			}
		}

		p.buckets = make(bucketSlice, p.BucketCount)
		for i := range p.buckets {
			if i < idx {
				// Set the 'New' function for all "small" buckets to
				// n.buckets[idx].Get, so as to allow reuse of buffers
				// smaller than MinAlloc that are passed to Put, while
				// still maximizing reuse of buffers allocated by Get.
				// Note that we cannot simply use n.buckets[idx].New,
				// as this would side-step pooling.
				p.buckets[i] = zeropool.New(p.buckets[idx].Get)
			} else {
				p.buckets[i] = zeropool.New(newAllocFunc(i))
			}
		}
	})
}

type bucketSlice []zeropool.Pool[[]byte]

func (bs bucketSlice) Get(size int) []byte {
	for i := range bs {
		if 1<<i >= size {
			return bs[i].Get()
		}
	}

	return nil
}

func (bs bucketSlice) Put(buf []byte) {
	for i := range bs {
		if cap(buf) >= 1<<i && cap(buf) < 1<<(i+1) {
			bs[i].Put(buf)
			break
		}
	}
}

func newAllocFunc(i int) func() []byte {
	return func() []byte {
		return make([]byte, 1<<i)
	}
}
