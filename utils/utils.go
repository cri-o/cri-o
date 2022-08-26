package utils

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"github.com/containers/podman/v4/pkg/lookup"
	"github.com/cri-o/cri-o/internal/dbusmgr"
	securejoin "github.com/cyphar/filepath-securejoin"
	"github.com/opencontainers/runc/libcontainer/user"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	"golang.org/x/sys/unix"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	systemdDbus "github.com/coreos/go-systemd/v22/dbus"
	"github.com/godbus/dbus/v5"
)

// StatusToExitCode converts wait status code to an exit code
func StatusToExitCode(status int) int {
	return ((status) & 0xff00) >> 8
}

// RunUnderSystemdScope adds the specified pid to a systemd scope
func RunUnderSystemdScope(mgr *dbusmgr.DbusConnManager, pid int, slice, unitName string, properties ...systemdDbus.Property) (err error) {
	ctx := context.Background()
	// sanity check
	if mgr == nil {
		return errors.New("dbus manager is nil")
	}
	defaultProperties := []systemdDbus.Property{
		newProp("PIDs", []uint32{uint32(pid)}),
		newProp("Delegate", true),
		newProp("DefaultDependencies", false),
	}
	properties = append(defaultProperties, properties...)
	if slice != "" {
		properties = append(properties, systemdDbus.PropSlice(slice))
	}
	// Make a buffered channel so that the sender (go-systemd's jobComplete)
	// won't be blocked on channel send while holding the jobListener lock
	// (RHBZ#2082344).
	ch := make(chan string, 1)
	if err := mgr.RetryOnDisconnect(func(c *systemdDbus.Conn) error {
		_, err = c.StartTransientUnitContext(ctx, unitName, "replace", properties, ch)
		return err
	}); err != nil {
		return fmt.Errorf("start transient unit %q: %w", unitName, err)
	}

	// Wait for the job status.
	select {
	case s := <-ch:
		close(ch)
		if s != "done" {
			return fmt.Errorf("error moving conmon with pid %d to systemd unit %s: got %s", pid, unitName, s)
		}
	case <-time.After(time.Minute * 6):
		// This case is a work around to catch situations where the dbus library sends the
		// request but it unexpectedly disappears. We set the timeout very high to make sure
		// we wait as long as possible to catch situations where dbus is overwhelmed.
		// We also don't use the native context cancelling behavior of the dbus library,
		// because experience has shown that it does not help.
		// TODO: Find cause of the request being dropped in the dbus library and fix it.
		return fmt.Errorf("timed out moving conmon with pid %d to systemd unit %s", pid, unitName)
	}

	return nil
}

func newProp(name string, units interface{}) systemdDbus.Property {
	return systemdDbus.Property{
		Name:  name,
		Value: dbus.MakeVariant(units),
	}
}

// DetachError is special error which returned in case of container detach.
type DetachError struct{}

func (DetachError) Error() string {
	return "detached from container"
}

// CopyDetachable is similar to io.Copy but support a detach key sequence to break out.
func CopyDetachable(dst io.Writer, src io.Reader, keys []byte) (int64, error) {
	var (
		written int64
		err     error
	)
	// Sanity check interfaces
	if dst == nil || src == nil {
		return 0, fmt.Errorf("src/dst reader/writer nil")
	}
	if len(keys) == 0 {
		// Default keys : ctrl-p ctrl-q
		keys = []byte{16, 17}
	}

	buf := make([]byte, 32*1024)
	for {
		nr, er := src.Read(buf)
		if nr > 0 {
			preservBuf := []byte{}
			for i, key := range keys {
				preservBuf = append(preservBuf, buf[0:nr]...)
				if nr != 1 || buf[0] != key {
					break
				}
				if i == len(keys)-1 {
					// src.Close()
					return 0, DetachError{}
				}
				nr, er = src.Read(buf)
			}
			nw, ew := dst.Write(preservBuf)
			nr = len(preservBuf)
			if nw > 0 {
				written += int64(nw)
			}
			if ew != nil {
				err = ew
				break
			}
			if nr != nw {
				err = io.ErrShortWrite
				break
			}
		}
		if er != nil {
			if er != io.EOF {
				err = er
			}
			break
		}
	}
	return written, err
}

// WriteGoroutineStacks writes out the goroutine stacks
// of the caller. Up to 32 MB is allocated to print the
// stack.
func WriteGoroutineStacks(w io.Writer) error {
	if w == nil {
		return fmt.Errorf("writer nil")
	}
	buf := make([]byte, 1<<20)
	for i := 0; ; i++ {
		n := runtime.Stack(buf, true)
		if n < len(buf) {
			buf = buf[:n]
			break
		}
		if len(buf) >= 32<<20 {
			break
		}
		buf = make([]byte, 2*len(buf))
	}
	_, err := w.Write(buf)
	return err
}

// WriteGoroutineStacksToFile write goroutine stacks
// to the specified file.
func WriteGoroutineStacksToFile(path string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o666)
	if err != nil {
		return err
	}
	defer f.Close()

	err = WriteGoroutineStacks(f)
	if err != nil {
		return err
	}
	return f.Sync()
}

// GenerateID generates a random unique id.
func GenerateID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate ID: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// openContainerFile opens a file inside a container rootfs safely
func openContainerFile(rootfs, path string) (io.ReadCloser, error) {
	fp, err := securejoin.SecureJoin(rootfs, path)
	if err != nil {
		return nil, err
	}
	fh, err := os.Open(fp)
	if err != nil {
		// This is needed because a nil *os.File is different to a nil
		// io.ReadCloser and this causes GetExecUser to not detect that the
		// container file is missing.
		return nil, err
	}
	return fh, nil
}

// GetUserInfo returns UID, GID and additional groups for specified user
// by looking them up in /etc/passwd and /etc/group
func GetUserInfo(rootfs, userName string) (uid, gid uint32, additionalGids []uint32, _ error) {
	// We don't care if we can't open the file because
	// not all images will have these files
	passwdFile, err := openContainerFile(rootfs, "/etc/passwd")
	if err != nil {
		logrus.Warnf("Failed to open /etc/passwd: %v", err)
	} else {
		defer passwdFile.Close()
	}

	groupFile, err := openContainerFile(rootfs, "/etc/group")
	if err != nil {
		logrus.Warnf("Failed to open /etc/group: %v", err)
	} else {
		defer groupFile.Close()
	}

	execUser, err := user.GetExecUser(userName, nil, passwdFile, groupFile)
	if err != nil {
		return 0, 0, nil, fmt.Errorf("get exec user: %w", err)
	}

	uid = uint32(execUser.Uid)
	gid = uint32(execUser.Gid)
	additionalGids = make([]uint32, 0, len(execUser.Sgids))
	for _, g := range execUser.Sgids {
		additionalGids = append(additionalGids, uint32(g))
	}

	return uid, gid, additionalGids, nil
}

// GeneratePasswd generates a container specific passwd file,
// iff uid is not defined in the containers /etc/passwd
func GeneratePasswd(username string, uid, gid uint32, homedir, rootfs, rundir string) (string, error) {
	// if UID exists inside of container rootfs /etc/passwd then
	// don't generate passwd
	if _, err := lookup.GetUser(rootfs, strconv.Itoa(int(uid))); err == nil {
		return "", nil
	}
	passwdFile := filepath.Join(rundir, "passwd")
	originPasswdFile, err := securejoin.SecureJoin(rootfs, "/etc/passwd")
	if err != nil {
		return "", fmt.Errorf("unable to follow symlinks to passwd file: %w", err)
	}
	var st unix.Stat_t
	err = unix.Stat(originPasswdFile, &st)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("unable to stat passwd file %s: %w", originPasswdFile, err)
	}
	// Check if passwd file is world writable.
	if st.Mode&0o022 != 0 {
		return "", nil
	}

	if uid == st.Uid && st.Mode&0o200 != 0 {
		return "", nil
	}

	orig, err := os.ReadFile(originPasswdFile)
	if err != nil {
		// If no /etc/passwd in container ignore and return
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read passwd file: %w", err)
	}
	if username == "" {
		username = "default"
	}
	if homedir == "" {
		homedir = "/tmp"
	}
	pwd := fmt.Sprintf("%s%s:x:%d:%d:%s user:%s:/sbin/nologin\n", orig, username, uid, gid, username, homedir)
	if err := os.WriteFile(passwdFile, []byte(pwd), os.FileMode(st.Mode)&os.ModePerm); err != nil {
		return "", fmt.Errorf("failed to create temporary passwd file: %w", err)
	}
	if err := os.Chown(passwdFile, int(st.Uid), int(st.Gid)); err != nil {
		return "", fmt.Errorf("failed to chown temporary passwd file: %w", err)
	}

	return passwdFile, nil
}

// Int32Ptr is a utility function to assign to integer pointer variables
func Int32Ptr(i int32) *int32 {
	return &i
}

// EnsureSaneLogPath is a hack to fix https://issues.k8s.io/44043 which causes
// logPath to be a broken symlink to some magical Docker path. Ideally we
// wouldn't have to deal with this, but until that issue is fixed we have to
// remove the path if it's a broken symlink.
func EnsureSaneLogPath(logPath string) error {
	// If the path exists but the resolved path does not, then we have a broken
	// symlink and we need to remove it.
	fi, err := os.Lstat(logPath)
	if err != nil || fi.Mode()&os.ModeSymlink == 0 {
		// Non-existent files and non-symlinks aren't our problem.
		return nil
	}

	_, err = os.Stat(logPath)
	if os.IsNotExist(err) {
		err = os.RemoveAll(logPath)
		if err != nil {
			return fmt.Errorf("failed to remove bad log path %s: %w", logPath, err)
		}
	}
	return nil
}

func GetLabelOptions(selinuxOptions *types.SELinuxOption) []string {
	labels := []string{}
	if selinuxOptions != nil {
		if selinuxOptions.User != "" {
			labels = append(labels, "user:"+selinuxOptions.User)
		}
		if selinuxOptions.Role != "" {
			labels = append(labels, "role:"+selinuxOptions.Role)
		}
		if selinuxOptions.Type != "" {
			labels = append(labels, "type:"+selinuxOptions.Type)
		}
		if selinuxOptions.Level != "" {
			labels = append(labels, "level:"+selinuxOptions.Level)
		}
	}
	return labels
}

// SyncParent ensures a path's parent directory is synced to disk
func SyncParent(path string) error {
	return Sync(filepath.Dir(path))
}

// Sync ensures a path is synced to disk
func Sync(path string) error {
	f, err := os.OpenFile(path, os.O_RDONLY, 0o755)
	if err != nil {
		return err
	}
	defer f.Close()

	if err := f.Sync(); err != nil {
		return err
	}
	return nil
}

// Syncfs ensures the file system at path is synced to disk
func Syncfs(path string) error {
	f, err := os.OpenFile(path, os.O_RDONLY, 0o755)
	if err != nil {
		return err
	}
	defer f.Close()

	if err := unix.Syncfs(int(f.Fd())); err != nil {
		return err
	}
	return nil
}
