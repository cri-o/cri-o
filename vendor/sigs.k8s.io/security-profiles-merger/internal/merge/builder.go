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

package merge

import (
	"strconv"
	"strings"

	"sigs.k8s.io/security-profiles-merger/spm"
)

// MaxFormattedBytes bounds the output of a formatter: seccomp.FormatProfile
// and the String methods of the profile and diff types, all of
// which render through a Builder. Escaping keeps an artifact's values from
// acting on a terminal or forging a log line, but not from choosing how
// much is written: an artifact ValidateArtifact accepts may hold a thousand
// paths of MaxPathLen bytes, each of which renders at four times its length
// once its bytes are escaped, so its diff would otherwise reach a runtime's
// log as a single line of 16 MB.
//
// The bound is set well above what a real profile renders to, so that
// none is cut: the CRI-O default seccomp profile renders to under 6 KiB,
// and its diff against a profile allowing everything, the largest output
// the profiles and examples in this repository produce, to under 13 KiB. It
// also holds a few paths of MaxPathLen bytes even when every one of their
// bytes is escaped, so an output is not cut at the first long path.
const MaxFormattedBytes = 64 << 10

// TruncationMarker ends the output of a formatter that reached
// MaxFormattedBytes, in place of what did not fit. It holds a space and a
// parenthesis, which safeText quotes in a value, so no value of a profile
// renders as the marker.
const TruncationMarker = "... (output truncated)"

// Truncated reports whether output, which a formatter returned, was cut at
// MaxFormattedBytes. Only a cut output ends in TruncationMarker: no value
// renders as the marker (see TruncationMarker), and no formatter writes it
// itself.
func Truncated(output string) bool {
	return strings.HasSuffix(output, TruncationMarker)
}

// maxWrittenBytes is what the writes to a Builder share, leaving room for
// the marker within MaxFormattedBytes.
const maxWrittenBytes = MaxFormattedBytes - len(TruncationMarker)

// Builder collects a formatter's output within MaxFormattedBytes, marker
// included: the writes share the bound less the length of TruncationMarker,
// so that the marker always fits behind them. Every write is all or nothing:
// the first one that does not fit is dropped, the TruncationMarker is
// written in its place, and every write after it is ignored. A value is
// written whole or not at all, so the output is never cut inside an escape
// sequence or a UTF-8 sequence, and the marker always directly follows the
// last write that fit. No value is cut short either: a path of MaxPathLen
// bytes is at most 16 KiB once escaped and is written whole wherever it
// fits, and a longer value, which only a profile nothing validated holds, is
// dropped as any other write that does not fit.
//
// Ignoring the writes past the bound bounds the cost of formatting as well,
// as long as the formatter does not build what it writes before writing it;
// a formatter walking a list of a profile's size also checks Full to stop
// early rather than visit every entry only to have each write ignored. The
// zero value is ready to use.
type Builder struct {
	builder strings.Builder
	full    bool
}

// WriteString writes text that the formatter chose, such as punctuation or
// a field name, rather than a value of the profile, which WriteText writes.
func (b *Builder) WriteString(text string) {
	if b.fits(len(text)) {
		b.builder.WriteString(text)
	}
}

// WriteText writes a value of the profile as safeText renders it. safeText
// never returns fewer bytes than it is given, so a value longer than the
// room left is dropped without being quoted: quoting it would cost as much
// as the value is long, which is the cost the bound is there to cap.
func (b *Builder) WriteText(value string) {
	if b.fits(len(value)) {
		b.WriteString(safeText(value))
	}
}

// WriteUint writes a number in decimal.
func (b *Builder) WriteUint(value uint64) {
	const decimal = 10

	var digits [20]byte

	b.WriteString(string(strconv.AppendUint(digits[:0], value, decimal)))
}

// Full reports whether the output reached its bound, so that every further
// write is ignored.
func (b *Builder) Full() bool {
	return b.full
}

// String returns the output, ending in TruncationMarker when it was cut.
func (b *Builder) String() string {
	return b.builder.String()
}

// fits reports whether size more bytes fit in the output. When they do
// not, the output is full from here on and ends in the marker.
func (b *Builder) fits(size int) bool {
	if b.full {
		return false
	}

	if size > maxWrittenBytes-b.builder.Len() {
		b.full = true
		b.builder.WriteString(TruncationMarker)

		return false
	}

	return true
}

// Render returns the output write writes to a new Builder.
func Render(write func(out *Builder)) string {
	var out Builder

	write(&out)

	return out.String()
}

// Framed returns open, then the parts body writes separated by spaces, then
// "}", as in "Diff{fs:+read_file net:-bind_tcp}".
func Framed(open string, body func(out *Builder, parts *Joiner)) string {
	return Render(func(out *Builder) {
		out.WriteString(open)

		parts := NewJoiner(out, " ")
		body(out, &parts)

		out.WriteString("}")
	})
}

// Joiner writes a separator between the parts of a list written to a
// Builder, for a list whose first part is not known in advance.
type Joiner struct {
	out     *Builder
	sep     string
	started bool
}

// NewJoiner returns a Joiner separating parts written to out with sep.
func NewJoiner(out *Builder, sep string) Joiner {
	return Joiner{out: out, sep: sep}
}

// Next starts a part, writing the separator unless it is the first.
func (j *Joiner) Next() {
	if j.started {
		j.out.WriteString(j.sep)
	}

	j.started = true
}

// WriteTexts writes values as WriteText does, separated by sep, stopping
// once the output is full.
func WriteTexts[T ~string](out *Builder, values []T, sep string) {
	for idx, value := range values {
		if out.Full() {
			return
		}

		if idx > 0 {
			out.WriteString(sep)
		}

		out.WriteText(string(value))
	}
}

// WriteParts writes count parts of a list, each after sign, stopping once
// the output is full. write writes the part at idx after its sign.
func WriteParts(out *Builder, parts *Joiner, count int, sign string, write func(idx int)) {
	for idx := range count {
		if out.Full() {
			return
		}

		parts.Next()
		out.WriteString(sign)
		write(idx)
	}
}

// WriteList writes values as WriteTexts does, separated by commas, after
// prefix as a part of a list, as in "exec:/bin/a,/bin/b", and writes nothing
// for an empty list.
func WriteList[T ~string](out *Builder, parts *Joiner, prefix string, values []T) {
	if len(values) == 0 {
		return
	}

	parts.Next()
	out.WriteString(prefix)
	WriteTexts(out, values, ",")
}

// WriteSliceDiff writes a SliceDiff as a part of a diff, listing the
// removed items with a "-" and then the added items with a "+", as in
// "caps:-CHOWN,+KILL", and writes nothing for a nil diff, which stands for
// an equal list (see NewSliceDiff). An item holding bytes that are not safe
// to print is quoted (see safeText): a diff is computed over profiles
// nothing validated, and its rendering is what a runtime logs.
func WriteSliceDiff[T ~string](out *Builder, parts *Joiner, prefix string, diff *spm.SliceDiff[T]) {
	if diff == nil {
		return
	}

	parts.Next()
	out.WriteString(prefix)
	out.WriteString(":")
	writeSigned(out, "-", diff.Removed, "")

	sep := ""
	if len(diff.Removed) > 0 {
		sep = ","
	}

	writeSigned(out, "+", diff.Added, sep)
}

// writeSigned writes values as WriteText does, each after sign and every
// one after the first also after a comma. sep is written before the first
// value, for a list that continues one written before it.
func writeSigned[T ~string](out *Builder, sign string, values []T, sep string) {
	for _, value := range values {
		if out.Full() {
			return
		}

		out.WriteString(sep)
		out.WriteString(sign)
		out.WriteText(string(value))

		sep = ","
	}
}
