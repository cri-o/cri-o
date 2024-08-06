package capnp

import (
	"encoding/binary"
	"errors"

	"capnproto.org/go/capnp/v3/exc"
	"capnproto.org/go/capnp/v3/internal/str"
)

// A SegmentID is a numeric identifier for a Segment.
type SegmentID uint32

// A Segment is an allocation arena for Cap'n Proto objects.
// It is part of a Message, which can contain other segments that
// reference each other.
type Segment struct {
	// msg associated with this segment. A Message instance m maintains the
	// invariant that that all m.segs[].msg == m.
	msg  *Message
	id   SegmentID
	data []byte
}

// Message returns the message that contains s.
func (s *Segment) Message() *Message {
	return s.msg
}

// ID returns the segment's ID.
func (s *Segment) ID() SegmentID {
	return s.id
}

// Data returns the raw byte slice for the segment.
func (s *Segment) Data() []byte {
	return s.data
}

func (s *Segment) inBounds(addr address) bool {
	return addr < address(len(s.data))
}

func (s *Segment) regionInBounds(base address, sz Size) bool {
	end, ok := base.addSize(sz)
	if !ok {
		return false
	}
	return end <= address(len(s.data))
}

// slice returns the segment of data from base to base+sz.
// It panics if the slice is out of bounds.
func (s *Segment) slice(base address, sz Size) []byte {
	return s.data[base:base.addSizeUnchecked(sz)]
}

func (s *Segment) readUint8(addr address) uint8 {
	return s.slice(addr, 1)[0]
}

func (s *Segment) readUint16(addr address) uint16 {
	return binary.LittleEndian.Uint16(s.slice(addr, 2))
}

func (s *Segment) readUint32(addr address) uint32 {
	return binary.LittleEndian.Uint32(s.slice(addr, 4))
}

func (s *Segment) readUint64(addr address) uint64 {
	return binary.LittleEndian.Uint64(s.slice(addr, 8))
}

func (s *Segment) readRawPointer(addr address) rawPointer {
	return rawPointer(s.readUint64(addr))
}

func (s *Segment) writeUint8(addr address, val uint8) {
	s.slice(addr, 1)[0] = val
}

func (s *Segment) writeUint16(addr address, val uint16) {
	binary.LittleEndian.PutUint16(s.slice(addr, 2), val)
}

func (s *Segment) writeUint32(addr address, val uint32) {
	binary.LittleEndian.PutUint32(s.slice(addr, 4), val)
}

func (s *Segment) writeUint64(addr address, val uint64) {
	binary.LittleEndian.PutUint64(s.slice(addr, 8), val)
}

func (s *Segment) writeRawPointer(addr address, val rawPointer) {
	s.writeUint64(addr, uint64(val))
}

// root returns a 1-element pointer list that references the first word
// in the segment.  This only makes sense to call on the first segment
// in a message.
func (s *Segment) root() PointerList {
	sz := ObjectSize{PointerCount: 1}
	if !s.regionInBounds(0, sz.totalSize()) {
		return PointerList{}
	}
	return PointerList{
		seg:        s,
		length:     1,
		size:       sz,
		depthLimit: s.msg.depthLimit(),
	}
}

func (s *Segment) lookupSegment(id SegmentID) (*Segment, error) {
	if s.id == id {
		return s, nil
	}
	return s.msg.Segment(id)
}

func (s *Segment) readPtr(paddr address, depthLimit uint) (ptr Ptr, err error) {
	s, base, val, err := s.resolveFarPointer(paddr)
	if err != nil {
		return Ptr{}, exc.WrapError("read pointer", err)
	}
	if val == 0 {
		return Ptr{}, nil
	}
	if depthLimit == 0 {
		return Ptr{}, errors.New("read pointer: depth limit reached")
	}
	switch val.pointerType() {
	case structPointer:
		sp, err := s.readStructPtr(base, val)
		if err != nil {
			return Ptr{}, exc.WrapError("read pointer", err)
		}
		if !s.msg.canRead(sp.readSize()) {
			return Ptr{}, errors.New("read pointer: read traversal limit reached")
		}
		sp.depthLimit = depthLimit - 1
		return sp.ToPtr(), nil
	case listPointer:
		lp, err := s.readListPtr(base, val)
		if err != nil {
			return Ptr{}, exc.WrapError("read pointer", err)
		}
		if !s.msg.canRead(lp.readSize()) {
			return Ptr{}, errors.New("read pointer: read traversal limit reached")
		}
		lp.depthLimit = depthLimit - 1
		return lp.ToPtr(), nil
	case otherPointer:
		if val.otherPointerType() != 0 {
			return Ptr{}, errors.New("read pointer: unknown pointer type")
		}
		return Interface{
			seg: s,
			cap: val.capabilityIndex(),
		}.ToPtr(), nil
	default:
		// Only other types are far pointers.
		return Ptr{}, errors.New("read pointer: far pointer landing pad is a far pointer")
	}
}

func (s *Segment) readStructPtr(base address, val rawPointer) (Struct, error) {
	addr, ok := val.offset().resolve(base)
	if !ok {
		return Struct{}, errors.New("struct pointer: invalid address")
	}
	sz := val.structSize()
	if !s.regionInBounds(addr, sz.totalSize()) {
		return Struct{}, errors.New("struct pointer: invalid address")
	}
	return Struct{
		seg:  s,
		off:  addr,
		size: sz,
	}, nil
}

func (s *Segment) readListPtr(base address, val rawPointer) (List, error) {
	addr, ok := val.offset().resolve(base)
	if !ok {
		return List{}, errors.New("list pointer: invalid address")
	}
	lsize, ok := val.totalListSize()
	if !ok {
		return List{}, errors.New("list pointer: size overflow")
	}
	if !s.regionInBounds(addr, lsize) {
		return List{}, errors.New("list pointer: address out of bounds")
	}
	lt := val.listType()
	if lt == compositeList {
		hdr := s.readRawPointer(addr)
		var ok bool
		addr, ok = addr.addSize(wordSize)
		if !ok {
			return List{}, errors.New("composite list pointer: content address overflow")
		}
		if hdr.pointerType() != structPointer {
			return List{}, errors.New("composite list pointer: tag word is not a struct")
		}
		sz := hdr.structSize()
		n := int32(hdr.offset())
		// TODO(someday): check that this has the same end address
		if tsize, ok := sz.totalSize().times(n); !ok {
			return List{}, errors.New("composite list pointer: size overflow")
		} else if !s.regionInBounds(addr, tsize) {
			return List{}, errors.New("composite list pointer: address out of bounds")
		}
		return List{
			seg:    s,
			size:   sz,
			off:    addr,
			length: n,
			flags:  isCompositeList,
		}, nil
	}
	if lt == bit1List {
		return List{
			seg:    s,
			off:    addr,
			length: val.numListElements(),
			flags:  isBitList,
		}, nil
	}
	return List{
		seg:    s,
		size:   val.elementSize(),
		off:    addr,
		length: val.numListElements(),
	}, nil
}

func (s *Segment) resolveFarPointer(paddr address) (dst *Segment, base address, resolved rawPointer, err error) {
	// Encoding details at https://capnproto.org/encoding.html#inter-segment-pointers

	val := s.readRawPointer(paddr)
	switch val.pointerType() {
	case doubleFarPointer:
		padSeg, err := s.lookupSegment(val.farSegment())
		if err != nil {
			return nil, 0, 0, exc.WrapError("double-far pointer", err)
		}
		padAddr := val.farAddress()
		if !padSeg.regionInBounds(padAddr, wordSize*2) {
			return nil, 0, 0, errors.New("double-far pointer: address out of bounds")
		}
		far := padSeg.readRawPointer(padAddr)
		if far.pointerType() != farPointer {
			return nil, 0, 0, errors.New("double-far pointer: first word in landing pad is not a far pointer")
		}
		tagAddr, ok := padAddr.addSize(wordSize)
		if !ok {
			return nil, 0, 0, errors.New("double-far pointer: landing pad address overflow")
		}
		tag := padSeg.readRawPointer(tagAddr)
		if pt := tag.pointerType(); (pt != structPointer && pt != listPointer) || tag.offset() != 0 {
			return nil, 0, 0, errors.New("double-far pointer: second word is not a struct or list with zero offset")
		}
		if dst, err = s.lookupSegment(far.farSegment()); err != nil {
			return nil, 0, 0, exc.WrapError("double-far pointer", err)
		}
		return dst, 0, landingPadNearPointer(far, tag), nil
	case farPointer:
		var err error
		dst, err = s.lookupSegment(val.farSegment())
		if err != nil {
			return nil, 0, 0, exc.WrapError("far pointer", err)
		}
		padAddr := val.farAddress()
		if !dst.regionInBounds(padAddr, wordSize) {
			return nil, 0, 0, errors.New("far pointer: address out of bounds")
		}
		var ok bool
		base, ok = padAddr.addSize(wordSize)
		if !ok {
			return nil, 0, 0, errors.New("far pointer: landing pad address overflow")
		}
		return dst, base, dst.readRawPointer(padAddr), nil
	default:
		var ok bool
		base, ok = paddr.addSize(wordSize)
		if !ok {
			return nil, 0, 0, errors.New("pointer base address overflow")
		}
		return s, base, val, nil
	}
}

func (s *Segment) writePtr(off address, src Ptr, forceCopy bool) error {
	if !src.IsValid() {
		s.writeRawPointer(off, 0)
		return nil
	}

	// Copy src, if needed, and process pointers where placement is
	// irrelevant (capabilities and zero-sized structs).
	var srcAddr address
	var srcRaw rawPointer
	switch src.flags.ptrType() {
	case structPtrType:
		st := src.Struct()
		if st.size.isZero() {
			// Zero-sized structs should always be encoded with offset -1 in
			// order to avoid conflating with null.  No allocation needed.
			s.writeRawPointer(off, rawStructPointer(-1, ObjectSize{}))
			return nil
		}
		if forceCopy || src.seg.msg != s.msg || st.flags&isListMember != 0 {
			newSeg, newAddr, err := alloc(s, st.size.totalSize())
			if err != nil {
				return exc.WrapError("write pointer: copy", err)
			}
			dst := Struct{
				seg:        newSeg,
				off:        newAddr,
				size:       st.size,
				depthLimit: maxDepth,
				// clear flags
			}
			if err := copyStruct(dst, st); err != nil {
				return exc.WrapError("write pointer", err)
			}
			st = dst
			src = dst.ToPtr()
		}
		srcAddr = st.off
		srcRaw = rawStructPointer(0, st.size)
	case listPtrType:
		l := src.List()
		if forceCopy || src.seg.msg != s.msg {
			sz := l.allocSize()
			newSeg, newAddr, err := alloc(s, sz)
			if err != nil {
				return exc.WrapError("write pointer: copy", err)
			}
			dst := List{
				seg:        newSeg,
				off:        newAddr,
				length:     l.length,
				size:       l.size,
				flags:      l.flags,
				depthLimit: maxDepth,
			}
			if dst.flags&isCompositeList != 0 {
				// Copy tag word
				newSeg.writeRawPointer(newAddr, l.seg.readRawPointer(l.off-address(wordSize)))
				var ok bool
				dst.off, ok = dst.off.addSize(wordSize)
				if !ok {
					return errors.New("write pointer: copy composite list: content address overflow")
				}
				sz -= wordSize
			}
			if dst.flags&isBitList != 0 || dst.size.PointerCount == 0 {
				end, _ := l.off.addSize(sz) // list was already validated
				copy(newSeg.data[dst.off:], l.seg.data[l.off:end])
			} else {
				for i := 0; i < l.Len(); i++ {
					err := copyStruct(dst.Struct(i), l.Struct(i))
					if err != nil {
						return exc.WrapError("write pointer: copy list element"+str.Itod(i), err)
					}
				}
			}
			l = dst
			src = dst.ToPtr()
		}
		srcAddr = l.off
		if l.flags&isCompositeList != 0 {
			srcAddr -= address(wordSize)
		}
		srcRaw = l.raw()
	case interfacePtrType:
		i := src.Interface()
		if src.seg.msg != s.msg {
			c := s.msg.CapTable().Add(i.Client().AddRef())
			i = NewInterface(s, c)
		}
		s.writeRawPointer(off, i.value(off))
		return nil
	default:
		panic("unreachable")
	}

	switch {
	case src.seg == s:
		// Common case: src is in same segment as pointer.
		// Use a near pointer.
		s.writeRawPointer(off, srcRaw.withOffset(nearPointerOffset(off, srcAddr)))
		return nil
	case hasCapacity(src.seg.data, wordSize):
		// Enough room adjacent to src to write a far pointer landing pad.
		_, padAddr, _ := alloc(src.seg, wordSize)
		src.seg.writeRawPointer(padAddr, srcRaw.withOffset(nearPointerOffset(padAddr, srcAddr)))
		s.writeRawPointer(off, rawFarPointer(src.seg.id, padAddr))
		return nil
	default:
		// Not enough room for a landing pad, need to use a double-far pointer.
		padSeg, padAddr, err := alloc(s, wordSize*2)
		if err != nil {
			return exc.WrapError("write pointer: make landing pad", err)
		}
		padSeg.writeRawPointer(padAddr, rawFarPointer(src.seg.id, srcAddr))
		padSeg.writeRawPointer(padAddr.addSizeUnchecked(wordSize), srcRaw)
		s.writeRawPointer(off, rawDoubleFarPointer(padSeg.id, padAddr))
		return nil
	}
}
