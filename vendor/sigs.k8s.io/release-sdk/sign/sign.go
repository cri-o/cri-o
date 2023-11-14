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
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/logs"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
	"github.com/jellydator/ttlcache/v3"
	"github.com/nozzle/throttler"
	cliOpts "github.com/sigstore/cosign/cmd/cosign/cli/options"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/release-utils/hash"
)

// Signer is the main structure to be used by API consumers.
type Signer struct {
	impl       impl
	options    *Options
	signedRefs *ttlcache.Cache[string, bool]                       // key: imageRef, value: isSigned
	parsedRefs *ttlcache.Cache[string, name.Reference]             // key: imageRef, value: parsedRef
	transports *ttlcache.Cache[name.Repository, http.RoundTripper] // key: repo of parsedRef, value: transport
	signedObjs *ttlcache.Cache[string, *SignedObject]              // key: imageRef, value: signed object
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

	signer := &Signer{
		impl:    &defaultImpl{},
		options: options,
		signedRefs: ttlcache.New(
			ttlcache.WithTTL[string, bool](options.CacheTimeout),
			ttlcache.WithCapacity[string, bool](options.MaxCacheItems),
		),
		parsedRefs: ttlcache.New(
			ttlcache.WithTTL[string, name.Reference](options.CacheTimeout),
			ttlcache.WithCapacity[string, name.Reference](options.MaxCacheItems),
		),
		transports: ttlcache.New(
			ttlcache.WithTTL[name.Repository, http.RoundTripper](options.CacheTimeout),
			ttlcache.WithCapacity[name.Repository, http.RoundTripper](options.MaxCacheItems),
		),
		signedObjs: ttlcache.New(
			ttlcache.WithTTL[string, *SignedObject](options.CacheTimeout),
			ttlcache.WithCapacity[string, *SignedObject](options.MaxCacheItems),
		),
	}

	go signer.signedRefs.Start()
	go signer.parsedRefs.Start()
	go signer.transports.Start()
	go signer.signedObjs.Start()

	return signer
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
	return s.SignImageWithOptions(s.options, reference)
}

// SignImageWithOptions can be used to sign any provided container image
// reference by using the provided custom options.
func (s *Signer) SignImageWithOptions(options *Options, reference string) (object *SignedObject, err error) {
	s.log().Infof("Signing reference: %s", reference)

	// Ensure options to sign are correct
	if err := options.verifySignOptions(); err != nil {
		return nil, fmt.Errorf("checking signing options: %w", err)
	}

	resetFn, err := s.enableExperimental()
	if err != nil {
		return nil, err
	}
	defer resetFn()

	ctx, cancel := options.context()
	defer cancel()

	// If we don't have a key path, we must ensure we can get an OIDC
	// token or there is no way to sign. Depending on the options set,
	// we may get the ID token from the cosign providers
	identityToken := ""
	if options.PrivateKeyPath == "" {
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
		KeyRef:     options.PrivateKeyPath,
		IDToken:    identityToken,
		PassFunc:   options.PassFunc,
		FulcioURL:  cliOpts.DefaultFulcioURL,
		RekorURL:   cliOpts.DefaultRekorURL,
		OIDCIssuer: cliOpts.DefaultOIDCIssuerURL,

		InsecureSkipFulcioVerify: false,
	}

	regOpts := cliOpts.RegistryOptions{
		AllowInsecure: options.AllowInsecure,
	}

	images := []string{reference}

	if err := s.impl.SignImageInternal(
		options.ToCosignRootOptions(), ko, regOpts, options.Annotations,
		images, "", options.AttachSignature, options.OutputSignaturePath,
		options.OutputCertificatePath, "", true, false, "", false,
	); err != nil {
		return nil, fmt.Errorf("sign reference: %s: %w", reference, err)
	}

	// After signing, registry consistency may not be there right
	// away. Retry the image verification if it fails
	// ref: https://github.com/kubernetes-sigs/promo-tools/issues/536
	waitErr := wait.ExponentialBackoff(wait.Backoff{
		Duration: 500 * time.Millisecond,
		Factor:   1.5,
		Steps:    int(options.MaxRetries),
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
// using keyless signing. It ignores unsigned images.
func (s *Signer) VerifyImage(reference string) (*SignedObject, error) {
	s.log().Infof("Verifying reference: %s", reference)

	item := s.signedObjs.Get(reference)
	if item != nil {
		return item.Value(), nil
	}

	res, err := s.VerifyImages(reference)
	if err != nil {
		return nil, fmt.Errorf("verify image: %w", err)
	}

	o, ok := res.Load(reference)
	if !ok {
		// Probably not signed
		return nil, nil
	}

	obj, ok := o.(*SignedObject)
	if !ok {
		return nil, fmt.Errorf("interface conversion error, result is not a *SignedObject: %v", o)
	}

	s.signedObjs.Set(reference, obj, ttlcache.DefaultTTL)

	return obj, nil
}

// VerifyImages can be used to validate any provided container image reference
// list by using keyless signing. It ignores unsigned images. Returns a sync map
// where the key is the ref (string) and the value is the *SignedObject
func (s *Signer) VerifyImages(refs ...string) (*sync.Map, error) {
	s.log().Debug("Checking cache")
	res := &sync.Map{}
	unknownRefs := []string{}
	for _, ref := range refs {
		item := s.signedObjs.Get(ref)
		if item != nil {
			res.Store(ref, item.Value())
			continue
		}

		unknownRefs = append(unknownRefs, ref)
	}

	if len(unknownRefs) == 0 {
		s.log().Debug("All references already available in cache")
		return res, nil
	}

	s.log().Infof("Verifying %d references", len(unknownRefs))

	resetFn, err := s.enableExperimental()
	if err != nil {
		return nil, fmt.Errorf("enable experimental cosign: %w", err)
	}
	defer resetFn()

	// checking whether the image being verified has a signature
	// if there is no signature, we should skip
	// ref: https://kubernetes.slack.com/archives/CJH2GBF7Y/p1647459428848859?thread_ts=1647428695.280269&cid=CJH2GBF7Y
	ctx, cancel := s.options.context()
	defer cancel()
	imagesSigned, err := s.impl.ImagesSigned(ctx, s, unknownRefs...)
	if err != nil {
		return nil, fmt.Errorf("verify if images are signed: %w", err)
	}
	unknownRefs = []string{}
	imagesSigned.Range(func(key, value any) bool {
		ref, ok := key.(string)
		if !ok {
			logrus.Errorf("Interface conversion failed: key is not a string: %v", key)
			return false
		}
		isSigned, ok := value.(bool)
		if !ok {
			logrus.Errorf("Interface conversion failed: value is not a bool: %v", value)
			return false
		}

		if isSigned {
			unknownRefs = append(unknownRefs, ref)
		}

		return true
	})

	t := throttler.New(int(s.options.MaxWorkers), len(unknownRefs))
	for _, ref := range unknownRefs {
		go func(ref string) {
			ctx, cancel := s.options.context()
			defer cancel()

			_, err = s.impl.VerifyImageInternal(ctx, s.options.PublicKeyPath, []string{ref})
			if err != nil {
				t.Done(fmt.Errorf("verify image reference: %s: %w", ref, err))
				return
			}

			var parsedRef name.Reference
			item := s.parsedRefs.Get(ref)
			if item != nil {
				parsedRef = item.Value()
			} else {
				parsedRef, err = s.impl.ParseReference(ref)
				if err != nil {
					t.Done(fmt.Errorf("parsing reference: %s: %w", ref, err))
					return
				}
			}

			digest, err := s.impl.Digest(parsedRef.String())
			if err != nil {
				t.Done(fmt.Errorf("getting the reference digest for %s: %w", ref, err))
				return
			}

			obj := &SignedObject{
				image: &SignedImage{
					digest:    digest,
					reference: parsedRef.String(),
					signature: repoDigestToSig(parsedRef.Context(), digest),
				},
			}

			res.Store(ref, obj)
			s.signedObjs.Set(ref, obj, ttlcache.DefaultTTL)
			t.Done(nil)
		}(ref)

		if t.Throttle() > 0 {
			break
		}
	}

	s.log().Debug("Done verifying references")

	if err := t.Err(); err != nil {
		return res, fmt.Errorf("verifying references: %w", err)
	}

	return res, nil
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
	item := s.signedRefs.Get(imageRef)
	if item != nil {
		return item.Value(), nil
	}

	res, err := s.impl.ImagesSigned(context.Background(), s, imageRef)
	if err != nil {
		return false, fmt.Errorf("check if image is signed: %w", err)
	}
	signed, ok := res.Load(imageRef)
	if !ok {
		return false, errors.New("ref is not part of result")
	}
	signedBool, ok := signed.(bool)
	if !ok {
		return false, fmt.Errorf("interface conversion error, result is not a bool: %v", signed)
	}

	s.signedRefs.Set(imageRef, signedBool, ttlcache.DefaultTTL)

	return signedBool, nil
}

// ImagesSigned verifies if the provided image references are signed. It
// returns a sync map where the key is the ref and the value is a boolean which
// indicates if the image is signed or not. The method runs highly parallel.
func (s *Signer) ImagesSigned(ctx context.Context, refs ...string) (*sync.Map, error) {
	s.log().Debug("Checking cache")
	res := &sync.Map{}
	unknownRefs := []string{}
	for _, ref := range refs {
		item := s.signedRefs.Get(ref)
		if item != nil {
			res.Store(ref, item.Value())
			continue
		}

		unknownRefs = append(unknownRefs, ref)
	}

	if len(unknownRefs) == 0 {
		s.log().Debug("All references already available in cache")
		return res, nil
	}

	s.log().Debug("Parsing references")
	repos := []name.Repository{}
	for _, ref := range unknownRefs {
		item := s.parsedRefs.Get(ref)
		if item != nil {
			repos = append(repos, item.Value().Context())
			continue
		}

		parsedRef, err := s.impl.ParseReference(ref)
		if err != nil {
			return nil, fmt.Errorf("parsing image reference: %w", err)
		}

		repos = append(repos, parsedRef.Context())
		s.parsedRefs.Set(ref, parsedRef, ttlcache.DefaultTTL)
	}

	s.log().Debug("Building transports")
	transports, count, err := s.transportsForRefs(ctx, repos...)
	if err != nil {
		return nil, fmt.Errorf("build transports: %w", err)
	}
	s.log().Debugf("Built %d transports for %d refs", count, len(unknownRefs))

	s.log().Debug("Checking if refs are signed")
	t := throttler.New(int(s.options.MaxWorkers), len(unknownRefs))
	for i, repo := range repos {
		go func(repo name.Repository, i int) {
			ref := unknownRefs[i]

			trans, ok := transports.Load(repo)
			if !ok {
				t.Done(fmt.Errorf("no transport found for repo: %s", repo.String()))
				return
			}
			tr, ok := trans.(http.RoundTripper)
			if !ok {
				t.Done(fmt.Errorf("transport has wrong type: %v", tr))
				return
			}

			digest, err := s.impl.Digest(ref, crane.WithTransport(tr))
			if err != nil {
				t.Done(fmt.Errorf("get digest for image reference: %w", err))
				return
			}

			if _, err := s.impl.Digest(repoDigestToSig(repo, digest), crane.WithTransport(tr)); err != nil {
				if transportErr, ok := err.(*transport.Error); ok && len(transportErr.Errors) > 0 {
					if transportErr.Errors[0].Code == transport.ManifestUnknownErrorCode {
						res.Store(ref, false)
						s.signedRefs.Set(ref, false, ttlcache.DefaultTTL)
						t.Done(nil)
						return
					}
				}

				t.Done(fmt.Errorf("get digest for signature: %w", err))
				return
			}

			res.Store(ref, true)
			s.signedRefs.Set(ref, true, ttlcache.DefaultTTL)
			t.Done(nil)
		}(repo, i)

		if t.Throttle() > 0 {
			break
		}
	}

	s.log().Debug("Done checking if refs are signed")

	if err := t.Err(); err != nil {
		return res, fmt.Errorf("check if images are signed: %w", err)
	}

	return res, nil
}

func (s *Signer) transportsForRefs(ctx context.Context, repos ...name.Repository) (*sync.Map, int, error) {
	count := 0
	t := throttler.New(int(s.options.MaxWorkers), len(repos))
	transports := &sync.Map{}

	for _, repo := range repos {
		go func(repo name.Repository) {
			if _, loaded := transports.LoadOrStore(repo, nil); loaded {
				t.Done(nil)
				return
			}

			item := s.transports.Get(repo)
			if item != nil {
				transports.Store(repo, item.Value())
				count++
				t.Done(nil)
				return
			}

			tr, err := s.transportForRepo(ctx, repo)
			if err != nil {
				t.Done(fmt.Errorf("create transport for repo %s: %w", repo.String(), err))
				return
			}

			s.transports.Set(repo, tr, ttlcache.DefaultTTL)
			transports.Store(repo, tr)
			count++
			t.Done(nil)
		}(repo)

		if t.Throttle() > 0 {
			break
		}
	}

	if err := t.Err(); err != nil {
		return nil, 0, fmt.Errorf("building transports: %w", err)
	}

	return transports, count, nil
}

func (s *Signer) transportForRepo(ctx context.Context, repo name.Repository) (http.RoundTripper, error) {
	scopes := []string{repo.Scope(transport.PullScope)}

	t := remote.DefaultTransport
	t = transport.NewLogger(t)
	t = transport.NewRetry(t)
	t = transport.NewUserAgent(t, "k8s-release-sdk")

	t, err := s.impl.NewWithContext(ctx, repo.Registry, authn.Anonymous, t, scopes)
	if err != nil {
		return nil, fmt.Errorf("create new transport: %w", err)
	}

	return t, nil
}

func repoDigestToSig(repo name.Repository, digest string) string {
	return repo.Name() + ":" + strings.Replace(digest, ":", "-", 1) + ".sig"
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
