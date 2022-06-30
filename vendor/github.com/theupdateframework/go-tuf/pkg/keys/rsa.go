package keys

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"

	"github.com/theupdateframework/go-tuf/data"
)

func init() {
	VerifierMap.Store(data.KeyTypeRSASSA_PSS_SHA256, NewRsaVerifier)
	SignerMap.Store(data.KeyTypeRSASSA_PSS_SHA256, NewRsaSigner)
}

func NewRsaVerifier() Verifier {
	return &rsaVerifier{}
}

func NewRsaSigner() Signer {
	return &rsaSigner{}
}

type rsaVerifier struct {
	PublicKey string `json:"public"`
	rsaKey    *rsa.PublicKey
	key       *data.PublicKey
}

func (p *rsaVerifier) Public() string {
	// Unique public key identifier, use a uniform encodng
	r, err := x509.MarshalPKIXPublicKey(p.rsaKey)
	if err != nil {
		// This shouldn't happen with a valid rsa key, but fallback on the
		// JSON public key string
		return string(p.PublicKey)
	}
	return string(r)
}

func (p *rsaVerifier) Verify(msg, sigBytes []byte) error {
	hash := sha256.Sum256(msg)

	return rsa.VerifyPSS(p.rsaKey, crypto.SHA256, hash[:], sigBytes, &rsa.PSSOptions{})
}

func (p *rsaVerifier) MarshalPublicKey() *data.PublicKey {
	return p.key
}

func (p *rsaVerifier) UnmarshalPublicKey(key *data.PublicKey) error {
	if err := json.Unmarshal(key.Value, p); err != nil {
		return err
	}
	var err error
	p.rsaKey, err = parseKey(p.PublicKey)
	if err != nil {
		return err
	}
	p.key = key
	return nil
}

// parseKey tries to parse a PEM []byte slice by attempting PKCS1 and PKIX in order.
func parseKey(data string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(data))
	if block == nil {
		return nil, errors.New("tuf: pem decoding public key failed")
	}
	rsaPub, err := x509.ParsePKCS1PublicKey(block.Bytes)
	if err == nil {
		return rsaPub, nil
	}
	key, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err == nil {
		rsaPub, ok := key.(*rsa.PublicKey)
		if !ok {
			return nil, errors.New("tuf: invalid rsa key")
		}
		return rsaPub, nil
	}
	return nil, errors.New("tuf: error unmarshalling rsa key")
}

type rsaSigner struct {
	*rsa.PrivateKey
}

type rsaPublic struct {
	// PEM encoded public key.
	PublicKey string `json:"public"`
}

func (s *rsaSigner) PublicData() *data.PublicKey {
	pub, _ := x509.MarshalPKIXPublicKey(s.Public().(*rsa.PublicKey))
	pubBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PUBLIC KEY",
		Bytes: pub,
	})

	keyValBytes, _ := json.Marshal(rsaPublic{PublicKey: string(pubBytes)})
	return &data.PublicKey{
		Type:       data.KeyTypeRSASSA_PSS_SHA256,
		Scheme:     data.KeySchemeRSASSA_PSS_SHA256,
		Algorithms: data.HashAlgorithms,
		Value:      keyValBytes,
	}
}

func (s *rsaSigner) SignMessage(message []byte) ([]byte, error) {
	hash := sha256.Sum256(message)
	return rsa.SignPSS(rand.Reader, s.PrivateKey, crypto.SHA256, hash[:], &rsa.PSSOptions{})
}

func (s *rsaSigner) ContainsID(id string) bool {
	return s.PublicData().ContainsID(id)
}

func (s *rsaSigner) MarshalPrivateKey() (*data.PrivateKey, error) {
	return nil, errors.New("not implemented for test")
}

func (s *rsaSigner) UnmarshalPrivateKey(key *data.PrivateKey) error {
	return errors.New("not implemented for test")
}

func GenerateRsaKey() (*rsaSigner, error) {
	privkey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	return &rsaSigner{privkey}, nil
}
