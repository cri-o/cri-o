//go:build linux

package statsserver

import (
	"testing"

	"github.com/prometheus/procfs"
)

func newNetDev(names ...string) procfs.NetDev {
	netDev := make(procfs.NetDev, len(names))

	for _, name := range names {
		netDev[name] = procfs.NetDevLine{Name: name}
	}

	return netDev
}

func TestSnapshotFiltering(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		netDev    procfs.NetDev
		podIfaces map[string]struct{}
		want      []string
	}{
		{
			name:      "no interfaces",
			netDev:    nil,
			podIfaces: nil,
			want:      nil,
		},
		{
			name:      "no exclusions",
			netDev:    newNetDev("eth0", "lo"),
			podIfaces: nil,
			want:      []string{"eth0", "lo"},
		},
		{
			name:   "excludes pod-owned interfaces",
			netDev: newNetDev("eth0", "veth123", "lo"),
			podIfaces: map[string]struct{}{
				"veth123": {},
			},
			want: []string{"eth0", "lo"},
		},
		{
			name:   "all excluded",
			netDev: newNetDev("veth1", "veth2"),
			podIfaces: map[string]struct{}{
				"veth1": {},
				"veth2": {},
			},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			for name := range tt.netDev {
				if _, excluded := tt.podIfaces[name]; excluded {
					delete(tt.netDev, name)
				}
			}

			if len(tt.netDev) != len(tt.want) {
				var gotNames []string
				for name := range tt.netDev {
					gotNames = append(gotNames, name)
				}

				t.Fatalf("got %d interfaces %v, want %d %v", len(tt.netDev), gotNames, len(tt.want), tt.want)
			}

			for _, name := range tt.want {
				if _, ok := tt.netDev[name]; !ok {
					t.Errorf("expected interface %q not found in result", name)
				}
			}
		})
	}
}
