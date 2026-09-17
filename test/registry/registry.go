package main

import (
	"crypto"
	"crypto/mldsa"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

var mldsaParameters = map[string]mldsa.Parameters{
	"ML-DSA-44": mldsa.MLDSA44(),
	"ML-DSA-65": mldsa.MLDSA65(),
	"ML-DSA-87": mldsa.MLDSA87(),
}

type identity struct {
	key  crypto.Signer
	cert *x509.Certificate
}

// issue creates a certificate for a new ML-DSA key, self-signed if ca is nil.
func issue(
	params mldsa.Parameters,
	serial int64,
	ca *identity,
	extKeyUsage x509.ExtKeyUsage,
) (*identity, error) {
	key, err := mldsa.GenerateKey(params)
	if err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(serial),
		Subject:      pkix.Name{CommonName: "cri-o test registry"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{extKeyUsage},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}

	parent, signer := template, crypto.Signer(key)
	if ca == nil {
		template.IsCA = true
		template.BasicConstraintsValid = true
		template.KeyUsage |= x509.KeyUsageCertSign
		template.ExtKeyUsage = nil
	} else {
		parent, signer = ca.cert, ca.key
	}

	der, err := x509.CreateCertificate(rand.Reader, template, parent, key.Public(), signer)
	if err != nil {
		return nil, fmt.Errorf("create certificate: %w", err)
	}

	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, fmt.Errorf("parse certificate: %w", err)
	}

	return &identity{key: key, cert: cert}, nil
}

func (i *identity) write(certPath, keyPath string) error {
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: i.cert.Raw})
	if err := os.WriteFile(certPath, certPEM, 0o644); err != nil {
		return fmt.Errorf("write certificate: %w", err)
	}

	if keyPath == "" {
		return nil
	}

	keyDER, err := x509.MarshalPKCS8PrivateKey(i.key)
	if err != nil {
		return fmt.Errorf("marshal key: %w", err)
	}

	if err := os.WriteFile(
		keyPath,
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}),
		0o600,
	); err != nil {
		return fmt.Errorf("write key: %w", err)
	}

	return nil
}

// generateCerts writes a CA, a registry server certificate and a client
// certificate, using the file names expected in a certs.d directory.
func generateCerts(c *cli.Context) error {
	params, ok := mldsaParameters[c.String("algorithm")]
	if !ok {
		return fmt.Errorf("unsupported algorithm %q", c.String("algorithm"))
	}

	dir := c.String("dir")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	ca, err := issue(params, 1, nil, 0)
	if err != nil {
		return err
	}

	server, err := issue(params, 2, ca, x509.ExtKeyUsageServerAuth)
	if err != nil {
		return err
	}

	client, err := issue(params, 3, ca, x509.ExtKeyUsageClientAuth)
	if err != nil {
		return err
	}

	return errors.Join(
		ca.write(filepath.Join(dir, "ca.crt"), ""),
		server.write(filepath.Join(dir, "server.crt"), filepath.Join(dir, "server.key")),
		client.write(filepath.Join(dir, "client.cert"), filepath.Join(dir, "client.key")),
	)
}

func keyDescription(key any) string {
	if k, ok := key.(*mldsa.PublicKey); ok {
		return k.Parameters().String()
	}

	return fmt.Sprintf("%T", key)
}

// serve runs an in-memory registry over TLS, which requires verified client
// certificates if a client CA is given, and logs each TLS handshake.
func serve(c *cli.Context) error {
	certificate, err := tls.LoadX509KeyPair(c.String("tls-cert"), c.String("tls-key"))
	if err != nil {
		return fmt.Errorf("load server certificate: %w", err)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{certificate},
		MinVersion:   tls.VersionTLS12,
		VerifyConnection: func(cs tls.ConnectionState) error {
			clientKey := "none"
			if len(cs.PeerCertificates) > 0 {
				clientKey = keyDescription(cs.PeerCertificates[0].PublicKey)
			}

			logrus.Infof(
				"TLS handshake: version=%s key-exchange=%s client-certificate=%s verified-client-chains=%d",
				tls.VersionName(cs.Version),
				cs.CurveID,
				clientKey,
				len(cs.VerifiedChains),
			)

			return nil
		},
	}

	if clientCA := c.String("client-ca"); clientCA != "" {
		caPEM, err := os.ReadFile(clientCA)
		if err != nil {
			return fmt.Errorf("read client CA: %w", err)
		}

		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caPEM) {
			return errors.New("no certificates found in client CA")
		}

		tlsConfig.ClientCAs = pool
		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
	}

	leaf, err := x509.ParseCertificate(certificate.Certificate[0])
	if err != nil {
		return fmt.Errorf("parse server certificate: %w", err)
	}

	server := &http.Server{
		Addr:              c.String("address"),
		Handler:           registry.New(),
		TLSConfig:         tlsConfig,
		ReadHeaderTimeout: 30 * time.Second,
		ErrorLog:          log.New(logrus.StandardLogger().WriterLevel(logrus.WarnLevel), "", 0),
	}

	logrus.Infof(
		"Serving registry on https://%s with %s server certificate (client certificates required: %t)",
		server.Addr,
		keyDescription(leaf.PublicKey),
		tlsConfig.ClientCAs != nil,
	)

	return server.ListenAndServeTLS("", "")
}

func main() {
	app := cli.NewApp()
	app.Name = "registry"
	app.Usage = "in-memory TLS container registry for integration tests"
	app.Commands = []*cli.Command{
		{
			Name:   "generate-certs",
			Usage:  "generate ML-DSA CA, server and client certificates",
			Action: generateCerts,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:  "algorithm",
					Value: "ML-DSA-65",
					Usage: "ML-DSA-44, ML-DSA-65 or ML-DSA-87",
				},
				&cli.StringFlag{Name: "dir", Required: true, Usage: "output directory"},
			},
		},
		{
			Name:   "serve",
			Usage:  "serve the registry",
			Action: serve,
			Flags: []cli.Flag{
				&cli.StringFlag{Name: "address", Value: "127.0.0.1:5000", Usage: "listen address"},
				&cli.StringFlag{Name: "tls-cert", Required: true, Usage: "server certificate"},
				&cli.StringFlag{Name: "tls-key", Required: true, Usage: "server private key"},
				&cli.StringFlag{
					Name:  "client-ca",
					Usage: "require client certificates issued by this CA",
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		logrus.Fatal(err)
	}
}
