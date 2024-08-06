package capnp

import (
	"errors"
	"sync"

	"capnproto.org/go/capnp/v3/exp/bufferpool"
	"capnproto.org/go/capnp/v3/internal/str"
)

// An Arena loads and allocates segments for a Message.
type Arena interface {
	// NumSegments returns the number of segments in the arena.
	// This must not be larger than 1<<32.
	NumSegments() int64

	// Data loads the data for the segment with the given ID.  IDs are in
	// the range [0, NumSegments()).
	// must be tightly packed in the range [0, NumSegments()).
	Data(id SegmentID) ([]byte, error)

	// Allocate selects a segment to place a new object in, creating a
	// segment or growing the capacity of a previously loaded segment if
	// necessary.  If Allocate does not return an error, then the
	// difference of the capacity and the length of the returned slice
	// must be at least minsz.  segs is a map of segments keyed by ID
	// using arrays returned by the Data method (although the length of
	// these slices may have changed by previous allocations).  Allocate
	// must not modify segs.
	//
	// If Allocate creates a new segment, the ID must be one larger than
	// the last segment's ID or zero if it is the first segment.
	//
	// If Allocate returns an previously loaded segment's ID, then the
	// arena is responsible for preserving the existing data in the
	// returned byte slice.
	Allocate(minsz Size, segs map[SegmentID]*Segment) (SegmentID, []byte, error)

	// Release all resources associated with the Arena. Callers MUST NOT
	// use the Arena after it has been released.
	//
	// Calling Release() is OPTIONAL, but may reduce allocations.
	//
	// Implementations MAY use Release() as a signal to return resources
	// to free lists, or otherwise reuse the Arena.   However, they MUST
	// NOT assume Release() will be called.
	Release()
}

// SingleSegmentArena is an Arena implementation that stores message data
// in a continguous slice.  Allocation is performed by first allocating a
// new slice and copying existing data. SingleSegment arena does not fail
// unless the caller attempts to access another segment.
type SingleSegmentArena []byte

// SingleSegment constructs a SingleSegmentArena from b.  b MAY be nil.
// Callers MAY use b to populate the segment for reading, or to reserve
// memory of a specific size.
func SingleSegment(b []byte) *SingleSegmentArena {
	return (*SingleSegmentArena)(&b)
}

func (ssa SingleSegmentArena) NumSegments() int64 {
	return 1
}

func (ssa SingleSegmentArena) Data(id SegmentID) ([]byte, error) {
	if id != 0 {
		return nil, errors.New("segment " + str.Utod(id) + " requested in single segment arena")
	}
	return ssa, nil
}

func (ssa *SingleSegmentArena) Allocate(sz Size, segs map[SegmentID]*Segment) (SegmentID, []byte, error) {
	data := []byte(*ssa)
	if segs[0] != nil {
		data = segs[0].data
	}
	if len(data)%int(wordSize) != 0 {
		return 0, nil, errors.New("segment size is not a multiple of word size")
	}
	if hasCapacity(data, sz) {
		return 0, data, nil
	}
	inc, err := nextAlloc(int64(len(data)), int64(maxAllocSize()), sz)
	if err != nil {
		return 0, nil, err
	}
	buf := bufferpool.Default.Get(cap(data) + inc)
	copied := copy(buf, data)
	buf = buf[:copied]
	bufferpool.Default.Put(data)
	*ssa = buf
	return 0, *ssa, nil
}

func (ssa SingleSegmentArena) String() string {
	return "single-segment arena [len=" + str.Itod(len(ssa)) + " cap=" + str.Itod(cap(ssa)) + "]"
}

// Return this arena to an internal sync.Pool of arenas that can be
// re-used. Any time SingleSegment(nil) is called, arenas from this
// pool will be used if available, which can help reduce memory
// allocations.
//
// All segments will be zeroed before re-use.
//
// Calling Release is optional; if not done the garbage collector
// will release the memory per usual.
func (ssa *SingleSegmentArena) Release() {
	bufferpool.Default.Put(*ssa)
	*ssa = nil
}

// MultiSegment is an arena that stores object data across multiple []byte
// buffers, allocating new buffers of exponentially-increasing size when
// full. This avoids the potentially-expensive slice copying of SingleSegment.
type MultiSegmentArena struct {
	ss    [][]byte
	delim int    // index of first segment in ss that is NOT in buf
	buf   []byte // full-sized buffer that was demuxed into ss.
}

// MultiSegment returns a new arena that allocates new segments when
// they are full.  b MAY be nil.  Callers MAY use b to populate the
// buffer for reading or to reserve memory of a specific size.
func MultiSegment(b [][]byte) *MultiSegmentArena {
	if b == nil {
		return multiSegmentPool.Get().(*MultiSegmentArena)
	}
	return multiSegment(b)
}

// Return this arena to an internal sync.Pool of arenas that can be
// re-used. Any time MultiSegment(nil) is called, arenas from this
// pool will be used if available, which can help reduce memory
// allocations.
//
// All segments will be zeroed before re-use.
//
// Calling Release is optional; if not done the garbage collector
// will release the memory per usual.
func (msa *MultiSegmentArena) Release() {
	for i, v := range msa.ss {
		msa.ss[i] = nil

		// segment not in buf?
		if i >= msa.delim {
			bufferpool.Default.Put(v)
		}
	}

	bufferpool.Default.Put(msa.buf) // nil is ok
	*msa = MultiSegmentArena{ss: msa.ss[:0]}
	multiSegmentPool.Put(msa)
}

// Like MultiSegment, but doesn't use the pool
func multiSegment(b [][]byte) *MultiSegmentArena {
	return &MultiSegmentArena{ss: b}
}

var multiSegmentPool = sync.Pool{
	New: func() any {
		return multiSegment(make([][]byte, 0, 16))
	},
}

// demuxArena slices data into a multi-segment arena.  It assumes that
// len(data) >= hdr.totalSize().
func (msa *MultiSegmentArena) demux(hdr streamHeader, data []byte) error {
	maxSeg := hdr.maxSegment()
	if int64(maxSeg) > int64(maxInt-1) {
		return errors.New("number of segments overflows int")
	}

	msa.buf = data
	msa.delim = int(maxSeg + 1)

	// We might be forced to allocate here, but hopefully it won't
	// happen to often.  We assume msa was freshly obtained from a
	// pool, and that no segments have been allocated yet.
	var segment []byte
	for i := 0; i < msa.delim; i++ {
		sz, err := hdr.segmentSize(SegmentID(i))
		if err != nil {
			return err
		}

		segment, data = data[:sz:sz], data[sz:]
		msa.ss = append(msa.ss, segment)
	}

	return nil
}

func (msa *MultiSegmentArena) NumSegments() int64 {
	return int64(len(msa.ss))
}

func (msa *MultiSegmentArena) Data(id SegmentID) ([]byte, error) {
	if int64(id) >= int64(len(msa.ss)) {
		return nil, errors.New("segment " + str.Utod(id) + " requested (arena only has " +
			str.Itod(len(msa.ss)) + " segments)")
	}
	return msa.ss[id], nil
}

func (msa *MultiSegmentArena) Allocate(sz Size, segs map[SegmentID]*Segment) (SegmentID, []byte, error) {
	var total int64
	for i, data := range msa.ss {
		id := SegmentID(i)
		if s := segs[id]; s != nil {
			data = s.data
		}

		if hasCapacity(data, sz) {
			return id, data, nil
		}

		if total += int64(cap(data)); total < 0 {
			// Overflow.
			return 0, nil, errors.New("alloc " + str.Utod(sz) + " bytes: message too large")
		}
	}

	n, err := nextAlloc(total, 1<<63-1, sz)
	if err != nil {
		return 0, nil, err
	}

	buf := bufferpool.Default.Get(n)
	buf = buf[:0]

	id := SegmentID(len(msa.ss))
	msa.ss = append(msa.ss, buf)
	return id, buf, nil
}

func (msa *MultiSegmentArena) String() string {
	return "multi-segment arena [" + str.Itod(len(msa.ss)) + " segments]"
}

// nextAlloc computes how much more space to allocate given the number
// of bytes allocated in the entire message and the requested number of
// bytes.  It will always return a multiple of wordSize.  max must be a
// multiple of wordSize.  The sum of curr and the returned size will
// always be less than max.
func nextAlloc(curr, max int64, req Size) (int, error) {
	if req == 0 {
		return 0, nil
	}
	if req > maxAllocSize() {
		return 0, errors.New("alloc " + req.String() + ": too large")
	}
	padreq := req.padToWord()
	want := curr + int64(padreq)
	if want <= curr || want > max {
		return 0, errors.New("alloc " + req.String() + ": message size overflow")
	}
	new := curr
	double := new + new
	switch {
	case want < 1024:
		next := (1024 - curr + 7) &^ 7
		if next < curr {
			return int((curr + 7) &^ 7), nil
		}
		return int(next), nil
	case want > double:
		return int(padreq), nil
	default:
		for 0 < new && new < want {
			new += new / 4
		}
		if new <= 0 {
			return int(padreq), nil
		}
		delta := new - curr
		if delta > int64(maxAllocSize()) {
			return int(maxAllocSize()), nil
		}
		return int((delta + 7) &^ 7), nil
	}
}

func hasCapacity(b []byte, sz Size) bool {
	return sz <= Size(cap(b)-len(b))
}
