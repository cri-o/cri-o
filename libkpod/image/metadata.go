package image

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/containers/image/types"
	"github.com/containers/storage"
)

// Metadata stores all of the metadata for an image
type Metadata struct {
	Tag            string              `json:"tag"`
	CreatedTime    time.Time           `json:"created-time"`
	ID             string              `json:"id"`
	Blobs          []types.BlobInfo    `json:"blob-list"`
	Layers         map[string][]string `json:"layers"`
	SignatureSizes []string            `json:"signature-sizes"`
}

// ParseMetadata takes an image, parses the json stored in it's metadata
// field, and converts it to a Metadata struct
func ParseMetadata(image storage.Image) (Metadata, error) {
	var m Metadata

	dec := json.NewDecoder(strings.NewReader(image.Metadata))
	if err := dec.Decode(&m); err != nil {
		return Metadata{}, err
	}
	return m, nil
}
