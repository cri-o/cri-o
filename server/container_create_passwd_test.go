package server

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/opencontainers/runtime-tools/generate"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func passwdTestRootfs(t *testing.T) string {
	t.Helper()

	rootfs := t.TempDir()
	if err := os.Mkdir(filepath.Join(rootfs, "etc"), 0o755); err != nil {
		t.Fatal(err)
	}

	for name, content := range map[string]string{
		"passwd": "root:x:0:0:root:/root:/bin/sh\n",
		"group":  "root:x:0:\nstaff:x:2345:\n",
	} {
		if err := os.WriteFile(
			filepath.Join(rootfs, "etc", name),
			[]byte(content),
			0o444,
		); err != nil {
			t.Fatal(err)
		}
	}

	return rootfs
}

func TestSetupContainerUserRejectsPasswdDelimiters(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		home     string
		cwd      string
		username string
	}{
		{name: "HOME colon", home: "/home/user:/bin/sh"},
		{name: "HOME newline", home: "/home/user\nextra:x:0:0::/:/bin/sh"},
		{name: "HOME carriage return", home: "/home/user\rextra"},
		{name: "Cwd colon", cwd: "/home/user:/bin/sh"},
		{name: "Cwd newline", cwd: "/home/user\nextra:x:0:0::/:/bin/sh"},
		{name: "Cwd carriage return", cwd: "/home/user\rextra"},
		{name: "username newline", username: "1000\nextra:x:0:0::/:/bin/sh"},
		{name: "username carriage return", username: "1000\rextra"},
		{name: "group newline", username: "1000:2345\nextra"},
		{name: "extra user separators", username: "1000:2345:extra"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			specgen, err := generate.New("linux")
			if err != nil {
				t.Fatal(err)
			}

			if tc.home != "" {
				specgen.AddProcessEnv("HOME", tc.home)
			}

			if tc.cwd != "" {
				specgen.SetProcessCwd(tc.cwd)
			}

			sc := &types.LinuxContainerSecurityContext{
				RunAsUser:     &types.Int64Value{Value: 1000},
				RunAsUsername: tc.username,
			}
			rundir := t.TempDir()

			err = setupContainerUser(
				t.Context(),
				&specgen,
				passwdTestRootfs(t),
				"",
				rundir,
				sc,
				nil,
			)
			if err == nil {
				t.Fatal("expected invalid passwd input to be rejected")
			}

			if _, err := os.Stat(filepath.Join(rundir, "passwd")); !os.IsNotExist(err) {
				t.Fatalf("rejected input must not generate a passwd file, got %v", err)
			}
		})
	}
}

func TestSetupContainerUserPreservesUserGroupSyntax(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name      string
		imageUser string
		username  string
		home      string
		cwd       string
		wantUID   uint32
		wantGID   uint32
		wantEntry string
	}{
		{
			name: "image uid and gid", imageUser: "1000:2345", wantUID: 1000, wantGID: 2345,
			wantEntry: "1000:x:1000:2345:1000 user:/:/sbin/nologin\n",
		},
		{
			name: "image uid and group name", imageUser: "1000:staff", wantUID: 1000, wantGID: 2345,
			wantEntry: "1000:x:1000:2345:1000 user:/:/sbin/nologin\n",
		},
		{
			name: "existing image user and gid", imageUser: "root:2345", wantGID: 2345,
		},
		{
			name: "empty image user component", imageUser: ":2345", wantGID: 2345,
		},
		{
			name: "empty image group component", imageUser: "1000:", wantUID: 1000,
			wantEntry: "1000:x:1000:0:1000 user:/:/sbin/nologin\n",
		},
		{
			name: "CRI user and group", username: "1000:staff", wantUID: 1000, wantGID: 2345,
			wantEntry: "1000:x:1000:2345:1000 user:/:/sbin/nologin\n",
		},
		{
			name: "numeric user HOME", imageUser: "1000", home: "/home/app", wantUID: 1000,
			wantEntry: "1000:x:1000:0:1000 user:/home/app:/sbin/nologin\n",
		},
		{
			name: "numeric user Cwd fallback", imageUser: "1000", cwd: "/work", wantUID: 1000,
			wantEntry: "1000:x:1000:0:1000 user:/work:/sbin/nologin\n",
		},
		{
			name: "HOME takes precedence", imageUser: "1000", home: "/home/app", cwd: "/work:ignored", wantUID: 1000,
			wantEntry: "1000:x:1000:0:1000 user:/home/app:/sbin/nologin\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			specgen, err := generate.New("linux")
			if err != nil {
				t.Fatal(err)
			}

			if tc.home != "" {
				specgen.AddProcessEnv("HOME", tc.home)
			}

			if tc.cwd != "" {
				specgen.SetProcessCwd(tc.cwd)
			}

			sc := &types.LinuxContainerSecurityContext{RunAsUsername: tc.username}
			image := &v1.Image{Config: v1.ImageConfig{User: tc.imageUser}}
			rundir := t.TempDir()

			err = setupContainerUser(
				t.Context(),
				&specgen,
				passwdTestRootfs(t),
				"",
				rundir,
				sc,
				image,
			)
			if err != nil {
				t.Fatal(err)
			}

			if user := specgen.Config.Process.User; user.UID != tc.wantUID ||
				user.GID != tc.wantGID {
				t.Fatalf(
					"got UID:GID %d:%d, want %d:%d",
					user.UID,
					user.GID,
					tc.wantUID,
					tc.wantGID,
				)
			}

			content, err := os.ReadFile(filepath.Join(rundir, "passwd"))
			if tc.wantEntry == "" {
				if !os.IsNotExist(err) {
					t.Fatalf("existing user must not generate a passwd file, got %v", err)
				}

				return
			}

			if err != nil {
				t.Fatal(err)
			}

			want := "root:x:0:0:root:/root:/bin/sh\n" + tc.wantEntry
			if string(content) != want {
				t.Fatalf("generated passwd = %q, want %q", content, want)
			}

			for line := range strings.SplitSeq(strings.TrimSuffix(string(content), "\n"), "\n") {
				if got := len(strings.Split(line, ":")); got != 7 {
					t.Errorf("passwd line has %d fields, want 7: %q", got, line)
				}
			}
		})
	}
}

func TestSetupContainerUserAllowsColonWhenPasswdIsUnchanged(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		user     string
		useCwd   bool
		mountDst string
	}{
		{name: "existing user HOME", user: "root"},
		{name: "existing user Cwd", user: "root", useCwd: true},
		{name: "mounted passwd HOME", user: "1000", mountDst: "/etc/passwd"},
		{name: "mounted etc HOME", user: "1000", mountDst: "/etc"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			specgen, err := generate.New("linux")
			if err != nil {
				t.Fatal(err)
			}

			if tc.useCwd {
				specgen.SetProcessCwd("/srv/a:b")
			} else {
				specgen.AddProcessEnv("HOME", "/srv/a:b")
			}

			if tc.mountDst != "" {
				specgen.AddMount(rspec.Mount{Destination: tc.mountDst})
			}

			rootfs := passwdTestRootfs(t)

			original, err := os.ReadFile(filepath.Join(rootfs, "etc", "passwd"))
			if err != nil {
				t.Fatal(err)
			}

			rundir := t.TempDir()
			sc := &types.LinuxContainerSecurityContext{RunAsUsername: tc.user}

			if err := setupContainerUser(
				t.Context(),
				&specgen,
				rootfs,
				"",
				rundir,
				sc,
				nil,
			); err != nil {
				t.Fatal(err)
			}

			if _, err := os.Stat(filepath.Join(rundir, "passwd")); !os.IsNotExist(err) {
				t.Fatalf("unchanged passwd must not generate a replacement, got %v", err)
			}

			content, err := os.ReadFile(filepath.Join(rootfs, "etc", "passwd"))
			if err != nil {
				t.Fatal(err)
			}

			if !bytes.Equal(content, original) {
				t.Fatal("original passwd changed")
			}
		})
	}
}
