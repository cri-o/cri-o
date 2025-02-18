package server

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/containers/image/v5/signature"
	imageTypes "github.com/containers/image/v5/types"
	encconfig "github.com/containers/ocicrypt/config"
	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/internal/storage"
	"github.com/cri-o/cri-o/server/metrics"
	"github.com/cri-o/cri-o/utils"
	"github.com/docker/distribution/registry/api/errcode"
	"github.com/opencontainers/go-digest"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
	crierrors "k8s.io/cri-api/pkg/errors"
)

var localRegistryHostname = "localhost"

// PullImage pulls a image with authentication config.
func (s *Server) PullImage(ctx context.Context, req *types.PullImageRequest) (*types.PullImageResponse, error) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	// TODO: what else do we need here? (Signatures when the story isn't just pulling from docker://)
	var err error
	image := ""
	img := req.Image
	if img != nil {
		image = img.Image
	}
	log.Infof(ctx, "Pulling image: %s", image)

	pullArgs := pullArguments{image: image}

	sc := req.SandboxConfig
	if sc != nil {
		if sc.Linux != nil {
			pullArgs.sandboxCgroup = sc.Linux.CgroupParent
		}
		if sc.Metadata != nil {
			pullArgs.namespace = sc.Metadata.Namespace
		}
	}

	if req.Auth != nil {
		username := req.Auth.Username
		password := req.Auth.Password
		if req.Auth.Auth != "" {
			username, password, err = decodeDockerAuth(req.Auth.Auth)
			if err != nil {
				log.Debugf(ctx, "Error decoding authentication for image %s: %v", image, err)
				return nil, err
			}
		}
		// Specifying a username indicates the user intends to send authentication to the registry.
		if username != "" {
			pullArgs.credentials = imageTypes.DockerAuthConfig{
				Username: username,
				Password: password,
			}
		}
	}

	// We use the server's pullOperationsInProgress to record which images are
	// currently being pulled. This allows for avoiding pulling the same image
	// in parallel. Hence, if a given image is currently being pulled, we queue
	// into the pullOperation's waitgroup and wait for the pulling goroutine to
	// unblock us and re-use its results.
	pullOp, pullInProcess := func() (pullOp *pullOperation, inProgress bool) {
		s.pullOperationsLock.Lock()
		defer s.pullOperationsLock.Unlock()
		pullOp, inProgress = s.pullOperationsInProgress[pullArgs]
		if !inProgress {
			pullOp = &pullOperation{}
			s.pullOperationsInProgress[pullArgs] = pullOp
			storage.ImageBeingPulled.Store(pullArgs.image, true)
			pullOp.wg.Add(1)
		}
		return pullOp, inProgress
	}()

	if !pullInProcess {
		pullOp.err = errors.New("pullImage was aborted by a Go panic")
		defer func() {
			s.pullOperationsLock.Lock()
			delete(s.pullOperationsInProgress, pullArgs)
			storage.ImageBeingPulled.Delete(pullArgs.image)
			pullOp.wg.Done()
			s.pullOperationsLock.Unlock()
		}()
		pullOp.imageRef, pullOp.err = s.pullImage(ctx, &pullArgs)
	} else {
		// Wait for the pull operation to finish.
		pullOp.wg.Wait()
	}

	if pullOp.err != nil {
		wrap := func(e error) error { return fmt.Errorf("%w: %w", e, pullOp.err) }

		if errors.Is(pullOp.err, syscall.ECONNREFUSED) {
			return nil, wrap(crierrors.ErrRegistryUnavailable)
		}

		var policyErr signature.PolicyRequirementError
		if errors.As(pullOp.err, &policyErr) {
			return nil, wrap(crierrors.ErrSignatureValidationFailed)
		}

		return nil, pullOp.err
	}

	log.Infof(ctx, "Pulled image: %v", pullOp.imageRef)
	return &types.PullImageResponse{
		ImageRef: pullOp.imageRef,
	}, nil
}

// pullImage performs the actual pull operation of PullImage. Used to separate
// the pull implementation from the pullCache logic in PullImage and improve
// readability and maintainability.
func (s *Server) pullImage(ctx context.Context, pullArgs *pullArguments) (string, error) {
	var err error
	ctx, span := log.StartSpan(ctx)
	defer span.End()

	sourceCtx := *s.config.SystemContext   // A shallow copy we can modify
	sourceCtx.DockerLogMirrorChoice = true // Add info level log of the pull source
	if pullArgs.credentials.Username != "" {
		sourceCtx.DockerAuthConfig = &pullArgs.credentials
	}

	if pullArgs.namespace != "" {
		policyPath := filepath.Join(s.config.SignaturePolicyDir, pullArgs.namespace+".json")
		if _, err := os.Stat(policyPath); err == nil {
			sourceCtx.SignaturePolicyPath = policyPath
		} else if !os.IsNotExist(err) {
			return "", fmt.Errorf("read policy path %s: %w", policyPath, err)
		}
	}
	log.Debugf(ctx, "Using pull policy path for image %s: %s", pullArgs.image, sourceCtx.SignaturePolicyPath)

	decryptConfig, err := getDecryptionKeys(s.config.DecryptionKeysPath)
	if err != nil {
		return "", err
	}

	cgroup := ""
	if s.config.SeparatePullCgroup != "" {
		if !s.config.CgroupManager().IsSystemd() {
			return "", errors.New("--separate-pull-cgroup is supported only with systemd")
		}
		if s.config.SeparatePullCgroup == utils.PodCgroupName {
			cgroup = pullArgs.sandboxCgroup
		} else {
			cgroup = s.config.SeparatePullCgroup
			if !strings.Contains(cgroup, ".slice") {
				return "", fmt.Errorf("invalid systemd cgroup %q", cgroup)
			}
		}
	}

	remoteCandidates, err := s.StorageImageServer().CandidatesForPotentiallyShortImageName(s.config.SystemContext, pullArgs.image)
	if err != nil {
		return "", err
	}
	// CandidatesForPotentiallyShortImageName is defined never to return an empty slice on success, so if the loop considers all candidates
	// and they all fail, this error value should be overwritten by a real failure.
	lastErr := errors.New("internal error: pullImage failed but reported no error reason")
	for _, remoteCandidateName := range remoteCandidates {
		err := s.pullImageCandidate(ctx, &sourceCtx, remoteCandidateName, decryptConfig, cgroup)
		if err == nil {
			// Update metric for successful image pulls
			metrics.Instance().MetricImagePullsSuccessesInc(remoteCandidateName)

			status, err := s.StorageImageServer().ImageStatusByName(s.config.SystemContext, remoteCandidateName)
			if err != nil {
				return "", err
			}
			imageRef := status.ID.IDStringForOutOfProcessConsumptionOnly()
			if len(status.RepoDigests) > 0 {
				imageRef = status.RepoDigests[0]
			}

			return imageRef, nil
		}
		lastErr = err
	}
	return "", lastErr
}

func (s *Server) pullImageCandidate(ctx context.Context, sourceCtx *imageTypes.SystemContext, remoteCandidateName storage.RegistryImageReference, decryptConfig *encconfig.DecryptConfig, cgroup string) error {
	tmpImg, err := s.StorageImageServer().PrepareImage(sourceCtx, remoteCandidateName)
	if err != nil {
		// We're not able to find the image remotely, check if it's
		// available locally, but only for localhost ones.
		// This allows pulling localhost images even if the
		// `imagePullPolicy` is set to `Always`.
		if remoteCandidateName.Registry() == localRegistryHostname {
			if _, err := s.StorageImageServer().ImageStatusByName(s.config.SystemContext, remoteCandidateName); err == nil {
				return nil
			}
		}
		log.Debugf(ctx, "Error preparing image %s: %v", remoteCandidateName, err)
		tryIncrementImagePullFailureMetric(remoteCandidateName, err)
		return err
	}
	defer tmpImg.Close()

	storedImage, err := s.StorageImageServer().ImageStatusByName(s.config.SystemContext, remoteCandidateName)
	if err == nil {
		tmpImgConfigDigest := tmpImg.ConfigInfo().Digest
		if tmpImgConfigDigest.String() == "" {
			// this means we are playing with a schema1 image, in which
			// case, we're going to repull the image in any case
			log.Debugf(ctx, "Image config digest is empty, re-pulling image")
		} else if tmpImgConfigDigest.String() == storedImage.ConfigDigest.String() {
			log.Debugf(ctx, "Image %s already in store, skipping pull", remoteCandidateName)

			// Skipped digests metrics
			tryRecordSkippedMetric(ctx, remoteCandidateName, tmpImgConfigDigest)

			// Skipped bytes metrics
			if storedImage.Size != nil {
				// Metrics for image pull skipped bytes
				metrics.Instance().MetricImagePullsSkippedBytesAdd(float64(*storedImage.Size))
			}

			return nil
		}
		log.Debugf(ctx, "Image in store has different ID, re-pulling %s", remoteCandidateName)
	}

	// Collect pull progress metrics
	progress := make(chan imageTypes.ProgressProperties)
	defer close(progress) // nolint:gocritic

	// Cancel the pull if no progress is made
	pullCtx, cancel := context.WithCancel(ctx)
	go consumeImagePullProgress(ctx, cancel, s.Config().PullProgressTimeout, progress, remoteCandidateName)

	_, err = s.StorageImageServer().PullImage(pullCtx, remoteCandidateName, &storage.ImageCopyOptions{
		SourceCtx:        sourceCtx,
		DestinationCtx:   s.config.SystemContext,
		OciDecryptConfig: decryptConfig,
		ProgressInterval: s.Config().PullProgressTimeout / 10,
		Progress:         progress,
		CgroupPull: storage.CgroupPullConfiguration{
			UseNewCgroup: s.config.SeparatePullCgroup != "",
			ParentCgroup: cgroup,
		},
	})
	if err != nil {
		log.Debugf(ctx, "Error pulling image %s: %v", remoteCandidateName, err)
		tryIncrementImagePullFailureMetric(remoteCandidateName, err)
		return err
	}
	return nil
}

// consumeImagePullProgress consumes progress and turns it into metrics updates.
// It also checks if progress is being made within a constant timeout.
// If the timeout is reached because no progress updates have been made, then
// the cancel function will be called.
func consumeImagePullProgress(ctx context.Context, cancel context.CancelFunc, pullProgressTimeout time.Duration, progress <-chan imageTypes.ProgressProperties, remoteCandidateName storage.RegistryImageReference) {
	timer := time.AfterFunc(pullProgressTimeout, func() {
		log.Warnf(ctx, "Timed out on waiting up to %s for image pull progress updates", pullProgressTimeout)
		cancel()
	})
	timer.Stop()       // don't start the timer immediately
	defer timer.Stop() // ensure that the timer is stopped when we exit the progress loop

	for p := range progress {
		timer.Reset(pullProgressTimeout)

		if p.Event == imageTypes.ProgressEventSkipped {
			// Skipped digests metrics
			tryRecordSkippedMetric(ctx, remoteCandidateName, p.Artifact.Digest)
		}
		if p.Artifact.Size > 0 {
			log.Debugf(ctx, "ImagePull (%v): %s (%s): %v bytes (%.2f%%)",
				p.Event, remoteCandidateName, p.Artifact.Digest, p.Offset,
				float64(p.Offset)/float64(p.Artifact.Size)*100,
			)
		} else {
			log.Debugf(ctx, "ImagePull (%v): %s (%s): %v bytes",
				p.Event, remoteCandidateName, p.Artifact.Digest, p.Offset,
			)
		}

		// Metrics for image pulls bytes
		metrics.Instance().MetricImagePullsBytesAdd(
			float64(p.OffsetUpdate),
			p.Artifact.MediaType,
			p.Artifact.Size,
		)

		// Metrics for size histogram
		if p.Event == imageTypes.ProgressEventDone {
			metrics.Instance().MetricImagePullsLayerSizeObserve(p.Artifact.Size)
		}
	}
}

func tryIncrementImagePullFailureMetric(img storage.RegistryImageReference, err error) {
	// We try to cover some basic use-cases
	const labelUnknown = "UNKNOWN"
	label := labelUnknown

	// Docker registry errors
	for _, desc := range errcode.GetErrorAllDescriptors() {
		if strings.Contains(err.Error(), desc.Message) {
			label = desc.Value
			break
		}
	}
	if label == labelUnknown {
		if strings.Contains(err.Error(), "connection refused") { // nolint: gocritic
			label = "CONNECTION_REFUSED"
		} else if strings.Contains(err.Error(), "connection timed out") {
			label = "CONNECTION_TIMEOUT"
		} else if strings.Contains(err.Error(), "404 (Not Found)") {
			label = "NOT_FOUND"
		}
	}

	// Update metric for failed image pulls
	metrics.Instance().MetricImagePullsFailuresInc(img, label)
}

func tryRecordSkippedMetric(ctx context.Context, name storage.RegistryImageReference, someBlobDigest digest.Digest) {
	// NOTE: This "layer" identification looks like a digested image reference, but
	// it isn’t one:
	// - the digest references a layer or config, not a manifest
	// - "name" may contain a digest already, so this results in name@manifestDigest@someOtherdigest
	layer := fmt.Sprintf("%s@%s", name.StringForOutOfProcessConsumptionOnly(), someBlobDigest.String())
	log.Debugf(ctx, "Skipped layer %s", layer)
	metrics.Instance().MetricImageLayerReuseInc(layer)
}

func decodeDockerAuth(s string) (user, password string, _ error) {
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
