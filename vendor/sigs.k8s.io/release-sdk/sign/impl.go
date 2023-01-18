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
	"fmt"
	"net/http"
	"os"
	"sync"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
	"github.com/sigstore/cosign/cmd/cosign/cli/options"
	"github.com/sigstore/cosign/cmd/cosign/cli/rekor"
	"github.com/sigstore/cosign/cmd/cosign/cli/sign"
	"github.com/sigstore/cosign/cmd/cosign/cli/verify"
	"github.com/sigstore/cosign/pkg/blob"
	"github.com/sigstore/cosign/pkg/cosign"
	"github.com/sigstore/cosign/pkg/providers"
	"github.com/sigstore/rekor/pkg/generated/client"
	"github.com/sirupsen/logrus"

	"sigs.k8s.io/release-utils/env"
	"sigs.k8s.io/release-utils/util"
)

type defaultImpl struct{}

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate
//counterfeiter:generate . impl
//go:generate /usr/bin/env bash -c "cat ../scripts/boilerplate/boilerplate.generatego.txt signfakes/fake_impl.go > signfakes/_fake_impl.go && mv signfakes/_fake_impl.go signfakes/fake_impl.go"
type impl interface {
	VerifyFileInternal(ctx context.Context, ko options.KeyOpts, outputSignature, outputCertificate, path string) error
	VerifyImageInternal(ctx context.Context, keyPath string, images []string) (*SignedObject, error)
	SignImageInternal(ro options.RootOptions, ko options.KeyOpts, regOpts options.RegistryOptions,
		annotations map[string]interface{}, imgs []string, certPath string, upload bool,
		outputSignature string, outputCertificate string, payloadPath string, force bool,
		recursive bool, attachment string, noTlogUpload bool) error
	SignFileInternal(ro options.RootOptions, ko options.KeyOpts, regOpts options.RegistryOptions,
		payloadPath string, b64 bool, outputSignature string, outputCertificate string) error
	Setenv(string, string) error
	EnvDefault(string, string) string
	TokenFromProviders(context.Context, *logrus.Logger) (string, error)
	FileExists(string) bool
	ParseReference(string, ...name.Option) (name.Reference, error)
	FindTLogEntriesByPayload(ctx context.Context, rClient *client.Rekor, blobBytes []byte) ([]string, error)
	Digest(ref string, opt ...crane.Option) (string, error)
	PayloadBytes(blobRef string) ([]byte, error)
	NewRekorClient(string) (*client.Rekor, error)
	NewWithContext(context.Context, name.Registry, authn.Authenticator, http.RoundTripper, []string) (http.RoundTripper, error)
	ImagesSigned(context.Context, *Signer, ...string) (*sync.Map, error)
}

func (*defaultImpl) VerifyFileInternal(ctx context.Context, ko options.KeyOpts, outputSignature, //nolint: gocritic
	outputCertificate, path string,
) error {
	return verify.VerifyBlobCmd(ctx, ko, outputCertificate, "", "", "", "", outputSignature, path, "", "", "", "", "", false)
}

func (*defaultImpl) VerifyImageInternal(ctx context.Context, publickeyPath string, images []string) (*SignedObject, error) {
	v := verify.VerifyCommand{KeyRef: publickeyPath}
	return &SignedObject{}, v.Exec(ctx, images)
}

func (*defaultImpl) SignImageInternal(ro options.RootOptions, ko options.KeyOpts, regOpts options.RegistryOptions, //nolint: gocritic
	annotations map[string]interface{}, imgs []string, certPath string, upload bool,
	outputSignature string, outputCertificate string, payloadPath string, force bool,
	recursive bool, attachment string, noTlogUpload bool,
) error {
	return sign.SignCmd(
		&ro, ko, regOpts, annotations, imgs, certPath, "", upload, outputSignature,
		outputCertificate, payloadPath, force, recursive, attachment, noTlogUpload,
	)
}

func (*defaultImpl) SignFileInternal(ro options.RootOptions, ko options.KeyOpts, regOpts options.RegistryOptions, //nolint: gocritic
	payloadPath string, b64 bool, outputSignature string, outputCertificate string,
) error {
	// Ignoring the signature return value for now as we are setting the outputSignature path and to keep an consistent impl API
	// Setting timeout as 0 is acceptable here because SignBlobCmd uses the passed context
	_, err := sign.SignBlobCmd(&ro, ko, regOpts, payloadPath, b64, outputSignature, outputCertificate)
	return err
}

func (*defaultImpl) Setenv(key, value string) error {
	return os.Setenv(key, value)
}

func (*defaultImpl) EnvDefault(key, def string) string {
	return env.Default(key, def)
}

// TokenFromProviders will try the cosign OIDC providers to get an
// oidc token from them.
func (d *defaultImpl) TokenFromProviders(ctx context.Context, logger *logrus.Logger) (string, error) {
	if !d.IdentityProvidersEnabled(ctx) {
		logger.Warn("No OIDC provider enabled. Token cannot be obtained autmatically.")
		return "", nil
	}

	tok, err := providers.Provide(ctx, "sigstore")
	if err != nil {
		return "", fmt.Errorf("fetching oidc token from environment: %w", err)
	}
	return tok, nil
}

// FileExists returns true if a file exists
func (*defaultImpl) FileExists(path string) bool {
	return util.Exists(path)
}

// IdentityProvidersEnabled returns true if any of the cosign
// identity providers is able to obteain an OIDC identity token
// suitable for keyless signing,
func (*defaultImpl) IdentityProvidersEnabled(ctx context.Context) bool {
	return providers.Enabled(ctx)
}

func (*defaultImpl) ParseReference(
	s string, opts ...name.Option,
) (name.Reference, error) {
	return name.ParseReference(s, opts...)
}

func (d *defaultImpl) FindTLogEntriesByPayload(
	ctx context.Context, rClient *client.Rekor, blobBytes []byte,
) ([]string, error) {
	return cosign.FindTLogEntriesByPayload(ctx, rClient, blobBytes)
}

func (*defaultImpl) Digest(
	ref string, opts ...crane.Option,
) (string, error) {
	return crane.Digest(ref, opts...)
}

func (*defaultImpl) PayloadBytes(blobRef string) (blobBytes []byte, err error) {
	blobBytes, err = blob.LoadFileOrURL(blobRef)
	if err != nil {
		return nil, fmt.Errorf("load file or url of sign payload: %w", err)
	}
	return blobBytes, nil
}

func (*defaultImpl) NewRekorClient(rekorURL string) (*client.Rekor, error) {
	return rekor.NewClient(rekorURL)
}

func (*defaultImpl) NewWithContext(
	ctx context.Context,
	reg name.Registry,
	auth authn.Authenticator,
	t http.RoundTripper,
	scopes []string,
) (http.RoundTripper, error) {
	return transport.NewWithContext(ctx, reg, auth, t, scopes)
}

func (d *defaultImpl) ImagesSigned(ctx context.Context, s *Signer, refs ...string) (*sync.Map, error) {
	return s.ImagesSigned(ctx, refs...)
}
