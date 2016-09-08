package copy

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"strings"

	"github.com/containers/image/image"
	"github.com/containers/image/signature"
	"github.com/containers/image/transports"
	"github.com/containers/image/types"
)

// supportedDigests lists the supported blob digest types.
var supportedDigests = map[string]func() hash.Hash{
	"sha256": sha256.New,
}

type digestingReader struct {
	source           io.Reader
	digest           hash.Hash
	expectedDigest   []byte
	validationFailed bool
}

// newDigestingReader returns an io.Reader implementation with contents of source, which will eventually return a non-EOF error
// and set validationFailed to true if the source stream does not match expectedDigestString.
func newDigestingReader(source io.Reader, expectedDigestString string) (*digestingReader, error) {
	fields := strings.SplitN(expectedDigestString, ":", 2)
	if len(fields) != 2 {
		return nil, fmt.Errorf("Invalid digest specification %s", expectedDigestString)
	}
	fn, ok := supportedDigests[fields[0]]
	if !ok {
		return nil, fmt.Errorf("Invalid digest specification %s: unknown digest type %s", expectedDigestString, fields[0])
	}
	digest := fn()
	expectedDigest, err := hex.DecodeString(fields[1])
	if err != nil {
		return nil, fmt.Errorf("Invalid digest value %s: %v", expectedDigestString, err)
	}
	if len(expectedDigest) != digest.Size() {
		return nil, fmt.Errorf("Invalid digest specification %s: length %d does not match %d", expectedDigestString, len(expectedDigest), digest.Size())
	}
	return &digestingReader{
		source:           source,
		digest:           digest,
		expectedDigest:   expectedDigest,
		validationFailed: false,
	}, nil
}

func (d *digestingReader) Read(p []byte) (int, error) {
	n, err := d.source.Read(p)
	if n > 0 {
		if n2, err := d.digest.Write(p[:n]); n2 != n || err != nil {
			// Coverage: This should not happen, the hash.Hash interface requires
			// d.digest.Write to never return an error, and the io.Writer interface
			// requires n2 == len(input) if no error is returned.
			return 0, fmt.Errorf("Error updating digest during verification: %d vs. %d, %v", n2, n, err)
		}
	}
	if err == io.EOF {
		actualDigest := d.digest.Sum(nil)
		if subtle.ConstantTimeCompare(actualDigest, d.expectedDigest) != 1 {
			d.validationFailed = true
			return 0, fmt.Errorf("Digest did not match, expected %s, got %s", hex.EncodeToString(d.expectedDigest), hex.EncodeToString(actualDigest))
		}
	}
	return n, err
}

// Options allows supplying non-default configuration modifying the behavior of CopyImage.
type Options struct {
	RemoveSignatures bool   // Remove any pre-existing signatures. SignBy will still add a new signature.
	SignBy           string // If non-empty, asks for a signature to be added during the copy, and specifies a key ID, as accepted by signature.NewGPGSigningMechanism().SignDockerManifest(),
}

// Image copies image from srcRef to destRef, using policyContext to validate source image admissibility.
func Image(ctx *types.SystemContext, policyContext *signature.PolicyContext, destRef, srcRef types.ImageReference, options *Options) error {
	dest, err := destRef.NewImageDestination(ctx)
	if err != nil {
		return fmt.Errorf("Error initializing destination %s: %v", transports.ImageName(destRef), err)
	}
	defer dest.Close()

	rawSource, err := srcRef.NewImageSource(ctx, dest.SupportedManifestMIMETypes())
	if err != nil {
		return fmt.Errorf("Error initializing source %s: %v", transports.ImageName(srcRef), err)
	}
	src := image.FromSource(rawSource)
	defer src.Close()

	// Please keep this policy check BEFORE reading any other information about the image.
	if allowed, err := policyContext.IsRunningImageAllowed(src); !allowed || err != nil { // Be paranoid and fail if either return value indicates so.
		return fmt.Errorf("Source image rejected: %v", err)
	}

	manifest, _, err := src.Manifest()
	if err != nil {
		return fmt.Errorf("Error reading manifest: %v", err)
	}

	var sigs [][]byte
	if options != nil && options.RemoveSignatures {
		sigs = [][]byte{}
	} else {
		s, err := src.Signatures()
		if err != nil {
			return fmt.Errorf("Error reading signatures: %v", err)
		}
		sigs = s
	}
	if len(sigs) != 0 {
		if err := dest.SupportsSignatures(); err != nil {
			return fmt.Errorf("Can not copy signatures: %v", err)
		}
	}

	blobDigests, err := src.BlobDigests()
	if err != nil {
		return fmt.Errorf("Error parsing manifest: %v", err)
	}
	for _, digest := range blobDigests {
		stream, blobSize, err := rawSource.GetBlob(digest)
		if err != nil {
			return fmt.Errorf("Error reading blob %s: %v", digest, err)
		}
		defer stream.Close()

		// Be paranoid; in case PutBlob somehow managed to ignore an error from digestingReader,
		// use a separate validation failure indicator.
		// Note that we don't use a stronger "validationSucceeded" indicator, because
		// dest.PutBlob may detect that the layer already exists, in which case we don't
		// read stream to the end, and validation does not happen.
		digestingReader, err := newDigestingReader(stream, digest)
		if err != nil {
			return fmt.Errorf("Error preparing to verify blob %s: %v", digest, err)
		}
		if _, _, err := dest.PutBlob(digestingReader, digest, blobSize); err != nil {
			return fmt.Errorf("Error writing blob: %v", err)
		}
		if digestingReader.validationFailed { // Coverage: This should never happen.
			return fmt.Errorf("Internal error uploading blob %s, digest verification failed but was ignored", digest)
		}
	}

	if options != nil && options.SignBy != "" {
		mech, err := signature.NewGPGSigningMechanism()
		if err != nil {
			return fmt.Errorf("Error initializing GPG: %v", err)
		}
		dockerReference := dest.Reference().DockerReference()
		if dockerReference == nil {
			return fmt.Errorf("Cannot determine canonical Docker reference for destination %s", transports.ImageName(dest.Reference()))
		}

		newSig, err := signature.SignDockerManifest(manifest, dockerReference.String(), mech, options.SignBy)
		if err != nil {
			return fmt.Errorf("Error creating signature: %v", err)
		}
		sigs = append(sigs, newSig)
	}

	if err := dest.PutManifest(manifest); err != nil {
		return fmt.Errorf("Error writing manifest: %v", err)
	}

	if err := dest.PutSignatures(sigs); err != nil {
		return fmt.Errorf("Error writing signatures: %v", err)
	}

	if err := dest.Commit(); err != nil {
		return fmt.Errorf("Error committing the finished image: %v", err)
	}

	return nil
}
