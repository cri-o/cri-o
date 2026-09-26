package cert_test

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/mldsa"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/cri-o/cri-o/internal/cert"
)

type testIdentity struct {
	key     crypto.Signer
	cert    *x509.Certificate
	certPEM []byte
	keyPEM  []byte
}

func generateKey(algorithm string) crypto.Signer {
	var (
		key crypto.Signer
		err error
	)

	switch algorithm {
	case "ECDSA":
		key, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	case "ML-DSA-44":
		key, err = mldsa.GenerateKey(mldsa.MLDSA44())
	case "ML-DSA-65":
		key, err = mldsa.GenerateKey(mldsa.MLDSA65())
	case "ML-DSA-87":
		key, err = mldsa.GenerateKey(mldsa.MLDSA87())
	default:
		Fail("unknown key algorithm " + algorithm)
	}

	Expect(err).NotTo(HaveOccurred())

	return key
}

// issue creates a certificate for a new key, self-signed if ca is nil.
func issue(algorithm string, serial int64, ca *testIdentity) *testIdentity {
	key := generateKey(algorithm)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(serial),
		Subject:      pkix.Name{CommonName: "cri-o cert test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}

	parent, signer := template, key
	if ca == nil {
		template.IsCA = true
		template.BasicConstraintsValid = true
		template.KeyUsage |= x509.KeyUsageCertSign
	} else {
		parent, signer = ca.cert, ca.key
	}

	der, err := x509.CreateCertificate(rand.Reader, template, parent, key.Public(), signer)
	Expect(err).NotTo(HaveOccurred())
	parsed, err := x509.ParseCertificate(der)
	Expect(err).NotTo(HaveOccurred())
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	Expect(err).NotTo(HaveOccurred())

	return &testIdentity{
		key:     key,
		cert:    parsed,
		certPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		keyPEM:  pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}),
	}
}

var _ = t.Describe("Config", func() {
	for _, algorithm := range []string{"ECDSA", "ML-DSA-44", "ML-DSA-65", "ML-DSA-87"} {
		It("should serve and reload "+algorithm+" certificates with mTLS", func() {
			dir := t.MustTempDir("cert")
			certPath := filepath.Join(dir, "tls.crt")
			keyPath := filepath.Join(dir, "tls.key")
			caPath := filepath.Join(dir, "ca.crt")

			ca := issue(algorithm, 1, nil)
			client := issue(algorithm, 2, ca)
			server := issue(algorithm, 3, ca)
			Expect(os.WriteFile(caPath, ca.certPEM, 0o600)).To(Succeed())
			Expect(os.WriteFile(certPath, server.certPEM, 0o600)).To(Succeed())
			Expect(os.WriteFile(keyPath, server.keyPEM, 0o600)).To(Succeed())

			doneChan := make(chan struct{})
			defer close(doneChan)

			cc, err := cert.NewCertConfig(
				context.Background(),
				doneChan,
				certPath,
				keyPath,
				caPath,
				tls.VersionTLS12,
				nil,
			)
			Expect(err).NotTo(HaveOccurred())

			srv := httptest.NewUnstartedServer(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusNoContent)
				}),
			)
			srv.TLS = &tls.Config{
				GetConfigForClient: cc.GetConfigForClient,
				MinVersion:         tls.VersionTLS12,
			}

			srv.StartTLS()
			defer srv.Close()

			roots := x509.NewCertPool()
			roots.AddCert(ca.cert)

			// connect returns the state of a new TLS connection to the server.
			connect := func(clientCerts []tls.Certificate, maxVersion uint16) (*tls.ConnectionState, error) {
				conn, err := tls.Dial("tcp", srv.Listener.Addr().String(), &tls.Config{
					RootCAs:      roots,
					Certificates: clientCerts,
					MinVersion:   tls.VersionTLS12,
					MaxVersion:   maxVersion,
				})
				if err != nil {
					return nil, err
				}
				defer conn.Close()

				// Complete the handshake, which includes the server verifying the client certificate.
				if _, err := conn.Write([]byte("GET / HTTP/1.0\r\n\r\n")); err != nil {
					return nil, err
				}

				if _, err := conn.Read(make([]byte, 1)); err != nil {
					return nil, err
				}

				state := conn.ConnectionState()

				return &state, nil
			}

			clientPair, err := tls.X509KeyPair(client.certPEM, client.keyPEM)
			Expect(err).NotTo(HaveOccurred())

			By("serving the initial certificate with a verified client certificate")

			state, err := connect([]tls.Certificate{clientPair}, 0)
			Expect(err).NotTo(HaveOccurred())
			Expect(state.Version).To(Equal(uint16(tls.VersionTLS13)))
			Expect(state.PeerCertificates[0].SerialNumber.Int64()).To(Equal(int64(3)))

			By("rejecting a client without a certificate")

			_, err = connect(nil, 0)
			Expect(err).To(MatchError(ContainSubstring("certificate required")))

			if algorithm == "ECDSA" {
				By("serving TLS 1.2 for ECDSA certificates")

				state, err = connect([]tls.Certificate{clientPair}, tls.VersionTLS12)
				Expect(err).NotTo(HaveOccurred())
				Expect(state.Version).To(Equal(uint16(tls.VersionTLS12)))
			} else {
				By("rejecting TLS 1.2, which does not support ML-DSA")

				_, err = connect([]tls.Certificate{clientPair}, tls.VersionTLS12)
				Expect(err).To(HaveOccurred())
			}

			By("reloading a replaced certificate")

			replacement := issue(algorithm, 4, ca)
			Expect(os.WriteFile(keyPath, replacement.keyPEM, 0o600)).To(Succeed())
			Expect(os.WriteFile(certPath, replacement.certPEM, 0o600)).To(Succeed())
			Eventually(func() (int64, error) {
				state, err := connect([]tls.Certificate{clientPair}, 0)
				if err != nil {
					return 0, err
				}

				return state.PeerCertificates[0].SerialNumber.Int64(), nil
			}).WithTimeout(10 * time.Second).WithPolling(100 * time.Millisecond).Should(Equal(int64(4)))
		})
	}
})
