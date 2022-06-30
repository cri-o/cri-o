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

package ca

import (
	"bytes"
	"context"
	"crypto/x509"
	"strings"

	"github.com/sigstore/fulcio/pkg/challenges"
	"github.com/sigstore/sigstore/pkg/cryptoutils"
)

type CodeSigningCertificate struct {
	Subject          *challenges.ChallengeResult
	FinalCertificate *x509.Certificate
	FinalChain       []*x509.Certificate
	finalPEM         []byte
	finalChainPEM    []byte
}

type CodeSigningPreCertificate struct {
	Subject *challenges.ChallengeResult
	PreCert *x509.Certificate
}

func CreateCSCFromPEM(subject *challenges.ChallengeResult, cert string, chain []string) (*CodeSigningCertificate, error) {
	c := &CodeSigningCertificate{
		Subject: subject,
	}

	// convert to X509 and store both formats
	finalCert, err := cryptoutils.UnmarshalCertificatesFromPEM([]byte(cert))
	if err != nil {
		return nil, err
	}
	c.finalPEM = []byte(cert)
	c.FinalCertificate = finalCert[0]

	// convert to X509 and store both formats
	chainBytes := []byte(strings.Join(chain, ""))
	if len(chainBytes) != 0 {
		c.FinalChain, err = cryptoutils.UnmarshalCertificatesFromPEM(chainBytes)
		if err != nil {
			return nil, err
		}
		c.finalChainPEM = chainBytes
	}
	return c, nil
}

func CreateCSCFromDER(subject *challenges.ChallengeResult, cert, chain []byte) (c *CodeSigningCertificate, err error) {
	c = &CodeSigningCertificate{
		Subject: subject,
	}

	// convert to X509 and store both formats
	c.finalPEM = cryptoutils.PEMEncode(cryptoutils.CertificatePEMType, cert)
	c.FinalCertificate, err = x509.ParseCertificate(cert)
	if err != nil {
		return nil, err
	}

	// convert to X509 and store both formats
	c.FinalChain, err = x509.ParseCertificates(chain)
	if err != nil {
		return nil, err
	}
	buf := bytes.Buffer{}
	for i, chainCert := range c.FinalChain {
		buf.Write(cryptoutils.PEMEncode(cryptoutils.CertificatePEMType, chainCert.Raw))
		if i != len(c.FinalChain) {
			buf.WriteRune('\n')
		}
	}
	c.finalChainPEM = buf.Bytes()
	return c, nil
}

func (c *CodeSigningCertificate) CertPEM() ([]byte, error) {
	var err error
	if c.finalPEM == nil {
		c.finalPEM, err = cryptoutils.MarshalCertificateToPEM(c.FinalCertificate)
	}
	return c.finalPEM, err
}

func (c *CodeSigningCertificate) ChainPEM() ([]byte, error) {
	var err error
	if c.finalChainPEM == nil && len(c.FinalChain) > 0 {
		c.finalChainPEM, err = cryptoutils.MarshalCertificatesToPEM(c.FinalChain)
	}
	return c.finalChainPEM, err
}

// CertificateAuthority only returns the SCT in detached format
type CertificateAuthority interface {
	CreateCertificate(ctx context.Context, challenge *challenges.ChallengeResult) (*CodeSigningCertificate, error)
	Root(ctx context.Context) ([]byte, error)
}

type EmbeddedSCTCA interface {
	CreatePrecertificate(ctx context.Context, challenge *challenges.ChallengeResult) (*CodeSigningPreCertificate, error)
	IssueFinalCertificate(ctx context.Context, precert *CodeSigningPreCertificate) (*CodeSigningCertificate, error)
}

// ValidationError indicates that there is an issue with the content in the HTTP Request that
// should result in an HTTP 400 Bad Request error being returned to the client
type ValidationError error
