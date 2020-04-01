package image

import (
	"context"

	"github.com/containers/buildah/manifests"
	"github.com/containers/image/v5/manifest"
	"github.com/containers/image/v5/transports/alltransports"
	"github.com/containers/image/v5/types"
	"github.com/opencontainers/go-digest"
	"github.com/pkg/errors"
)

// Options for adding a manifest
// swagger:model ManifestAddOpts
type ManifestAddOpts struct {
	All        bool              `json:"all"`
	Annotation map[string]string `json:"annotation"`
	Arch       string            `json:"arch"`
	Features   []string          `json:"features"`
	Images     []string          `json:"images"`
	OSVersion  string            `json:"os_version"`
	Variant    string            `json:"variant"`
}

// InspectManifest returns a dockerized version of the manifest list
func (i *Image) InspectManifest() (*manifest.Schema2List, error) {
	list, err := i.getManifestList()
	if err != nil {
		return nil, err
	}
	return list.Docker(), nil
}

// RemoveManifest removes the given digest from the manifest list.
func (i *Image) RemoveManifest(d digest.Digest) (string, error) {
	list, err := i.getManifestList()
	if err != nil {
		return "", err
	}
	if err := list.Remove(d); err != nil {
		return "", err
	}
	return list.SaveToImage(i.imageruntime.store, i.ID(), nil, "")
}

// getManifestList is a helper to obtain a manifest list
func (i *Image) getManifestList() (manifests.List, error) {
	_, list, err := manifests.LoadFromImage(i.imageruntime.store, i.ID())
	return list, err
}

// CreateManifestList creates a new manifest list and can optionally add given images
// to the list
func CreateManifestList(rt *Runtime, systemContext types.SystemContext, names []string, imgs []string, all bool) (string, error) {
	list := manifests.Create()
	opts := ManifestAddOpts{Images: names, All: all}
	for _, img := range imgs {
		var ref types.ImageReference
		newImage, err := rt.NewFromLocal(img)
		if err == nil {
			ir, err := newImage.toImageRef(context.Background())
			if err != nil {
				return "", err
			}
			if ir == nil {
				return "", errors.New("unable to convert image to ImageReference")
			}
			ref = ir.Reference()
		} else {
			ref, err = alltransports.ParseImageName(img)
			if err != nil {
				return "", err
			}
		}
		list, err = addManifestToList(ref, list, systemContext, opts)
		if err != nil {
			return "", err
		}
	}
	return list.SaveToImage(rt.store, "", names, manifest.DockerV2ListMediaType)
}

func addManifestToList(ref types.ImageReference, list manifests.List, systemContext types.SystemContext, opts ManifestAddOpts) (manifests.List, error) {
	d, err := list.Add(context.Background(), &systemContext, ref, opts.All)
	if err != nil {
		return nil, err
	}
	if len(opts.OSVersion) > 0 {
		if err := list.SetOSVersion(d, opts.OSVersion); err != nil {
			return nil, err
		}
	}
	if len(opts.Features) > 0 {
		if err := list.SetFeatures(d, opts.Features); err != nil {
			return nil, err
		}
	}
	if len(opts.Arch) > 0 {
		if err := list.SetArchitecture(d, opts.Arch); err != nil {
			return nil, err
		}
	}
	if len(opts.Variant) > 0 {
		if err := list.SetVariant(d, opts.Variant); err != nil {
			return nil, err
		}
	}
	if len(opts.Annotation) > 0 {
		if err := list.SetAnnotations(&d, opts.Annotation); err != nil {
			return nil, err
		}
	}
	return list, err
}

// AddManifest adds a manifest to a given manifest list.
func (i *Image) AddManifest(systemContext types.SystemContext, opts ManifestAddOpts) (string, error) {
	var (
		ref types.ImageReference
	)
	newImage, err := i.imageruntime.NewFromLocal(opts.Images[0])
	if err == nil {
		ir, err := newImage.toImageRef(context.Background())
		if err != nil {
			return "", err
		}
		ref = ir.Reference()
	} else {
		ref, err = alltransports.ParseImageName(opts.Images[0])
		if err != nil {
			return "", err
		}
	}
	list, err := i.getManifestList()
	if err != nil {
		return "", err
	}
	list, err = addManifestToList(ref, list, systemContext, opts)
	if err != nil {
		return "", err
	}
	return list.SaveToImage(i.imageruntime.store, i.ID(), nil, "")
}

// PushManifest pushes a manifest to a destination
func (i *Image) PushManifest(dest types.ImageReference, opts manifests.PushOptions) (digest.Digest, error) {
	list, err := i.getManifestList()
	if err != nil {
		return "", err
	}
	_, d, err := list.Push(context.Background(), dest, opts)
	return d, err
}
