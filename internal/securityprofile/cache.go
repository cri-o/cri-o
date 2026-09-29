package securityprofile

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/opencontainers/go-digest"
	ispec "github.com/opencontainers/image-spec/specs-go/v1"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"go.podman.io/image/v5/manifest"
	crierrors "k8s.io/cri-api/pkg/errors"

	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/server/metrics"
)

const (
	// indexFile names the index of the cache in its directory.
	indexFile = "profiles.json"

	// maxManifestSize bounds the manifest of a profile, which describes a
	// config and a layer in a few hundred bytes.
	maxManifestSize = 64 * 1024

	// blobGracePeriod protects blobs of a pull that did not commit yet from
	// the cleanup of blobs that no entry references.
	blobGracePeriod = 10 * time.Minute
)

// otherAlgorithms are the digest algorithms besides SHA-256 that references
// may pin a manifest with.
var otherAlgorithms = []digest.Algorithm{digest.SHA384, digest.SHA512}

// errStorage marks a profile that the cache cannot read back, which makes the
// profile absent rather than invalid.
var errStorage = errors.New("security profile storage")

// cache stores the pulled profiles: a directory of blobs addressed by their
// digests and an index. The index records the profiles, under which signature
// policies they were verified for which repositories, and when they were used
// last. For a profile that is invalid, it keeps only why. The profiles are
// parsed from digest verified, immutable blobs when they are added or loaded
// and kept in memory, so that looking one up reads nothing but a stat.
type cache struct {
	dir     string
	maxSize int64

	mu      sync.Mutex
	entries map[digest.Digest]*entry

	// pending counts the pulls that wrote a blob but did not commit it yet,
	// by blob path, so that no removal takes the blob away.
	pending map[string]int

	// beforeCommit runs between writing the blobs of a pull and committing
	// them, for tests.
	beforeCommit func()
}

// entry is a profile of the cache, keyed by the SHA-256 digest of its
// manifest. Apart from LastUsed, an entry is never changed once it is in the
// cache, only replaced.
type entry struct {
	Digest digest.Digest `json:"digest"`

	// Digests are the digests of the manifest with other algorithms.
	Digests []digest.Digest `json:"digests,omitempty"`

	// Verified maps repositories to the IDs of the signature policies the
	// profile was verified under when it was pulled from them.
	Verified map[string][]string `json:"verified,omitempty"`

	// LastUsed is the last time a pull or a merge used the profile.
	LastUsed time.Time `json:"lastUsed"`

	// Invalid is why the profile is invalid. Nothing else of such a
	// profile is kept.
	Invalid string `json:"invalid,omitempty"`

	// layer describes the profile layer, unless the profile is invalid.
	layer *ispec.Descriptor
	size  int64

	// profile is the validated profile, or err why it is invalid.
	profile *rspec.LinuxSeccomp
	err     error
}

type index struct {
	Profiles []*entry `json:"profiles"`
}

// openCache loads the cache in dir. An index that cannot be read resets the
// cache, which only costs pulling the profiles again.
func openCache(ctx context.Context, dir string, maxSize int64) (*cache, error) {
	c := &cache{
		dir:     dir,
		maxSize: maxSize,
		entries: map[digest.Digest]*entry{},
		pending: map[string]int{},
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create security profile directory: %w", err)
	}

	loaded, err := c.readIndex()
	changed := err != nil

	if err != nil {
		log.Warnf(ctx, "Resetting the security profile store: %v", err)

		if err := c.reset(); err != nil {
			return nil, err
		}
	}

	for _, e := range loaded {
		if err := c.load(e); err != nil {
			log.Warnf(ctx, "Dropping security profile %s from the store: %v", e.Digest, err)

			changed = true

			continue
		}

		c.entries[e.Digest] = e
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if changed {
		if err := c.commit(c.entries); err != nil {
			return nil, err
		}
	}

	c.publishMetrics()
	c.removeOrphanedBlobs()

	return c, nil
}

func (c *cache) readIndex() ([]*entry, error) {
	data, err := os.ReadFile(filepath.Join(c.dir, indexFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("read index: %w", err)
	}

	var idx index
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, fmt.Errorf("decode index: %w", err)
	}

	var loaded []*entry

	for _, e := range idx.Profiles {
		if e != nil && e.Digest.Validate() == nil {
			loaded = append(loaded, e)
		}
	}

	return loaded, nil
}

func (c *cache) reset() error {
	for _, name := range []string{indexFile, "blobs"} {
		if err := os.RemoveAll(filepath.Join(c.dir, name)); err != nil {
			return fmt.Errorf("reset security profile store: %w", err)
		}
	}

	return nil
}

// load reads and validates the profile of an entry from the index.
func (c *cache) load(e *entry) error {
	if e.Invalid != "" {
		e.err = fmt.Errorf("%w: %s", crierrors.ErrSecurityProfileInvalid, e.Invalid)

		return nil
	}

	raw, err := readFile(c.blobPath(e.Digest), e.Digest, maxManifestSize)
	if err != nil {
		return err
	}

	m, err := manifest.OCI1FromManifest(raw)
	if err != nil {
		return fmt.Errorf("%w: parse manifest: %w", errStorage, err)
	}

	// A profile the current configuration rejects is pulled again, which
	// rejects it for the right reason.
	if err := validateManifest(&m.Manifest, SeccompConfigMediaType, c.maxSize); err != nil {
		return fmt.Errorf("%w: %w", errStorage, err)
	}

	layer, err := readFile(c.blobPath(m.Layers[0].Digest), m.Layers[0].Digest, c.maxSize)
	if err != nil {
		return err
	}

	e.set(raw, &m.Manifest, layer)

	if e.err != nil {
		return fmt.Errorf("%w: %w", errStorage, e.err)
	}

	return nil
}

// set fills in what an entry holds in memory.
func (e *entry) set(raw []byte, m *ispec.Manifest, layer []byte) {
	e.Digests = nil
	for _, algorithm := range otherAlgorithms {
		e.Digests = append(e.Digests, algorithm.FromBytes(raw))
	}

	e.layer = &m.Layers[0]
	e.size = int64(len(raw)) + m.Layers[0].Size
	e.profile, e.err = parseSeccomp(layer)

	// Only the reason is kept of an invalid profile.
	if e.err != nil {
		e.Invalid = strings.TrimPrefix(
			e.err.Error(), crierrors.ErrSecurityProfileInvalid.Error()+": ",
		)
		e.layer = nil
		e.size = 0
	}
}

func (c *cache) blobPath(dgst digest.Digest) string {
	return filepath.Join(c.dir, "blobs", dgst.Algorithm().String(), dgst.Encoded())
}

// blobPaths returns the paths of the blobs of the entries.
func (c *cache) blobPaths(entries map[digest.Digest]*entry) map[string]struct{} {
	paths := map[string]struct{}{}

	for _, e := range entries {
		if e.layer != nil {
			paths[c.blobPath(e.Digest)] = struct{}{}
			paths[c.blobPath(e.layer.Digest)] = struct{}{}
		}
	}

	return paths
}

// readFile reads a regular file of at most limit bytes and verifies it
// against its digest. It does not block on anything but a regular file.
func readFile(path string, dgst digest.Digest, limit int64) ([]byte, error) {
	if err := dgst.Validate(); err != nil {
		return nil, fmt.Errorf("%w: invalid digest: %w", errStorage, err)
	}

	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errStorage, err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errStorage, err)
	}

	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: %s is not a regular file", errStorage, path)
	}

	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, fmt.Errorf("%w: read %s: %w", errStorage, path, err)
	}

	if int64(len(data)) > limit || dgst.Algorithm().FromBytes(data) != dgst {
		return nil, fmt.Errorf("%w: %s does not match its digest", errStorage, path)
	}

	return data, nil
}

// writeFile writes a file durably: through a synced temporary file, which the
// rename makes appear complete, and a synced directory.
func writeFile(path string, data []byte) error {
	dir := filepath.Dir(path)

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".tmp-")
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}

	_, err = tmp.Write(data)
	if err == nil {
		err = tmp.Sync()
	}

	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}

	if err == nil {
		err = os.Rename(tmp.Name(), path)
	}

	if err != nil {
		os.Remove(tmp.Name())

		return fmt.Errorf("write %s: %w", path, err)
	}

	d, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("sync %s: %w", dir, err)
	}
	defer d.Close()

	if err := d.Sync(); err != nil {
		return fmt.Errorf("sync %s: %w", dir, err)
	}

	return nil
}

// lookup returns the entry with the provided manifest digest, of any
// algorithm, or nil. An entry whose blobs went missing is dropped, so that
// the profile is pulled again.
func (c *cache) lookup(ctx context.Context, dgst digest.Digest) *entry {
	c.mu.Lock()
	defer c.mu.Unlock()

	e := c.entries[dgst]
	if e == nil && dgst.Algorithm() != digest.SHA256 {
		for _, candidate := range c.entries {
			if slices.Contains(candidate.Digests, dgst) {
				e = candidate

				break
			}
		}
	}

	if e == nil {
		return nil
	}

	if err := c.stat(e); err != nil {
		log.Warnf(ctx, "Dropping security profile %s from the store: %v", e.Digest, err)

		remaining := maps.Clone(c.entries)
		delete(remaining, e.Digest)

		if err := c.commit(remaining); err != nil {
			log.Warnf(ctx, "Unable to update the security profile store: %v", err)
		}

		return nil
	}

	return e
}

// stat checks that the blobs of an entry are still on the disk.
func (c *cache) stat(e *entry) error {
	if e.layer == nil {
		return nil
	}

	for _, blob := range []ispec.Descriptor{{Digest: e.Digest, Size: -1}, *e.layer} {
		info, err := os.Stat(c.blobPath(blob.Digest))
		if err != nil {
			return fmt.Errorf("%w: %w", errStorage, err)
		}

		if blob.Size >= 0 && info.Size() != blob.Size {
			return fmt.Errorf("%w: blob %s changed its size", errStorage, blob.Digest)
		}
	}

	return nil
}

// verified returns true if the profile was verified for the repository under
// the signature policy.
func (c *cache) verified(e *entry, repository, policy string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	return slices.Contains(e.Verified[repository], policy)
}

// touch records that the profile was used.
func (c *cache) touch(e *entry) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if current := c.entries[e.Digest]; current != nil {
		current.LastUsed = time.Now()
	}
}

// add stores a pulled profile, or only why it is invalid. policy is the
// signature policy it was verified under for the repository, or empty if
// that is unknown.
func (c *cache) add(raw []byte, m *ispec.Manifest, layer []byte, repository, policy string) error {
	e := &entry{Digest: digest.FromBytes(raw), Verified: map[string][]string{}}
	e.set(raw, m, layer)

	var blobs map[string][]byte
	if e.layer != nil {
		blobs = map[string][]byte{
			c.blobPath(e.Digest):       raw,
			c.blobPath(e.layer.Digest): layer,
		}
	}

	c.mu.Lock()
	for path := range blobs {
		c.pending[path]++
	}
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		defer c.mu.Unlock()

		for path := range blobs {
			c.pending[path]--
			if c.pending[path] == 0 {
				delete(c.pending, path)
			}
		}
	}()

	// The blobs go to the disk before the index references them.
	for path, data := range blobs {
		if err := writeFile(path, data); err != nil {
			c.removeUncommitted(blobs)

			return err
		}
	}

	if c.beforeCommit != nil {
		c.beforeCommit()
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if current := c.entries[e.Digest]; current != nil {
		for repo, policies := range current.Verified {
			e.Verified[repo] = slices.Clone(policies)
		}
	}

	if policy != "" && !slices.Contains(e.Verified[repository], policy) {
		e.Verified[repository] = append(e.Verified[repository], policy)
	}

	e.LastUsed = time.Now()

	entries := maps.Clone(c.entries)
	entries[e.Digest] = e

	if err := c.commit(entries); err != nil {
		c.removeUncommittedLocked(blobs)

		return err
	}

	return nil
}

// removeUncommitted removes blobs of a failed add that no entry and no other
// pull needs.
func (c *cache) removeUncommitted(blobs map[string][]byte) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.removeUncommittedLocked(blobs)
}

func (c *cache) removeUncommittedLocked(blobs map[string][]byte) {
	referenced := c.blobPaths(c.entries)

	for path := range blobs {
		if _, ok := referenced[path]; !ok && c.pending[path] <= 1 {
			os.Remove(path)
		}
	}
}

// gc removes the profiles that are not referenced and were not used for the
// provided duration.
func (c *cache) gc(ctx context.Context, referenced func(*entry) bool, unused time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entries := maps.Clone(c.entries)

	for dgst, e := range entries {
		if !referenced(e) && time.Since(e.LastUsed) > unused {
			log.Infof(ctx, "Removing security profile %s, which was not used for %s", dgst, unused)
			delete(entries, dgst)
		}
	}

	if len(entries) == len(c.entries) {
		return
	}

	if err := c.commit(entries); err != nil {
		log.Warnf(ctx, "Unable to remove security profiles: %v", err)
	}
}

// commit writes the index for entries, replaces the entries in memory once
// that succeeded, and removes the blobs no entry and no pending pull
// references anymore. It has to be called with c.mu held.
func (c *cache) commit(entries map[digest.Digest]*entry) error {
	idx := index{Profiles: slices.SortedFunc(maps.Values(entries), func(a, b *entry) int {
		return strings.Compare(a.Digest.String(), b.Digest.String())
	})}

	data, err := json.Marshal(idx)
	if err != nil {
		return fmt.Errorf("encode security profile index: %w", err)
	}

	if err := writeFile(filepath.Join(c.dir, indexFile), data); err != nil {
		return fmt.Errorf("write security profile index: %w", err)
	}

	// A blob that is not removed now is removed on the next start.
	remaining := c.blobPaths(entries)
	for path := range c.blobPaths(c.entries) {
		if _, ok := remaining[path]; !ok && c.pending[path] == 0 {
			os.Remove(path)
		}
	}

	c.entries = entries
	c.publishMetrics()

	return nil
}

// publishMetrics sets the metrics of the store. It has to be called with c.mu
// held, so that the metrics follow the order of the changes.
func (c *cache) publishMetrics() {
	var size int64
	for _, e := range c.entries {
		size += e.size
	}

	metrics.Instance().MetricSecurityProfilesStoredSet(len(c.entries), size)
}

// removeOrphanedBlobs removes the blobs no entry references, which a crash
// may leave behind. Blobs that a concurrent pull wrote but not committed yet
// are pending or recent, so only old ones are removed.
func (c *cache) removeOrphanedBlobs() {
	referenced := c.blobPaths(c.entries)

	//nolint:errcheck // A blob that is not removed now is removed later.
	filepath.WalkDir(
		filepath.Join(c.dir, "blobs"),
		func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}

			if _, ok := referenced[path]; ok || c.pending[path] > 0 {
				return nil
			}

			if info, err := d.Info(); err == nil && time.Since(info.ModTime()) > blobGracePeriod {
				os.Remove(path)
			}

			return nil
		},
	)
}
