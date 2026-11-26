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
	"reflect"
	"slices"
	"sync"

	specs "github.com/opencontainers/runtime-spec/specs-go"
)

// The struct tag `strict:"required"` marks a field a document has to hold.
// It is for a field whose zero value is a value of its own, so that a member
// left out cannot be told from one that names it. requiredFields marks the
// fields of types whose declaration cannot carry the tag.
const (
	requiredTagKey   = "strict"
	requiredTagValue = "required"
)

// requiredFields lists, by struct type and Go field name, the fields a
// document has to hold besides those the tag marks, for types another
// module declares. It is keyed by type, so an entry only reaches a document
// whose type holds that type.
//
// A seccomp condition without "index" or "op" decodes as a condition on
// argument 0 or without an operator: {"value":5} reads as a condition on
// a0, which the document never said. "value" and "valueTwo" may be left
// out: the Security Profiles Operator writes them with omitempty, so
// a0 == 0 arrives as {"index":0,"op":"SCMP_CMP_EQ"}, and 0 is what the
// member would say.
//
// A syscall entry has to hold "names" and "action", as the runtime spec
// requires: an entry without either says nothing a runtime can load, and
// a null element of "syscalls", which decodes as an entry holding neither,
// is refused as missing both, as a null rule of a Landlock profile is.
var requiredFields = map[reflect.Type][]string{
	reflect.TypeFor[specs.LinuxSeccompArg](): {"Index", "Op"},
	reflect.TypeFor[specs.LinuxSyscall]():    {"Names", "Action"},
}

// requiresCache caches requiresFields, by type.
var requiresCache sync.Map

// isRequired reports whether a field of the struct type owner is required:
// its tag marks it, or requiredFields lists it.
func isRequired(owner reflect.Type, field *reflect.StructField) bool {
	return field.Tag.Get(requiredTagKey) == requiredTagValue ||
		slices.Contains(requiredFields[owner], field.Name)
}

// checksMissing reports whether a document is read for missing members
// (see scanDocument): a required member left out or null decodes as the
// zero value, so only a type with a required field, and a value decoded
// from the document that holds one of them zero, need it.
func checksMissing[T any](decoded *T) bool {
	return requiresFields(reflect.TypeFor[T]()) &&
		holdsZeroRequired(reflect.ValueOf(decoded))
}

// requiresFields reports whether a struct type reachable from target has a
// required field.
func requiresFields(target reflect.Type) bool {
	if cached, ok := requiresCache.Load(target); ok {
		found, _ := cached.(bool)

		return found
	}

	found := false
	seen := map[reflect.Type]bool{}
	pending := []reflect.Type{target}

	for len(pending) > 0 && !found {
		typ := elementType(pending[len(pending)-1])
		pending = pending[:len(pending)-1]

		if typ.Kind() != reflect.Struct || seen[typ] {
			continue
		}

		seen[typ] = true

		for idx := range typ.NumField() {
			field := typ.Field(idx)
			found = found || isRequired(typ, &field)
			pending = append(pending, field.Type)
		}
	}

	requiresCache.Store(target, found)

	return found
}

// holdsZeroRequired reports whether a decoded value holds the zero value in
// a required field, at any depth.
func holdsZeroRequired(value reflect.Value) bool {
	kind := value.Kind()

	if kind == reflect.Pointer {
		return !value.IsNil() && holdsZeroRequired(value.Elem())
	}

	if kind == reflect.Slice {
		for idx := range value.Len() {
			if holdsZeroRequired(value.Index(idx)) {
				return true
			}
		}

		return false
	}

	return kind == reflect.Struct && structHoldsZeroRequired(value)
}

// structHoldsZeroRequired is holdsZeroRequired for a struct.
func structHoldsZeroRequired(value reflect.Value) bool {
	for idx := range value.NumField() {
		field := value.Type().Field(idx)
		if !field.IsExported() {
			continue
		}

		if isRequired(value.Type(), &field) && value.Field(idx).IsZero() ||
			holdsZeroRequired(value.Field(idx)) {
			return true
		}
	}

	return false
}

// reportMissing collects the required members of an object of a struct
// type at path that named holds no value for, and those of a struct member
// it holds no value for, whose fields are all zero then (see reportZero).
// named holds the position of each field a member of the object gives a
// value other than null; nil stands for an object naming nothing.
//
// A path is only built for a member the collector keeps, since an object
// closes with nothing missing far more often than not. Until then the path
// is the names path holds, which reportMissing extends by a member and
// takes back, so that a member no path is built for costs no allocation.
func reportMissing(
	typ reflect.Type, path *missingPath, named *fieldMask, found *pathCollector,
) {
	for idx, member := range fieldsOf(typ).fields {
		if named.has(idx) {
			continue
		}

		path.members = append(path.members, member.name)

		switch {
		case !member.required:
		case found.full():
			found.omitted++
		default:
			found.keep(path.build())
		}

		reportZero(member.typ, path, found)

		path.members = path.members[:len(path.members)-1]
	}
}

// reportZero collects the required members of a value of type typ the
// decoder leaves zero at path: those of a struct. A pointer or slice stays
// nil and holds nothing to miss.
func reportZero(typ reflect.Type, path *missingPath, found *pathCollector) {
	if typ != nil && typ.Kind() == reflect.Struct && requiresFields(typ) {
		reportMissing(typ, path, nil, found)
	}
}

// missingPath is the path of a member reportMissing looks at: that of the
// value the innermost frame of stack is at (see memberPath), followed by
// the names of members, each a member of a struct the one before it holds.
// It is built only when a path is kept. A scan keeps one and reuses its
// names, which grow no longer than the profile types nest structs.
type missingPath struct {
	stack   []scanFrame
	members []string
}

// at returns path reset to the value the innermost frame of stack is at.
func (path *missingPath) at(stack []scanFrame) *missingPath {
	path.stack = stack
	path.members = path.members[:0]

	return path
}

// build returns the path, and whether memberPath cut it.
func (path *missingPath) build() (string, bool) {
	built, cut := memberPath(path.stack)
	for _, member := range path.members {
		built = joinFieldPath(built, member)
	}

	return built, cut
}
