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
	"time"

	"github.com/sigstore/cosign/cmd/cosign/cli/options"
	"github.com/sigstore/cosign/pkg/cosign"
	"github.com/sirupsen/logrus"
)

// Options can be used to modify the behavior of the signer.
type Options struct {
	// Logger is the custom logger to be used for message printing.
	Logger *logrus.Logger

	// Verbose can be used to enable a higher log verbosity
	Verbose bool

	// Timeout is the default timeout for network operations.
	// Defaults to 3 minutes
	Timeout time.Duration

	AllowInsecure bool

	// AttachSignature tells the signer to attach or not the new
	// signature to its image
	AttachSignature bool

	OutputSignaturePath   string
	OutputCertificatePath string
	Annotations           map[string]interface{}
	PrivateKeyPath        string
	PublicKeyPath         string

	// Identity token for keyless signing
	IdentityToken string

	// EnableTokenProviders tells signer to try to get a
	// token from the cosign providers when needed.
	EnableTokenProviders bool

	// PassFunc is a function that returns a slice of bytes that will be used
	// as a password for decrypting the cosign key. It is used only if PrivateKeyPath
	// is provided (i.e. it's not used for keyless signing).
	// Defaults to nil, which acts as having no password provided at all.
	PassFunc cosign.PassFunc

	// MaxRetries indicates the number of times to retry operations
	// when transient failures occur
	MaxRetries uint

	// The amount of maximum workers for parallel executions.
	// Defaults to 100.
	MaxWorkers uint

	// CacheTimeout is the timeout for the internal caches.
	// Defaults to 2 hours.
	CacheTimeout time.Duration

	// MaxCacheItems is the maximumg amount of items the internal caches can hold.
	// Defaults to 10000.
	MaxCacheItems uint64
}

// Default returns a default Options instance.
func Default() *Options {
	return &Options{
		Logger:               logrus.StandardLogger(),
		Timeout:              3 * time.Minute,
		EnableTokenProviders: true,
		AttachSignature:      true,
		MaxRetries:           3,
		MaxWorkers:           100,
		CacheTimeout:         2 * time.Hour,
		MaxCacheItems:        10000,
	}
}

func (o *Options) ToCosignRootOptions() options.RootOptions {
	return options.RootOptions{
		Timeout: o.Timeout,
	}
}

// verifySignOptions checks that options have the minimum settings
// for signing files or images:
func (o *Options) verifySignOptions() error {
	// Our library is not designed to run in interactive mode
	// this means that we will only support signing if we get a keypair or
	// identity token to run keyless signing:
	if o.PrivateKeyPath != "" && o.IdentityToken == "" && !o.EnableTokenProviders {
		return errors.New("signing can only be done if a key or identity token are set")
	}

	// Ensure that the private key file exists
	i := defaultImpl{}
	if o.PrivateKeyPath != "" && !i.FileExists(o.PrivateKeyPath) {
		return errors.New("specified private key file not found")
	}
	return nil
}

// context creates a new context with the timeout set within the options.
func (o *Options) context() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), o.Timeout)
}
