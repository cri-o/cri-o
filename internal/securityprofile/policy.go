package securityprofile

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/opencontainers/go-digest"
	"go.podman.io/image/v5/signature"
	imagetypes "go.podman.io/image/v5/types"
	"go.podman.io/storage/pkg/configfile"
)

// policyCache keeps the IDs of signature policies until one of the files they
// derive from changes, so that pulls and container creations do not read the
// policy and its keys every time.
type policyCache struct {
	mu      sync.Mutex
	entries map[string]*cachedPolicy
}

type cachedPolicy struct {
	id    string
	files map[string]fileStamp
}

// fileStamp identifies a version of a file.
type fileStamp struct {
	modTime time.Time
	size    int64
	exists  bool
}

func stampFile(path string) fileStamp {
	info, err := os.Stat(path)
	if err != nil {
		return fileStamp{}
	}

	return fileStamp{modTime: info.ModTime(), size: info.Size(), exists: true}
}

// id returns the ID of the signature policy of sys.
func (c *policyCache) id(sys *imagetypes.SystemContext) (string, error) {
	path := policyPath(sys)
	if path == "" {
		return policyID(sys)
	}

	c.mu.Lock()
	cached := c.entries[path]
	c.mu.Unlock()

	if cached != nil && !changed(cached.files) {
		return cached.id, nil
	}

	// The stamps are taken before the files are read, so that a change
	// while reading them makes the next call read them again.
	stamps := map[string]fileStamp{path: stampFile(path)}

	pathSys := &imagetypes.SystemContext{SignaturePolicyPath: path}

	if sys != nil {
		copied := *sys
		copied.SignaturePolicyPath = path
		pathSys = &copied
	}

	id, err := computePolicyID(pathSys, stamps)
	if err != nil {
		return "", err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.entries == nil {
		c.entries = map[string]*cachedPolicy{}
	}

	c.entries[path] = &cachedPolicy{id: id, files: stamps}

	return id, nil
}

func changed(files map[string]fileStamp) bool {
	for path, stamp := range files {
		if !stamp.exists || stampFile(path) != stamp {
			return true
		}
	}

	return false
}

// policyPath returns the policy file of sys, or empty if it cannot tell.
func policyPath(sys *imagetypes.SystemContext) string {
	if sys != nil && sys.SignaturePolicyPath != "" {
		return sys.SignaturePolicyPath
	}

	// The lookup of signature.DefaultPolicy.
	files := configfile.File{
		Name:                 "policy",
		Extension:            "json",
		DoNotLoadDropInFiles: true,
		EnvironmentName:      "CONTAINERS_POLICY_JSON",
		ErrorIfNotFound:      true,
	}

	if sys != nil {
		files.RootForImplicitAbsolutePaths = sys.RootForImplicitAbsolutePaths
	}

	for item, err := range configfile.Read(&files) {
		if err != nil {
			return ""
		}

		return item.Name
	}

	return ""
}

// policyID identifies the signature policy of sys by its content and by the
// content of the files it references, such as keys and certificates, so that
// a profile verified under a policy is verified again once a key rotates, and
// namespaces with the same policy share verified profiles.
func policyID(sys *imagetypes.SystemContext) (string, error) {
	return computePolicyID(sys, nil)
}

// computePolicyID returns the ID of the policy, stamping the files it
// references in stamps before reading them if it is not nil.
func computePolicyID(sys *imagetypes.SystemContext, stamps map[string]fileStamp) (string, error) {
	policy, err := signature.DefaultPolicy(sys)
	if err != nil {
		return "", fmt.Errorf("load signature policy: %w", err)
	}

	data, err := json.Marshal(policy)
	if err != nil {
		return "", fmt.Errorf("encode signature policy: %w", err)
	}

	var tree any
	if err := json.Unmarshal(data, &tree); err != nil {
		return "", fmt.Errorf("decode signature policy: %w", err)
	}

	h := sha256.New()
	h.Write(data)

	for _, file := range referencedFiles("", tree, nil) {
		if stamps != nil {
			stamps[file] = stampFile(file)
		}

		fmt.Fprintf(h, "\x00%s\x00", file)

		content, err := os.ReadFile(file)
		if err != nil {
			fmt.Fprintf(h, "error: %v", err)
		} else {
			h.Write(content)
		}
	}

	return digest.NewDigest(digest.SHA256, h).String(), nil
}

// referencedFiles returns the files the policy references, the values of
// members named like keyPath, caPath, keyPaths or rekorPublicKeyPaths, in a
// stable order.
func referencedFiles(key string, value any, files []string) []string {
	switch value := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(value))
		for k := range value {
			keys = append(keys, k)
		}

		slices.Sort(keys)

		for _, k := range keys {
			files = referencedFiles(k, value[k], files)
		}

	case []any:
		for _, item := range value {
			files = referencedFiles(key, item, files)
		}

	case string:
		if strings.HasSuffix(key, "Path") || strings.HasSuffix(key, "Paths") {
			files = append(files, value)
		}
	}

	return files
}
