/*
Copyright 2014 The Kubernetes Authors.

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

package authenticator

import (
	"time"

	"github.com/go-openapi/spec"

	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/authenticatorfactory"
	"k8s.io/apiserver/pkg/authentication/group"
	"k8s.io/apiserver/pkg/authentication/request/anonymous"
	"k8s.io/apiserver/pkg/authentication/request/bearertoken"
	"k8s.io/apiserver/pkg/authentication/request/headerrequest"
	"k8s.io/apiserver/pkg/authentication/request/union"
	"k8s.io/apiserver/pkg/authentication/request/x509"
	"k8s.io/apiserver/pkg/authentication/token/tokenfile"
	"k8s.io/apiserver/plugin/pkg/authenticator/password/keystone"
	"k8s.io/apiserver/plugin/pkg/authenticator/password/passwordfile"
	"k8s.io/apiserver/plugin/pkg/authenticator/request/basicauth"
	"k8s.io/apiserver/plugin/pkg/authenticator/token/anytoken"
	"k8s.io/apiserver/plugin/pkg/authenticator/token/oidc"
	"k8s.io/apiserver/plugin/pkg/authenticator/token/webhook"
	certutil "k8s.io/client-go/util/cert"
	"k8s.io/kubernetes/pkg/serviceaccount"

	// Initialize all known client auth plugins.
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
)

type AuthenticatorConfig struct {
	Anonymous                   bool
	AnyToken                    bool
	BasicAuthFile               string
	BootstrapToken              bool
	ClientCAFile                string
	TokenAuthFile               string
	OIDCIssuerURL               string
	OIDCClientID                string
	OIDCCAFile                  string
	OIDCUsernameClaim           string
	OIDCGroupsClaim             string
	ServiceAccountKeyFiles      []string
	ServiceAccountLookup        bool
	KeystoneURL                 string
	KeystoneCAFile              string
	WebhookTokenAuthnConfigFile string
	WebhookTokenAuthnCacheTTL   time.Duration

	RequestHeaderConfig *authenticatorfactory.RequestHeaderConfig

	// TODO, this is the only non-serializable part of the entire config.  Factor it out into a clientconfig
	ServiceAccountTokenGetter   serviceaccount.ServiceAccountTokenGetter
	BootstrapTokenAuthenticator authenticator.Token
}

// New returns an authenticator.Request or an error that supports the standard
// Kubernetes authentication mechanisms.
func (config AuthenticatorConfig) New() (authenticator.Request, *spec.SecurityDefinitions, error) {
	var authenticators []authenticator.Request
	securityDefinitions := spec.SecurityDefinitions{}
	hasBasicAuth := false
	hasTokenAuth := false

	// front-proxy, BasicAuth methods, local first, then remote
	// Add the front proxy authenticator if requested
	if config.RequestHeaderConfig != nil {
		requestHeaderAuthenticator, err := headerrequest.NewSecure(
			config.RequestHeaderConfig.ClientCA,
			config.RequestHeaderConfig.AllowedClientNames,
			config.RequestHeaderConfig.UsernameHeaders,
			config.RequestHeaderConfig.GroupHeaders,
			config.RequestHeaderConfig.ExtraHeaderPrefixes,
		)
		if err != nil {
			return nil, nil, err
		}
		authenticators = append(authenticators, requestHeaderAuthenticator)
	}

	if len(config.BasicAuthFile) > 0 {
		basicAuth, err := newAuthenticatorFromBasicAuthFile(config.BasicAuthFile)
		if err != nil {
			return nil, nil, err
		}
		authenticators = append(authenticators, basicAuth)
		hasBasicAuth = true
	}
	if len(config.KeystoneURL) > 0 {
		keystoneAuth, err := newAuthenticatorFromKeystoneURL(config.KeystoneURL, config.KeystoneCAFile)
		if err != nil {
			return nil, nil, err
		}
		authenticators = append(authenticators, keystoneAuth)
		hasBasicAuth = true
	}

	// X509 methods
	if len(config.ClientCAFile) > 0 {
		certAuth, err := newAuthenticatorFromClientCAFile(config.ClientCAFile)
		if err != nil {
			return nil, nil, err
		}
		authenticators = append(authenticators, certAuth)
	}

	// Bearer token methods, local first, then remote
	if len(config.TokenAuthFile) > 0 {
		tokenAuth, err := newAuthenticatorFromTokenFile(config.TokenAuthFile)
		if err != nil {
			return nil, nil, err
		}
		authenticators = append(authenticators, tokenAuth)
		hasTokenAuth = true
	}
	if len(config.ServiceAccountKeyFiles) > 0 {
		serviceAccountAuth, err := newServiceAccountAuthenticator(config.ServiceAccountKeyFiles, config.ServiceAccountLookup, config.ServiceAccountTokenGetter)
		if err != nil {
			return nil, nil, err
		}
		authenticators = append(authenticators, serviceAccountAuth)
		hasTokenAuth = true
	}
	if config.BootstrapToken {
		if config.BootstrapTokenAuthenticator != nil {
			// TODO: This can sometimes be nil because of
			authenticators = append(authenticators, bearertoken.New(config.BootstrapTokenAuthenticator))
			hasTokenAuth = true
		}
	}
	// NOTE(ericchiang): Keep the OpenID Connect after Service Accounts.
	//
	// Because both plugins verify JWTs whichever comes first in the union experiences
	// cache misses for all requests using the other. While the service account plugin
	// simply returns an error, the OpenID Connect plugin may query the provider to
	// update the keys, causing performance hits.
	if len(config.OIDCIssuerURL) > 0 && len(config.OIDCClientID) > 0 {
		oidcAuth, err := newAuthenticatorFromOIDCIssuerURL(config.OIDCIssuerURL, config.OIDCClientID, config.OIDCCAFile, config.OIDCUsernameClaim, config.OIDCGroupsClaim)
		if err != nil {
			return nil, nil, err
		}
		authenticators = append(authenticators, oidcAuth)
		hasTokenAuth = true
	}
	if len(config.WebhookTokenAuthnConfigFile) > 0 {
		webhookTokenAuth, err := newWebhookTokenAuthenticator(config.WebhookTokenAuthnConfigFile, config.WebhookTokenAuthnCacheTTL)
		if err != nil {
			return nil, nil, err
		}
		authenticators = append(authenticators, webhookTokenAuth)
		hasTokenAuth = true
	}

	// always add anytoken last, so that every other token authenticator gets to try first
	if config.AnyToken {
		authenticators = append(authenticators, bearertoken.New(anytoken.AnyTokenAuthenticator{}))
		hasTokenAuth = true
	}

	if hasBasicAuth {
		securityDefinitions["HTTPBasic"] = &spec.SecurityScheme{
			SecuritySchemeProps: spec.SecuritySchemeProps{
				Type:        "basic",
				Description: "HTTP Basic authentication",
			},
		}
	}

	if hasTokenAuth {
		securityDefinitions["BearerToken"] = &spec.SecurityScheme{
			SecuritySchemeProps: spec.SecuritySchemeProps{
				Type:        "apiKey",
				Name:        "authorization",
				In:          "header",
				Description: "Bearer Token authentication",
			},
		}
	}

	if len(authenticators) == 0 {
		if config.Anonymous {
			return anonymous.NewAuthenticator(), &securityDefinitions, nil
		}
	}

	switch len(authenticators) {
	case 0:
		return nil, &securityDefinitions, nil
	}

	authenticator := union.New(authenticators...)

	authenticator = group.NewAuthenticatedGroupAdder(authenticator)

	if config.Anonymous {
		// If the authenticator chain returns an error, return an error (don't consider a bad bearer token
		// or invalid username/password combination anonymous).
		authenticator = union.NewFailOnError(authenticator, anonymous.NewAuthenticator())
	}

	return authenticator, &securityDefinitions, nil
}

// IsValidServiceAccountKeyFile returns true if a valid public RSA key can be read from the given file
func IsValidServiceAccountKeyFile(file string) bool {
	_, err := serviceaccount.ReadPublicKeys(file)
	return err == nil
}

// newAuthenticatorFromBasicAuthFile returns an authenticator.Request or an error
func newAuthenticatorFromBasicAuthFile(basicAuthFile string) (authenticator.Request, error) {
	basicAuthenticator, err := passwordfile.NewCSV(basicAuthFile)
	if err != nil {
		return nil, err
	}

	return basicauth.New(basicAuthenticator), nil
}

// newAuthenticatorFromTokenFile returns an authenticator.Request or an error
func newAuthenticatorFromTokenFile(tokenAuthFile string) (authenticator.Request, error) {
	tokenAuthenticator, err := tokenfile.NewCSV(tokenAuthFile)
	if err != nil {
		return nil, err
	}

	return bearertoken.New(tokenAuthenticator), nil
}

// newAuthenticatorFromOIDCIssuerURL returns an authenticator.Request or an error.
func newAuthenticatorFromOIDCIssuerURL(issuerURL, clientID, caFile, usernameClaim, groupsClaim string) (authenticator.Request, error) {
	tokenAuthenticator, err := oidc.New(oidc.OIDCOptions{
		IssuerURL:     issuerURL,
		ClientID:      clientID,
		CAFile:        caFile,
		UsernameClaim: usernameClaim,
		GroupsClaim:   groupsClaim,
	})
	if err != nil {
		return nil, err
	}

	return bearertoken.New(tokenAuthenticator), nil
}

// newServiceAccountAuthenticator returns an authenticator.Request or an error
func newServiceAccountAuthenticator(keyfiles []string, lookup bool, serviceAccountGetter serviceaccount.ServiceAccountTokenGetter) (authenticator.Request, error) {
	allPublicKeys := []interface{}{}
	for _, keyfile := range keyfiles {
		publicKeys, err := serviceaccount.ReadPublicKeys(keyfile)
		if err != nil {
			return nil, err
		}
		allPublicKeys = append(allPublicKeys, publicKeys...)
	}

	tokenAuthenticator := serviceaccount.JWTTokenAuthenticator(allPublicKeys, lookup, serviceAccountGetter)
	return bearertoken.New(tokenAuthenticator), nil
}

// newAuthenticatorFromClientCAFile returns an authenticator.Request or an error
func newAuthenticatorFromClientCAFile(clientCAFile string) (authenticator.Request, error) {
	roots, err := certutil.NewPool(clientCAFile)
	if err != nil {
		return nil, err
	}

	opts := x509.DefaultVerifyOptions()
	opts.Roots = roots

	return x509.New(opts, x509.CommonNameUserConversion), nil
}

// newAuthenticatorFromKeystoneURL returns an authenticator.Request or an error
func newAuthenticatorFromKeystoneURL(keystoneURL string, keystoneCAFile string) (authenticator.Request, error) {
	keystoneAuthenticator, err := keystone.NewKeystoneAuthenticator(keystoneURL, keystoneCAFile)
	if err != nil {
		return nil, err
	}

	return basicauth.New(keystoneAuthenticator), nil
}

func newWebhookTokenAuthenticator(webhookConfigFile string, ttl time.Duration) (authenticator.Request, error) {
	webhookTokenAuthenticator, err := webhook.New(webhookConfigFile, ttl)
	if err != nil {
		return nil, err
	}

	return bearertoken.New(webhookTokenAuthenticator), nil
}
