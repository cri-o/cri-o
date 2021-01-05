package unshare

import (
	"fmt"
	"log"
	"os"
	"os/user"
	"sync"

	"github.com/pkg/errors"
)

var (
	homeDirOnce sync.Once
	homeDirErr  error
	homeDir     string
)

// HomeDir returns the home directory for the current user.
func HomeDir() (string, error) {
	homeDirOnce.Do(func() {
		home := os.Getenv("HOME")
		if home == "" {
			id := GetRootlessUID()
			log.Println("============= rootless uid", id)
			usr, err := user.LookupId(fmt.Sprintf("%d", id))
			log.Println(" =========  usr lookupid", usr)
			if err != nil {
				log.Println("============= error resolve HOME", err)
				homeDir, homeDirErr = "", errors.Wrapf(err, "unable to resolve HOME directory")
				return
			}
			homeDir, homeDirErr = usr.HomeDir, nil
			log.Println("================ HOME empty, homeDir: ", homeDir)
		}
		log.Println("============ HOME is not empty: ", home)
		homeDir, homeDirErr = home, nil
	})
	return homeDir, homeDirErr
}
