/*
Copyright The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package strictjson finds what encoding/json accepts silently and an
// artifact must not carry: members repeated within one object, members the
// target type has no field for, members that name a field only ignoring
// case, required members left out or null, bytes that are not valid UTF-8,
// data behind the document, and a document that is not an object. The
// command and the three profile packages share it, so that a library caller
// can refuse the same documents the command refuses.
package strictjson

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"sigs.k8s.io/security-profiles-merger/internal/merge"
)

// scanFrame is an open object or array during scanDocument. It holds the
// name or index of the value it is at rather than that value's path: a path
// carries its whole ancestry, so keeping one per open frame, or building one
// per value, costs memory quadratic in a document that nests deeply or puts
// many values under a long name. A path is built from the stack only when a
// finding is reported. The stack holds frames by value, so that opening an
// object or array allocates nothing once the stack is as deep as the
// document has nested so far.
type scanFrame struct {
	// keys holds the names of the members seen so far in an object, folded
	// where the object decodes into a struct (see addKey), mapped to
	// whether they were already reported as repeated. It is made at the
	// second member: most objects of a profile hold one member or none, and
	// the name of the first is kept in first until then.
	keys map[string]bool
	// first is the name of the first member of an object, as keys holds it.
	first string
	// member is the name of the object member whose value comes next.
	member string
	// index is the index of the next array element, and for an object the
	// count of its members so far.
	index int
	// object reports an object rather than an array.
	object bool
	// wantKey reports whether the next token in an object is a member name.
	wantKey bool
	// tracked reports an object whose missing members the scan looks for
	// (see named).
	tracked bool
	// typ is the struct or slice type the value of the frame decodes into,
	// behind any pointers; nil for any other type and where the scan follows
	// no type, which leaves every value inside unchecked.
	typ reflect.Type
	// named holds the position (see jsonField) of each field a member
	// gives a value other than null, for an object of a struct type with
	// required fields where the scan looks for missing members. It is
	// kept by field rather than by folded member name, since two fields
	// whose names fold alike are filled apart. field is the position of
	// the field the member whose value comes next fills, -1 for none, and
	// child the type of the value of the member or element that comes
	// next.
	named fieldMask
	field int
	child reflect.Type
}

// fieldMask is a set of field positions: a bit per field, the first
// fieldMaskBits in one word and the rest, for a struct of more fields than
// that, in words made as they are needed. Those are held behind a pointer,
// so that every frame, most of which hold no fieldMask at all, is smaller.
type fieldMask struct {
	low  uint64
	high *[]uint64
}

// fieldMaskBits is how many fields one word of a fieldMask holds.
const fieldMaskBits = 64

// add puts the field at position idx in the set.
func (mask *fieldMask) add(idx int) {
	if idx < fieldMaskBits {
		mask.low |= 1 << idx

		return
	}

	if mask.high == nil {
		mask.high = new([]uint64)
	}

	word := idx/fieldMaskBits - 1
	for len(*mask.high) <= word {
		*mask.high = append(*mask.high, 0)
	}

	(*mask.high)[word] |= 1 << (idx % fieldMaskBits)
}

// has reports whether the field at position idx is in the set. A nil mask
// is the empty set.
func (mask *fieldMask) has(idx int) bool {
	switch {
	case mask == nil:
		return false
	case idx < fieldMaskBits:
		return mask.low&(1<<idx) != 0
	}

	word := idx/fieldMaskBits - 1

	return mask.high != nil && word < len(*mask.high) &&
		(*mask.high)[word]&(1<<(idx%fieldMaskBits)) != 0
}

// documentScan holds what scanDocument found, each in document order: the
// members repeated within one object, the members no field reads, the
// members a field reads only ignoring case, and the required members left
// out or null.
type documentScan struct {
	duplicates, unknown, misspelled, missing pathCollector
	// failed reports a syntax error before the end of the document.
	failed bool
}

// scanDocument reads a document token by token, once, following the type
// target each value decodes into, and collects what documentScan holds. The
// required members, marked by their tag or by requiredFields, are only
// looked for where checkMissing is set. A nil target follows no type and
// finds only repeated members.
//
// Repeated members are compared the way encoding/json matches names to struct
// fields, ignoring case, because a case-sensitive parser reads
// "defaultAction" and "DefaultAction" as different members where
// encoding/json fills one field from both. The keys of every other object,
// one nothing decodes into, are compared as written, since encoding/json
// folds case for struct fields alone.
// encoding/json decodes each occurrence into what the one before it filled,
// so a later scalar or array replaces the earlier one while a later object
// merges into it field by field, and other parsers keep the first or the last
// occurrence: a profile with repeated members means different things to
// different readers, and the other findings, which read every occurrence on
// its own, are only worth reporting for a document without one.
//
// Members are matched to fields the way encoding/json does: by the exact
// JSON name first, by the folded name otherwise. A misspelled member is one
// the decoder reads while a reader that compares names exactly, as a C
// runtime does, drops it. The scan stops at the first syntax error.
func scanDocument(
	raw []byte, target reflect.Type, checkMissing bool,
) *documentScan {
	state := newScanState(target, checkMissing, nil)
	state.run(raw)

	return &state.scan
}

// PathAt returns the JSON path of the value of raw that holds the byte
// before offset, such as "syscalls[0].action", and the offset the value
// starts at. encoding/json reports a value of the wrong type
// (json.UnmarshalTypeError) at the end of a literal, or just past the
// opening bracket of an object or array, so that byte is the value's own;
// the error's Field names the value too, but spells it differently from one
// Go release to the next. The path is built by the same read of the
// document's tokens as scanDocument's. It is "" for the document itself,
// and the offset -1 where raw does not reach offset or is not valid JSON up
// to it.
func PathAt(raw []byte, offset int64) (string, int64) {
	path, start := "", int64(-1)

	state := newScanState(nil, false, func(stack []scanFrame, after, end int64) bool {
		if end < offset {
			return true
		}

		path, _ = memberPath(stack)
		start = valueStart(raw, after)

		return false
	})
	state.run(raw)

	return path, start
}

// valueStart returns the offset of the first byte from offset on that is
// none of the whitespace, colon or comma JSON puts between two tokens.
func valueStart(raw []byte, offset int64) int64 {
	for offset < int64(len(raw)) && strings.IndexByte(Space+":,", raw[offset]) >= 0 {
		offset++
	}

	return offset
}

// scanState is scanDocument under way: the open objects and arrays, the
// type the document decodes into, and what it found so far.
type scanState struct {
	stack  []scanFrame
	target reflect.Type
	// checkMissing reports whether the scan looks for missing members.
	checkMissing bool
	// onValue, where it is not nil, is passed every token that starts a
	// value, with the stack it sits in, its innermost frame at that value,
	// where the token before it ended and where it ends; the scan stops
	// when it returns false.
	onValue func(stack []scanFrame, after, end int64) bool
	scan    documentScan
	// missing is the path of the member the scan looks at for missing
	// members, kept so that its names are reused.
	missing missingPath
}

// newScanState returns a scan of a document against target (see
// scanDocument), passing every token that starts a value to onValue, where
// it is not nil (see scanState).
func newScanState(
	target reflect.Type, checkMissing bool,
	onValue func(stack []scanFrame, after, end int64) bool,
) *scanState {
	return &scanState{
		target:       target,
		checkMissing: checkMissing,
		onValue:      onValue,
	}
}

// run reads the document token by token, until its end, its first syntax
// error, or onValue stopping it.
func (state *scanState) run(raw []byte) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	// Numbers are kept as text: they only need to be skipped, and a value
	// out of the float64 range must not stop the scan.
	decoder.UseNumber()

	for {
		after := decoder.InputOffset()

		token, err := decoder.Token()
		if err != nil {
			// A document the decoder read must not read as one without
			// missing members because the scan could not finish it.
			if !errors.Is(err, io.EOF) {
				state.scan.failed = true

				if state.checkMissing {
					state.scan.missing.add(unreadableDocument)
				}
			}

			return
		}

		if !state.next(token, after, decoder.InputOffset()) {
			return
		}
	}
}

// valueStarted passes a token that starts a value, spanning after to end,
// to onValue, and reports whether the scan goes on.
func (state *scanState) valueStarted(after, end int64) bool {
	return state.onValue == nil || state.onValue(state.stack, after, end)
}

// next reads one token, which spans after to end, and reports whether the
// scan goes on.
func (state *scanState) next(token json.Token, after, end int64) bool {
	// top points into the stack, so it is not used once a frame is pushed,
	// which may move the stack. A frame popped stays where it is until the
	// next one is pushed.
	var top *scanFrame
	if len(state.stack) > 0 {
		top = &state.stack[len(state.stack)-1]
	}

	delim, isDelim := token.(json.Delim)

	switch {
	case isClosing(delim):
		state.stack = state.stack[:len(state.stack)-1]
		state.closed(top)
	case top.expectsKey():
		key, _ := token.(string)
		state.member(top, key)
	case isDelim:
		child := top.valueType(state.target)

		top.startValue()
		top.markNamed()

		if !state.valueStarted(after, end) {
			return false
		}

		// The stack doubles as it grows, where append grows a long slice
		// by a quarter and allocates about five times its final size in
		// all on the way to it.
		if len(state.stack) == cap(state.stack) {
			state.stack = slices.Grow(state.stack, max(len(state.stack), minStackFrames))
		}

		frame := state.newFrame(delim == '{', child)
		frame.keys = state.reusedKeys()
		state.stack = append(state.stack, frame)
	default:
		state.scalar(top, token)

		if !state.valueStarted(after, end) {
			return false
		}
	}

	return true
}

// member reads a member name of the object of frame: it records a name the
// object repeats, and where the frame follows a struct type, a name no field
// reads or one a field reads only ignoring case. It also notes the type of
// the member's value.
func (state *scanState) member(frame *scanFrame, key string) {
	path := func() (string, bool) { return memberPath(state.stack) }

	folded, repeated := frame.addKey(key)
	if repeated {
		state.scan.duplicates.addLazily(path)
	}

	frame.child = nil
	frame.field = -1

	if frame.typ == nil || frame.typ.Kind() != reflect.Struct {
		return
	}

	// addKey folded the name already.
	field, known, exact := fieldsOf(frame.typ).lookupFolded(key, folded)

	switch {
	case !known:
		state.scan.unknown.addLazily(path)
	case !exact:
		state.scan.misspelled.addWant(path, field.name)
	}

	if known {
		frame.child, frame.field = field.typ, field.index
	}
}

// closed reports, where the scan looks for missing members, the required
// members of an object of frame left out or null.
func (state *scanState) closed(frame *scanFrame) {
	if frame.tracked {
		reportMissing(
			frame.typ, state.missing.at(state.stack), &frame.named, &state.scan.missing,
		)
	}
}

// scalar reads a value that is neither an object nor an array.
func (state *scanState) scalar(top *scanFrame, token json.Token) {
	child := top.valueType(state.target)

	top.startValue()

	switch {
	case token != nil:
		top.markNamed()
	case !state.checkMissing:
	case top == nil || !top.tracked:
		// encoding/json leaves a value set to null where nothing else
		// names it, as an element of a list, as it was.
		reportZero(child, state.missing.at(state.stack), &state.scan.missing)
	default:
		// A member of an object read as null is missing where the object
		// closes, as one left out is.
	}
}

// minStackFrames is how many frames the stack of a scan holds at first,
// as deep as a profile document nests.
const minStackFrames = 8

// maxReusedKeys is how many names the keys of a frame may have held for a
// frame opened in its place to reuse them. Clearing a map costs time in
// the space it grew to, so a large one is dropped rather than cleared for
// every small object after it.
const maxReusedKeys = 8

// reusedKeys returns the keys of the frame last closed where the next one
// opens, cleared, so that the objects of a document allocate a map per
// level it nests rather than one each, or nil where there are none.
func (state *scanState) reusedKeys() map[string]bool {
	depth := len(state.stack)
	if depth == cap(state.stack) {
		return nil
	}

	keys := state.stack[:depth+1][depth].keys
	if len(keys) > maxReusedKeys {
		return nil
	}

	clear(keys)

	return keys
}

func isClosing(delim json.Delim) bool {
	return delim == '}' || delim == ']'
}

// newFrame opens an object or array whose value decodes into typ, or whose
// type is not followed where typ is nil.
func (state *scanState) newFrame(object bool, typ reflect.Type) scanFrame {
	frame := scanFrame{object: object, wantKey: object, field: -1}

	for typ != nil && typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}

	if typ == nil || typ.Kind() != reflect.Struct && typ.Kind() != reflect.Slice {
		return frame
	}

	frame.typ = typ

	frame.tracked = state.checkMissing && object && typ.Kind() == reflect.Struct &&
		requiresFields(typ)

	return frame
}

// valueType returns the type the value that comes next decodes into, or nil
// where the frame does not follow types. A nil frame is the document root,
// whose value decodes into target.
func (frame *scanFrame) valueType(target reflect.Type) reflect.Type {
	if frame == nil {
		return target
	}

	if frame.typ == nil {
		return nil
	}

	if frame.typ.Kind() == reflect.Slice {
		return frame.typ.Elem()
	}

	return frame.child
}

// markNamed records that the member whose value starts next is given one.
func (frame *scanFrame) markNamed() {
	if frame != nil && frame.tracked && frame.field >= 0 {
		frame.named.add(frame.field)
	}
}

// expectsKey reports whether the next token is a member name. A nil frame
// is the document root.
func (frame *scanFrame) expectsKey() bool {
	return frame != nil && frame.wantKey
}

// addKey records a member name and returns it as the object compares it,
// and whether this is the first time the name repeats, which is when its
// path is collected. In an object decoding into a struct, names are folded
// and repeat when they fold alike, as encoding/json matches them to struct
// fields, so that "Name" and "name" fill one field. Every other object,
// one nothing decodes into, such as an object inside a member no field
// reads, repeats a name only when it repeats it exactly: encoding/json
// folds case for struct fields alone, and there "Name" and "name" are two
// members, of which the member holding them is reported as unknown.
func (frame *scanFrame) addKey(key string) (string, bool) {
	frame.member = key
	frame.wantKey = false
	frame.index++

	name := key
	if frame.typ != nil && frame.typ.Kind() == reflect.Struct {
		name = foldName(key)
	}

	switch {
	case frame.index == 1:
		frame.first = name

		return name, false
	case frame.keys == nil:
		frame.keys = map[string]bool{frame.first: false}
	case len(frame.keys) == 0:
		// Keys reused from a frame closed before (see reusedKeys).
		frame.keys[frame.first] = false
	}

	reported, seen := frame.keys[name]
	if !seen {
		frame.keys[name] = false

		return name, false
	}

	if reported {
		return name, false
	}

	frame.keys[name] = true

	return name, true
}

// startValue advances the frame past the value that starts next. A nil frame
// is the document root.
func (frame *scanFrame) startValue() {
	if frame == nil {
		return
	}

	if frame.object {
		frame.wantKey = true

		return
	}

	frame.index++
}

// maxPathBytes is how much of a path memberPath builds before it stops. A
// path is only ever shown through merge.QuoteBounded, which cuts it at
// merge.MaxQuotedBytes, so a path past that only has to stay past it. A
// document chooses both how deep it nests and how long its names are, and
// appending every level copies the path so far: a five-megabyte document of
// five thousand levels under thousand-byte names took most of a minute to
// name the members it repeats.
const maxPathBytes = merge.MaxQuotedBytes + utf8.UTFMax

// memberPath returns the path of the member the innermost frame is at, or
// the start of it once it is longer than any report shows, and whether it
// cut the path there. Every outer frame is at the value holding the next
// one: an object at its current member, an array at the element before its
// next index.
func memberPath(stack []scanFrame) (string, bool) {
	path := ""

	for idx := range stack {
		frame := &stack[idx]

		if len(path) > maxPathBytes {
			return path, idx < len(stack)
		}

		if frame.object {
			path = joinFieldPath(path, frame.member)

			continue
		}

		path += "[" + strconv.Itoa(frame.index-1) + "]"
	}

	return path, false
}

// pathOf builds the path of a member, and reports whether memberPath cut
// it, only when called, so that a path nobody keeps is never built.
type pathOf func() (string, bool)

// maxReportedPaths is the number of field paths one message lists, the same
// ceiling the library puts on the failures it joins.
const maxReportedPaths = merge.MaxJoinedErrors

// pathCollector gathers the field paths of one message, bounded the way the
// library bounds its own joined failures: past merge.MaxJoinedErrors it
// counts what it leaves out rather than keeping it. A document chooses both
// how many paths it holds and how long each one is, since a path carries its
// whole ancestry, so an unbounded collector turns a profile of a few hundred
// kilobytes into hundreds of megabytes of warning.
//
// It keeps each path once, in the order first seen: a member repeated in
// an object that is itself repeated is found once per occurrence of the
// object. A path memberPath cut (see maxPathBytes) stands for every member
// beneath its start, so two such paths that read alike may name different
// members, and each is kept and counts toward the bound. A path is compared
// only once it is built, so past the bound, where none is built, a repeat
// counts among the ones left out.
type pathCollector struct {
	paths []string
	// wants holds, for a collector of misspelled members, the spelling of
	// the field the member at the same index of paths stands for.
	wants   []string
	omitted int
}

// err wraps kind with the paths collected, or returns nil when there are
// none (see pathsError).
func (c *pathCollector) err(kind error) error {
	return pathsError(kind, c.paths, c.wants, c.omitted)
}

func (c *pathCollector) add(path string) {
	c.addLazily(func() (string, bool) { return path, false })
}

// full reports whether the collector keeps no more paths. A path it is
// given then counts among the ones left out.
func (c *pathCollector) full() bool {
	return len(c.paths) >= maxReportedPaths
}

// keep adds a path built for a collector that is not full, unless it holds
// the path already (see pathCollector), and reports whether it added it.
func (c *pathCollector) keep(built string, cut bool) bool {
	if !cut && slices.Contains(c.paths, built) {
		return false
	}

	c.paths = append(c.paths, built)

	return true
}

// addWant is addLazily for a misspelled member, with the spelling of the
// field it stands for.
func (c *pathCollector) addWant(path pathOf, want string) {
	if c.full() {
		c.omitted++

		return
	}

	if c.keep(path()) {
		c.wants = append(c.wants, want)
	}
}

// addLazily is add for a path that is only worth building when it is kept.
func (c *pathCollector) addLazily(path pathOf) {
	if c.full() {
		c.omitted++

		return
	}

	c.keep(path())
}

// foldName returns the name in the case-folded form encoding/json uses to
// match a member to a struct field.
func foldName(name string) string {
	var builder strings.Builder

	builder.Grow(len(name))

	for idx := 0; idx < len(name); idx++ {
		char := name[idx]

		switch {
		case 'a' <= char && char <= 'z':
			builder.WriteByte(char - 'a' + 'A')
		case char < utf8.RuneSelf:
			builder.WriteByte(char)
		default:
			folded, size := utf8.DecodeRuneInString(name[idx:])
			builder.WriteRune(foldRune(folded))

			idx += size - 1
		}
	}

	return builder.String()
}

// foldRune returns the smallest rune in the simple case folding orbit of
// char, as encoding/json does.
func foldRune(char rune) rune {
	for {
		next := unicode.SimpleFold(char)
		if next <= char {
			return next
		}

		char = next
	}
}

// quotePath renders a field path for a message. A path holding a bracketed
// segment (see joinFieldPath) has quoted the name in it already, so quoting
// the whole path again would escape those quotes and turn ["a.b"] into
// "[\"a.b\"]". Such a path is written with its plain runs quoted and its
// bracketed names as they are, as in ["a.b"] or "rules[0]"["x.y"]: every
// part is quoted once, so a list of paths reads one way even where a plain
// name holds a comma or a space. Every other path is quoted whole by
// merge.QuoteBounded, and so is one longer than merge.MaxQuotedBytes, which
// that cuts, or one joinFieldPath does not build: brackets that do not
// parse, or a bracketed name that is a plain path segment, which
// joinFieldPath writes after a dot. Read as a path, `a["b"]` would name the
// member b of the object a.
func quotePath(path string) string {
	if !strings.Contains(path, `["`) || len(path) > merge.MaxQuotedBytes {
		return merge.QuoteBounded(path)
	}

	var rendered strings.Builder

	for rest := path; rest != ""; {
		start := strings.Index(rest, `["`)
		if start < 0 {
			start = len(rest)
		}

		if start > 0 {
			rendered.WriteString(merge.QuoteBounded(rest[:start]))
		}

		rest = rest[start:]
		if rest == "" {
			break
		}

		name, size, ok := bracketedName(rest)
		if !ok {
			return merge.QuoteBounded(path)
		}

		rendered.WriteString("[" + merge.QuoteBounded(name) + "]")

		rest = rest[size:]
	}

	return rendered.String()
}

// bracketedName returns the name of the bracketed segment a path starts
// with, as joinFieldPath writes it, and its length, or false where the path
// starts with none: brackets that do not parse, or a name that is a plain
// path segment, which joinFieldPath writes after a dot instead.
func bracketedName(path string) (string, int, bool) {
	literal, err := strconv.QuotedPrefix(strings.TrimPrefix(path, "["))
	if err != nil || !strings.HasPrefix(path[1+len(literal):], "]") {
		return "", 0, false
	}

	name, err := strconv.Unquote(literal)
	if err != nil || plainFieldName(name) {
		return "", 0, false
	}

	return name, len(literal) + len("[]"), true
}

// pathsError wraps kind with the quoted paths, pluralizing the kind when
// there are several, as in `unknown fields "a", "b"`, or returns nil when
// there are none. Each path is quoted with a length bound and omitted counts
// the ones the collector left out, so that the message a runtime logs is
// bounded however the document that produced it was shaped. A path with a
// non-empty entry in wants is followed by the spelling of the field it
// stands for, as in `"Syscalls" (want "syscalls")`.
func pathsError(kind error, paths, wants []string, omitted int) error {
	if len(paths) == 0 {
		return nil
	}

	quoted := make([]string, len(paths))
	for idx, field := range paths {
		quoted[idx] = quotePath(field)

		if idx < len(wants) && wants[idx] != "" {
			quoted[idx] += " (want " + merge.QuoteBounded(wants[idx]) + ")"
		}
	}

	if len(paths) == 1 && omitted == 0 {
		return fmt.Errorf("%w %s", kind, quoted[0])
	}

	listed := strings.Join(quoted, ", ")
	if omitted > 0 {
		listed = fmt.Sprintf("%s and %d more", listed, omitted)
	}

	return fmt.Errorf("%ws %s", kind, listed)
}
