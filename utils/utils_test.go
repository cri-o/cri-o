package utils_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"

	"github.com/containers/podman/v4/pkg/rootless"
	"github.com/cri-o/cri-o/internal/dbusmgr"
	"github.com/cri-o/cri-o/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type errorReaderWriter struct{}

func (m *errorReaderWriter) Write(p []byte) (int, error) {
	return 0, t.TestError
}

func (m *errorReaderWriter) Read(p []byte) (int, error) {
	return 0, t.TestError
}

// The actual test suite
var _ = t.Describe("Utils", func() {
	t.Describe("StatusToExitCode", func() {
		It("should succeed", func() {
			// Given
			// When
			code := utils.StatusToExitCode(20000)

			// Then
			Expect(code).To(Equal(78))
		})
	})

	t.Describe("DetachError", func() {
		It("should return an error", func() {
			// Given
			err := &utils.DetachError{}

			// When
			str := err.Error()

			// Then
			Expect(str).To(Equal("detached from container"))
		})
	})

	t.Describe("CopyDetachable", func() {
		It("should succeed", func() {
			// Given
			reader := strings.NewReader("test")
			writer := &bytes.Buffer{}
			keys := []byte{}

			// When
			written, err := utils.CopyDetachable(writer, reader, keys)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(written).To(SatisfyAll(BeNumerically(">", 0)))
		})

		It("should succeed with keys", func() {
			// Given
			reader := strings.NewReader("x")
			writer := &bytes.Buffer{}
			keys := []byte("xe")

			// When
			written, err := utils.CopyDetachable(writer, reader, keys)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(written).To(SatisfyAll(BeNumerically(">", 0)))
		})

		It("should fail with nil reader/writer", func() {
			// Given
			// When
			written, err := utils.CopyDetachable(nil, nil, nil)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(written).To(BeEquivalentTo(0))
		})

		It("should fail with error reader", func() {
			// Given
			reader := &errorReaderWriter{}
			writer := &bytes.Buffer{}
			keys := []byte{}

			// When
			written, err := utils.CopyDetachable(writer, reader, keys)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(written).To(BeEquivalentTo(0))
		})

		It("should fail with error writer", func() {
			// Given
			reader := strings.NewReader("x")
			writer := &errorReaderWriter{}
			keys := []byte{}

			// When
			written, err := utils.CopyDetachable(writer, reader, keys)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(written).To(BeEquivalentTo(0))
		})

		It("should fail on detach error", func() {
			// Given
			reader := strings.NewReader("x")
			writer := &bytes.Buffer{}
			keys := []byte("x")

			// When
			written, err := utils.CopyDetachable(writer, reader, keys)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(written).To(BeEquivalentTo(0))
		})
	})

	t.Describe("WriteGoroutineStacksToFile", func() {
		It("should succeed", func() {
			// Given
			const testFile = "testFile"

			// When
			err := utils.WriteGoroutineStacksToFile(testFile)

			// Then
			Expect(err).ToNot(HaveOccurred())
			os.Remove(testFile)
		})

		It("should fail on invalid file path", func() {
			// Given

			// When
			err := utils.WriteGoroutineStacksToFile("")

			// Then
			Expect(err).To(HaveOccurred())
		})
	})

	t.Describe("RunUnderSystemdScope", func() {
		It("should fail unauthenticated", func() {
			// Given
			// When
			err := utils.RunUnderSystemdScope(dbusmgr.NewDbusConnManager(rootless.IsRootless()), 1, "", "")

			// Then
			Expect(err).To(HaveOccurred())
		})
	})

	t.Describe("GetUserInfo and GeneratePasswd", func() {
		It("should succeed with nothing set i.e user=root", func() {
			dir := createEtcFiles()
			defer os.RemoveAll(dir)
			uid, gid, addgids, err := utils.GetUserInfo(dir, "root")
			Expect(err).ToNot(HaveOccurred())

			// passwdFile should be empty because an updated /etc/passwd file isn't created.
			passwdFile, err := utils.GeneratePasswd("", uid, gid, "", dir, dir)
			Expect(err).ToNot(HaveOccurred())
			Expect(passwdFile).To(BeEmpty())

			// Double check that the uid, gid, and additional gids didn't change.
			newuid, newgid, newaddgids, err := utils.GetUserInfo(dir, "root")
			Expect(err).ToNot(HaveOccurred())
			Expect(newuid).To(Equal(uid))
			Expect(newgid).To(Equal(gid))
			Expect(newaddgids).To(Equal(addgids))
		})

		It("should succeed with existing username", func() {
			dir := createEtcFiles()
			defer os.RemoveAll(dir)
			uid, gid, addgids, err := utils.GetUserInfo(dir, "daemon")
			Expect(err).ToNot(HaveOccurred())

			// passwdFile should be empty because an updated /etc/passwd file isn't created.
			passwdFile, err := utils.GeneratePasswd("", uid, gid, "", dir, dir)
			Expect(err).ToNot(HaveOccurred())
			Expect(passwdFile).To(BeEmpty())

			// Double check that the uid, gid, and additional gids didn't change.
			newuid, newgid, newaddgids, err := utils.GetUserInfo(dir, "daemon")
			Expect(err).ToNot(HaveOccurred())
			Expect(newuid).To(Equal(uid))
			Expect(newgid).To(Equal(gid))
			Expect(newaddgids).To(Equal(addgids))
		})

		It("should succeed with existing uid", func() {
			dir := createEtcFiles()
			defer os.RemoveAll(dir)
			uid, gid, addgids, err := utils.GetUserInfo(dir, "25")
			Expect(err).ToNot(HaveOccurred())

			// passwdFile should be empty because an updated /etc/passwd file isn't created.
			passwdFile, err := utils.GeneratePasswd("", uid, gid, "", dir, dir)
			Expect(err).ToNot(HaveOccurred())
			Expect(passwdFile).To(BeEmpty())

			// Double check that the uid, gid, and additional gids didn't change.
			newuid, newgid, newaddgids, err := utils.GetUserInfo(dir, "25")
			Expect(err).ToNot(HaveOccurred())
			Expect(newuid).To(Equal(uid))
			Expect(newgid).To(Equal(gid))
			Expect(newaddgids).To(Equal(addgids))
		})

		It("should succeed with uid that doesn't exist in /etc/passwd", func() {
			dir := createEtcFiles()
			defer os.RemoveAll(dir)
			uid, gid, addgids, err := utils.GetUserInfo(dir, "300")
			Expect(err).ToNot(HaveOccurred())

			// passwdFile should not be empty because an updated /etc/passwd file is created.
			passwdFile, err := utils.GeneratePasswd("", uid, gid, "", dir, dir)
			Expect(err).ToNot(HaveOccurred())
			Expect(passwdFile).ToNot(BeEmpty())

			// Double check that the uid, gid, and additional gids didn't change.
			newuid, newgid, newaddgids, err := utils.GetUserInfo(dir, "300")
			Expect(err).ToNot(HaveOccurred())
			Expect(newuid).To(Equal(uid))
			Expect(newgid).To(Equal(gid))
			Expect(newaddgids).To(Equal(addgids))
		})

		It("should fail with username that desn't exist in /etc/passwd", func() {
			dir := createEtcFiles()
			defer os.RemoveAll(dir)
			_, _, _, err := utils.GetUserInfo(dir, "blah")
			Expect(err).To(HaveOccurred())
		})

		It("should succeed with existing user and group", func() {
			dir := createEtcFiles()
			defer os.RemoveAll(dir)
			uid, gid, addgids, err := utils.GetUserInfo(dir, "daemon:mail")
			Expect(err).ToNot(HaveOccurred())

			// passwdFile should be empty because an updated /etc/passwd file is not created.
			passwdFile, err := utils.GeneratePasswd("", uid, gid, "", dir, dir)
			Expect(err).ToNot(HaveOccurred())
			Expect(passwdFile).To(BeEmpty())

			// Double check that the uid, gid, and additional gids didn't change.
			newuid, newgid, newaddgids, err := utils.GetUserInfo(dir, "daemon:mail")
			Expect(err).ToNot(HaveOccurred())
			Expect(newuid).To(Equal(uid))
			Expect(newgid).To(Equal(gid))
			Expect(newaddgids).To(Equal(addgids))
		})

		It("should succeed with existing uid and gid", func() {
			dir := createEtcFiles()
			defer os.RemoveAll(dir)
			uid, gid, addgids, err := utils.GetUserInfo(dir, "2:22")
			Expect(err).ToNot(HaveOccurred())

			// passwdFile should be empty because an updated /etc/passwd file is not created.
			passwdFile, err := utils.GeneratePasswd("", uid, gid, "", dir, dir)
			Expect(err).ToNot(HaveOccurred())
			Expect(passwdFile).To(BeEmpty())

			// Double check that the uid, gid, and additional gids didn't change.
			newuid, newgid, newaddgids, err := utils.GetUserInfo(dir, "2:22")
			Expect(err).ToNot(HaveOccurred())
			Expect(newuid).To(Equal(uid))
			Expect(newgid).To(Equal(gid))
			Expect(newaddgids).To(Equal(addgids))
		})

		It("should succeed with existing user and non-existing numeric gid", func() {
			dir := createEtcFiles()
			defer os.RemoveAll(dir)
			uid, gid, addgids, err := utils.GetUserInfo(dir, "daemon:250")
			Expect(err).ToNot(HaveOccurred())

			// passwdFile should be empty because an updated /etc/passwd file is not created.
			passwdFile, err := utils.GeneratePasswd("", uid, gid, "", dir, dir)
			Expect(err).ToNot(HaveOccurred())
			Expect(passwdFile).To(BeEmpty())

			// Double check that the uid, gid, and additional gids didn't change.
			newuid, newgid, newaddgids, err := utils.GetUserInfo(dir, "daemon:250")
			Expect(err).ToNot(HaveOccurred())
			Expect(newuid).To(Equal(uid))
			Expect(newgid).To(Equal(gid))
			Expect(newaddgids).To(Equal(addgids))
		})

		It("should succeed with non-existing uid and non-existing gid", func() {
			dir := createEtcFiles()
			defer os.RemoveAll(dir)
			uid, gid, addgids, err := utils.GetUserInfo(dir, "300:250")
			Expect(err).ToNot(HaveOccurred())

			// passwdFile should not be empty because an updated /etc/passwd file is created.
			passwdFile, err := utils.GeneratePasswd("", uid, gid, "", dir, dir)
			Expect(err).ToNot(HaveOccurred())
			Expect(passwdFile).ToNot(BeEmpty())

			// Double check that the uid, gid, and additional gids didn't change.
			newuid, newgid, newaddgids, err := utils.GetUserInfo(dir, "300:250")
			Expect(err).ToNot(HaveOccurred())
			Expect(newuid).To(Equal(uid))
			Expect(newgid).To(Equal(gid))
			Expect(newaddgids).To(Equal(addgids))
		})

		It("should succeed with non-existing uid and existing group", func() {
			dir := createEtcFiles()
			defer os.RemoveAll(dir)
			uid, gid, addgids, err := utils.GetUserInfo(dir, "300:mail")
			Expect(err).ToNot(HaveOccurred())

			// passwdFile should not be empty because an updated /etc/passwd file is created.
			passwdFile, err := utils.GeneratePasswd("", uid, gid, "", dir, dir)
			Expect(err).ToNot(HaveOccurred())
			Expect(passwdFile).ToNot(BeEmpty())

			// Double check that the uid, gid, and additional gids didn't change.
			newuid, newgid, newaddgids, err := utils.GetUserInfo(dir, "300:mail")
			Expect(err).ToNot(HaveOccurred())
			Expect(newuid).To(Equal(uid))
			Expect(newgid).To(Equal(gid))
			Expect(newaddgids).To(Equal(addgids))
		})
	})
})

func createEtcFiles() string {
	// Create an /etc/passwd and /etc/group file that match
	// those of the alpine image
	// This will be created in a temp directory like /tmp/uid-test*
	// nolint: gosec
	alpinePasswdFile := `root:x:0:0:root:/root:/bin/ash
bin:x:1:1:bin:/bin:/sbin/nologin
daemon:x:2:2:daemon:/sbin:/sbin/nologin
adm:x:3:4:adm:/var/adm:/sbin/nologin
lp:x:4:7:lp:/var/spool/lpd:/sbin/nologin
sync:x:5:0:sync:/sbin:/bin/sync
shutdown:x:6:0:shutdown:/sbin:/sbin/shutdown
halt:x:7:0:halt:/sbin:/sbin/halt
mail:x:8:12:mail:/var/spool/mail:/sbin/nologin
news:x:9:13:news:/usr/lib/news:/sbin/nologin
uucp:x:10:14:uucp:/var/spool/uucppublic:/sbin/nologin
operator:x:11:0:operator:/root:/bin/sh
man:x:13:15:man:/usr/man:/sbin/nologin
postmaster:x:14:12:postmaster:/var/spool/mail:/sbin/nologin
cron:x:16:16:cron:/var/spool/cron:/sbin/nologin
ftp:x:21:21::/var/lib/ftp:/sbin/nologin
sshd:x:22:22:sshd:/dev/null:/sbin/nologin
at:x:25:25:at:/var/spool/cron/atjobs:/sbin/nologin
squid:x:31:31:Squid:/var/cache/squid:/sbin/nologin
xfs:x:33:33:X Font Server:/etc/X11/fs:/sbin/nologin
games:x:35:35:games:/usr/games:/sbin/nologin
postgres:x:70:70::/var/lib/postgresql:/bin/sh
cyrus:x:85:12::/usr/cyrus:/sbin/nologin
vpopmail:x:89:89::/var/vpopmail:/sbin/nologin
ntp:x:123:123:NTP:/var/empty:/sbin/nologin
smmsp:x:209:209:smmsp:/var/spool/mqueue:/sbin/nologin
guest:x:405:100:guest:/dev/null:/sbin/nologin
nobody:x:65534:65534:nobody:/:/sbin/nologin`

	alpineGroupFile := `root:x:0:root
bin:x:1:root,bin,daemon
daemon:x:2:root,bin,daemon
sys:x:3:root,bin,adm
adm:x:4:root,adm,daemon
tty:x:5:
disk:x:6:root,adm
lp:x:7:lp
mem:x:8:
kmem:x:9:
wheel:x:10:root
floppy:x:11:root
mail:x:12:mail
news:x:13:news
uucp:x:14:uucp
man:x:15:man
cron:x:16:cron
console:x:17:
audio:x:18:
cdrom:x:19:
dialout:x:20:root
ftp:x:21:
sshd:x:22:
input:x:23:
at:x:25:at
tape:x:26:root
video:x:27:root
netdev:x:28:
readproc:x:30:
squid:x:31:squid
xfs:x:33:xfs
kvm:x:34:kvm
games:x:35:
shadow:x:42:
postgres:x:70:
cdrw:x:80:
usb:x:85:
vpopmail:x:89:
users:x:100:games
ntp:x:123:
nofiles:x:200:
smmsp:x:209:smmsp
locate:x:245:
abuild:x:300:
utmp:x:406:
ping:x:999:
nogroup:x:65533:
nobody:x:65534:`

	dir, err := os.MkdirTemp("/tmp", "uid-test")
	Expect(err).ToNot(HaveOccurred())
	err = os.Mkdir(filepath.Join(dir, "etc"), 0o755)
	Expect(err).ToNot(HaveOccurred())
	err = os.WriteFile(filepath.Join(dir, "etc", "passwd"), []byte(alpinePasswdFile), 0o755)
	Expect(err).ToNot(HaveOccurred())
	err = os.WriteFile(filepath.Join(dir, "etc", "group"), []byte(alpineGroupFile), 0o755)
	Expect(err).ToNot(HaveOccurred())
	return dir
}
