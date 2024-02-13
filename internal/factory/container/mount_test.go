package container

import "testing"

func TestIsSubDirectoryOf(t *testing.T) {
	tests := []struct {
		base, target string
		want         bool
	}{
		{"/var/lib/containers/storage", "/", true},
		{"/var/lib/containers/storage", "/var/lib", true},
		{"/var/lib/containers/storage", "/var/lib/containers", true},
		{"/var/lib/containers/storage", "/var/lib/containers/storage", true},
		{"/var/lib/containers/storage", "/var/lib/containers/storage/extra", false},
		{"/var/lib/containers/storage", "/va", false},
		{"/var/lib/containers/storage", "/var/tmp/containers", false},
	}

	for _, tt := range tests {
		testname := tt.base + " " + tt.target
		t.Run(testname, func(t *testing.T) {
			ans := isSubDirectoryOf(tt.base, tt.target)
			if ans != tt.want {
				t.Errorf("got %v, want %v", ans, tt.want)
			}
		})
	}
}
