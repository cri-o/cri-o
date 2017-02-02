package policyconfiguration

import (
	"strings"
	"testing"

	"fmt"

	"github.com/containers/image/docker/reference"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDockerReference tests DockerReferenceIdentity and DockerReferenceNamespaces simulatenously
// to ensure they are consistent.
func TestDockerReference(t *testing.T) {
	sha256Digest := "@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	// Test both that DockerReferenceIdentity returns the expected value (fullName+suffix),
	// and that DockerReferenceNamespaces starts with the expected value (fullName), i.e. that the two functions are
	// consistent.
	for inputName, expectedNS := range map[string][]string{
		"example.com/ns/repo": {"example.com/ns/repo", "example.com/ns", "example.com"},
		"example.com/repo":    {"example.com/repo", "example.com"},
		"localhost/ns/repo":   {"localhost/ns/repo", "localhost/ns", "localhost"},
		// Note that "localhost" is special here: notlocalhost/repo is parsed as docker.io/notlocalhost.repo:
		"localhost/repo":         {"localhost/repo", "localhost"},
		"notlocalhost/repo":      {"docker.io/notlocalhost/repo", "docker.io/notlocalhost", "docker.io"},
		"docker.io/ns/repo":      {"docker.io/ns/repo", "docker.io/ns", "docker.io"},
		"docker.io/library/repo": {"docker.io/library/repo", "docker.io/library", "docker.io"},
		"docker.io/repo":         {"docker.io/library/repo", "docker.io/library", "docker.io"},
		"ns/repo":                {"docker.io/ns/repo", "docker.io/ns", "docker.io"},
		"library/repo":           {"docker.io/library/repo", "docker.io/library", "docker.io"},
		"repo":                   {"docker.io/library/repo", "docker.io/library", "docker.io"},
	} {
		for inputSuffix, mappedSuffix := range map[string]string{
			":tag":       ":tag",
			sha256Digest: sha256Digest,
			// A github.com/distribution/reference value can have a tag and a digest at the same time!
			// github.com/docker/reference handles that by dropping the tag. That is not obviously the
			// right thing to do, but it is at least reasonable, so test that we keep behaving reasonably.
			// This test case should not be construed to make this an API promise.
			":tag" + sha256Digest: sha256Digest,
		} {
			fullInput := inputName + inputSuffix
			ref, err := reference.ParseNamed(fullInput)
			require.NoError(t, err, fullInput)

			identity, err := DockerReferenceIdentity(ref)
			require.NoError(t, err, fullInput)
			assert.Equal(t, expectedNS[0]+mappedSuffix, identity, fullInput)

			ns := DockerReferenceNamespaces(ref)
			require.NotNil(t, ns, fullInput)
			require.Len(t, ns, len(expectedNS), fullInput)
			moreSpecific := identity
			for i := range expectedNS {
				assert.Equal(t, ns[i], expectedNS[i], fmt.Sprintf("%s item %d", fullInput, i))
				assert.True(t, strings.HasPrefix(moreSpecific, ns[i]))
				moreSpecific = ns[i]
			}
		}
	}
}

// refWithTagAndDigest is a reference.NamedTagged and reference.Canonical at the same time.
type refWithTagAndDigest struct{ reference.Canonical }

func (ref refWithTagAndDigest) Tag() string {
	return "notLatest"
}

func TestDockerReferenceIdentity(t *testing.T) {
	// TestDockerReference above has tested the core of the functionality, this tests only the failure cases.

	// Neither a tag nor digest
	parsed, err := reference.ParseNamed("busybox")
	require.NoError(t, err)
	id, err := DockerReferenceIdentity(parsed)
	assert.Equal(t, "", id)
	assert.Error(t, err)

	// A github.com/distribution/reference value can have a tag and a digest at the same time!
	parsed, err = reference.ParseNamed("busybox@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	require.NoError(t, err)
	refDigested, ok := parsed.(reference.Canonical)
	require.True(t, ok)
	tagDigestRef := refWithTagAndDigest{refDigested}
	id, err = DockerReferenceIdentity(tagDigestRef)
	assert.Equal(t, "", id)
	assert.Error(t, err)
}
