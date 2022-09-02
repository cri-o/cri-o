/*
Copyright 2022 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package sign

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/logs"
	cliOpts "github.com/sigstore/cosign/cmd/cosign/cli/options"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/release-utils/hash"
)

// Signer is the main structure to be used by API consumers.
type Signer struct {
	impl    impl
	options *Options
}

// New returns a new Signer instance.
func New(options *Options) *Signer {
	if options == nil {
		options = Default()
	}

	if options.Logger == nil {
		options.Logger = logrus.New()
	}

	if options.Verbose {
		logs.Debug.SetOutput(os.Stderr)
		options.Logger.SetLevel(logrus.DebugLevel)
	}

	return &Signer{
		impl:    &defaultImpl{},
		options: options,
	}
}

// SetImpl can be used to set the internal implementation, which is mainly used
// for testing.
func (s *Signer) SetImpl(impl impl) {
	s.impl = impl
}

// log returns the internally set logger.
func (s *Signer) log() *logrus.Logger {
	return s.options.Logger
}

func (s *Signer) UploadBlob(path string) error {
	s.log().Infof("Uploading blob: %s", path)

	// TODO: unimplemented

	return nil
}

// SignImage can be used to sign any provided container image reference by
// using keyless signing.
func (s *Signer) SignImage(reference string) (object *SignedObject, err error) {
	s.log().Infof("Signing reference: %s", reference)

	// Ensure options to sign are correct
	if err := s.options.verifySignOptions(); err != nil {
		return nil, fmt.Errorf("checking signing options: %w", err)
	}

	resetFn, err := s.enableExperimental()
	if err != nil {
		return nil, err
	}
	defer resetFn()

	ctx, cancel := s.options.context()
	defer cancel()

	// If we don't have a key path, we must ensure we can get an OIDC
	// token or there is no way to sign. Depending on the options set,
	// we may get the ID token from the cosign providers
	identityToken := ""
	if s.options.PrivateKeyPath == "" {
		tok, err := s.identityToken(ctx)
		if err != nil {
			return nil, fmt.Errorf("getting identity token for keyless signing: %w", err)
		}
		identityToken = tok
		if identityToken == "" {
			return nil, errors.New(
				"no private key or identity token are available, unable to sign",
			)
		}
	}

	ko := cliOpts.KeyOpts{
		KeyRef:     s.options.PrivateKeyPath,
		IDToken:    identityToken,
		PassFunc:   s.options.PassFunc,
		FulcioURL:  cliOpts.DefaultFulcioURL,
		RekorURL:   cliOpts.DefaultRekorURL,
		OIDCIssuer: cliOpts.DefaultOIDCIssuerURL,

		InsecureSkipFulcioVerify: false,
	}

	regOpts := cliOpts.RegistryOptions{
		AllowInsecure: s.options.AllowInsecure,
	}

	images := []string{reference}

	if err := s.impl.SignImageInternal(
		s.options.ToCosignRootOptions(), ko, regOpts, s.options.Annotations,
		images, "", s.options.AttachSignature, s.options.OutputSignaturePath,
		s.options.OutputCertificatePath, "", true, false, "", false,
	); err != nil {
		return nil, fmt.Errorf("sign reference: %s: %w", reference, err)
	}

	// After signing, registry consistency may not be there right
	// away. Retry the image verification if it fails
	// ref: https://github.com/kubernetes-sigs/promo-tools/issues/536
	waitErr := wait.ExponentialBackoff(wait.Backoff{
		Duration: 500 * time.Millisecond,
		Factor:   1.5,
		Steps:    int(s.options.MaxRetries),
	}, func() (bool, error) {
		object, err = s.VerifyImage(images[0])
		if err != nil {
			err = fmt.Errorf("verifying reference %s: %w", images[0], err)
			return false, nil
		}
		return true, nil
	})

	if waitErr != nil {
		return nil, fmt.Errorf("retrying image verification: %w", waitErr)
	}

	return object, err
}

// SignFile can be used to sign any provided file path by using keyless
// signing.
func (s *Signer) SignFile(path string) (*SignedObject, error) {
	s.log().Infof("Signing file path: %s", path)

	resetFn, err := s.enableExperimental()
	if err != nil {
		return nil, err
	}
	defer resetFn()

	ctx, cancel := s.options.context()
	defer cancel()

	// If we don't have a key path, we must ensure we can get an OIDC
	// token or there is no way to sign. Depending on the options set,
	// we may get the ID token from the cosign providers
	identityToken := ""
	if s.options.PrivateKeyPath == "" {
		tok, err := s.identityToken(ctx)
		if err != nil {
			return nil, fmt.Errorf("getting identity token for keyless signing: %w", err)
		}
		identityToken = tok
		if identityToken == "" {
			return nil, errors.New(
				"no private key or identity token are available, unable to sign",
			)
		}
	}

	ko := cliOpts.KeyOpts{
		KeyRef:     s.options.PrivateKeyPath,
		IDToken:    identityToken,
		PassFunc:   s.options.PassFunc,
		FulcioURL:  cliOpts.DefaultFulcioURL,
		RekorURL:   cliOpts.DefaultRekorURL,
		OIDCIssuer: cliOpts.DefaultOIDCIssuerURL,

		InsecureSkipFulcioVerify: false,
	}

	regOpts := cliOpts.RegistryOptions{
		AllowInsecure: s.options.AllowInsecure,
	}

	if s.options.OutputCertificatePath == "" {
		s.options.OutputCertificatePath = fmt.Sprintf("%s.cert", path)
	}
	if s.options.OutputSignaturePath == "" {
		s.options.OutputSignaturePath = fmt.Sprintf("%s.sig", path)
	}

	fileSHA, err := hash.SHA256ForFile(path)
	if err != nil {
		return nil, fmt.Errorf("file retrieve sha256: %s: %w", path, err)
	}

	if err := s.impl.SignFileInternal(
		s.options.ToCosignRootOptions(), ko, regOpts, path, true,
		s.options.OutputSignaturePath, s.options.OutputCertificatePath,
	); err != nil {
		return nil, fmt.Errorf("sign file: %s: %w", path, err)
	}

	verifyKo := ko
	verifyKo.KeyRef = s.options.PublicKeyPath

	err = s.impl.VerifyFileInternal(ctx, verifyKo, s.options.OutputSignaturePath, s.options.OutputCertificatePath, path)
	if err != nil {
		return nil, fmt.Errorf("verifying signed file: %s: %w", path, err)
	}

	return &SignedObject{
		file: &SignedFile{
			path:            path,
			sha256:          fileSHA,
			signaturePath:   s.options.OutputSignaturePath,
			certificatePath: s.options.OutputCertificatePath,
		},
	}, nil
}

// VerifyImage can be used to validate any provided container image reference by
// using keyless signing.
func (s *Signer) VerifyImage(reference string) (*SignedObject, error) {
	s.log().Infof("Verifying reference: %s", reference)

	// checking whether the image being verified has a signature
	// if there is no signature, we should skip
	// ref: https://kubernetes.slack.com/archives/CJH2GBF7Y/p1647459428848859?thread_ts=1647428695.280269&cid=CJH2GBF7Y
	isSigned, err := s.IsImageSigned(reference)
	if err != nil {
		return nil, fmt.Errorf("checking if %s is signed: %w", reference, err)
	}

	if !isSigned {
		s.log().Infof("Skipping unsigned image: %s", reference)
		return nil, nil
	}

	resetFn, err := s.enableExperimental()
	if err != nil {
		return nil, err
	}
	defer resetFn()

	ctx, cancel := s.options.context()
	defer cancel()

	images := []string{reference}
	_, err = s.impl.VerifyImageInternal(ctx, s.options.PublicKeyPath, images)
	if err != nil {
		return nil, fmt.Errorf("verify image reference: %s: %w", images, err)
	}

	ref, err := s.impl.ParseReference(reference)
	if err != nil {
		return &SignedObject{}, fmt.Errorf("parsing reference: %s: %w", reference, err)
	}

	dig, err := s.impl.Digest(ref.String())
	if err != nil {
		return &SignedObject{}, fmt.Errorf("getting the reference digest for %s: %w", reference, err)
	}

	sigParsed := strings.ReplaceAll(dig, "sha256:", "sha256-")
	obj := &SignedObject{
		image: &SignedImage{
			digest:    dig,
			reference: ref.String(),
			signature: fmt.Sprintf("%s:%s.sig", ref.Context().Name(), sigParsed),
		},
	}

	return obj, nil
}

// VerifyFile can be used to validate any provided file path.
// If no signed entry is found we skip the file without errors.
func (s *Signer) VerifyFile(path string) (*SignedObject, error) {
	s.log().Infof("Verifying file path: %s", path)

	resetFn, err := s.enableExperimental()
	if err != nil {
		return nil, err
	}
	defer resetFn()

	ko := cliOpts.KeyOpts{
		KeyRef:   s.options.PublicKeyPath,
		RekorURL: cliOpts.DefaultRekorURL,
	}

	if s.options.OutputCertificatePath == "" {
		s.options.OutputCertificatePath = fmt.Sprintf("%s.cert", path)
	}
	if s.options.OutputSignaturePath == "" {
		s.options.OutputSignaturePath = fmt.Sprintf("%s.sig", path)
	}

	ctx, cancel := s.options.context()
	defer cancel()

	isSigned, err := s.IsFileSigned(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("checking if file is signed. file: %s, error: %w", path, err)
	}

	fileSHA, err := hash.SHA256ForFile(path)
	if err != nil {
		return nil, fmt.Errorf("file retrieve sha256 error: %s: %w", path, err)
	}

	if !isSigned {
		s.log().Infof("Skipping unsigned file: %s", path)
		return nil, nil
	}

	err = s.impl.VerifyFileInternal(ctx, ko, s.options.OutputSignaturePath, s.options.OutputCertificatePath, path)
	if err != nil {
		return nil, fmt.Errorf("verify file reference: %s: %w", path, err)
	}

	return &SignedObject{
		file: &SignedFile{
			path:            path,
			sha256:          fileSHA,
			signaturePath:   s.options.OutputSignaturePath,
			certificatePath: s.options.OutputCertificatePath,
		},
	}, nil
}

// enableExperimental sets the cosign experimental mode to true. It also
// returns a resetFn to recover the original state within the environment.
func (s *Signer) enableExperimental() (resetFn func(), err error) {
	const key = "COSIGN_EXPERIMENTAL"
	previousValue := s.impl.EnvDefault(key, "")
	if err := s.impl.Setenv(key, "true"); err != nil {
		return nil, fmt.Errorf("enable cosign experimental mode: %w", err)
	}
	return func() {
		if err := s.impl.Setenv(key, previousValue); err != nil {
			s.log().Errorf("Unable to reset cosign experimental mode: %v", err)
		}
	}, nil
}

// IsImageSigned takes an image reference and returns true if there are
// signatures available for it. It makes no signature verification, only
// checks to see if more than one signature is available.
func (s *Signer) IsImageSigned(imageRef string) (bool, error) {
	ref, err := s.impl.ParseReference(imageRef)
	if err != nil {
		return false, fmt.Errorf("parsing image reference: %w", err)
	}

	simg, err := s.impl.SignedEntity(ref)
	if err != nil {
		return false, fmt.Errorf("getting signed entity from image reference: %w", err)
	}

	sigs, err := s.impl.Signatures(simg)
	if err != nil {
		return false, fmt.Errorf("remote image: %w", err)
	}

	signatures, err := s.impl.SignaturesList(sigs)
	if err != nil {
		return false, fmt.Errorf("fetching signatures: %w", err)
	}

	return len(signatures) > 0, nil
}

// IsFileSigned takes an path reference and retrusn true if there is a signature
// available for it. It makes no signature verification, only checks to see if
// there is a TLog to be found on Rekor.
func (s *Signer) IsFileSigned(ctx context.Context, path string) (bool, error) {
	ko := cliOpts.KeyOpts{
		KeyRef:   s.options.PublicKeyPath,
		RekorURL: cliOpts.DefaultRekorURL,
	}

	rClient, err := s.impl.NewRekorClient(ko.RekorURL)
	if err != nil {
		return false, fmt.Errorf("creating rekor client: %w", err)
	}

	blobBytes, err := s.impl.PayloadBytes(path)
	if err != nil {
		return false, err
	}

	uuids, err := s.impl.FindTLogEntriesByPayload(ctx, rClient, blobBytes)
	if err != nil {
		return false, fmt.Errorf("find rekor tlog entries: %w", err)
	}

	return len(uuids) > 0, nil
}

// identityToken returns an identity token to perform keyless signing.
// If there is one set in the options we will use that one. If not,
// signer will try to get one from the cosign OIDC identity providers
// if options.EnableTokenProviders is set
func (s *Signer) identityToken(ctx context.Context) (string, error) {
	tok := s.options.IdentityToken
	if s.options.PrivateKeyPath == "" && s.options.IdentityToken == "" {
		// We only attempt to pull from the providers if the option is set
		if !s.options.EnableTokenProviders {
			s.log().Warn("No token set in options and OIDC providers are disabled")
			return "", nil
		}

		s.log().Info("No identity token was provided. Attempting to get one from supported providers.")
		token, err := s.impl.TokenFromProviders(ctx, s.log())
		if err != nil {
			return "", fmt.Errorf("getting identity token from providers: %w", err)
		}
		tok = token
	}
	return tok, nil
}
