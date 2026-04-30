package oci

import "strings"

// NormalizeExecCmdArgs returns a copy of args with any leading \r and \n removed from
// each string. A shell treats leading blank lines in a -c script as insignificant;
// trimming avoids exec I/O path bugs when an argument starts with a newline, which
// in some environments can surface as leaked OCI process metadata in the stream
// (https://github.com/cri-o/cri-o/issues/9885).
func NormalizeExecCmdArgs(args []string) []string {
	if len(args) == 0 {
		// Preserve nil vs empty-slice identity for json.Marshal of the OCI process spec.
		return args
	}
	out := make([]string, len(args))
	for i, a := range args {
		out[i] = strings.TrimLeft(a, "\n\r")
	}
	return out
}
