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

	// Segment returns the segment identified with the specified id. This
	// may return nil if the segment with the specified ID does not exist.
	Segment(id SegmentID) *Segment

	// Allocate selects a segment to place a new object in, creating a
	// segment or growing the capacity of a previously loaded segment if
	// necessary.  If Allocate does not return an error, then the returned
	// segment may store up to minsz bytes starting at the returned address
	// offset.
	//
	// Some allocators may specifically choose to grow the passed seg (if
	// non nil), but that is not a requirement.
	//
	// If Allocate creates a new segment, the ID must be one larger than
	// the last segment's ID or zero if it is the first segment.
	//
	// If Allocate returns an previously loaded segment, then the arena is
	// responsible for preserving the existing data.
	Allocate(minsz Size, msg *Message, seg *Segment) (*Segment, address, error)

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

// singleSegmentPool is a pool of *SingleSegmentArena.
var singleSegmentPool = sync.Pool{
	New: func() any {
		return &SingleSegmentArena{}
	},
}

// SingleSegmentArena is an Arena implementation that stores message data
// in a continguous slice.  Allocation is performed by first allocating a
// new slice and copying existing data. SingleSegment arena does not fail
// unless the caller attempts to access another segment.
type SingleSegmentArena struct {
	seg Segment

	// bp is the bufferpool assotiated with this arena if it was initialized
	// for writing.
	bp *bufferpool.Pool

	// fromPool determines if this should return to the pool when released.
	fromPool bool
}

func zeroSlice(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

// SingleSegment constructs a SingleSegmentArena from b.  b MAY be nil.
// Callers MAY use b to populate the segment for reading, or to reserve
// memory of a specific size.
func SingleSegment(b []byte) Arena {
	if b == nil {
		ssa := singleSegmentPool.Get().(*SingleSegmentArena)
		ssa.fromPool = true
		ssa.bp = &bufferpool.Default
		return ssa
	}

	return &SingleSegmentArena{seg: Segment{data: b}}
}

func (ssa *SingleSegmentArena) NumSegments() int64 {
	return 1
}

func (ssa *SingleSegmentArena) Segment(id SegmentID) *Segment {
	if id != 0 {
		return nil
	}
	return &ssa.seg
}

func (ssa *SingleSegmentArena) Allocate(sz Size, msg *Message, seg *Segment) (*Segment, address, error) {
	if seg != nil && seg != &ssa.seg {
		return nil, 0, errors.New("segment is not associated with arena")
	}
	data := ssa.seg.data
	if len(data)%int(wordSize) != 0 {
		return nil, 0, errors.New("segment size is not a multiple of word size")
	}
	ssa.seg.BindTo(msg)
	if hasCapacity(data, sz) {
		addr := address(len(ssa.seg.data))
		ssa.seg.data = ssa.seg.data[:len(ssa.seg.data)+int(sz)]
		return &ssa.seg, addr, nil
	}
	inc, err := nextAlloc(int64(len(data)), int64(maxAllocSize()), sz)
	if err != nil {
		return nil, 0, err
	}
	if ssa.bp == nil {
		return nil, 0, errors.New("cannot allocate on read-only SingleSegmentArena")
	}
	addr := address(len(ssa.seg.data))
	ssa.seg.data = ssa.bp.Get(cap(data) + inc)[:len(data)+int(sz)]
	copy(ssa.seg.data, data)
	zeroSlice(data)
	ssa.bp.Put(data)
	return &ssa.seg, addr, nil
}

func (ssa *SingleSegmentArena) String() string {
	return "single-segment arena [len=" + str.Itod(len(ssa.seg.data)) + " cap=" + str.Itod(cap(ssa.seg.data)) + "]"
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
	if ssa.bp != nil {
		zeroSlice(ssa.seg.data)
		ssa.bp.Put(ssa.seg.data)
	}
	ssa.seg.BindTo(nil)
	ssa.seg.data = nil
	if ssa.fromPool {
		ssa.fromPool = false // Prevent double return
		singleSegmentPool.Put(ssa)
	}
}

// MultiSegment is an arena that stores object data across multiple []byte
// buffers, allocating new buffers of exponentially-increasing size when
// full. This avoids the potentially-expensive slice copying of SingleSegment.
type MultiSegmentArena struct {
	segs []Segment

	// rawData is set when the individual segments were all demuxed from
	// the passed raw data slice.
	rawData []byte

	// bp is the bufferpool assotiated with this arena's segments if it was
	// initialized for writing.
	bp *bufferpool.Pool

	// fromPool is true if this msa instance was obtained from the
	// multiSegmentPool and should be returned there upon release.
	fromPool bool
}

// MultiSegment returns a new arena that allocates new segments when
// they are full.  b MAY be nil.  Callers MAY use b to populate the
// buffer for reading or to reserve memory of a specific size.
func MultiSegment(b [][]byte) *MultiSegmentArena {
	if b == nil {
		msa := multiSegmentPool.Get().(*MultiSegmentArena)
		msa.fromPool = true
		msa.bp = &bufferpool.Default
		return msa
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
	// When this was demuxed from a single slice, return the entire slice.
	if msa.rawData != nil && msa.bp != nil {
		zeroSlice(msa.rawData)
		msa.bp.Put(msa.rawData)
		msa.bp = nil
	}
	msa.rawData = nil

	for i := range msa.segs {
		if msa.bp != nil {
			zeroSlice(msa.segs[i].data)
			msa.bp.Put(msa.segs[i].data)
		}
		msa.segs[i].data = nil
		msa.segs[i].BindTo(nil)
	}

	if msa.segs != nil {
		msa.segs = msa.segs[:0]
	}

	if msa.fromPool {
		// Prevent double inclusion if it is used after release.
		msa.fromPool = false

		multiSegmentPool.Put(msa)
	}
}

// Like MultiSegment, but doesn't use the pool
func multiSegment(b [][]byte) *MultiSegmentArena {
	var bp *bufferpool.Pool
	var segs []Segment
	if b == nil {
		bp = &bufferpool.Default
		segs = make([]Segment, 0, 5) // Typical size.
	} else {
		segs = make([]Segment, len(b))
		for i := range b {
			segs[i].data = b[i]
			segs[i].id = SegmentID(i)
		}
	}
	return &MultiSegmentArena{segs: segs, bp: bp}
}

var multiSegmentPool = sync.Pool{
	New: func() any {
		return multiSegment(nil)
	},
}

// demuxArena slices data into a multi-segment arena.  It assumes that
// len(data) >= hdr.totalSize().
//
// bp should point to the bufferpool which will receive back data once the
// arena is released. It may be nil if this should not be returned anywhere.
func (msa *MultiSegmentArena) demux(hdr streamHeader, data []byte, bp *bufferpool.Pool) error {
	maxSeg := hdr.maxSegment()
	if int64(maxSeg) > int64(maxInt-1) {
		return errors.New("number of segments overflows int")
	}

	// Grow list of existing segments as needed.
	numSegs := int(maxSeg + 1)
	if cap(msa.segs) >= numSegs {
		msa.segs = msa.segs[:numSegs]
	} else {
		inc := numSegs - len(msa.segs)
		msa.segs = append(msa.segs, make([]Segment, inc)...)
	}

	rawData := data
	for i := SegmentID(0); i <= maxSeg; i++ {
		sz, err := hdr.segmentSize(SegmentID(i))
		if err != nil {
			return err
		}

		msa.segs[i].data, data = data[:sz:sz], data[sz:]
		msa.segs[i].id = i
	}

	msa.rawData = rawData
	msa.bp = bp
	return nil
}

func (msa *MultiSegmentArena) NumSegments() int64 {
	return int64(len(msa.segs))
}

func (msa *MultiSegmentArena) Segment(id SegmentID) *Segment {
	if int(id) >= len(msa.segs) {
		return nil
	}
	return &msa.segs[id]
}

func (msa *MultiSegmentArena) Allocate(sz Size, msg *Message, seg *Segment) (*Segment, address, error) {
	// Prefer allocating in seg if it has capacity.
	if seg != nil && hasCapacity(seg.data, sz) {
		// Double check this segment is part of this arena.
		contains := false
		for i := range msa.segs {
			if &msa.segs[i] == seg {
				contains = true
				break
			}
		}

		if !contains {
			// This is a usage error.
			return nil, 0, errors.New("preferred segment is not part of the arena")
		}

		// Double check this segment is for this message.
		if seg.Message() != nil && seg.Message() != msg {
			return nil, 0, errors.New("attempt to allocate in segment for different message")
		}

		addr := address(len(seg.data))
		newLen := int(addr) + int(sz)
		seg.data = seg.data[:newLen]
		seg.BindTo(msg)
		return seg, addr, nil
	}

	var total int64
	for i := range msa.segs {
		data := msa.segs[i].data
		if hasCapacity(data, sz) {
			// Found segment with spare capacity.
			addr := address(len(msa.segs[i].data))
			newLen := int(addr) + int(sz)
			msa.segs[i].data = msa.segs[i].data[:newLen]
			msa.segs[i].BindTo(msg)
			return &msa.segs[i], addr, nil
		}

		if total += int64(cap(data)); total < 0 {
			// Overflow.
			return nil, 0, errors.New("alloc " + str.Utod(sz) + " bytes: message too large")
		}
	}

	// Check for read-only arena.
	if msa.bp == nil {
		return nil, 0, errors.New("cannot allocate segment in read-only multi-segment arena")
	}

	// If this is the very first segment and the requested allocation
	// size is zero, modify the requested size to at least one word.
	//
	// FIXME: this is to maintain compatibility to existing behavior and
	// tests in NewMessage(), which assumes this. Remove once arenas
	// enforce the contract of always having at least one segment.
	compatFirstSegLenZeroAddSize := Size(0)
	if len(msa.segs) == 0 && sz == 0 {
		compatFirstSegLenZeroAddSize = wordSize
	}

	// Determine actual allocation size (may be greater than sz).
	n, err := nextAlloc(total, 1<<63-1, sz+compatFirstSegLenZeroAddSize)
	if err != nil {
		return nil, 0, err
	}

	// We have determined this will be a new segment. Get the backing
	// buffer for it.
	buf := msa.bp.Get(n)
	buf = buf[:sz]

	// Setup the segment.
	id := SegmentID(len(msa.segs))
	msa.segs = append(msa.segs, Segment{
		data: buf,
		id:   id,
	})
	res := &msa.segs[int(id)]
	res.BindTo(msg)
	return res, 0, nil
}

func (msa *MultiSegmentArena) String() string {
	return "multi-segment arena [" + str.Itod(len(msa.segs)) + " segments]"
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

type ReadOnlySingleSegment struct {
	seg Segment
}

// NumSegments returns the number of segments in the arena.
// This must not be larger than 1<<32.
func (r *ReadOnlySingleSegment) NumSegments() int64 {
	return 1
}

// Segment returns the segment identified with the specified id. This
// may return nil if the segment with the specified ID does not exist.
func (r *ReadOnlySingleSegment) Segment(id SegmentID) *Segment {
	if id == 0 {
		return &r.seg
	}

	return nil
}

// Allocate selects a segment to place a new object in, creating a
// segment or growing the capacity of a previously loaded segment if
// necessary.  If Allocate does not return an error, then the
// difference of the capacity and the length of the returned slice
// must be at least minsz.  Some allocators may specifically choose to
// grow the passed seg (if non nil).
//
// If Allocate creates a new segment, the ID must be one larger than
// the last segment's ID or zero if it is the first segment.
//
// If Allocate returns an previously loaded segment, then the
// arena is responsible for preserving the existing data.
func (r *ReadOnlySingleSegment) Allocate(minsz Size, msg *Message, seg *Segment) (*Segment, address, error) {
	return nil, 0, errors.New("readOnly segment cannot allocate data")
}

// Release all resources associated with the Arena. Callers MUST NOT
// use the Arena after it has been released.
//
// Calling Release() is OPTIONAL, but may reduce allocations.
//
// Implementations MAY use Release() as a signal to return resources
// to free lists, or otherwise reuse the Arena.   However, they MUST
// NOT assume Release() will be called.
func (r *ReadOnlySingleSegment) Release() {
	r.seg.data = nil
}

// ReplaceData replaces the current data of the arena. This should ONLY be
// called on an empty or released arena, or else it panics.
func (r *ReadOnlySingleSegment) ReplaceData(b []byte) {
	if r.seg.data != nil {
		panic("replacing data on unreleased ReadOnlyArena")
	}

	r.seg.data = b
}

// NewReadOnlySingleSegment creates a new read only arena with the given data.
func NewReadOnlySingleSegment(b []byte) *ReadOnlySingleSegment {
	return &ReadOnlySingleSegment{seg: Segment{data: b}}
}
