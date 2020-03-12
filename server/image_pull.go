package server

import (
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/containers/image/v5/copy"
	"github.com/containers/image/v5/types"
	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/internal/storage"
	"github.com/cri-o/cri-o/server/metrics"
	"golang.org/x/net/context"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// PullImage pulls a image with authentication config.
func (s *Server) PullImage(ctx context.Context, req *pb.PullImageRequest) (resp *pb.PullImageResponse, err error) {
	// TODO: what else do we need here? (Signatures when the story isn't just pulling from docker://)
	image := ""
	img := req.GetImage()
	if img != nil {
		image = img.Image
	}

	sourceCtx := *s.config.SystemContext // A shallow copy we can modify
	if req.GetAuth() != nil {
		username := req.GetAuth().Username
		password := req.GetAuth().Password
		if req.GetAuth().Auth != "" {
			username, password, err = decodeDockerAuth(req.GetAuth().Auth)
			if err != nil {
				log.Debugf(ctx, "error decoding authentication for image %s: %v", img, err)
				return nil, err
			}
		}
		// Specifying a username indicates the user intends to send authentication to the registry.
		if username != "" {
			sourceCtx.DockerAuthConfig = &types.DockerAuthConfig{
				Username: username,
				Password: password,
			}
		}
	}

	decryptConfig, err := getDecryptionKeys(s.config.DecryptionKeysPath)
	if err != nil {
		return nil, err
	}

	var (
		images []string
		pulled string
	)
	images, err = s.StorageImageServer().ResolveNames(s.config.SystemContext, image)
	if err != nil {
		return nil, err
	}
	for _, img := range images {
		var tmpImg types.ImageCloser
		tmpImg, err = s.StorageImageServer().PrepareImage(&sourceCtx, img)
		if err != nil {
			log.Debugf(ctx, "error preparing image %s: %v", img, err)
			continue
		}
		defer tmpImg.Close()

		var storedImage *storage.ImageResult
		storedImage, err = s.StorageImageServer().ImageStatus(s.config.SystemContext, img)
		if err == nil {
			tmpImgConfigDigest := tmpImg.ConfigInfo().Digest
			if tmpImgConfigDigest.String() == "" {
				// this means we are playing with a schema1 image, in which
				// case, we're going to repull the image in any case
				log.Debugf(ctx, "image config digest is empty, re-pulling image")
			} else if tmpImgConfigDigest.String() == storedImage.ConfigDigest.String() {
				log.Debugf(ctx, "image %s already in store, skipping pull", img)
				pulled = img

				// Skipped bytes metrics
				if storedImage.Size != nil {
					counter, err := metrics.CRIOImagePullsByNameSkipped.GetMetricWithLabelValues(img)
					if err != nil {
						log.Warnf(ctx, "Unable to write image pull name (skipped) metrics: %v", err)
					} else {
						counter.Add(float64(*storedImage.Size))
					}
				}

				break
			}
			log.Debugf(ctx, "image in store has different ID, re-pulling %s", img)
		}

		// Pull by collecting progress metrics
		progress := make(chan types.ProgressProperties)
		go func() {
			for p := range progress {
				if p.Artifact.Size > 0 {
					log.Debugf(ctx, "ImagePull (%v): %s (%s): %v bytes (%.2f%%)",
						p.Event, img, p.Artifact.Digest, p.Offset,
						float64(p.Offset)/float64(p.Artifact.Size)*100,
					)
				} else {
					log.Debugf(ctx, "ImagePull (%v): %s (%s): %v bytes",
						p.Event, img, p.Artifact.Digest, p.Offset,
					)
				}

				// Metrics for every digest
				digestCounter, err := metrics.CRIOImagePullsByDigest.GetMetricWithLabelValues(
					img, p.Artifact.Digest.String(), p.Artifact.MediaType,
					fmt.Sprintf("%d", p.Artifact.Size),
				)
				if err != nil {
					log.Warnf(ctx, "Unable to write image pull digest metrics: %v", err)
				} else {
					digestCounter.Add(float64(p.OffsetUpdate))
				}

				// Metrics for the overall image
				nameCounter, err := metrics.CRIOImagePullsByName.GetMetricWithLabelValues(
					img, fmt.Sprintf("%d", imageSize(tmpImg)),
				)
				if err != nil {
					log.Warnf(ctx, "Unable to write image pull name metrics: %v", err)
				} else {
					nameCounter.Add(float64(p.OffsetUpdate))
				}
			}
		}()
		_, err = s.StorageImageServer().PullImage(s.config.SystemContext, img, &copy.Options{
			SourceCtx:        &sourceCtx,
			DestinationCtx:   s.config.SystemContext,
			OciDecryptConfig: decryptConfig,
			ProgressInterval: time.Second,
			Progress:         progress,
		})
		if err != nil {
			log.Debugf(ctx, "error pulling image %s: %v", img, err)
			continue
		}
		pulled = img
		break
	}
	if pulled == "" && err != nil {
		return nil, err
	}
	status, err := s.StorageImageServer().ImageStatus(s.config.SystemContext, pulled)
	if err != nil {
		return nil, err
	}
	imageRef := status.ID
	if len(status.RepoDigests) > 0 {
		imageRef = status.RepoDigests[0]
	}
	resp = &pb.PullImageResponse{
		ImageRef: imageRef,
	}
	return resp, nil
}

func decodeDockerAuth(s string) (user, password string, err error) {
	decoded, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return "", "", err
	}
	parts := strings.SplitN(string(decoded), ":", 2)
	if len(parts) != 2 {
		// if it's invalid just skip, as docker does
		return "", "", nil
	}
	user = parts[0]
	password = strings.Trim(parts[1], "\x00")
	return user, password, nil
}

func imageSize(img types.ImageCloser) (size int64) {
	for _, layer := range img.LayerInfos() {
		if layer.Size > 0 {
			size += layer.Size
		} else {
			return -1
		}
	}

	configSize := img.ConfigInfo().Size
	if configSize >= 0 {
		size += configSize
	} else {
		return -1
	}

	return size
}
