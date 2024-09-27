package server

import (
	"context"
	b64 "encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	encconfig "github.com/containers/ocicrypt/config"
	cryptUtils "github.com/containers/ocicrypt/utils"
	"github.com/containers/storage/pkg/mount"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/sirupsen/logrus"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
	kubeletTypes "k8s.io/kubelet/pkg/types"

	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/server/metrics"
)

const (
	maxLabelSize = 4096
)

func validateLabels(labels map[string]string) error {
	for k, v := range labels {
		if (len(k) + len(v)) > maxLabelSize {
			if len(k) > 10 {
				k = k[:10]
			}
			return fmt.Errorf("label key and value greater than maximum size (%d bytes), key: %s", maxLabelSize, k)
		}
	}
	return nil
}

func mergeEnvs(imageConfig *v1.Image, kubeEnvs []*types.KeyValue) []string {
	envs := []string{}
	if kubeEnvs == nil && imageConfig != nil {
		envs = imageConfig.Config.Env
	} else {
		for _, item := range kubeEnvs {
			if item.Key == "" {
				continue
			}
			envs = append(envs, item.Key+"="+item.Value)
		}
		if imageConfig != nil {
			for _, imageEnv := range imageConfig.Config.Env {
				var found bool
				parts := strings.SplitN(imageEnv, "=", 2)
				if len(parts) != 2 {
					continue
				}
				imageEnvKey := parts[0]
				if imageEnvKey == "" {
					continue
				}
				for _, kubeEnv := range envs {
					kubeEnvKey := strings.SplitN(kubeEnv, "=", 2)[0]
					if kubeEnvKey == "" {
						continue
					}
					if imageEnvKey == kubeEnvKey {
						found = true
						break
					}
				}
				if !found {
					envs = append(envs, imageEnv)
				}
			}
		}
	}
	return envs
}

// Translate container labels to a description of the container.
func translateLabelsToDescription(labels map[string]string) string {
	return fmt.Sprintf("%s/%s/%s", labels[kubeletTypes.KubernetesPodNamespaceLabel], labels[kubeletTypes.KubernetesPodNameLabel], labels[kubeletTypes.KubernetesContainerNameLabel])
}

// getDecryptionKeys reads the keys from the given directory.
func getDecryptionKeys(keysPath string) (*encconfig.DecryptConfig, error) {
	if _, err := os.Stat(keysPath); os.IsNotExist(err) {
		logrus.Debugf("Skipping non-existing decryption_keys_path: %s", keysPath)
		return &encconfig.DecryptConfig{}, nil
	}

	base64Keys := []string{}
	walkFn := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		// Handle symlinks
		if info.Mode()&os.ModeSymlink == os.ModeSymlink {
			return errors.New("symbolic links not supported in decryption keys paths")
		}

		privateKey, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read private key file: %w", err)
		}

		sEnc := b64.StdEncoding.EncodeToString(privateKey)
		base64Keys = append(base64Keys, sEnc)

		return nil
	}

	if err := filepath.Walk(keysPath, walkFn); err != nil {
		return nil, err
	}

	sortedDc, err := cryptUtils.SortDecryptionKeys(strings.Join(base64Keys, ","))
	if err != nil {
		return nil, err
	}

	return encconfig.InitDecryption(sortedDc).DecryptConfig, nil
}

func getSourceMount(source string, mountinfos []*mount.Info) (path, optional string, _ error) {
	var res *mount.Info

	for _, mi := range mountinfos {
		// check if mi can be a parent of source
		if strings.HasPrefix(source, mi.Mountpoint) {
			// look for a longest one
			if res == nil || len(mi.Mountpoint) > len(res.Mountpoint) {
				res = mi
			}
		}
	}
	if res == nil {
		return "", "", fmt.Errorf("could not find source mount of %s", source)
	}

	return res.Mountpoint, res.Optional, nil
}

func isContextError(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func (s *Server) getResourceOrWait(ctx context.Context, name, resourceType string) (string, error) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()

	// In 99% of cases, we shouldn't hit this timeout. Instead, the context should be cancelled.
	// This is really to catch an unlikely case where the kubelet doesn't cancel the context.
	// Adding on top of the specified deadline ensures this deadline will be respected, regardless of
	// how Kubelet's runtime-request-timeout changes.
	resourceCreationWaitTime := time.Minute * 4
	if initialDeadline, ok := ctx.Deadline(); ok {
		resourceCreationWaitTime += time.Until(initialDeadline)
	}

	if cachedID := s.resourceStore.Get(name); cachedID != "" {
		log.Infof(ctx, "Found %s %s with ID %s in resource cache; using it", resourceType, name, cachedID)
		return cachedID, nil
	}
	watcher, stage := s.resourceStore.WatcherForResource(name)
	if watcher == nil {
		return "", fmt.Errorf("error attempting to watch for %s %s: no longer found", resourceType, name)
	}
	log.Infof(ctx, "Creation of %s %s not yet finished. Currently at stage %v. Waiting up to %v for it to finish", resourceType, name, stage, resourceCreationWaitTime)
	metrics.Instance().MetricResourcesStalledAtStage(stage)
	var err error
	select {
	// We should wait as long as we can (within reason), thus stalling the kubelet's sync loop.
	// This will prevent "name is reserved" errors popping up every two seconds.
	case <-ctx.Done():
		err = ctx.Err()
	// This is probably overly cautious, but it doesn't hurt to have a way to terminate
	// independent of the kubelet's signal.
	case <-time.After(resourceCreationWaitTime):
		err = fmt.Errorf("waited too long for request to timeout or %s %s to be created", resourceType, name)
	// If the resource becomes available while we're watching for it, we still need to error on this request.
	// When we pull the resource from the cache after waiting, we won't run the cleanup funcs.
	// However, we don't know how long we've been making the kubelet wait for the request, and the request could time out
	// after we stop paying attention. This would cause CRI-O to attempt to send back a resource that the kubelet
	// will not receive, causing a resource leak.
	case <-watcher:
		// We need to wait again here. If we error out to the Kubelet before it times out
		// it will bump the attempt number, nulllifying all of the work we've done so far.
		// Just the same as above, use resourceCreationWaitTime to make sure we catch cases where the context
		// is never done.
		select {
		case <-time.After(resourceCreationWaitTime):
		case <-ctx.Done():
		}
		err = fmt.Errorf("the requested %s %s is now ready and will be provided to the kubelet on next retry", resourceType, name)
	}

	return "", fmt.Errorf("kubelet may be retrying requests that are timing out in CRI-O due to system load. Currently at stage %v: %w", stage, err)
}

// FilterDisallowedAnnotations is a common place to have a map of annotations filtered for both runtimes and workloads.
// This function exists until the support for runtime level allowed annotations is dropped.
// toFind is used to find the workload for the specific pod or container, toFilter are the annotations
// for which disallowed annotations will be filtered. They may be the same.
// After this function, toFilter will no longer container disallowed annotations.
func (s *Server) FilterDisallowedAnnotations(toFind, toFilter map[string]string, runtimeHandler string) error {
	// Combine the two lists to create one. Both will ultimately end up filtering, and FilterDisallowedAnnotations
	// will handle duplicates, if any.
	// TODO: eventually, this should be in the container package, but it's going through a lot of churn
	// and SpecAddAnnotations is already passed too many arguments
	allowed, err := s.Runtime().AllowedAnnotations(runtimeHandler)
	if err != nil {
		return err
	}
	allowed = append(allowed, s.config.Workloads.AllowedAnnotations(toFind)...)

	return s.config.Workloads.FilterDisallowedAnnotations(allowed, toFilter)
}
