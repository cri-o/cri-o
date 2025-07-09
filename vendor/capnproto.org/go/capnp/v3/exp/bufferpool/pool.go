// Package bufferpool supports object pooling for byte buffers.
package bufferpool

import (
	"math/bits"

	"github.com/colega/zeropool"
)

const (
	defaultMinSize     = 1024
	defaultBucketCount = 20
)

// A default global pool.
//
// This pool defaults to bucketCount=20 and minAlloc=1024 (max size = ~1MiB).
var Default Pool = *NewPool(defaultMinSize, defaultBucketCount)

// Pool maintains a list of buffers, exponentially increasing in size. Values
// MUST be initialized by NewPool().
//
// Buffer instances are safe for concurrent access.
type Pool struct {
	minAlloc int
	buckets  bucketSlice
}

// Get a buffer of len(buf) == size and cap >= size.
func (p *Pool) Get(size int) []byte {
	if buf := p.buckets.Get(size); buf != nil {
		return buf[:size]
	}

	return make([]byte, size)
}

// Put returns the buffer to the pool.  The first len(buf) bytes
// of the buffer are zeroed.
func (p *Pool) Put(buf []byte) {
	for i := range buf {
		buf[i] = 0
	}

	// Do not store buffers less than the min alloc size (prevents storing
	// buffers that do not conform to the min alloc policy of this pool).
	if cap(buf) < p.minAlloc {
		return
	}

	p.buckets.Put(buf[:cap(buf)])
}

// NewPool creates a list of BucketCount buckets that contain buffers
// of exponentially-increasing capacity, 1 << 0 to 1 << BucketCount.
//
// The minAlloc field specifies the minimum capacity of new buffers
// allocated by Pool, which improves reuse of small buffers. For the
// avoidance of doubt:  calls to Get() with size < minAlloc return a
// buffer of len(buf) = size and cap(buf) >= minAlloc. MinAlloc MUST
// NOT exceed 1 << BucketCount, or method calls to Pool will panic.
//
// Passing zero to the parameters will default bucketCount to 20
// and minAlloc to 1024 (max size = ~1MiB).
//
// As a general rule, increasing MinAlloc reduces GC latency at the
// expense of increased memory usage.  Increasing BucketCount can
// reduce GC latency in applications that frequently allocate large
// buffers.
func NewPool(minAlloc, bucketCount int) *Pool {
	if minAlloc <= 0 {
		minAlloc = defaultMinSize
	}

	if bucketCount <= 0 {
		bucketCount = defaultBucketCount
	}

	if minAlloc > (1 << bucketCount) {
		panic("MinAlloc greater than largest bucket")
	}

	if !isPowerOf2(minAlloc) {
		panic("MinAlloc not a power of two")
	}

	return &Pool{
		minAlloc: minAlloc,
		buckets:  makeBucketSlice(minAlloc, bucketCount),
	}
}

type bucketSlice []*zeropool.Pool[[]byte]

func isPowerOf2(i int) bool {
	return i&(i-1) == 0
}

func bucketToGet(size int) int {
	i := bits.Len(uint(size))
	if isPowerOf2(size) && size > 0 {
		// When the size is a power of two, reduce by one (because
		// bucket i is for sizes <= 1<< i).
		i -= 1
	}
	return i
}

func bucketToPut(size int) int {
	i := bits.Len(uint(size))

	// Always put on the bucket whose upper bound is size == 1<<i.
	i -= 1
	return i
}

func (bs bucketSlice) Get(size int) []byte {
	i := bucketToGet(size)
	if i < len(bs) {
		r := bs[i].Get()
		return r
	}
	return nil
}

func (bs bucketSlice) Put(buf []byte) {
	i := bucketToPut(cap(buf))
	if i < len(bs) {
		bs[i].Put(buf)
	}
}

// makeBucketSlice creates a new bucketSlice with the given parameters. These
// are NOT validated.
func makeBucketSlice(minAlloc, bucketCount int) bucketSlice {
	// Create all buckets that are >= the bucket that stores the min
	// allocation size.
	minBucket := bucketToGet(minAlloc)
	buckets := make(bucketSlice, bucketCount)
	for i := minBucket; i < bucketCount; i++ {
		bp := zeropool.New(newAllocFuncForBucket(i))
		buckets[i] = &bp
	}

	// Buckets smaller than the min bucket size all get/put buffers in the
	// minimum bucket size.
	for i := 0; i < minBucket; i++ {
		buckets[i] = buckets[minBucket]
	}

	return buckets
}

// newAllocFuncForBucket returns a function to allocate a byte slice of size
// 2^i.
func newAllocFuncForBucket(i int) func() []byte {
	return func() []byte {
		return make([]byte, 1<<i)
	}
}
