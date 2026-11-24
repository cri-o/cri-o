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
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// safeText returns a value taken from a profile as it can be printed. A
// value holding a control character or a byte that is not valid UTF-8 is
// quoted, which spells those bytes out, and so is one that holds whitespace
// or the punctuation the formatters build their output from (see
// formatPunctuation); every other value is returned as it is, so that the
// usual rendering of a profile is unchanged. The empty value is quoted too:
// written as nothing, a Landlock path "" added by a diff would read as
// "fs:+" and a seccomp entry naming "" as "->SCMP_ACT_ALLOW".
//
// The formatters write into a runtime's log and onto an operator's terminal.
// A path or a syscall name of an artifact is chosen by its author, and a
// newline in one forges a log line while an escape sequence moves the cursor
// or repaints what is already on screen. The validators already report every
// value this way (see QuoteBounded); this is the same rule for the values a
// result carries rather than the ones a failure names.
//
// The punctuation is quoted for the same reason: the formatters join values
// with commas and spaces and wrap them in braces, parentheses, "->" and
// "<none>". Unquoted, the capability "KILL,-CHOWN" would render as two
// entries of a diff, and the AppArmor path "/etc/{a,b}" as the two paths
// "/etc/{a" and "b}". An AppArmor alternation is therefore always quoted,
// which is noisier than it needs to be but never ambiguous.
func safeText(value string) string {
	if plainText(value) {
		return value
	}

	return quoteVisible(value)
}

// SafeName returns a name the command prints, such as a file argument, as
// it can be written to a terminal. It quotes only what safeText quotes for
// being unprintable: a name is not joined with other values, so the
// punctuation that makes a profile value ambiguous means nothing in it, and
// quoting it would spell out every Windows path, whose drive letter and
// separators hold ':' and '\'.
func SafeName(name string) string {
	if printableText(name) {
		return name
	}

	return quoteVisible(name)
}

// formatPunctuation holds the characters the formatters build their output
// from: separators, brackets, the arrow of "->" and the brackets of "<none>",
// and the quote that starts a quoted value. Square brackets are left out: an
// AppArmor path holds them for a character class, and every format that
// uses them also puts one of these characters beside them.
const formatPunctuation = `,"{}()<>:`

// plainText reports whether a value can be written as it is: not empty,
// printable (see printableText) and free of whitespace and
// formatPunctuation.
func plainText(value string) bool {
	return value != "" && printableText(value) &&
		!strings.ContainsAny(value, formatPunctuation) &&
		strings.IndexFunc(value, unicode.IsSpace) < 0
}

// printableText reports whether every rune of a value prints as itself: valid
// UTF-8 whose every rune PrintsAsItself. That is the same line QuoteBounded
// draws, so a value the validators quote is a value the formatters quote:
// besides the C0 controls and DEL it covers the C1 controls and the format
// characters, such as U+202E, which reorder what is already on screen
// without moving the cursor, and the runes that print as nothing at all.
func printableText(value string) bool {
	if !utf8.ValidString(value) {
		return false
	}

	return strings.IndexFunc(value, func(char rune) bool {
		return !PrintsAsItself(char)
	}) < 0
}

// PrintsAsItself reports whether a rune shows as what it is when written to
// a terminal or a log. strconv.IsPrint draws most of that line, leaving out
// the controls, the format characters and every space but U+0020, such as
// U+00A0 and U+3000; besides those, the runes it takes for printable that
// render as nothing or as a blank are left out too: the default ignorable
// code points, such as U+3164 HANGUL FILLER, U+115F, U+1160, U+FFA0 and the
// variation selectors, and U+2800 BRAILLE PATTERN BLANK. "/usr/bin" with
// one of them inside reads as "/usr/bin", or as two words, while naming
// another file.
func PrintsAsItself(char rune) bool {
	return strconv.IsPrint(char) && !invisible(char)
}

// brailleBlank is U+2800 BRAILLE PATTERN BLANK, a symbol that renders as a
// space.
const brailleBlank = '\u2800'

// invisible reports whether a rune that strconv.IsPrint takes for printable
// renders as nothing or as a blank: a default ignorable code point or
// brailleBlank. The format characters, which make up the rest of the
// default ignorable ones, and the spaces are not printable to it already.
func invisible(char rune) bool {
	return char == brailleBlank ||
		unicode.In(char, unicode.Other_Default_Ignorable_Code_Point, unicode.Variation_Selector)
}

// quoteVisible is strconv.Quote, also escaping the runes strconv.Quote
// leaves as they are although they print as nothing (see PrintsAsItself),
// so that a quoted value never hides part of itself.
func quoteVisible(value string) string {
	quoted := strconv.Quote(value)
	if strings.IndexFunc(quoted, invisible) < 0 {
		return quoted
	}

	var builder strings.Builder

	for _, char := range quoted {
		switch {
		case !invisible(char):
			builder.WriteRune(char)
		case char <= maxBMP:
			_, _ = fmt.Fprintf(&builder, `\u%04x`, char)
		default:
			_, _ = fmt.Fprintf(&builder, `\U%08x`, char)
		}
	}

	return builder.String()
}

// maxBMP is the last rune a \u escape spells.
const maxBMP = 0xFFFF
