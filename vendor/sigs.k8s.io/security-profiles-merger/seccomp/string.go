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
	specs "github.com/opencontainers/runtime-spec/specs-go"

	"sigs.k8s.io/security-profiles-merger/internal/merge"
)

// FormatProfile returns the profile as a single line that is safe to log: a
// value holding a control character, an invisible rune or the punctuation
// the format is built from is quoted, and the line is cut at 64 KiB, ending
// in "... (output truncated)".
func FormatProfile(profile *specs.LinuxSeccomp) string {
	if profile == nil {
		return "Profile{<nil>}"
	}

	var out merge.Builder

	out.WriteString("Profile{default:")
	out.WriteText(string(profile.DefaultAction))

	if profile.DefaultErrnoRet != nil {
		out.WriteString(" defaultErrno:")
		out.WriteUint(uint64(*profile.DefaultErrnoRet))
	}

	if len(profile.Architectures) > 0 {
		out.WriteString(" arch:")
		merge.WriteTexts(&out, profile.Architectures, ",")
	}

	if profile.Flags != nil {
		out.WriteString(" ")
		writeFlags(&out, profile.Flags)
	}

	if profile.ListenerPath != "" {
		out.WriteString(" listener:")
		out.WriteText(profile.ListenerPath)
	}

	// Printed without a listenerPath too: Diff reports it, ValidateArtifact
	// rejects it and Intersect refuses a later input setting it, so two
	// profiles differing only there must not print alike.
	if profile.ListenerMetadata != "" {
		out.WriteString(" listenerMeta:")
		out.WriteText(profile.ListenerMetadata)
	}

	for idx := range profile.Syscalls {
		if out.Full() {
			break
		}

		out.WriteString(" ")
		writeSyscall(&out, &profile.Syscalls[idx])
	}

	out.WriteString("}")

	return out.String()
}

// writeFlags renders a flag list that is set. An empty one reads "none"
// rather than nothing: it turns off the flags a runtime sets for a profile
// without a list, so it is not the same profile.
func writeFlags(out *merge.Builder, flags []specs.LinuxSeccompFlag) {
	if len(flags) == 0 {
		out.WriteString("flags:none")

		return
	}

	out.WriteString("flags:")
	merge.WriteTexts(out, flags, ",")
}

// String returns a human-readable representation of the syscall entry,
// quoted and bounded as FormatProfile is.
func (e SyscallEntry) String() string {
	return merge.Render(e.writeTo)
}

// writeTo renders the entry: its name, the arguments it applies to and its
// result.
func (e SyscallEntry) writeTo(out *merge.Builder) {
	out.WriteText(e.Name)
	writeResult(out, e.Args, e.Action, e.ErrnoRet)
}

// String returns a human-readable representation of the syscall detail,
// quoted and bounded as FormatProfile is.
func (d SyscallDetail) String() string {
	return merge.Render(d.writeTo)
}

// writeTo renders the detail: its result, after the arguments it applies
// to if it has any.
func (d SyscallDetail) writeTo(out *merge.Builder) {
	if len(d.Args) == 0 {
		writeAction(out, d.Action, d.ErrnoRet)

		return
	}

	writeResult(out, d.Args, d.Action, d.ErrnoRet)
}

// writeResult renders what follows the names of an entry: its arguments in
// parentheses, if it has any, and then "->" and its action.
func writeResult(
	out *merge.Builder, args []specs.LinuxSeccompArg,
	action specs.LinuxSeccompAction, errnoRet *uint,
) {
	if len(args) > 0 {
		out.WriteString("(")
		writeArgs(out, args)
		out.WriteString(")")
	}

	out.WriteString("->")
	writeAction(out, action, errnoRet)
}

// writeAction renders an action, followed by its errno value if it has one.
func writeAction(out *merge.Builder, action specs.LinuxSeccompAction, errnoRet *uint) {
	out.WriteText(string(action))

	if errnoRet != nil {
		out.WriteString("(errno:")
		out.WriteUint(uint64(*errnoRet))
		out.WriteString(")")
	}
}

func writeArgs(out *merge.Builder, args []specs.LinuxSeccompArg) {
	for idx := range args {
		if out.Full() {
			return
		}

		arg := &args[idx]

		if idx > 0 {
			out.WriteString(",")
		}

		out.WriteString("[")
		out.WriteUint(uint64(arg.Index))
		out.WriteString("]")
		out.WriteText(string(arg.Op))
		out.WriteString(":")
		out.WriteUint(arg.Value)

		if arg.Op == specs.OpMaskedEqual {
			out.WriteString(":")
			out.WriteUint(arg.ValueTwo)
		}
	}
}

// FormatSyscall returns one syscall entry of a profile as FormatProfile
// writes it, quoted and bounded as FormatProfile is: its names, the
// arguments it applies to and its result, as in "read,write->SCMP_ACT_ALLOW".
// A nil entry reads "<nil>".
func FormatSyscall(syscall *specs.LinuxSyscall) string {
	if syscall == nil {
		return "<nil>"
	}

	return merge.Render(func(out *merge.Builder) { writeSyscall(out, syscall) })
}

func writeSyscall(out *merge.Builder, syscall *specs.LinuxSyscall) {
	merge.WriteTexts(out, syscall.Names, ",")
	writeResult(out, syscall.Args, syscall.Action, syscall.ErrnoRet)
}
