package server

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

const testHostnamePath = "/etc/hostname"

func TestResolveSymbolicLink(t *testing.T) {
	t.Parallel()

	scope := t.TempDir()
	for _, dir := range []string{"subdir", filepath.Join("deep", "dir")} {
		if err := os.MkdirAll(filepath.Join(scope, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	for _, file := range []string{filepath.Join("subdir", "file"), "real"} {
		if err := os.WriteFile(filepath.Join(scope, file), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	for name, target := range map[string]string{
		"link":                                "real",
		"escape":                              "/etc",
		"link1":                               "/tmp",
		filepath.Join("deep", "dir", "link2"): "../..",
	} {
		if err := os.Symlink(target, filepath.Join(scope, name)); err != nil {
			t.Fatal(err)
		}
	}

	tests := []struct {
		name     string
		scope    string
		path     string
		want     string
		notExist bool
	}{
		{
			name:  "regular file stays within scope",
			scope: scope,
			path:  "subdir/file",
			want:  filepath.Join(scope, "subdir", "file"),
		},
		{
			// Traversal is clamped at scope. The error comes from the missing
			// etc/shadow inside scope, rather than from the parent components.
			name:     "path traversal is blocked",
			scope:    scope,
			path:     "../../etc/shadow",
			want:     filepath.Join(scope, "etc", "shadow"),
			notExist: true,
		},
		{
			name:     "absolute path is confined to scope",
			scope:    scope,
			path:     testHostnamePath,
			want:     filepath.Join(scope, "etc", "hostname"),
			notExist: true,
		},
		{
			name:  "symlink within scope resolves correctly",
			scope: scope,
			path:  "link",
			want:  filepath.Join(scope, "real"),
		},
		{
			// The intermediate symlink targets /etc, which must be resolved
			// relative to scope even if /etc/hostname exists on the host.
			name:     "symlink escaping scope is contained",
			scope:    scope,
			path:     "escape/hostname",
			want:     filepath.Join(scope, "etc", "hostname"),
			notExist: true,
		},
		{
			// The caller needs the confined path even on error to create the missing source.
			name:     "non-existent path returns resolved path with error",
			scope:    scope,
			path:     "does/not/exist",
			want:     filepath.Join(scope, "does", "not", "exist"),
			notExist: true,
		},
		{name: "empty scope defaults to root", path: "/tmp", want: "/tmp"},
		{
			// Host /tmp exists, but link1 must resolve to the missing directory inside scope.
			name:     "absolute symlink target is confined to scope",
			scope:    scope,
			path:     "link1",
			want:     filepath.Join(scope, "tmp"),
			notExist: true,
		},
		{
			// Resolve ../.. from the link's parent, deep/dir, to reach the existing scope root.
			name:  "relative symlink resolves to scope root",
			scope: scope,
			path:  "deep/dir/link2",
			want:  scope,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := resolveSymbolicLink(tc.scope, tc.path)

			if tc.notExist {
				if !os.IsNotExist(err) {
					t.Fatalf("expected IsNotExist, got: %v", err)
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestResolveSymbolicLinkUnscoped(t *testing.T) {
	t.Parallel()

	for _, scope := range []string{"", "/"} {
		t.Run("scope="+scope, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()

			realDir := filepath.Join(dir, "real")
			if err := os.Mkdir(realDir, 0o755); err != nil {
				t.Fatal(err)
			}

			file := filepath.Join(realDir, "file")
			if err := os.WriteFile(file, nil, 0o644); err != nil {
				t.Fatal(err)
			}

			for name, target := range map[string]string{
				"link":     "real",
				"dangling": "missing",
			} {
				if err := os.Symlink(target, filepath.Join(dir, name)); err != nil {
					t.Fatal(err)
				}
			}

			cases := []struct {
				name     string
				path     string
				want     string
				notExist bool
			}{
				{name: "regular file", path: file, want: file},
				{
					name: "intermediate symlink",
					path: filepath.Join(dir, "link", "file"),
					want: filepath.Join(dir, "link", "file"),
				},
				{name: "final symlink", path: filepath.Join(dir, "link"), want: realDir},
				{
					name: "dangling final symlink",
					path: filepath.Join(dir, "dangling"),
					want: filepath.Join(dir, "missing"),
				},
				{
					name:     "missing path",
					path:     filepath.Join(dir, "missing"),
					want:     filepath.Join(dir, "missing"),
					notExist: true,
				},
				{
					name:     "missing path through symlink",
					path:     filepath.Join(dir, "link", "missing"),
					want:     filepath.Join(dir, "link", "missing"),
					notExist: true,
				},
				{name: "relative path", path: ".", want: filepath.Join(scope, ".")},
			}
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					got, err := resolveSymbolicLink(scope, tc.path)
					if tc.notExist {
						if !os.IsNotExist(err) {
							t.Fatalf("expected IsNotExist, got: %v", err)
						}
					} else if err != nil {
						t.Fatalf("unexpected error: %v", err)
					}

					if got != tc.want {
						t.Errorf("got %q, want %q", got, tc.want)
					}
				})
			}

			t.Run("intermediate proc magic link", func(t *testing.T) {
				if runtime.GOOS != "linux" {
					t.Skip("requires Linux procfs")
				}

				path := filepath.Join(string(filepath.Separator), "proc", "self", "root", dir)

				got, err := resolveSymbolicLink(scope, path)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}

				if got != path {
					t.Errorf("got %q, want %q", got, path)
				}
			})
		})
	}
}
