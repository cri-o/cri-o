package oci

import (
	"slices"
	"strings"
)

// mergeProcessEnvOverlay returns a new environment list for an OCI process: it
// starts from base (entries are typically KEY=value as in spec process.env),
// then applies overlay strings (CRI exec env: KEY=value lines) by case-sensitive
// key match.
//
// Semantics:
//   - Each overlay line with a non-empty key (per first '=') replaces any base
//     entry with the same key, or is appended if that key did not appear in base.
//   - Duplicate keys in overlay: the last entry wins.
//   - Relative order of base entries is preserved; new keys (not present in base)
//     are appended in first-appearance order among overlay keys (after last-wins
//     folding within overlay).
//   - Base entries that do not contain '=' are preserved as-is when not overridden.
//   - Empty overlay lines and lines with no valid key before '=' are skipped.
func mergeProcessEnvOverlay(base, overlay []string) []string {
	if len(overlay) == 0 {
		return slices.Clone(base)
	}

	ov := make(map[string]string, len(overlay))
	order := make([]string, 0, len(overlay))
	seen := make(map[string]struct{}, len(overlay))

	for _, line := range overlay {
		if line == "" {
			continue
		}

		k, ok := envLineKey(line)
		if !ok || k == "" {
			continue
		}

		parts := strings.SplitN(line, "=", 2)

		val := ""
		if len(parts) == 2 {
			val = parts[1]
		}

		ov[k] = val
		if _, ok := seen[k]; !ok {
			seen[k] = struct{}{}
			order = append(order, k)
		}
	}

	baseKeys := make(map[string]struct{}, len(base))
	for _, line := range base {
		k, ok := envLineKey(line)
		if ok {
			baseKeys[k] = struct{}{}
		}
	}

	out := make([]string, 0, len(base)+len(order))
	for _, line := range base {
		k, ok := envLineKey(line)
		if !ok {
			out = append(out, line)

			continue
		}

		if val, replace := ov[k]; replace {
			out = append(out, k+"="+val)

			continue
		}

		out = append(out, line)
	}

	for _, k := range order {
		if _, inBase := baseKeys[k]; inBase {
			continue
		}

		out = append(out, k+"="+ov[k])
	}

	return out
}

// envLineKey returns the key portion of a process.env entry. The OCI spec uses
// NAME=value strings; value may be empty and may contain '=' (use SplitN).
func envLineKey(line string) (key string, ok bool) {
	parts := strings.SplitN(line, "=", 2)
	if len(parts) == 0 || parts[0] == "" {
		return "", false
	}

	return parts[0], true
}
