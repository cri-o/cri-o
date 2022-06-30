package keys

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/asn1"
	"encoding/json"
	"errors"
	"math/big"

	"github.com/theupdateframework/go-tuf/data"
)

func init() {
	VerifierMap.Store(data.KeyTypeECDSA_SHA2_P256, NewEcdsaVerifier)
}

func NewEcdsaVerifier() Verifier {
	return &p256Verifier{}
}

type ecdsaSignature struct {
	R, S *big.Int
}

type p256Verifier struct {
	PublicKey data.HexBytes `json:"public"`
	key       *data.PublicKey
}

func (p *p256Verifier) Public() string {
	return p.PublicKey.String()
}

func (p *p256Verifier) Verify(msg, sigBytes []byte) error {
	x, y := elliptic.Unmarshal(elliptic.P256(), p.PublicKey)
	k := &ecdsa.PublicKey{
		Curve: elliptic.P256(),
		X:     x,
		Y:     y,
	}

	var sig ecdsaSignature
	if _, err := asn1.Unmarshal(sigBytes, &sig); err != nil {
		return err
	}

	hash := sha256.Sum256(msg)

	if !ecdsa.Verify(k, hash[:], sig.R, sig.S) {
		return errors.New("tuf: ecdsa signature verification failed")
	}
	return nil
}

func (p *p256Verifier) MarshalPublicKey() *data.PublicKey {
	return p.key
}

func (p *p256Verifier) UnmarshalPublicKey(key *data.PublicKey) error {
	if err := json.Unmarshal(key.Value, p); err != nil {
		return err
	}
	x, _ := elliptic.Unmarshal(elliptic.P256(), p.PublicKey)
	if x == nil {
		return errors.New("tuf: invalid ecdsa public key point")
	}
	p.key = key
	return nil
}
