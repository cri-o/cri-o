// +build libpod

package server

import (
	"fmt"
	"github.com/containers/storage"
	"github.com/projectatomic/libpod/libpod/image"
)

// ImageRuntime returns a libpod image.runtime to access the image store
func (s *Server) ImageRuntime() *image.Runtime {
	options := storage.StoreOptions{
		RunRoot:   s.Config().RunRoot,
		GraphRoot: s.Config().RootConfig.Root,
	}
	r, err := image.NewImageRuntimeFromOptions(options)
	// For now, if we get an error trying to get the imageruntime, we panic. this can be addressed
	// as integration continues.
	if err != nil {
		panic(fmt.Sprintf("unable to get libpod image runtime: %s", err.Error()))
	}
	return r
}
