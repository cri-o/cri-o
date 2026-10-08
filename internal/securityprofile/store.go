package securityprofile

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/opencontainers/go-digest"
	ispec "github.com/opencontainers/image-spec/specs-go/v1"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
	"go.podman.io/image/v5/docker"
	"go.podman.io/image/v5/docker/reference"
	"go.podman.io/image/v5/image"
	"go.podman.io/image/v5/pkg/blobinfocache/none"
	"go.podman.io/image/v5/signature"
	imagetypes "go.podman.io/image/v5/types"
	"golang.org/x/sync/singleflight"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
	crierrors "k8s.io/cri-api/pkg/errors"
	"sigs.k8s.io/security-profiles-merger/seccomp"

	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/internal/ociartifact"
	"github.com/cri-o/cri-o/server/metrics"
)

// MergeConstrainedEvent identifies the log entry written when the merge with
// the node-local profiles takes permissions away from an OCI profile.
const MergeConstrainedEvent = "SecurityProfileMergeConstrained"

// pullTimeout bounds a pull shared by concurrent callers, which no single
// caller may cancel.
const pullTimeout = 2 * time.Minute

// AdditionalStores provides the read-only additional artifact stores.
type AdditionalStores interface {
	ListAdditional(ctx context.Context) []*ociartifact.Artifact
	ReadBlob(artifact *ociartifact.Artifact, dgst digest.Digest, limit int64) ([]byte, error)
}

// Options configure a Store.
type Options struct {
	// Dir holds the pulled profiles.
	Dir string

	// MaxSize bounds the size of a profile layer in bytes.
	MaxSize int64

	// Additional provides the additional artifact stores. The node
	// administrator fills them, so their profiles are trusted like
	// Localhost profiles: they are neither pulled nor checked against a
	// signature policy. They are validated like pulled profiles, and the
	// result for their digest verified, immutable blobs is cached.
	Additional AdditionalStores

	// SystemContext returns the system context of a namespace, which
	// selects its signature policy.
	SystemContext func(namespace string) (*imagetypes.SystemContext, error)
}

// Auth provides credentials that are only needed for a pull.
type Auth struct {
	// Scope identifies the credentials, such as the namespace they belong to.
	Scope string

	// Prepare adds the credentials to the system context of a pull and
	// returns a function that cleans them up.
	Prepare func(*imagetypes.SystemContext) (cleanup func(), err error)

	// Discard cleans the credentials up when no pull needs them.
	Discard func()
}

func (a *Auth) discard() {
	if a != nil && a.Discard != nil {
		a.Discard()
	}
}

// Store pulls security profiles and provides the validated profiles to the
// sandbox and container setup.
type Store struct {
	opts  Options
	cache *cache

	// err is why the store cannot be used, which makes every call fail.
	err error

	policies   policyCache
	openSource func(context.Context, reference.Named, *imagetypes.SystemContext) (imagetypes.ImageSource, error)
	pulls      singleflight.Group

	// joined runs once a caller joined a pull, for tests.
	joined func()

	// beforeRecord runs before a pull records its reference, for tests.
	beforeRecord func()

	trustedMu sync.Mutex
	trusted   map[digest.Digest]*trustedProfile
}

// trustedProfile is a profile of an additional store.
type trustedProfile struct {
	// paths are the blobs of the profile, which disappear with it.
	paths   []string
	profile *rspec.LinuxSeccomp
	err     error
}

// New creates a profile store, loading the profiles pulled before. A store
// that cannot be opened fails every call, but does not keep CRI-O from
// starting.
func New(ctx context.Context, opts *Options) *Store {
	s := &Store{
		opts:       *opts,
		openSource: openSource,
		trusted:    map[digest.Digest]*trustedProfile{},
	}

	s.cache, s.err = openCache(ctx, opts.Dir, opts.MaxSize)
	if s.err != nil {
		log.Errorf(ctx, "Security profiles are unavailable: %v", s.err)
	}

	return s
}

// Available returns true if the store can be used.
func (s *Store) Available() bool {
	return s.err == nil
}

func (s *Store) unavailable() error {
	return fmt.Errorf("security profile store unavailable: %w", s.err)
}

func openSource(
	ctx context.Context,
	ref reference.Named,
	sys *imagetypes.SystemContext,
) (imagetypes.ImageSource, error) {
	imageRef, err := docker.NewReference(ref)
	if err != nil {
		return nil, err
	}

	return imageRef.NewImageSource(ctx, sys)
}

// Pull makes the profile at ref available to the sandbox and container setup
// and reports whether it was already present. Every call reports the result
// of validating the profile, which the store derives from digest verified,
// immutable blobs and caches. A present profile is pulled again only if it
// was not verified for the repository of ref under the signature policy of
// sys, since the store keeps no signatures to check again: signature
// policies are scoped by repository, so a profile pulled from a new
// repository is verified once against the registry even if the policy is the
// same. Profiles of the additional stores are never pulled. Every pull that
// stores or finds a pulled profile records ref, as the kubelet matches the
// references of pods with it. auth is optional.
func (s *Store) Pull(
	ctx context.Context,
	ref string,
	kind types.SecurityProfileKind,
	sys *imagetypes.SystemContext,
	auth *Auth,
) (cached bool, err error) {
	mediaType, err := mediaTypeForKind(kind)
	if err != nil {
		return false, err
	}

	canonical, err := parseReference(ref)
	if err != nil {
		return false, err
	}

	if s.err != nil {
		return false, s.unavailable()
	}

	policy, err := s.policies.id(sys)
	if err != nil {
		return false, err
	}

	if e := s.cache.lookup(ctx, canonical.Digest()); e != nil {
		// The digest pins the content, so pulling it again cannot make an
		// invalid profile valid.
		if e.err != nil {
			auth.discard()

			return true, s.recordRef(ctx, e, ref)
		}

		if s.cache.verified(e, canonical.Name(), policy) {
			log.Debugf(ctx, "Security profile %s is already present", ref)
			auth.discard()

			if err := s.recordRef(ctx, e, ref); err != nil {
				return false, err
			}

			return true, nil
		}

		log.Infof(
			ctx,
			"Security profile %s was not verified under the signature policy of this pull",
			ref,
		)
	}

	if t := s.findTrusted(ctx, canonical.Digest()); t != nil {
		log.Debugf(ctx, "Security profile %s is present in an additional store", ref)
		auth.discard()

		return true, t.err
	}

	// containers/image verifies manifests against SHA-256 digests only, so
	// it cannot pull one pinned otherwise, nor check its signatures.
	if canonical.Digest().Algorithm() != digest.SHA256 {
		auth.discard()

		return false, invalidRequestf(
			"security profile reference %q uses the unsupported digest algorithm %s, "+
				"only sha256 references can be pulled",
			ref, canonical.Digest().Algorithm(),
		)
	}

	if err := s.sharedPull(ctx, canonical, ref, mediaType, policy, sys, auth); err != nil {
		return false, err
	}

	e := s.cache.lookup(ctx, canonical.Digest())
	if e == nil {
		return false, fmt.Errorf("pulled security profile %s is not in the store", ref)
	}

	// A caller that joined the pull of another one records its own ref.
	return false, s.recordRef(ctx, e, ref)
}

// recordRef records ref for the profile of e and returns the outcome of the
// pull: an error if ref cannot be recorded, so that every successful pull is
// recorded, or why the profile is invalid.
func (s *Store) recordRef(ctx context.Context, e *entry, ref string) error {
	if s.beforeRecord != nil {
		s.beforeRecord()
	}

	err := s.cache.recordRef(e.Digest, ref)
	if e.err == nil {
		return err
	}

	if err != nil {
		log.Warnf(ctx, "Unable to record the reference of security profile %s: %v", ref, err)
	}

	return e.err
}

// sharedPull pulls once for concurrent callers with the same request. The
// pull, including recording the verification, outlives a caller that gives
// up, so that it neither fails the others nor is lost.
func (s *Store) sharedPull(
	ctx context.Context,
	ref reference.Canonical,
	requested, mediaType, policy string,
	sys *imagetypes.SystemContext,
	auth *Auth,
) error {
	key := pullKey(ref, mediaType, policy, sys, auth)

	results := s.pulls.DoChan(key, func() (any, error) {
		pullCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), pullTimeout)
		defer cancel()

		pullSys := *sys

		if auth != nil && auth.Prepare != nil {
			cleanup, err := auth.Prepare(&pullSys)
			if err != nil {
				return nil, err
			}
			defer cleanup()
		}

		p, err := s.fetch(pullCtx, ref, mediaType, &pullSys)
		if err != nil {
			return nil, err
		}

		// A policy or key that changed during the pull may not be the one
		// the pull was verified under.
		if after, err := s.policies.id(sys); err != nil || after != policy {
			// The pull did not succeed, so it records no reference.
			if err := s.cache.add(p, ref.Name(), "", ""); err != nil {
				return nil, err
			}

			return nil, fmt.Errorf(
				"the signature policy changed while pulling security profile %s", ref,
			)
		}

		return nil, s.cache.add(p, ref.Name(), requested, policy)
	})

	if s.joined != nil {
		s.joined()
	}

	select {
	case <-ctx.Done():
		return ctx.Err()

	case result := <-results:
		return result.Err
	}
}

// pullKey identifies a pull by everything that decides its outcome. The
// credentials are part of it, so that a pull never uses those of another pod.
func pullKey(
	ref reference.Named,
	mediaType, policy string,
	sys *imagetypes.SystemContext,
	auth *Auth,
) string {
	key := ref.String() + "\x00" + mediaType + "\x00" + policy + "\x00" + sys.AuthFilePath

	if config := sys.DockerAuthConfig; config != nil {
		key += "\x00" + config.Username + "\x00" + config.Password + "\x00" + config.IdentityToken
	}

	if auth != nil {
		key += "\x00" + auth.Scope
	}

	return key
}

// pulled is the content of a pulled profile. The config is not, because
// nothing but its media type is used.
type pulled struct {
	raw      []byte
	manifest *ispec.Manifest
	layer    []byte
}

// fetch downloads a profile after checking its manifest, so that nothing is
// downloaded for an artifact that is not a profile or too large, and verifies
// it under the signature policy of sys.
func (s *Store) fetch(
	ctx context.Context,
	ref reference.Canonical,
	mediaType string,
	sys *imagetypes.SystemContext,
) (*pulled, error) {
	log.Infof(ctx, "Pulling security profile %s from the registry", ref)

	src, err := s.openSource(ctx, ref, sys)
	if err != nil {
		return nil, classifyPullError(fmt.Errorf("open %s: %w", ref, err))
	}
	defer src.Close()

	// The unparsed image verifies the manifest against the digest of ref.
	unparsed := image.UnparsedInstance(src, nil)

	raw, mimeType, err := unparsed.Manifest(ctx)
	if err != nil {
		return nil, classifyPullError(fmt.Errorf("fetch manifest: %w", err))
	}

	if len(raw) > maxManifestSize {
		return nil, invalidf(
			"manifest of %d bytes exceeds the maximum of %d bytes",
			len(raw),
			maxManifestSize,
		)
	}

	m, err := parseManifest(raw, mimeType)
	if err != nil {
		return nil, err
	}

	if err := validateManifest(m, mediaType, s.opts.MaxSize); err != nil {
		return nil, err
	}

	if err := checkPolicy(ctx, sys, unparsed); err != nil {
		return nil, classifyPullError(err)
	}

	layer, err := fetchBlob(ctx, src, &m.Layers[0])
	if err != nil {
		return nil, err
	}

	return &pulled{raw: raw, manifest: m, layer: layer}, nil
}

// checkPolicy evaluates the signature policy of sys for the profile.
func checkPolicy(
	ctx context.Context,
	sys *imagetypes.SystemContext,
	unparsed imagetypes.UnparsedImage,
) error {
	policy, err := signature.DefaultPolicy(sys)
	if err != nil {
		return fmt.Errorf("load signature policy: %w", err)
	}

	policyContext, err := signature.NewPolicyContext(policy)
	if err != nil {
		return fmt.Errorf("create signature policy context: %w", err)
	}

	defer func() {
		if err := policyContext.Destroy(); err != nil {
			log.Warnf(ctx, "Unable to destroy signature policy context: %v", err)
		}
	}()

	if _, err := policyContext.IsRunningImageAllowed(ctx, unparsed); err != nil {
		return err
	}

	return nil
}

// fetchBlob downloads a blob of a validated manifest. The descriptor bounds
// the size already, so a blob that does not match it is a fault of the
// registry, not of the profile.
func fetchBlob(
	ctx context.Context,
	src imagetypes.ImageSource,
	desc *ispec.Descriptor,
) ([]byte, error) {
	if err := desc.Digest.Validate(); err != nil {
		return nil, invalidf("blob digest %q: %v", desc.Digest, err)
	}

	blob, _, err := src.GetBlob(
		ctx, imagetypes.BlobInfo{Digest: desc.Digest, Size: desc.Size}, none.NoCache,
	)
	if err != nil {
		return nil, classifyPullError(fmt.Errorf("fetch blob %s: %w", desc.Digest, err))
	}
	defer blob.Close()

	data, err := io.ReadAll(io.LimitReader(blob, desc.Size+1))
	if err != nil {
		return nil, classifyPullError(fmt.Errorf("fetch blob %s: %w", desc.Digest, err))
	}

	if int64(len(data)) != desc.Size || desc.Digest.Algorithm().FromBytes(data) != desc.Digest {
		return nil, fmt.Errorf(
			"%w: blob %s does not match its descriptor",
			crierrors.ErrRegistryUnavailable,
			desc.Digest,
		)
	}

	return data, nil
}

// findTrusted returns the profile with the provided manifest digest from the
// additional stores, or nil. A match that cannot be read is skipped.
func (s *Store) findTrusted(ctx context.Context, dgst digest.Digest) *trustedProfile {
	if s.opts.Additional == nil {
		return nil
	}

	s.trustedMu.Lock()
	defer s.trustedMu.Unlock()

	if t := s.trusted[dgst]; t != nil {
		present := true

		for _, path := range t.paths {
			if _, err := os.Stat(path); err != nil {
				present = false
			}
		}

		if present {
			return t
		}

		delete(s.trusted, dgst)
	}

	for _, artifact := range s.opts.Additional.ListAdditional(ctx) {
		if !s.matches(artifact, dgst) {
			continue
		}

		t, err := s.loadTrusted(artifact)
		if err != nil {
			log.Warnf(ctx, "Skipping security profile %s of an additional store: %v", dgst, err)

			continue
		}

		s.trusted[dgst] = t

		return t
	}

	return nil
}

// matches returns true if the manifest of the artifact has the digest. The
// store digests manifests with SHA-256, so another algorithm needs the
// manifest.
func (s *Store) matches(artifact *ociartifact.Artifact, dgst digest.Digest) bool {
	if artifact.Digest() == dgst {
		return true
	}

	if dgst.Algorithm() == digest.SHA256 || !dgst.Algorithm().Available() {
		return false
	}

	raw, err := s.opts.Additional.ReadBlob(artifact, artifact.Digest(), maxManifestSize)

	return err == nil && dgst.Algorithm().FromBytes(raw) == dgst
}

func (s *Store) loadTrusted(artifact *ociartifact.Artifact) (*trustedProfile, error) {
	if artifact.Manifest == nil {
		return nil, errors.New("artifact without a manifest")
	}

	m := &artifact.Manifest.Manifest

	manifestPath, err := ociartifact.BlobPath(artifact, artifact.Digest())
	if err != nil {
		return nil, err
	}

	t := &trustedProfile{paths: []string{manifestPath}}

	if t.err = validateManifest(m, SeccompConfigMediaType, s.opts.MaxSize); t.err != nil {
		return t, nil
	}

	layerPath, err := ociartifact.BlobPath(artifact, m.Layers[0].Digest)
	if err != nil {
		return nil, err
	}

	data, err := s.opts.Additional.ReadBlob(artifact, m.Layers[0].Digest, s.opts.MaxSize)
	if err != nil {
		return nil, err
	}

	t.paths = append(t.paths, layerPath)
	t.profile, t.err = parseSeccomp(data)

	return t, nil
}

// List returns the pulled profiles. Profiles of the additional stores are not
// pulled, so they are not listed.
func (s *Store) List() ([]*types.SecurityProfileInfo, error) {
	if s.err != nil {
		return nil, s.unavailable()
	}

	infos := s.cache.list()
	profiles := make([]*types.SecurityProfileInfo, 0, len(infos))

	for _, info := range infos {
		profiles = append(profiles, &types.SecurityProfileInfo{
			Digest: info.digest.String(),
			Refs:   info.refs,
			Size:   uint64(max(info.size, 0)),
		})
	}

	return profiles, nil
}

// Remove removes the pulled profile with the manifest digest dgst, of any
// algorithm. A profile that is not present is not an error. Sandboxes and
// containers that use the profile keep it, and a pull that runs concurrently
// adds it again. Profiles of the additional stores are never removed.
func (s *Store) Remove(ctx context.Context, dgst string) error {
	parsed, err := digest.Parse(dgst)
	if err != nil {
		return status.Errorf(
			codes.InvalidArgument, "invalid security profile digest %q: %v", dgst, err,
		)
	}

	if s.err != nil {
		return s.unavailable()
	}

	removed, err := s.cache.remove(parsed)
	if err != nil {
		return err
	}

	if removed {
		log.Infof(ctx, "Removed security profile %s", parsed)
	}

	return nil
}

// For returns the profiles for a sandbox or container of the namespace. The
// name identifies it in the log.
func (s *Store) For(namespace, name string) *Profiles {
	return &Profiles{store: s, namespace: namespace, name: name}
}

// Profiles provides the profiles to a sandbox or container.
type Profiles struct {
	store     *Store
	namespace string
	name      string
}

// seccomp returns the validated seccomp profile pulled for ref. A profile
// that is not present is an error, the kubelet pulls it on the next sync. So
// is one that was not verified under the signature policy of the namespace,
// unless an additional store provides it.
func (s *Store) seccomp(ctx context.Context, ref, namespace string) (*rspec.LinuxSeccomp, error) {
	canonical, err := parseReference(ref)
	if err != nil {
		return nil, err
	}

	if s.err != nil {
		return nil, s.unavailable()
	}

	e := s.cache.lookup(ctx, canonical.Digest())
	if e != nil {
		if e.err != nil {
			return nil, e.err
		}

		sys, err := s.opts.SystemContext(namespace)
		if err != nil {
			return nil, err
		}

		policy, err := s.policies.id(sys)
		if err != nil {
			return nil, err
		}

		if s.cache.verified(e, canonical.Name(), policy) {
			return e.profile, nil
		}
	}

	if t := s.findTrusted(ctx, canonical.Digest()); t != nil {
		return t.profile, t.err
	}

	if e != nil {
		return nil, status.Errorf(
			codes.FailedPrecondition,
			"security profile %s was not verified under the signature policy of namespace %q",
			ref, namespace,
		)
	}

	return nil, fmt.Errorf("security profile %s has not been pulled", ref)
}

// MergeSeccomp intersects the seccomp profile pulled for ref with the baseline
// and the base profile, in this order from most to least trusted. Either is
// nil if it does not restrict anything. The result is what gets loaded, never
// the pulled profile itself.
func (p *Profiles) MergeSeccomp(
	ctx context.Context,
	ref string,
	baseline, base *rspec.LinuxSeccomp,
) (*rspec.LinuxSeccomp, error) {
	artifact, err := p.store.seccomp(ctx, ref, p.namespace)
	if err != nil {
		return nil, err
	}

	var (
		inputs []*rspec.LinuxSeccomp
		names  []string
	)

	for _, input := range []struct {
		name    string
		profile *rspec.LinuxSeccomp
	}{
		{"baseline", baseline},
		{"base profile", base},
		{"OCI profile " + ref, artifact},
	} {
		if input.profile != nil {
			inputs = append(inputs, input.profile)
			names = append(names, input.name)
		}
	}

	effective, err := seccomp.Intersect(inputs...)
	if err != nil {
		var inputErr *seccomp.InputError
		if errors.As(err, &inputErr) && inputErr.Index >= 0 && inputErr.Index < len(names) {
			return nil, fmt.Errorf(
				"merge seccomp profiles: invalid %s: %w", names[inputErr.Index], err,
			)
		}

		return nil, fmt.Errorf("merge seccomp profile %s: %w", ref, err)
	}

	if baseline != nil || base != nil {
		reportConstrained(ctx, p.name, ref, baseline, base, artifact, effective)
	}

	return effective, nil
}

// reportConstrained logs what the merge took away from the pulled profile and
// which node-local profile did it, as the only signal a user gets in alpha.
func reportConstrained(
	ctx context.Context,
	name, ref string,
	baseline, base, artifact, effective *rspec.LinuxSeccomp,
) {
	diff, err := seccomp.Diff(artifact, effective)
	if err != nil {
		log.Warnf(ctx, "Unable to compare the merged seccomp profile %s: %v", ref, err)

		return
	}

	if diff.Equal {
		return
	}

	metrics.Instance().MetricSecurityProfileMergesConstrainedInc()

	fields := map[string]any{"profile": ref}
	if name != "" {
		fields["container"] = name
	}

	switch {
	case baseline != nil && base == nil:
		fields["baseline"] = formatDiff(diff)

	case baseline == nil && base != nil:
		fields["baseProfile"] = formatDiff(diff)

	case !splitConstraints(fields, baseline, artifact, effective):
		// The merge also settles rules and architectures on its own.
		fields["constrained"] = formatDiff(diff)
	}

	log.WithFields(ctx, fields).Warnf(
		"%s: the node-local seccomp profiles constrained the OCI profile", MergeConstrainedEvent,
	)

	if logrus.IsLevelEnabled(logrus.DebugLevel) {
		log.Debugf(
			ctx,
			"Complete difference of the merged seccomp profile %s: %s",
			ref,
			seccomp.FormatDiff(diff),
		)
	}
}

// maxDiffLength bounds a difference in the warning, since merging a profile
// that allows by default lists every syscall the baseline denies.
const maxDiffLength = 2048

func formatDiff(diff *seccomp.ProfileDiff) string {
	text := seccomp.FormatDiff(diff)
	if len(text) <= maxDiffLength {
		return text
	}

	return fmt.Sprintf(
		"%s... (%d more bytes, see the debug log)",
		text[:maxDiffLength],
		len(text)-maxDiffLength,
	)
}

// splitConstraints adds to fields what the baseline and what the base profile
// took away from the pulled profile. It returns false if it cannot tell them
// apart.
func splitConstraints(
	fields map[string]any,
	baseline, artifact, effective *rspec.LinuxSeccomp,
) bool {
	byBaseline, err := seccomp.Intersect(baseline, artifact)
	if err != nil {
		return false
	}

	baselineDiff, err := seccomp.Diff(artifact, byBaseline)
	if err != nil {
		return false
	}

	baseDiff, err := seccomp.Diff(byBaseline, effective)
	if err != nil {
		return false
	}

	if !baselineDiff.Equal {
		fields["baseline"] = formatDiff(baselineDiff)
	}

	if !baseDiff.Equal {
		fields["baseProfile"] = formatDiff(baseDiff)
	}

	return !baselineDiff.Equal || !baseDiff.Equal
}
