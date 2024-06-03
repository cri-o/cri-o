package utils

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/pprof"
	"strconv"

	securejoin "github.com/cyphar/filepath-securejoin"
	"github.com/opencontainers/runc/libcontainer/user"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
	"k8s.io/client-go/tools/remotecommand"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	systemdDbus "github.com/coreos/go-systemd/v22/dbus"
	"github.com/godbus/dbus/v5"
)

// StatusToExitCode converts wait status code to an exit code
func StatusToExitCode(status int) int {
	return ((status) & 0xff00) >> 8
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
		return 0, errors.New("src/dst reader/writer nil")
	}
	if len(keys) == 0 {
		// Default keys : ctrl-p ctrl-q
		keys = []byte{16, 17}
	}

	buf := make([]byte, 32*1024)
	for {
		nr, er := src.Read(buf)
		if nr > 0 {
			preserveBuf := []byte{}
			for i, key := range keys {
				preserveBuf = append(preserveBuf, buf[0:nr]...)
				if nr != 1 || buf[0] != key {
					break
				}
				if i == len(keys)-1 {
					// src.Close()
					return 0, DetachError{}
				}
				nr, er = src.Read(buf)
			}
			nw, ew := dst.Write(preserveBuf)
			nr = len(preserveBuf)
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

// WriteGoroutineStacksToFile write goroutine stacks
// to the specified file.
func WriteGoroutineStacksToFile(path string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o666)
	if err != nil {
		return err
	}
	defer f.Close()

	// Print goroutines stacks using the same format
	// as if an unrecoverable panic would occur. The
	// internal buffer is 64 MiB, which hopefully
	// will be sufficient.
	err = pprof.Lookup("goroutine").WriteTo(f, 2)
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
	if _, err := GetUser(rootfs, strconv.Itoa(int(uid))); err == nil {
		return "", nil
	}

	passwdFilePath, stat, err := secureFilePath(rootfs, "/etc/passwd")
	if err != nil || stat.Size == 0 {
		return "", err
	}

	if checkFilePermissions(&stat, uid, stat.Uid) {
		return "", nil
	}

	origContent, err := readFileContent(passwdFilePath)
	if err != nil || origContent == nil {
		return "", err
	}

	if username == "" {
		username = "default"
	}
	if homedir == "" {
		homedir = "/tmp"
	}

	pwdContent := fmt.Sprintf("%s%s:x:%d:%d:%s user:%s:/sbin/nologin\n", string(origContent), username, uid, gid, username, homedir)
	passwdFile := filepath.Join(rundir, "passwd")

	return createAndSecureFile(passwdFile, pwdContent, os.FileMode(stat.Mode), int(stat.Uid), int(stat.Gid))
}

// GenerateGroup generates a container specific group file,
// iff gid is not defined in the containers /etc/group
func GenerateGroup(gid uint32, rootfs, rundir string) (string, error) {
	if _, err := GetGroup(rootfs, strconv.Itoa(int(gid))); err == nil {
		return "", nil
	}

	groupFilePath, stat, err := secureFilePath(rootfs, "/etc/group")
	if err != nil {
		return "", err
	}

	if checkFilePermissions(&stat, gid, stat.Gid) {
		return "", nil
	}

	origContent, err := readFileContent(groupFilePath)
	if err != nil || origContent == nil {
		return "", err
	}

	groupContent := fmt.Sprintf("%s%d:x:%d:\n", string(origContent), gid, gid)
	groupFile := filepath.Join(rundir, "group")

	return createAndSecureFile(groupFile, groupContent, os.FileMode(stat.Mode), int(stat.Uid), int(stat.Gid))
}

func secureFilePath(rootfs, file string) (string, unix.Stat_t, error) {
	path, err := securejoin.SecureJoin(rootfs, file)
	if err != nil {
		return "", unix.Stat_t{}, fmt.Errorf("unable to follow symlinks to %s file: %w", file, err)
	}

	var st unix.Stat_t
	err = unix.Stat(path, &st)
	if err != nil {
		if os.IsNotExist(err) {
			return "", unix.Stat_t{}, nil // File does not exist
		}
		return "", unix.Stat_t{}, fmt.Errorf("unable to stat file %s: %w", path, err)
	}
	return path, st, nil
}

// checkFilePermissions checks file permissions to decide whether to skip file modification.
func checkFilePermissions(stat *unix.Stat_t, id, statID uint32) bool {
	if stat.Mode&0o022 != 0 {
		return true
	}

	// Check if the UID/GID matches and if the file is owner writable.
	if id == statID && stat.Mode&0o200 != 0 {
		return true
	}

	return false
}

func readFileContent(path string) ([]byte, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // File does not exist
		}
		return nil, fmt.Errorf("read file: %w", err)
	}
	return content, nil
}

func createAndSecureFile(path, content string, mode os.FileMode, uid, gid int) (string, error) {
	if err := os.WriteFile(path, []byte(content), mode&os.ModePerm); err != nil {
		return "", fmt.Errorf("failed to create file: %w", err)
	}
	if err := os.Chown(path, uid, gid); err != nil {
		return "", fmt.Errorf("failed to chown file: %w", err)
	}
	return path, nil
}

// GetGroup searches for a group in the container's /etc/group file using the provided
// container mount path and group identifier (either name or ID). It returns a matching
// user.Group structure if found. If no matching group is located, it returns
// ErrNoGroupEntries.
func GetGroup(containerMount, groupIDorName string) (*user.Group, error) {
	var inputIsName bool
	gid, err := strconv.Atoi(groupIDorName)
	if err != nil {
		inputIsName = true
	}
	groupDest, err := securejoin.SecureJoin(containerMount, "/etc/group")
	if err != nil {
		return nil, err
	}
	groups, err := user.ParseGroupFileFilter(groupDest, func(g user.Group) bool {
		if inputIsName {
			return g.Name == groupIDorName
		}
		return g.Gid == gid
	})
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if len(groups) > 0 {
		return &groups[0], nil
	}
	if !inputIsName {
		return &user.Group{Gid: gid}, user.ErrNoGroupEntries
	}
	return nil, user.ErrNoGroupEntries
}

// GetUser takes a containermount path and user name or ID and returns
// a matching User structure from /etc/passwd.  If it cannot locate a user
// with the provided information, an ErrNoPasswdEntries is returned.
// When the provided user name was an ID, a User structure with Uid
// set is returned along with ErrNoPasswdEntries.
func GetUser(containerMount, userIDorName string) (*user.User, error) {
	var inputIsName bool
	uid, err := strconv.Atoi(userIDorName)
	if err != nil {
		inputIsName = true
	}
	passwdDest, err := securejoin.SecureJoin(containerMount, "/etc/passwd")
	if err != nil {
		return nil, err
	}
	users, err := user.ParsePasswdFileFilter(passwdDest, func(u user.User) bool {
		if inputIsName {
			return u.Name == userIDorName
		}
		return u.Uid == uid
	})
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if len(users) > 0 {
		return &users[0], nil
	}
	if !inputIsName {
		return &user.User{Uid: uid}, user.ErrNoPasswdEntries
	}
	return nil, user.ErrNoPasswdEntries
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

// HandleResizing spawns a goroutine that processes the resize channel, calling
// resizeFunc for each TerminalSize received from the channel. The resize
// channel must be closed elsewhere to stop the goroutine.
func HandleResizing(resize <-chan remotecommand.TerminalSize, resizeFunc func(size remotecommand.TerminalSize)) {
	if resize == nil {
		return
	}

	go func() {
		for {
			size, ok := <-resize
			if !ok {
				return
			}
			if size.Height < 1 || size.Width < 1 {
				continue
			}
			resizeFunc(size)
		}
	}()
}
