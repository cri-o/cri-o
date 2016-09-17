package storage

import (
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/containers/storage/pkg/stringid"
)

// A Locker represents a file lock where the file is used to cache an
// identifier of the last party that made changes to whatever's being protected
// by the lock.
//
// Touch() records, for others sharing the lock, that it was updated by the
// caller.  It should only be called with the lock held.
//
// Modified() checks if the most recent writer was a party other than the
// caller.  It should only be called with the lock held.
type Locker interface {
	sync.Locker
	Touch() error
	Modified() (bool, error)
}

type lockfile struct {
	file string
	fd   uintptr
	me   string
}

// GetLockfile opens a lock file, creating it if necessary.  The Locker object
// return will be returned unlocked.
func GetLockfile(path string) (Locker, error) {
	fd, err := syscall.Open(path, os.O_RDWR|os.O_CREATE, syscall.S_IRUSR|syscall.S_IWUSR)
	if err != nil {
		return nil, err
	}
	return &lockfile{file: path, fd: uintptr(fd)}, nil
}

func (l *lockfile) Lock() {
	lk := syscall.Flock_t{
		Type:   syscall.F_WRLCK,
		Whence: int16(os.SEEK_SET),
		Start:  0,
		Len:    0,
		Pid:    int32(os.Getpid()),
	}
	for syscall.FcntlFlock(l.fd, syscall.F_SETLKW, &lk) != nil {
		time.Sleep(10 * time.Millisecond)
	}
}

func (l *lockfile) Unlock() {
	lk := syscall.Flock_t{
		Type:   syscall.F_UNLCK,
		Whence: int16(os.SEEK_SET),
		Start:  0,
		Len:    0,
		Pid:    int32(os.Getpid()),
	}
	for syscall.FcntlFlock(l.fd, syscall.F_SETLKW, &lk) != nil {
		time.Sleep(10 * time.Millisecond)
	}
}

func (l *lockfile) Touch() error {
	l.me = stringid.GenerateRandomID()
	id := []byte(l.me)
	_, err := syscall.Seek(int(l.fd), 0, os.SEEK_SET)
	if err != nil {
		return err
	}
	n, err := syscall.Write(int(l.fd), id)
	if err != nil {
		return err
	}
	if n != len(id) {
		return syscall.ENOSPC
	}
	return nil
}

func (l *lockfile) Modified() (bool, error) {
	id := []byte(l.me)
	_, err := syscall.Seek(int(l.fd), 0, os.SEEK_SET)
	if err != nil {
		return true, err
	}
	n, err := syscall.Read(int(l.fd), id)
	if err != nil {
		return true, err
	}
	if n != len(id) {
		return true, syscall.ENOSPC
	}
	return string(id) != l.me, nil
}
