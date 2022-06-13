// Copyright 2021 The Sigstore Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package config

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
	lru "github.com/hashicorp/golang-lru"
	"github.com/sigstore/fulcio/pkg/log"
)

type FulcioConfig struct {
	OIDCIssuers map[string]OIDCIssuer `json:"OIDCIssuers,omitempty"`

	// A meta issuer has a templated URL of the form:
	//   https://oidc.eks.*.amazonaws.com/id/*
	// Where * can match a single hostname or URI path parts
	// (in particular, no '.' or '/' are permitted, among
	// other special characters)  Some examples we want to match:
	// * https://oidc.eks.us-west-2.amazonaws.com/id/B02C93B6A2D30341AD01E1B6D48164CB
	// * https://container.googleapis.com/v1/projects/mattmoor-credit/locations/us-west1-b/clusters/tenant-cluster
	MetaIssuers map[string]OIDCIssuer `json:"MetaIssuers,omitempty"`

	// verifiers is a fixed mapping from our OIDCIssuers to their OIDC verifiers.
	verifiers map[string]*oidc.IDTokenVerifier
	// lru is an LRU cache of recently used verifiers for our meta issuers.
	lru *lru.TwoQueueCache
}

type OIDCIssuer struct {
	IssuerURL   string     `json:"IssuerURL,omitempty"`
	ClientID    string     `json:"ClientID"`
	Type        IssuerType `json:"Type"`
	IssuerClaim string     `json:"IssuerClaim,omitempty"`
}

func metaRegex(issuer string) (*regexp.Regexp, error) {
	// Quote all of the "meta" characters like `.` to avoid
	// those literal characters in the URL matching any character.
	// This will ALSO quote `*`, so we replace the quoted version.
	quoted := regexp.QuoteMeta(issuer)

	// Replace the quoted `*` with a regular expression that
	// will match alpha-numeric parts with common additional
	// "special" characters.
	replaced := strings.ReplaceAll(quoted, regexp.QuoteMeta("*"), "[-_a-zA-Z0-9]+")

	// Compile into a regular expression.
	return regexp.Compile(replaced)
}

// GetIssuer looks up the issuer configuration for an `issuerURL`
// coming from an incoming OIDC token.  If no matching configuration
// is found, then it returns `false`.
func (fc *FulcioConfig) GetIssuer(issuerURL string) (OIDCIssuer, bool) {
	iss, ok := fc.OIDCIssuers[issuerURL]
	if ok {
		return iss, ok
	}

	for meta, iss := range fc.MetaIssuers {
		re, err := metaRegex(meta)
		if err != nil {
			continue // Shouldn't happen, we check parsing the config
		}
		if re.MatchString(issuerURL) {
			// If it matches, then return a concrete OIDCIssuer
			// configuration for this issuer URL.
			return OIDCIssuer{
				IssuerURL:   issuerURL,
				ClientID:    iss.ClientID,
				Type:        iss.Type,
				IssuerClaim: iss.IssuerClaim,
			}, true
		}
	}

	return OIDCIssuer{}, false
}

// GetVerifier fetches a token verifier for the given `issuerURL`
// coming from an incoming OIDC token.  If no matching configuration
// is found, then it returns `false`.
func (fc *FulcioConfig) GetVerifier(issuerURL string) (*oidc.IDTokenVerifier, bool) {
	// Look up our fixed issuer verifiers
	v, ok := fc.verifiers[issuerURL]
	if ok {
		return v, true
	}

	// Look in the LRU cache for a verifier
	untyped, ok := fc.lru.Get(issuerURL)
	if ok {
		return untyped.(*oidc.IDTokenVerifier), true
	}
	// If this issuer hasn't been recently used, then create a new verifier
	// and add it to the LRU cache.

	iss, ok := fc.GetIssuer(issuerURL)
	if !ok {
		return nil, false
	}

	provider, err := oidc.NewProvider(context.Background(), issuerURL)
	if err != nil {
		log.Logger.Warnf("Failed to create provider for issuer URL %q: %v", issuerURL, err)
		return nil, false
	}
	verifier := provider.Verifier(&oidc.Config{ClientID: iss.ClientID})
	fc.lru.Add(issuerURL, verifier)
	return verifier, true
}

func (fc *FulcioConfig) prepare() error {
	fc.verifiers = make(map[string]*oidc.IDTokenVerifier, len(fc.OIDCIssuers))
	for _, iss := range fc.OIDCIssuers {
		provider, err := oidc.NewProvider(context.Background(), iss.IssuerURL)
		if err != nil {
			return err
		}
		fc.verifiers[iss.IssuerURL] = provider.Verifier(&oidc.Config{ClientID: iss.ClientID})
	}

	cache, err := lru.New2Q(100 /* size */)
	if err != nil {
		return err
	}
	fc.lru = cache
	return nil
}

type IssuerType string

const (
	IssuerTypeEmail          = "email"
	IssuerTypeGithubWorkflow = "github-workflow"
	IssuerTypeKubernetes     = "kubernetes"
	IssuerTypeSpiffe         = "spiffe"
)

func parseConfig(b []byte) (cfg *FulcioConfig, err error) {
	cfg = &FulcioConfig{}
	if err := json.Unmarshal(b, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

var DefaultConfig = &FulcioConfig{
	OIDCIssuers: map[string]OIDCIssuer{
		"https://oauth2.sigstore.dev/auth": {
			IssuerURL:   "https://oauth2.sigstore.dev/auth",
			ClientID:    "sigstore",
			IssuerClaim: "$.federated_claims.connector_id",
			Type:        IssuerTypeEmail,
		},
		"https://accounts.google.com": {
			IssuerURL: "https://accounts.google.com",
			ClientID:  "sigstore",
			Type:      IssuerTypeEmail,
		},
		"https://token.actions.githubusercontent.com": {
			IssuerURL: "https://token.actions.githubusercontent.com",
			ClientID:  "sigstore",
			Type:      IssuerTypeGithubWorkflow,
		},
	},
}

var originalTransport = http.DefaultTransport

type configKey struct{}

func With(ctx context.Context, cfg *FulcioConfig) context.Context {
	ctx = context.WithValue(ctx, configKey{}, cfg)
	return ctx
}

func FromContext(ctx context.Context) *FulcioConfig {
	untyped := ctx.Value(configKey{})
	if untyped == nil {
		return nil
	}
	return untyped.(*FulcioConfig)
}

// Load a config from disk, or use defaults
func Load(configPath string) (*FulcioConfig, error) {
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		log.Logger.Infof("No config at %s, using defaults: %v", configPath, DefaultConfig)
		config := DefaultConfig
		if err := config.prepare(); err != nil {
			return nil, err
		}
		return config, nil
	}
	b, err := ioutil.ReadFile(configPath)
	if err != nil {
		return nil, err
	}
	return Read(b)
}

// Read parses the bytes of a config
func Read(b []byte) (*FulcioConfig, error) {
	config, err := parseConfig(b)
	if err != nil {
		return nil, err
	}

	if _, ok := config.GetIssuer("https://kubernetes.default.svc"); ok {
		// Add the Kubernetes cluster's CA to the system CA pool, and to
		// the default transport.
		rootCAs, _ := x509.SystemCertPool()
		if rootCAs == nil {
			rootCAs = x509.NewCertPool()
		}
		const k8sCA = "/var/run/fulcio/ca.crt"
		certs, err := ioutil.ReadFile(k8sCA)
		if err != nil {
			return nil, err
		}
		if ok := rootCAs.AppendCertsFromPEM(certs); !ok {
			return nil, err
		}

		t := originalTransport.(*http.Transport).Clone()
		t.TLSClientConfig.RootCAs = rootCAs
		http.DefaultTransport = t
	} else {
		// If we parse a config that doesn't include a cluster issuer
		// signed with the cluster'sCA, then restore the original transport
		// (in case we overwrote it)
		http.DefaultTransport = originalTransport
	}

	if err := config.prepare(); err != nil {
		return nil, err
	}
	return config, nil
}
