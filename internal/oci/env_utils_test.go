package oci

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestMergeProcessEnvOverlay(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		base    []string
		overlay []string
		want    []string
	}{
		"nil overlay": {
			base:    []string{"A=1", "B=2"},
			overlay: nil,
			want:    []string{"A=1", "B=2"},
		},
		"empty overlay": {
			base:    []string{"A=1"},
			overlay: []string{},
			want:    []string{"A=1"},
		},
		"replace existing": {
			base:    []string{"A=1", "B=2"},
			overlay: []string{"B=3"},
			want:    []string{"A=1", "B=3"},
		},
		"append new": {
			base:    []string{"A=1"},
			overlay: []string{"C=3"},
			want:    []string{"A=1", "C=3"},
		},
		"replace and append": {
			base:    []string{"A=1", "B=2"},
			overlay: []string{"B=x", "C=y"},
			want:    []string{"A=1", "B=x", "C=y"},
		},
		"duplicate overlay keys last wins": {
			base:    []string{"A=1"},
			overlay: []string{"B=first", "B=second"},
			want:    []string{"A=1", "B=second"},
		},
		"duplicate overlay keys replace base last wins": {
			base:    []string{"A=1", "B=old"},
			overlay: []string{"B=mid", "B=new"},
			want:    []string{"A=1", "B=new"},
		},
		"value with equals sign": {
			base:    []string{"A=b=c"},
			overlay: []string{"A=d=e=f"},
			want:    []string{"A=d=e=f"},
		},
		"empty overlay value": {
			base:    []string{"A=1"},
			overlay: []string{"B="},
			want:    []string{"A=1", "B="},
		},
		"overlay key only implies empty value": {
			base:    []string{"A=1"},
			overlay: []string{"B"},
			want:    []string{"A=1", "B="},
		},
		"skip empty overlay line": {
			base:    []string{"A=1"},
			overlay: []string{"", "B=2"},
			want:    []string{"A=1", "B=2"},
		},
		"skip line with empty key before equals": {
			base:    []string{"A=1"},
			overlay: []string{"=x", "B=2"},
			want:    []string{"A=1", "B=2"},
		},
		"new key order follows first appearance in overlay": {
			base:    []string{"A=1"},
			overlay: []string{"Z=z", "Y=y"},
			want:    []string{"A=1", "Z=z", "Y=y"},
		},
		"base line without equals preserved when not overridden": {
			base:    []string{"A=1", "WEIRD"},
			overlay: []string{"B=2"},
			want:    []string{"A=1", "WEIRD", "B=2"},
		},
		"case sensitive keys": {
			base:    []string{"a=1", "A=2"},
			overlay: []string{"A=upper"},
			want:    []string{"a=1", "A=upper"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := mergeProcessEnvOverlay(tc.base, tc.overlay)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatalf("mergeProcessEnvOverlay() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
