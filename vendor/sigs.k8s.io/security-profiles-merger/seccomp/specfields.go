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

package seccomp

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"

	specs "github.com/opencontainers/runtime-spec/specs-go"

	"sigs.k8s.io/security-profiles-merger/internal/merge"
)

// ErrUnsupportedSpecField is returned by Validate and Marshal for a profile
// setting a field of specs.LinuxSeccomp, specs.LinuxSyscall or
// specs.LinuxSeccompArg that this package does not read (it reads those of
// runtime-spec v1.3.0), which a merge would drop. UnmarshalStrict decodes
// such a field and Diff ignores it.
var ErrUnsupportedSpecField = errors.New("field this package does not read")

// The fields of the runtime-spec types the package reads. A field a newer
// runtime-spec adds is not in these lists, and Validate rejects a profile
// that sets it.
var (
	readProfileFields = []string{
		"DefaultAction", "DefaultErrnoRet", "Architectures", "Flags",
		"ListenerPath", "ListenerMetadata", "Syscalls",
	}
	readSyscallFields = []string{"Names", "Action", "ErrnoRet", "Args"}
	readArgFields     = []string{"Index", "Value", "ValueTwo", "Op"}

	unreadProfileFields = unreadFields(reflect.TypeFor[specs.LinuxSeccomp](), readProfileFields)
	unreadSyscallFields = unreadFields(reflect.TypeFor[specs.LinuxSyscall](), readSyscallFields)
	unreadArgFields     = unreadFields(reflect.TypeFor[specs.LinuxSeccompArg](), readArgFields)
)

// unreadFields returns the indices of the fields of the struct type that
// are not in read.
func unreadFields(typ reflect.Type, read []string) []int {
	var unread []int

	for idx := range typ.NumField() {
		if !slices.Contains(read, typ.Field(idx).Name) {
			unread = append(unread, idx)
		}
	}

	return unread
}

// reportSetFields reports each field of the struct value at one of the
// indices that holds anything but its zero value, by the JSON path of the
// member, which path, the path of the value, starts.
func reportSetFields(
	errs *merge.Collector, value reflect.Value, unread []int, path func() string,
) {
	for _, idx := range unread {
		if value.Field(idx).IsZero() {
			continue
		}

		errs.Report(func() error {
			field := value.Type().Field(idx)
			member := memberPath(path(), &field)

			return fmt.Errorf("%s: %w", member, ErrUnsupportedSpecField)
		})
	}
}

// memberPath returns the JSON path of the member a struct field fills, in
// the value at path, which is empty for the profile itself.
func memberPath(path string, field *reflect.StructField) string {
	name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
	if name == "" {
		name = field.Name
	}

	if path == "" {
		return name
	}

	return path + "." + name
}

// reportUnreadProfileFields reports the fields of the profile this package
// does not read (see ErrUnsupportedSpecField). With the runtime-spec this
// package is built against there are none, and it looks at nothing.
func reportUnreadProfileFields(errs *merge.Collector, profile *specs.LinuxSeccomp) {
	if len(unreadProfileFields) > 0 {
		reportSetFields(errs, reflect.ValueOf(profile).Elem(), unreadProfileFields,
			func() string { return "" })
	}
}

// reportUnreadEntryFields reports the fields of the syscall entry at idx and
// of its conditions that this package does not read, as
// reportUnreadProfileFields does for the profile.
func reportUnreadEntryFields(errs *merge.Collector, idx int, entry *specs.LinuxSyscall) {
	if len(unreadSyscallFields) > 0 {
		reportSetFields(errs, reflect.ValueOf(entry).Elem(), unreadSyscallFields,
			func() string { return fmt.Sprintf("syscalls[%d]", idx) })
	}

	if len(unreadArgFields) == 0 {
		return
	}

	for argIdx := range entry.Args {
		reportSetFields(errs, reflect.ValueOf(&entry.Args[argIdx]).Elem(), unreadArgFields,
			func() string { return fmt.Sprintf("syscalls[%d].args[%d]", idx, argIdx) })
	}
}

// validateSpecFields reports the fields of the profile, its syscall entries
// and their conditions that this package does not read, which Validate
// reports among the rest of what it finds.
func validateSpecFields(profile *specs.LinuxSeccomp) error {
	var errs merge.Collector

	reportUnreadProfileFields(&errs, profile)

	for idx := range profile.Syscalls {
		reportUnreadEntryFields(&errs, idx, &profile.Syscalls[idx])
	}

	return errs.Err()
}
