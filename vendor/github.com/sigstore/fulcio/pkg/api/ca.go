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

package api

import (
	"context"
	"crypto"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
	certauth "github.com/sigstore/fulcio/pkg/ca"
	"github.com/sigstore/fulcio/pkg/challenges"
	"github.com/sigstore/fulcio/pkg/config"
	"github.com/sigstore/fulcio/pkg/ctl"
	"github.com/sigstore/fulcio/pkg/log"
	"github.com/sigstore/sigstore/pkg/cryptoutils"
)

type Key struct {
	// +required
	Content   []byte `json:"content"`
	Algorithm string `json:"algorithm,omitempty"`
}

type CertificateRequest struct {
	// +required
	PublicKey Key `json:"publicKey"`

	// +required
	SignedEmailAddress []byte `json:"signedEmailAddress"`
}

const (
	signingCertPath = "/api/v1/signingCert"
	rootCertPath    = "/api/v1/rootCert"
)

// NewHandler creates a new http.Handler for serving the Fulcio API.
func NewHandler() http.Handler {
	handler := http.NewServeMux()
	handler.HandleFunc(signingCertPath, signingCert)
	handler.HandleFunc(rootCertPath, rootCert)
	return handler
}

func extractIssuer(token string) (string, error) {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return "", fmt.Errorf("oidc: malformed jwt, expected 3 parts got %d", len(parts))
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("oidc: malformed jwt payload: %w", err)
	}
	var payload struct {
		Issuer string `json:"iss"`
	}

	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", fmt.Errorf("oidc: failed to unmarshal claims: %w", err)
	}
	return payload.Issuer, nil
}

// We do this to bypass needing actual OIDC tokens for unit testing.
var authorize = actualAuthorize

func actualAuthorize(req *http.Request) (*oidc.IDToken, error) {
	// Strip off the "Bearer" prefix.
	token := strings.Replace(req.Header.Get("Authorization"), "Bearer ", "", 1)

	issuer, err := extractIssuer(token)
	if err != nil {
		return nil, err
	}

	verifier, ok := config.FromContext(req.Context()).GetVerifier(issuer)
	if !ok {
		return nil, fmt.Errorf("unsupported issuer: %s", issuer)
	}
	return verifier.Verify(req.Context(), token)
}

func signingCert(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		err := fmt.Errorf("signing cert handler must receive POSTs, got %s", req.Method)
		handleFulcioAPIError(w, req, http.StatusMethodNotAllowed, err, err.Error())
		return
	}
	if gotContentType, wantContentType := req.Header.Get("Content-Type"), "application/json"; gotContentType != wantContentType {
		err := fmt.Errorf("signing cert handler must receive %q, got %q", wantContentType, gotContentType)
		handleFulcioAPIError(w, req, http.StatusUnsupportedMediaType, err, err.Error())
		return
	}

	ctx := req.Context()
	logger := log.ContextLogger(ctx)

	principal, err := authorize(req)
	if err != nil {
		handleFulcioAPIError(w, req, http.StatusUnauthorized, err, invalidCredentials)
		return
	}

	// Parse the request body.
	cr := CertificateRequest{}
	if err := json.NewDecoder(req.Body).Decode(&cr); err != nil {
		handleFulcioAPIError(w, req, http.StatusBadRequest, err, invalidCertificateRequest)
		return
	}

	publicKeyBytes := cr.PublicKey.Content
	// try to unmarshal as DER
	publicKey, err := x509.ParsePKIXPublicKey(publicKeyBytes)
	if err != nil {
		// try to unmarshal as PEM
		logger.Debugf("error parsing public key as DER, trying pem: %v", err.Error())
		publicKey, err = cryptoutils.UnmarshalPEMToPublicKey(publicKeyBytes)
		if err != nil {
			handleFulcioAPIError(w, req, http.StatusBadRequest, err, invalidPublicKey)
			return
		}
	}

	subject, err := ExtractSubject(ctx, principal, publicKey, cr.SignedEmailAddress)
	if err != nil {
		handleFulcioAPIError(w, req, http.StatusBadRequest, err, invalidSignature)
		return
	}

	ca := GetCA(ctx)

	var csc *certauth.CodeSigningCertificate
	var sctBytes []byte
	// TODO: prefer embedding SCT if possible
	if _, ok := ca.(certauth.EmbeddedSCTCA); !ok {
		// currently configured CA doesn't support pre-certificate flow required to embed SCT in final certificate
		csc, err = ca.CreateCertificate(ctx, subject)
		if err != nil {
			// if the error was due to invalid input in the request, return HTTP 400
			if _, ok := err.(certauth.ValidationError); ok {
				handleFulcioAPIError(w, req, http.StatusBadRequest, err, err.Error())
				return
			}
			// otherwise return a 500 error to reflect that it is a transient server issue that the client can't resolve
			handleFulcioAPIError(w, req, http.StatusInternalServerError, err, genericCAError)
			return
		}

		// TODO: initialize CTL client once
		// Submit to CTL
		logger.Info("Submitting CTL inclusion for OIDC grant: ", subject.Value)
		ctURL := GetCTLogURL(ctx)
		if ctURL != "" {
			c := ctl.New(ctURL)
			sct, err := c.AddChain(csc)
			if err != nil {
				handleFulcioAPIError(w, req, http.StatusInternalServerError, err, fmt.Sprintf(failedToEnterCertInCTL, ctURL))
				return
			}
			sctBytes, err = json.Marshal(sct)
			if err != nil {
				handleFulcioAPIError(w, req, http.StatusInternalServerError, err, failedToMarshalSCT)
				return
			}
			logger.Info("CTL Submission Signature Received: ", sct.Signature)
			logger.Info("CTL Submission ID Received: ", sct.ID)
		} else {
			logger.Info("Skipping CT log upload.")
		}
	}

	metricNewEntries.Inc()

	var ret strings.Builder
	finalPEM, err := csc.CertPEM()
	if err != nil {
		handleFulcioAPIError(w, req, http.StatusInternalServerError, err, failedToMarshalCert)
		return
	}
	fmt.Fprintf(&ret, "%s\n", finalPEM)
	finalChainPEM, err := csc.ChainPEM()
	if err != nil {
		handleFulcioAPIError(w, req, http.StatusInternalServerError, err, failedToMarshalCert)
		return
	}
	if len(finalChainPEM) > 0 {
		fmt.Fprintf(&ret, "%s\n", finalChainPEM)
	}

	// Set the SCT and Content-Type headers, and then respond with a 201 Created.
	w.Header().Add("SCT", base64.StdEncoding.EncodeToString(sctBytes))
	w.Header().Add("Content-Type", "application/pem-certificate-chain")
	w.WriteHeader(http.StatusCreated)
	// Write the PEM encoded certificate chain to the response body.
	if _, err := w.Write([]byte(strings.TrimSpace(ret.String()))); err != nil {
		logger.Error("Error writing response: ", err)
	}
}

func rootCert(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	logger := log.ContextLogger(ctx)

	ca := GetCA(ctx)
	root, err := ca.Root(ctx)
	if err != nil {
		logger.Error("Error retrieving root cert: ", err)
	}
	w.Header().Add("Content-Type", "application/pem-certificate-chain")
	if _, err := w.Write(root); err != nil {
		logger.Error("Error writing response: ", err)
	}
}

func ExtractSubject(ctx context.Context, tok *oidc.IDToken, publicKey crypto.PublicKey, challenge []byte) (*challenges.ChallengeResult, error) {
	iss, ok := config.FromContext(ctx).GetIssuer(tok.Issuer)
	if !ok {
		return nil, fmt.Errorf("configuration can not be loaded for issuer %v", tok.Issuer)
	}
	switch iss.Type {
	case config.IssuerTypeEmail:
		return challenges.Email(ctx, tok, publicKey, challenge)
	case config.IssuerTypeSpiffe:
		return challenges.Spiffe(ctx, tok, publicKey, challenge)
	case config.IssuerTypeGithubWorkflow:
		return challenges.GithubWorkflow(ctx, tok, publicKey, challenge)
	case config.IssuerTypeKubernetes:
		return challenges.Kubernetes(ctx, tok, publicKey, challenge)
	default:
		return nil, fmt.Errorf("unsupported issuer: %s", iss.Type)
	}
}

type caKey struct{}

// WithCA associates the provided certificate authority with the provided context.
func WithCA(ctx context.Context, ca certauth.CertificateAuthority) context.Context {
	return context.WithValue(ctx, caKey{}, ca)
}

// GetCA accesses the certificate authority associated with the provided context.
func GetCA(ctx context.Context) certauth.CertificateAuthority {
	untyped := ctx.Value(caKey{})
	if untyped == nil {
		return nil
	}
	return untyped.(certauth.CertificateAuthority)
}

type ctKey struct{}

// WithCTLogURL associates the provided certificate transparency log URL with
// the provided context.
func WithCTLogURL(ctx context.Context, ct string) context.Context {
	return context.WithValue(ctx, ctKey{}, ct)
}

// GetCTLogURL accesses the certificate transparency log URL associated with
// the provided context.
func GetCTLogURL(ctx context.Context) string {
	untyped := ctx.Value(ctKey{})
	if untyped == nil {
		return ""
	}
	return untyped.(string)
}
