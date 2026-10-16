package node

import (
	"slices"
	"testing"

	"github.com/moby/sys/mountinfo"
)

func TestCgroupMountOptions(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		mounts  []*mountinfo.Info
		want    []string
		wantErr bool
	}{
		{
			name:   "superblock options without the access mode",
			mounts: []*mountinfo.Info{{VFSOptions: "rw,nsdelegate,memory_recursiveprot"}},
			want:   []string{"nsdelegate", "memory_recursiveprot"},
		},
		{
			name:   "no superblock options",
			mounts: []*mountinfo.Info{{VFSOptions: "rw"}},
		},
		{
			name: "last mount at the path wins",
			mounts: []*mountinfo.Info{
				{VFSOptions: "rw,nsdelegate"},
				{VFSOptions: "rw,memory_recursiveprot"},
			},
			want: []string{"memory_recursiveprot"},
		},
		{
			name:    "no cgroup2 mount",
			wantErr: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := cgroupMountOptions(tc.mounts)
			if (err != nil) != tc.wantErr {
				t.Fatalf("got error %v, want error %t", err, tc.wantErr)
			}

			if !slices.Equal(got, tc.want) {
				t.Fatalf("got options %v, want %v", got, tc.want)
			}
		})
	}
}
