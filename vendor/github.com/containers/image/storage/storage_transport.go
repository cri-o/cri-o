package storage

import (
	"errors"
	"regexp"

	"github.com/Sirupsen/logrus"
	"github.com/containers/image/types"
	"github.com/containers/storage/storage"
)

var (
	// Transport is an ImageTransport that uses a default storage.Store or
	// one that's it's explicitly told to use.
	Transport StoreTransport = &storageTransport{}
	// ErrInvalidReference is returned when ParseReference() is passed an
	// empty reference.
	ErrInvalidReference = errors.New("invalid reference")
	idRegexp            = regexp.MustCompile("^(sha256:)?([0-9a-fA-F]{64})$")
)

// StoreTransport is an ImageTransport that uses a storage.Store to parse
// references, either its own or one that it's told to use.
type StoreTransport interface {
	types.ImageTransport
	SetStore(storage.Store)
	ParseStoreReference(store storage.Store, reference string) (types.ImageReference, error)
}

type storageTransport struct {
	store storage.Store
}

func (s *storageTransport) Name() string {
	return "oci-storage"
}

// SetStore sets the Store object which the Transport will use for parsing
// references when a Store is not directly specified.  If one is not set, the
// library will attempt to initialize one with default settings when a
// reference needs to be parsed.  Calling SetStore does not affect previously
// parsed references.
func (s *storageTransport) SetStore(store storage.Store) {
	s.store = store
}

// ParseStoreReference takes a name or an ID, tries to figure out which it is
// relative to a given store, and returns it in a reference object.
func (s *storageTransport) ParseStoreReference(store storage.Store, reference string) (types.ImageReference, error) {
	if reference == "" {
		return nil, ErrInvalidReference
	}
	id := ""
	if matches := idRegexp.FindStringSubmatch(reference); len(matches) > 1 {
		id = matches[len(matches)-1]
		logrus.Debugf("parsed reference %q into ID %q", reference, id)
		reference = ""
	} else {
		logrus.Debugf("treating reference %q as a name", reference)
	}
	return newReference(store, s, reference, id), nil
}

// ParseReference initializes the storage library and then takes a name or an
// ID, tries to figure out which it is, and returns it in a reference object.
func (s *storageTransport) ParseReference(reference string) (types.ImageReference, error) {
	if s.store == nil {
		store, err := storage.MakeStore("", "", "", []string{}, nil, nil)
		if err != nil {
			return nil, err
		}
		s.store = store
	}
	return s.ParseStoreReference(s.store, reference)
}

func (s *storageTransport) ValidatePolicyConfigurationScope(scope string) error {
	return nil
}
