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

package strictjson

import (
	"cmp"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"

	"sigs.k8s.io/security-profiles-merger/spm"
)

// invalidUTF8 reports the first byte of raw that does not start a valid
// UTF-8 sequence, or the first \u escape that spells half a surrogate pair,
// as a *UTF8Error, or nil when there is neither. encoding/json replaces both
// with U+FFFD, so two profiles whose syscall names differ only in them
// decode to the same name and merge into one rule; in an artifact, a name
// spelled that way is a sign the bytes were crafted.
func invalidUTF8(raw []byte) error {
	if utf8.Valid(raw) {
		offset := loneSurrogateEscape(raw)
		if offset < 0 {
			return nil
		}

		return &UTF8Error{Offset: offset, Escape: true}
	}

	offset := 0

	for offset < len(raw) {
		_, size := utf8.DecodeRune(raw[offset:])
		if size == 1 && raw[offset] >= utf8.RuneSelf {
			break
		}

		offset += size
	}

	return &UTF8Error{Offset: offset}
}

// UTF8Error is spm.ErrInvalidUTF8 at a byte offset of a document, which
// the command moves to count bytes of the stdin array a document is an
// element of.
type UTF8Error struct {
	// Offset is the offset of the byte, or of the escape, in the document.
	Offset int
	// Escape reports a \u escape that spells half a surrogate pair rather
	// than a byte that is not valid UTF-8.
	Escape bool
}

func (e *UTF8Error) Error() string {
	if e.Escape {
		return fmt.Sprintf("%v (lone surrogate escape at byte %d)", spm.ErrInvalidUTF8, e.Offset)
	}

	return fmt.Sprintf("%v (first at byte %d)", spm.ErrInvalidUTF8, e.Offset)
}

// Unwrap returns spm.ErrInvalidUTF8.
func (e *UTF8Error) Unwrap() error { return spm.ErrInvalidUTF8 }

// loneSurrogateEscape returns the offset of the first \u escape in raw that
// names a surrogate without its other half, or -1. A backslash only occurs
// inside a string in valid JSON, and raw that is not valid JSON is refused
// by the decoder, so the scan does not track strings.
func loneSurrogateEscape(raw []byte) int {
	for offset := 0; offset < len(raw); offset++ {
		if raw[offset] != '\\' {
			continue
		}

		char, isEscape := unicodeEscape(raw[offset:])
		if !isEscape {
			// Skip the escaped byte, which may be a backslash itself.
			offset++

			continue
		}

		low, lowIsEscape := unicodeEscape(raw[min(offset+unicodeEscapeLen, len(raw)):])

		switch {
		case !utf16.IsSurrogate(char):
			offset += unicodeEscapeLen - 1
		case lowIsEscape && utf16.DecodeRune(char, low) != unicode.ReplacementChar:
			offset += unicodeEscapeLen + unicodeEscapeLen - 1
		default:
			return offset
		}
	}

	return -1
}

// unicodeEscapeLen is the length of a \uXXXX escape.
const unicodeEscapeLen = 6

// unicodeEscape returns the code unit a \uXXXX escape at the start of raw
// names.
func unicodeEscape(raw []byte) (rune, bool) {
	if len(raw) < unicodeEscapeLen || raw[0] != '\\' || raw[1] != 'u' {
		return 0, false
	}

	value, err := strconv.ParseUint(string(raw[2:unicodeEscapeLen]), 16, 16)
	if err != nil {
		return 0, false
	}

	return rune(value), true
}

// decoderFieldName returns the member name out of encoding/json's unknown
// field error, and whether err is one.
func decoderFieldName(err error) (string, bool) {
	if err == nil {
		return "", false
	}

	quoted, found := strings.CutPrefix(err.Error(), "json: unknown field ")
	if !found {
		return "", false
	}

	name, unquoteErr := strconv.Unquote(quoted)
	if unquoteErr != nil {
		return quoted, true
	}

	return name, true
}

// HasField reports whether a member of the given name fills a field of the
// struct type target, matched the way encoding/json matches it: exactly or
// ignoring case.
func HasField(target reflect.Type, name string) bool {
	for target.Kind() == reflect.Pointer {
		target = target.Elem()
	}

	if target.Kind() != reflect.Struct {
		return false
	}

	_, known, _ := fieldsOf(target).lookup(name)

	return known
}

// unreadableDocument is the path reported when the scan cannot read a
// document the decoder read, so that the check fails closed. Numbers are
// kept as text, so this takes a document past what encoding/json reads at
// all, which the decoder refuses first.
const unreadableDocument = "(document)"

// elementType returns the type a value of typ holds its members in: the
// type itself, or the element type of a pointer or slice.
func elementType(typ reflect.Type) reflect.Type {
	for typ.Kind() == reflect.Pointer || typ.Kind() == reflect.Slice {
		typ = typ.Elem()
	}

	return typ
}

// joinFieldPath appends a member name to a field path. A name that cannot
// be spelled as a plain path segment is bracketed and quoted, the way an
// array index is, so that one path names one member: written with a dot
// unconditionally, the member "a.b" and the member "b" of the object "a"
// spell the same path, and a member named "" spells the path of the object
// holding it. Every member of a profile document is a plain name, so this
// only shows up for a document that is not one.
func joinFieldPath(prefix, key string) string {
	if !plainFieldName(key) {
		return prefix + "[" + strconv.Quote(key) + "]"
	}

	if prefix == "" {
		return key
	}

	return prefix + "." + key
}

// plainFieldName reports whether a member name can be a path segment as it
// is: a non-empty, valid UTF-8 name holding none of the punctuation a path
// is built from and nothing unprintable. A name outside that, which no
// profile document has, is bracketed and quoted instead, which also spells
// out a byte a terminal would otherwise swallow.
func plainFieldName(name string) bool {
	if name == "" || !utf8.ValidString(name) {
		return false
	}

	for _, char := range name {
		switch {
		case char == '.', char == '[', char == ']', char == '"', char == '\\':
			return false
		case unicode.IsControl(char):
			return false
		}
	}

	return true
}

// fieldSet holds the JSON-visible fields of a struct type: in field order,
// by exact name, and by name folded the way encoding/json folds a name for
// the fallback match. That folding is not strings.ToLower: "\u0130" lowers
// to "i" but folds to itself, so a lowered lookup takes a member the
// decoder drops for a known one.
type fieldSet struct {
	fields []jsonField
	exact  map[string]jsonField
	folded map[string]jsonField
}

// jsonField is a field of a struct as a member name matches it.
type jsonField struct {
	// name is the JSON name of the field.
	name string
	// typ is the type of the field.
	typ reflect.Type
	// index is the position of the field in fieldSet.fields.
	index int
	// required reports whether a document has to hold the member (see
	// isRequired).
	required bool
}

// lookup returns the field a member fills, whether one does, and whether
// the member names it exactly rather than only ignoring case.
func (set fieldSet) lookup(key string) (jsonField, bool, bool) {
	return set.lookupFolded(key, foldName(key))
}

// lookupFolded is lookup for a member whose folded name (see foldName) the
// caller holds already, which it does not fold again.
func (set fieldSet) lookupFolded(key, folded string) (jsonField, bool, bool) {
	if field, ok := set.exact[key]; ok {
		return field, true, true
	}

	field, ok := set.folded[folded]

	return field, ok, false
}

// fieldSets holds the field set of every struct type asked for, by type. A
// walk asks once per object of a document and type detection once per
// member, and a document chooses how many of both it has, while the types
// are the handful a profile is made of.
var fieldSets sync.Map

// fieldsOf returns jsonFields of a struct type, computed once per type.
func fieldsOf(typ reflect.Type) fieldSet {
	if cached, ok := fieldSets.Load(typ); ok {
		set, _ := cached.(fieldSet)

		return set
	}

	set := jsonFields(typ)
	fieldSets.Store(typ, set)

	return set
}

// jsonFields collects the fields encoding/json decodes the members of an
// object into for a struct type: every exported field not tagged "-", by
// the name its json tag gives it or by its Go name. Where two names fold
// alike, the folded lookup takes the field encoding/json takes, the first
// in field order.
//
// That is all of encoding/json's field resolution the profile types need.
// None of them embeds a struct, carries a json tag that is no valid name or
// gives two fields one name, the shapes encoding/json resolves by further
// rules, and its releases built on encoding/json/v2 resolve some of them
// differently. TestProfileTypesHaveFlatFields keeps it that way.
func jsonFields(typ reflect.Type) fieldSet {
	set := fieldSet{
		fields: make([]jsonField, 0, typ.NumField()),
		exact:  make(map[string]jsonField, typ.NumField()),
		folded: make(map[string]jsonField, typ.NumField()),
	}

	for idx := range typ.NumField() {
		field := typ.Field(idx)

		tag := field.Tag.Get("json")
		if !field.IsExported() || tag == "-" {
			continue
		}

		name, _, _ := strings.Cut(tag, ",")
		member := jsonField{
			name:     cmp.Or(name, field.Name),
			typ:      field.Type,
			index:    len(set.fields),
			required: isRequired(typ, &field),
		}

		set.fields = append(set.fields, member)
		set.exact[member.name] = member

		folded := foldName(member.name)
		if _, taken := set.folded[folded]; !taken {
			set.folded[folded] = member
		}
	}

	return set
}
