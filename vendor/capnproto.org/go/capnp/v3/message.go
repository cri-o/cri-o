package capnp

import (
	"encoding/binary"
	"errors"
	"io"
	"sync"
	"sync/atomic"

	"capnproto.org/go/capnp/v3/exc"
	"capnproto.org/go/capnp/v3/internal/str"
	"capnproto.org/go/capnp/v3/packed"
)

// Security limits. Matches C++ implementation.
const (
	defaultTraverseLimit = 64 << 20 // 64 MiB
	defaultDepthLimit    = 64

	maxStreamSegments = 512

	defaultDecodeLimit = 64 << 20 // 64 MiB
)

const maxDepth = ^uint(0)

// A Message is a tree of Cap'n Proto objects, split into one or more
// segments of contiguous memory.  The only required field is Arena.
// A Message is safe to read from multiple goroutines.
//
// A message must be set up with a fully valid Arena when reading or with
// a valid and empty arena by calling NewArena.
type Message struct {
	// rlimit must be first so that it is 64-bit aligned.
	// See sync/atomic docs.
	rlimit     atomic.Uint64
	rlimitInit sync.Once

	Arena Arena

	capTable CapTable

	// TraverseLimit limits how many total bytes of data are allowed to be
	// traversed while reading.  Traversal is counted when a Struct or
	// List is obtained.  This means that calling a getter for the same
	// sub-struct multiple times will cause it to be double-counted.  Once
	// the traversal limit is reached, pointer accessors will report
	// errors. See https://capnproto.org/encoding.html#amplification-attack
	// for more details on this security measure.
	//
	// If not set, this defaults to 64 MiB.
	TraverseLimit uint64

	// DepthLimit limits how deeply-nested a message structure can be.
	// If not set, this defaults to 64.
	DepthLimit uint
}

// NewMessage creates a message with a new root and returns the first segment.
// The arena may or may not have any data.
//
// The new message is guaranteed to contain at least one segment and that
// segment is guaranteed to contain enough space for the root struct pointer.
// This might involve allocating data if the arena is empty.
func NewMessage(arena Arena) (*Message, *Segment, error) {
	var msg Message
	first, err := msg.Reset(arena)
	return &msg, first, err
}

// NewSingleSegmentMessage(b) is equivalent to NewMessage(SingleSegment(b)),
// except that it panics instead of returning an error.
func NewSingleSegmentMessage(b []byte) (msg *Message, first *Segment) {
	msg, first, err := NewMessage(SingleSegment(b))
	if err != nil {
		panic(err)
	}
	return msg, first
}

// Analogous to NewSingleSegmentMessage, but using MultiSegment.
func NewMultiSegmentMessage(b [][]byte) (msg *Message, first *Segment) {
	msg, first, err := NewMessage(MultiSegment(b))
	if err != nil {
		panic(err)
	}
	return msg, first
}

// Release is syntactic sugar for Message.Reset(nil).  See
// docstring for Reset for an important warning.
func (m *Message) Release() {
	m.Reset(nil)
}

// Reset the message to use a different arena, allowing it to be reused. This
// invalidates any existing pointers in the Message, releases all clients in
// the cap table, and releases the current Arena, so use with caution.
//
// Reset fails if the new arena is empty and is not able to allocate the first
// segment.
func (m *Message) Reset(arena Arena) (first *Segment, err error) {
	m.capTable.Reset()

	if m.Arena != nil {
		m.Arena.Release()
	}

	*m = Message{
		Arena:         arena,
		TraverseLimit: m.TraverseLimit,
		DepthLimit:    m.DepthLimit,
		capTable:      m.capTable,
	}

	if arena != nil {
		switch arena.NumSegments() {
		case 0:
			if first, _, err = arena.Allocate(0, m, nil); err != nil {
				return nil, exc.WrapError("new message", err)
			}

		default:
			if first, err = m.Segment(0); err != nil {
				return nil, exc.WrapError("Reset.Segment(0)", err)
			}
		}

		if first.ID() != 0 {
			return nil, errors.New("new message: arena allocated first segment with non-zero ID")
		}
	}

	return
}

func (m *Message) initReadLimit() {
	if m.TraverseLimit == 0 {
		m.rlimit.Store(defaultTraverseLimit)
		return
	}
	m.rlimit.Store(m.TraverseLimit)
}

// canRead reports whether the amount of bytes can be stored safely.
func (m *Message) canRead(sz Size) (ok bool) {
	m.rlimitInit.Do(m.initReadLimit)
	for {
		curr := m.rlimit.Load()

		var new uint64
		if ok = curr >= uint64(sz); ok {
			new = curr - uint64(sz)
		}

		if m.rlimit.CompareAndSwap(curr, new) {
			return
		}
	}
}

// ResetReadLimit sets the number of bytes allowed to be read from this message.
func (m *Message) ResetReadLimit(limit uint64) {
	m.rlimitInit.Do(func() {})
	m.rlimit.Store(limit)
}

// Unread increases the read limit by sz.
func (m *Message) Unread(sz Size) {
	m.rlimitInit.Do(m.initReadLimit)
	m.rlimit.Add(uint64(sz))
}

func (m *Message) allocRootPointerSpace() (*Segment, error) {
	// TODO: This may be simplified once NewMessage is the only acceptable
	// way to create a message and it ensures at least one segment exists.
	//
	// This may be achieved by making the Arena field of message private.
	var first *Segment
	var err error
	if m.NumSegments() == 0 {
		if m.Arena == nil {
			return nil, errors.New("cannot allocate root pointer without arena")
		}

		if first, _, err = m.Arena.Allocate(wordSize, m, nil); err != nil {
			return nil, exc.WrapError("new message", err)
		}
		if first.id != 0 {
			return nil, errors.New("allocRootPointer: segment allocated is not segment 0")
		}
	} else {
		if first, err = m.Segment(0); err != nil {
			return nil, exc.WrapError("allocRootPointer", err)
		}

		if len(first.Data()) < int(wordSize) {
			if first, _, err = m.Arena.Allocate(wordSize, m, first); err != nil {
				return nil, exc.WrapError("new message", err)
			}
			if first.id != 0 {
				return nil, errors.New("allocRootPointer: segment allocated is not segment 0")
			}
		}
	}
	return first, err
}

// Root returns the pointer to the message's root object.
func (m *Message) Root() (Ptr, error) {
	s, err := m.Segment(0)
	if err != nil {
		return Ptr{}, exc.WrapError("read root", err)
	}
	if len(s.Data()) == 0 {
		return Ptr{}, errors.New("message does not contain root pointer")
	}
	p, err := s.root().At(0)
	if err != nil {
		return Ptr{}, exc.WrapError("read root", err)
	}
	return p, nil
}

// SetRoot sets the message's root object to p.
func (m *Message) SetRoot(p Ptr) error {
	// TODO: enforcement of root pointer space is only needed here because
	// a single call in package capnpc-go (in file fileparts.go) calls
	// SetRoot() without first allocating space. This is likely an erroneous
	// usage and should be investigated in the future. If that is fixed,
	// then this can be simplified (and improved for performance).
	s, err := m.allocRootPointerSpace()
	if err != nil {
		return exc.WrapError("set root", err)
	}
	if err := s.root().Set(0, p); err != nil {
		return exc.WrapError("set root", err)
	}
	return nil
}

// CapTable is the indexed list of the clients referenced in the
// message. Capability pointers inside the message will use this
// table to map pointers to Clients.   The table is populated by
// the RPC system.
//
// https://capnproto.org/encoding.html#capabilities-interfaces
func (m *Message) CapTable() *CapTable {
	return &m.capTable
}

// Compute the total size of the message in bytes, when serialized as
// a stream. This is the same as the length of the slice returned by
// m.Marshal()
func (m *Message) TotalSize() (uint64, error) {
	nsegs := uint64(m.NumSegments())
	totalSize := (nsegs/2 + 1) * 8
	for i := uint64(0); i < nsegs; i++ {
		seg, err := m.Segment(SegmentID(i))
		if err != nil {
			return 0, err
		}
		totalSize += uint64(len(seg.Data()))
	}
	return totalSize, nil
}

func (m *Message) depthLimit() uint {
	if m.DepthLimit != 0 {
		return m.DepthLimit
	}
	return defaultDepthLimit
}

// NumSegments returns the number of segments in the message.
func (m *Message) NumSegments() int64 {
	return int64(m.Arena.NumSegments())
}

// Segment returns the segment with the given ID.
func (m *Message) Segment(id SegmentID) (*Segment, error) {
	seg := m.Arena.Segment(id)
	if seg == nil {
		return nil, errors.New("segment " + str.Utod(id) + " out of bounds in arena")
	}
	segMsg := seg.Message()
	if segMsg == nil {
		seg.BindTo(m)
	} else if segMsg != m {
		return nil, errors.New("segment " + str.Utod(id) + ": not of the same message")
	}
	return seg, nil
}

func (m *Message) WriteTo(w io.Writer) (int64, error) {
	wc := &writeCounter{Writer: w}
	err := NewEncoder(wc).Encode(m)
	return wc.N, err
}

// Marshal concatenates the segments in the message into a single byte
// slice including framing.
func (m *Message) Marshal() ([]byte, error) {
	// Compute buffer size.
	nsegs := m.NumSegments()
	if nsegs == 0 {
		return nil, errors.New("marshal: message has no segments")
	}
	hdrSize := streamHeaderSize(SegmentID(nsegs - 1))
	if hdrSize > uint64(maxInt) {
		return nil, errors.New("marshal: header size overflows int")
	}
	var dataSize uint64
	for i := int64(0); i < nsegs; i++ {
		s, err := m.Segment(SegmentID(i))
		if err != nil {
			return nil, exc.WrapError("marshal", err)
		}
		n := uint64(len(s.data))
		if n%uint64(wordSize) != 0 {
			return nil, errors.New("marshal: segment " + str.Itod(i) + " not word-aligned")
		}
		if n > uint64(maxSegmentSize) {
			return nil, errors.New("marshal: segment " + str.Itod(i) + " too large")
		}
		dataSize += n
		if dataSize > uint64(maxInt) {
			return nil, errors.New("marshal: message size overflows int")
		}
	}
	total := hdrSize + dataSize
	if total > uint64(maxInt) {
		return nil, errors.New("marshal: message size overflows int")
	}

	// Fill buffer.
	buf := make([]byte, int(hdrSize), int(total))
	binary.LittleEndian.PutUint32(buf, uint32(nsegs-1))
	for i := int64(0); i < nsegs; i++ {
		s, err := m.Segment(SegmentID(i))
		if err != nil {
			return nil, exc.WrapError("marshal", err)
		}
		if len(s.data)%int(wordSize) != 0 {
			return nil, errors.New("marshal: segment " + str.Itod(i) + " not word-aligned")
		}
		binary.LittleEndian.PutUint32(buf[int(i+1)*4:], uint32(len(s.data)/int(wordSize)))
		buf = append(buf, s.data...)
	}
	return buf, nil
}

// MarshalPacked marshals the message in packed form.
func (m *Message) MarshalPacked() ([]byte, error) {
	data, err := m.Marshal()
	if err != nil {
		return nil, err
	}
	buf := make([]byte, 0, len(data))
	buf = packed.Pack(buf, data)
	return buf, nil
}

type writeCounter struct {
	N int64
	io.Writer
}

func (wc *writeCounter) Write(b []byte) (n int, err error) {
	n, err = wc.Writer.Write(b)
	wc.N += int64(n)
	return
}

// alloc allocates sz zero-filled bytes.  It prefers using s, but may
// use a different segment in the same message if there's not sufficient
// capacity.
func alloc(s *Segment, sz Size) (*Segment, address, error) {
	if sz > maxAllocSize() {
		return nil, 0, errors.New("allocation: too large")
	}
	sz = sz.padToWord()

	msg := s.Message()
	if msg == nil {
		return nil, 0, errors.New("segment does not have a message assotiated with it")
	}
	if msg.Arena == nil {
		return nil, 0, errors.New("message does not have an arena")
	}

	if _, err := msg.allocRootPointerSpace(); err != nil {
		return nil, 0, err
	}

	// TODO: From this point on, this could be changed to be a requirement
	// for Arena implementations instead of relying on alloc() to do it.

	s, addr, err := msg.Arena.Allocate(sz, msg, s)
	if err != nil {
		return s, addr, err
	}

	end, ok := addr.addSize(sz)
	if !ok {
		return nil, 0, errors.New("allocation: address overflow")
	}

	zeroSlice(s.data[addr:end])
	return s, addr, nil
}
