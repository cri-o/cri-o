package storage

import (
	"github.com/Sirupsen/logrus"
	"github.com/containers/image/types"
	"github.com/containers/storage/storage"
	"github.com/docker/docker/reference"
)

// A storageReference holds a name and/or an ID, which is a 32-byte value
// hex-encoded into a 64-character string.
type storageReference struct {
	store     storage.Store
	transport *storageTransport
	reference string
	id        string
}

func newReference(store storage.Store, transport *storageTransport, reference, id string) *storageReference {
	return &storageReference{
		store:     store,
		transport: transport,
		reference: reference,
		id:        id,
	}
}

// Resolve the reference's name to an image ID in the storage library if
// there's already one present with the same name or ID.
func (s *storageReference) ID() string {
	if s.id == "" {
		image, err := s.store.GetImage(s.reference)
		if image != nil && err == nil {
			s.id = image.ID
		}
	}
	return s.id
}

func (s *storageReference) Transport() types.ImageTransport {
	return s.transport
}

func (s *storageReference) DockerReference() reference.Named {
	return nil
}

func (s *storageReference) StringWithinTransport() string {
	if s.reference == "" {
		return s.id
	}
	return s.reference
}

func (s *storageReference) PolicyConfigurationIdentity() string {
	if s.reference == "" {
		return s.id
	}
	return s.reference
}

func (s *storageReference) PolicyConfigurationNamespaces() []string {
	return nil
}

func (s *storageReference) NewImage(ctx *types.SystemContext) (types.Image, error) {
	return newImage(s), nil
}

func (s *storageReference) DeleteImage(ctx *types.SystemContext) error {
	layers, err := s.store.DeleteImage(s.ID(), true)
	if err == nil {
		logrus.Debugf("deleted image %q", s.ID())
		s.id = ""
		for _, layer := range layers {
			logrus.Debugf("deleted layer %q", layer)
		}
	}
	s.id = ""
	return err
}

func (s *storageReference) NewImageSource(ctx *types.SystemContext, requestedManifestMIMETypes []string) (types.ImageSource, error) {
	return newImageSource(s), nil
}

func (s *storageReference) NewImageDestination(ctx *types.SystemContext) (types.ImageDestination, error) {
	return newImageDestination(s), nil
}
