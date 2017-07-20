package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"runtime"
	"time"

	"github.com/containers/image/docker/reference"
	is "github.com/containers/image/storage"
	"github.com/containers/image/types"
	"github.com/containers/storage"
	"github.com/containers/storage/pkg/archive"
	"github.com/kubernetes-incubator/cri-o/cmd/kpod/docker" // Get rid of this eventually
	digest "github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go/v1"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"
)

const (
	// OCIv1ImageManifest is the MIME type of an OCIv1 image manifest,
	// suitable for specifying as a value of the PreferredManifestType
	// member of a CommitOptions structure.  It is also the default.
	OCIv1ImageManifest = v1.MediaTypeImageManifest
)

type imagePushData struct {
	store storage.Store
	// Type is used to help a build container's metadata
	Type string `json:"type"`
	// FromImage is the name of the source image which ws used to create
	// the container, if one was used
	FromImage string `json:"image,omitempty"`
	// FromImageID is the id of the source image
	FromImageID string `json:"imageid"`
	// Config is the source image's configuration
	Config []byte `json:"config,omitempty"`
	// Manifest is the source image's manifest
	Manifest []byte `json:"manifest,omitempty"`
	// ImageAnnotations is a set of key-value pairs which is stored in the
	// image's manifest
	ImageAnnotations map[string]string `json:"annotations,omitempty"`
	// ImageCreatedBy is a description of how this container was built
	ImageCreatedBy string `json:"created-by,omitempty"`

	// Image metadata and runtime settings, in multiple formats
	OCIv1  ociv1.Image    `json:"ociv1,omitempty"`
	Docker docker.V2Image `json:"docker,omitempty"`
}

func (i *imagePushData) initConfig() {
	image := ociv1.Image{}
	dimage := docker.V2Image{}
	if len(i.Config) > 0 {
		// Try to parse the image config.  If we fail, try to start over from scratch
		if err := json.Unmarshal(i.Config, &dimage); err == nil && dimage.DockerVersion != "" {
			image, err = makeOCIv1Image(&dimage)
			if err != nil {
				image = ociv1.Image{}
			}
		} else {
			if err := json.Unmarshal(i.Config, &image); err != nil {
				if dimage, err = makeDockerV2S2Image(&image); err != nil {
					dimage = docker.V2Image{}
				}
			}
		}
		i.OCIv1 = image
		i.Docker = dimage
	} else {
		// Try to dig out the image configuration from the manifest
		manifest := docker.V2S1Manifest{}
		if err := json.Unmarshal(i.Manifest, &manifest); err == nil && manifest.SchemaVersion == 1 {
			if dimage, err = makeDockerV2S1Image(manifest); err == nil {
				if image, err = makeOCIv1Image(&dimage); err != nil {
					image = ociv1.Image{}
				}
			}
		}
		i.OCIv1 = image
		i.Docker = dimage
	}

	if len(i.Manifest) > 0 {
		// Attempt to recover format-specific data from the manifest
		v1Manifest := ociv1.Manifest{}
		if json.Unmarshal(i.Manifest, &v1Manifest) == nil {
			i.ImageAnnotations = v1Manifest.Annotations
		}
	}

	i.fixupConfig()
}

func (i *imagePushData) fixupConfig() {
	if i.Docker.Config != nil {
		// Prefer image-level settings over those from the container it was built from
		i.Docker.ContainerConfig = *i.Docker.Config
	}
	i.Docker.Config = &i.Docker.ContainerConfig
	i.Docker.DockerVersion = ""
	now := time.Now().UTC()
	if i.Docker.Created.IsZero() {
		i.Docker.Created = now
	}
	if i.OCIv1.Created.IsZero() {
		i.OCIv1.Created = &now
	}
	if i.OS() == "" {
		i.SetOS(runtime.GOOS)
	}
	if i.Architecture() == "" {
		i.SetArchitecture(runtime.GOARCH)
	}
	if i.WorkDir() == "" {
		i.SetWorkDir(string(filepath.Separator))
	}
}

// OS returns a name of the OS on which a container built using this image
//is intended to be run.
func (i *imagePushData) OS() string {
	return i.OCIv1.OS
}

// SetOS sets the name of the OS on which a container built using this image
// is intended to be run.
func (i *imagePushData) SetOS(os string) {
	i.OCIv1.OS = os
	i.Docker.OS = os
}

// Architecture returns a name of the architecture on which a container built
// using this image is intended to be run.
func (i *imagePushData) Architecture() string {
	return i.OCIv1.Architecture
}

// SetArchitecture sets the name of the architecture on which ta container built
// using this image is intended to be run.
func (i *imagePushData) SetArchitecture(arch string) {
	i.OCIv1.Architecture = arch
	i.Docker.Architecture = arch
}

// WorkDir returns the default working directory for running commands in a container
// built using this image.
func (i *imagePushData) WorkDir() string {
	return i.OCIv1.Config.WorkingDir
}

// SetWorkDir sets the location of the default working directory for running commands
// in a container built using this image.
func (i *imagePushData) SetWorkDir(there string) {
	i.OCIv1.Config.WorkingDir = there
	i.Docker.Config.WorkingDir = there
}

// makeOCIv1Image builds the best OCIv1 image structure we can from the
// contents of the docker image structure.
func makeOCIv1Image(dimage *docker.V2Image) (ociv1.Image, error) {
	config := dimage.Config
	if config == nil {
		config = &dimage.ContainerConfig
	}
	dimageCreatedTime := dimage.Created.UTC()
	image := ociv1.Image{
		Created:      &dimageCreatedTime,
		Author:       dimage.Author,
		Architecture: dimage.Architecture,
		OS:           dimage.OS,
		Config: ociv1.ImageConfig{
			User:         config.User,
			ExposedPorts: map[string]struct{}{},
			Env:          config.Env,
			Entrypoint:   config.Entrypoint,
			Cmd:          config.Cmd,
			Volumes:      config.Volumes,
			WorkingDir:   config.WorkingDir,
			Labels:       config.Labels,
		},
		RootFS: ociv1.RootFS{
			Type:    "",
			DiffIDs: []digest.Digest{},
		},
		History: []ociv1.History{},
	}
	for port, what := range config.ExposedPorts {
		image.Config.ExposedPorts[string(port)] = what
	}
	RootFS := docker.V2S2RootFS{}
	if dimage.RootFS != nil {
		RootFS = *dimage.RootFS
	}
	if RootFS.Type == docker.TypeLayers {
		image.RootFS.Type = docker.TypeLayers
		for _, id := range RootFS.DiffIDs {
			image.RootFS.DiffIDs = append(image.RootFS.DiffIDs, digest.Digest(id.String()))
		}
	}
	for _, history := range dimage.History {
		historyCreatedTime := history.Created.UTC()
		ohistory := ociv1.History{
			Created:    &historyCreatedTime,
			CreatedBy:  history.CreatedBy,
			Author:     history.Author,
			Comment:    history.Comment,
			EmptyLayer: history.EmptyLayer,
		}
		image.History = append(image.History, ohistory)
	}
	return image, nil
}

// makeDockerV2S2Image builds the best docker image structure we can from the
// contents of the OCI image structure.
func makeDockerV2S2Image(oimage *ociv1.Image) (docker.V2Image, error) {
	image := docker.V2Image{
		V1Image: docker.V1Image{Created: oimage.Created.UTC(),
			Author:       oimage.Author,
			Architecture: oimage.Architecture,
			OS:           oimage.OS,
			ContainerConfig: docker.Config{
				User:         oimage.Config.User,
				ExposedPorts: docker.PortSet{},
				Env:          oimage.Config.Env,
				Entrypoint:   oimage.Config.Entrypoint,
				Cmd:          oimage.Config.Cmd,
				Volumes:      oimage.Config.Volumes,
				WorkingDir:   oimage.Config.WorkingDir,
				Labels:       oimage.Config.Labels,
			},
		},
		RootFS: &docker.V2S2RootFS{
			Type:    "",
			DiffIDs: []digest.Digest{},
		},
		History: []docker.V2S2History{},
	}
	for port, what := range oimage.Config.ExposedPorts {
		image.ContainerConfig.ExposedPorts[docker.Port(port)] = what
	}
	if oimage.RootFS.Type == docker.TypeLayers {
		image.RootFS.Type = docker.TypeLayers
		for _, id := range oimage.RootFS.DiffIDs {
			d, err := digest.Parse(id.String())
			if err != nil {
				return docker.V2Image{}, err
			}
			image.RootFS.DiffIDs = append(image.RootFS.DiffIDs, d)
		}
	}
	for _, history := range oimage.History {
		dhistory := docker.V2S2History{
			Created:    history.Created.UTC(),
			CreatedBy:  history.CreatedBy,
			Author:     history.Author,
			Comment:    history.Comment,
			EmptyLayer: history.EmptyLayer,
		}
		image.History = append(image.History, dhistory)
	}
	image.Config = &image.ContainerConfig
	return image, nil
}

// makeDockerV2S1Image builds the best docker image structure we can from the
// contents of the V2S1 image structure.
func makeDockerV2S1Image(manifest docker.V2S1Manifest) (docker.V2Image, error) {
	// Treat the most recent (first) item in the history as a description of the image.
	if len(manifest.History) == 0 {
		return docker.V2Image{}, errors.Errorf("error parsing image configuration from manifest")
	}
	dimage := docker.V2Image{}
	err := json.Unmarshal([]byte(manifest.History[0].V1Compatibility), &dimage)
	if err != nil {
		return docker.V2Image{}, err
	}
	if dimage.DockerVersion == "" {
		return docker.V2Image{}, errors.Errorf("error parsing image configuration from history")
	}
	// The DiffID list is intended to contain the sums of _uncompressed_ blobs, and these are most
	// likely compressed, so leave the list empty to avoid potential confusion later on.  We can
	// construct a list with the correct values when we prep layers for pushing, so we don't lose.
	// information by leaving this part undone.
	rootFS := &docker.V2S2RootFS{
		Type:    docker.TypeLayers,
		DiffIDs: []digest.Digest{},
	}
	// Build a filesystem history.
	history := []docker.V2S2History{}
	for i := range manifest.History {
		h := docker.V2S2History{
			Created:    time.Now().UTC(),
			Author:     "",
			CreatedBy:  "",
			Comment:    "",
			EmptyLayer: false,
		}
		dcompat := docker.V1Compatibility{}
		if err2 := json.Unmarshal([]byte(manifest.History[i].V1Compatibility), &dcompat); err2 == nil {
			h.Created = dcompat.Created.UTC()
			h.Author = dcompat.Author
			h.Comment = dcompat.Comment
			if len(dcompat.ContainerConfig.Cmd) > 0 {
				h.CreatedBy = fmt.Sprintf("%v", dcompat.ContainerConfig.Cmd)
			}
			h.EmptyLayer = dcompat.ThrowAway
		}
		// Prepend this layer to the list, because a v2s1 format manifest's list is in reverse order
		// compared to v2s2, which lists earlier layers before later ones.
		history = append([]docker.V2S2History{h}, history...)
	}
	dimage.RootFS = rootFS
	dimage.History = history
	return dimage, nil
}

func (i *imagePushData) Annotations() map[string]string {
	return copyStringStringMap(i.ImageAnnotations)
}

func (i *imagePushData) makeImageRef(manifestType string, compress archive.Compression, names []string, layerID string, historyTimestamp *time.Time) (types.ImageReference, error) {
	var name reference.Named
	if len(names) > 0 {
		if parsed, err := reference.ParseNamed(names[0]); err == nil {
			name = parsed
		}
	}
	if manifestType == "" {
		manifestType = OCIv1ImageManifest
	}
	oconfig, err := json.Marshal(&i.OCIv1)
	if err != nil {
		return nil, errors.Wrapf(err, "error encoding OCI-format image configuration")
	}
	dconfig, err := json.Marshal(&i.Docker)
	if err != nil {
		return nil, errors.Wrapf(err, "error encoding docker-format image configuration")
	}
	created := time.Now().UTC()
	if historyTimestamp != nil {
		created = historyTimestamp.UTC()
	}
	ref := &containerImageRef{
		store:                 i.store,
		compression:           compress,
		name:                  name,
		names:                 names,
		layerID:               layerID,
		addHistory:            false,
		oconfig:               oconfig,
		dconfig:               dconfig,
		created:               created,
		createdBy:             i.ImageCreatedBy,
		annotations:           i.ImageAnnotations,
		preferredManifestType: manifestType,
		exporting:             true,
	}
	return ref, nil
}

func importImagePushDataFromImage(store storage.Store, img *storage.Image, systemContext *types.SystemContext) (*imagePushData, error) {
	manifest := []byte{}
	config := []byte{}
	imageName := ""

	if img.ID != "" {
		ref, err := is.Transport.ParseStoreReference(store, "@"+img.ID)
		if err != nil {
			return nil, errors.Wrapf(err, "no such image %q", "@"+img.ID)
		}
		src, err2 := ref.NewImage(systemContext)
		if err2 != nil {
			return nil, errors.Wrapf(err2, "error reading image configuration")
		}
		defer src.Close()
		config, err = src.ConfigBlob()
		if err != nil {
			return nil, errors.Wrapf(err, "error reading image manfest")
		}
		manifest, _, err = src.Manifest()
		if err != nil {
			return nil, errors.Wrapf(err, "error reading image manifest")
		}
		if len(img.Names) > 0 {
			imageName = img.Names[0]
		}
	}

	ipd := &imagePushData{
		store:            store,
		FromImage:        imageName,
		FromImageID:      img.ID,
		Config:           config,
		Manifest:         manifest,
		ImageAnnotations: map[string]string{},
		ImageCreatedBy:   "",
	}

	ipd.initConfig()

	return ipd, nil
}
