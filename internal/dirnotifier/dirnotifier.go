package dirnotifier

import (
	"sync"

	"github.com/fsnotify/fsnotify"
	"github.com/pkg/errors"
)

type DirectoryNotifier struct {
	watcher          *fsnotify.Watcher
	filesToNotifiers sync.Map
	opsToWatch       []fsnotify.Op
	dir              string
}

func New(dir string, opsToWatch ...fsnotify.Op) (*DirectoryNotifier, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	dn := &DirectoryNotifier{
		watcher:    watcher,
		opsToWatch: opsToWatch,
		dir:        dir,
	}

	go dn.initializeWatcher()

	if err := watcher.Add(dir); err != nil {
		return nil, err
	}
	return dn, nil
}

func (dn *DirectoryNotifier) initializeWatcher() {
	defer dn.watcher.Close()
	for event := range dn.watcher.Events {
		for op := range dn.opsToWatch {
			if event.Op&fsnotify.Op(op) == fsnotify.Op(op) {
				if notifyChan, ok := dn.filesToNotifiers.Load(event.Name); ok {
					dn.filesToNotifiers.Delete(event.Name)
					close(notifyChan.(chan struct{}))
					break
				}
			}
		}
	}
}

func (dn *DirectoryNotifier) NotifierForFile(file string) (chan struct{}, error) {
	if _, ok := dn.filesToNotifiers.Load(file); ok {
		return nil, errors.Errorf("exec watcher already watching file %s", file)
	}
	c := make(chan struct{}, 1)
	dn.filesToNotifiers.Store(file, c)
	return c, nil
}

func (dn *DirectoryNotifier) Directory() string {
	return dn.dir
}
