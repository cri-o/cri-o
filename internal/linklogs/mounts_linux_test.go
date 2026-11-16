package linklogs

import (
	"os"
	"path/filepath"
	"testing"

	"go.podman.io/storage/pkg/mount"
)

func TestMountsContains(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	logs := filepath.Join(dir, "logs")

	for _, path := range []string{filepath.Join(logs, "container"), filepath.Join(dir, "logs-other")} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	logFile := filepath.Join(logs, "container", "0.log")
	if err := os.WriteFile(logFile, []byte("log contents"), 0o644); err != nil {
		t.Fatal(err)
	}

	alias := filepath.Join(dir, "alias")
	if err := os.Symlink(logs, alias); err != nil {
		t.Fatal(err)
	}

	infos, err := mount.GetMounts()
	if err != nil {
		t.Fatal(err)
	}

	// A hidden mount with a longer path must not override the actual mount ID.
	infos = append(infos, &mount.Info{ID: -1, Mountpoint: logs, Root: "/hidden", Major: -1})

	matcher, err := newMounts(logs, infos)
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name string
		path string
		want bool
	}{
		{name: "volume root", path: logs, want: true},
		{name: "container directory", path: filepath.Dir(logFile), want: true},
		{name: "log file", path: logFile, want: true},
		{name: "symlink alias", path: alias, want: true},
		{name: "intermediate symlink", path: filepath.Join(alias, "container", "0.log"), want: true},
		{name: "sibling prefix", path: filepath.Join(dir, "logs-other")},
		{name: "parent", path: dir},
		{name: "different filesystem", path: "/proc"},
		{name: "non-path source", path: "tmpfs"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := matcher.Contains(tc.path)
			if err != nil {
				t.Fatal(err)
			}

			if got != tc.want {
				t.Errorf("Contains(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}

func TestMountsMissingVolume(t *testing.T) {
	t.Parallel()

	infos, err := mount.GetMounts()
	if err != nil {
		t.Fatal(err)
	}

	matcher, err := newMounts(filepath.Join(t.TempDir(), "missing"), infos)
	if err != nil || matcher != nil {
		t.Fatalf("missing log volume must disable matching, got %v, %v", matcher, err)
	}

	if linked, err := matcher.Contains("/"); err != nil || linked {
		t.Fatalf("missing log volume must not protect the host root, got %v, %v", linked, err)
	}

	for _, volume := range []string{"", "../other", "invalid name"} {
		matcher, err := NewMounts("pod-uid", volume, infos)
		if err != nil || matcher != nil {
			t.Errorf("invalid volume %q must disable matching, got %v, %v", volume, matcher, err)
		}
	}
}

func TestMountsDeviceIdentity(t *testing.T) {
	t.Parallel()

	logs := t.TempDir()

	infos, err := mount.GetMounts()
	if err != nil {
		t.Fatal(err)
	}

	// Btrfs mountinfo device numbers can differ from those returned by stat.
	for _, info := range infos {
		info.Major, info.Minor = -1, -1
	}

	matcher, err := newMounts(logs, infos)
	if err != nil {
		t.Fatal(err)
	}

	if linked, err := matcher.Contains(logs); err != nil || !linked {
		t.Fatalf(
			"log volume must match itself despite mountinfo device numbers: %v, %v",
			linked,
			err,
		)
	}
}

func TestMountsRefreshSnapshot(t *testing.T) {
	t.Parallel()

	logs := t.TempDir()

	matcher, err := newMounts(logs, nil)
	if err != nil {
		t.Fatal(err)
	}

	if linked, err := matcher.Contains(logs); err != nil || !linked {
		t.Fatalf("log volume must match after refreshing the mount table: %v, %v", linked, err)
	}
}
