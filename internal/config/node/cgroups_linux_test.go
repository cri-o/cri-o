//go:build linux

package node

import (
	"errors"
	"strings"
	"testing"

	"github.com/moby/sys/mountinfo"
)

func TestCgroupHasNsdelegate(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name      string
		mountInfo string
		err       error
		want      bool
	}{
		{
			name:      "present among superblock options",
			mountInfo: "31 23 0:27 / /sys/fs/cgroup rw - cgroup2 cgroup rw,nsdelegate,memory_recursiveprot\n",
			want:      true,
		},
		{
			name:      "absent",
			mountInfo: "31 23 0:27 / /sys/fs/cgroup rw - cgroup2 cgroup rw\n",
		},
		{
			name: "no mounts",
		},
		{
			name:      "different mountpoint",
			mountInfo: "31 23 0:27 / /other rw - cgroup2 cgroup rw,nsdelegate\n",
		},
		{
			name:      "cgroup v1",
			mountInfo: "31 23 0:27 / /sys/fs/cgroup rw - cgroup none rw,nsdelegate\n",
		},
		{
			name:      "option substring",
			mountInfo: "31 23 0:27 / /sys/fs/cgroup rw - cgroup2 cgroup rw,notnsdelegate\n",
		},
		{
			name:      "per-mount option only",
			mountInfo: "31 23 0:27 / /sys/fs/cgroup rw,nsdelegate - cgroup2 cgroup rw\n",
		},
		{
			name: "read error",
			err:  errors.New("cannot read mountinfo"),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			getMounts := func(filter mountinfo.FilterFunc) ([]*mountinfo.Info, error) {
				if tc.err != nil {
					return nil, tc.err
				}

				return mountinfo.GetMountsFromReader(strings.NewReader(tc.mountInfo), filter)
			}

			hasNsdelegate, err := cgroupMountHasNsdelegate(getMounts)
			if hasNsdelegate != tc.want || !errors.Is(err, tc.err) {
				t.Fatalf("got (%v, %v), want (%v, %v)", hasNsdelegate, err, tc.want, tc.err)
			}
		})
	}
}
