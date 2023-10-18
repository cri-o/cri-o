// Copyright 2020 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package piv

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"strconv"
	"strings"
)

// errMismatchingAlgorithms is returned when a cryptographic operation
// is given keys using different algorithms.
var errMismatchingAlgorithms = errors.New("mismatching key algorithms")

// errUnsupportedKeySize is returned when a key has an unsupported size
var errUnsupportedKeySize = errors.New("unsupported key size")

// unsupportedCurveError is used when a key has an unsupported curve
type unsupportedCurveError struct {
	curve int
}

func (e unsupportedCurveError) Error() string {
	return fmt.Sprintf("unsupported curve: %d", e.curve)
}

// Slot is a private key and certificate combination managed by the security key.
type Slot struct {
	// Key is a reference for a key type.
	//
	// See: https://nvlpubs.nist.gov/nistpubs/SpecialPublications/NIST.SP.800-73-4.pdf#page=32
	Key uint32
	// Object is a reference for data object.
	//
	// See: https://nvlpubs.nist.gov/nistpubs/SpecialPublications/NIST.SP.800-73-4.pdf#page=30
	Object uint32
}

var (
	extIDFirmwareVersion = asn1.ObjectIdentifier([]int{1, 3, 6, 1, 4, 1, 41482, 3, 3})
	extIDSerialNumber    = asn1.ObjectIdentifier([]int{1, 3, 6, 1, 4, 1, 41482, 3, 7})
	extIDKeyPolicy       = asn1.ObjectIdentifier([]int{1, 3, 6, 1, 4, 1, 41482, 3, 8})
	extIDFormFactor      = asn1.ObjectIdentifier([]int{1, 3, 6, 1, 4, 1, 41482, 3, 9})
)

// Version encodes a major, minor, and patch version.
type Version struct {
	Major int
	Minor int
	Patch int
}

// Formfactor enumerates the physical set of forms a key can take. USB-A vs.
// USB-C and Keychain vs. Nano (and FIPS variants for these).
type Formfactor int

// The mapping between known Formfactor values and their descriptions.
var formFactorStrings = map[Formfactor]string{
	FormfactorUSBAKeychain:          "USB-A Keychain",
	FormfactorUSBANano:              "USB-A Nano",
	FormfactorUSBCKeychain:          "USB-C Keychain",
	FormfactorUSBCNano:              "USB-C Nano",
	FormfactorUSBCLightningKeychain: "USB-C/Lightning Keychain",

	FormfactorUSBAKeychainFIPS:          "USB-A Keychain FIPS",
	FormfactorUSBANanoFIPS:              "USB-A Nano FIPS",
	FormfactorUSBCKeychainFIPS:          "USB-C Keychain FIPS",
	FormfactorUSBCNanoFIPS:              "USB-C Nano FIPS",
	FormfactorUSBCLightningKeychainFIPS: "USB-C/Lightning Keychain FIPS",
}

// String returns the human-readable description for the given form-factor
// value, or a fallback value for any other, unknown form-factor.
func (f Formfactor) String() string {
	if s, ok := formFactorStrings[f]; ok {
		return s
	}
	return fmt.Sprintf("unknown(0x%02x)", int(f))
}

// Formfactors recognized by this package. See the reference for more information:
// https://developers.yubico.com/yubikey-manager/Config_Reference.html#_form_factor
const (
	FormfactorUSBAKeychain          = 0x1
	FormfactorUSBANano              = 0x2
	FormfactorUSBCKeychain          = 0x3
	FormfactorUSBCNano              = 0x4
	FormfactorUSBCLightningKeychain = 0x5

	FormfactorUSBAKeychainFIPS          = 0x81
	FormfactorUSBANanoFIPS              = 0x82
	FormfactorUSBCKeychainFIPS          = 0x83
	FormfactorUSBCNanoFIPS              = 0x84
	FormfactorUSBCLightningKeychainFIPS = 0x85
)

// Prefix in the x509 Subject Common Name for YubiKey attestations
// https://developers.yubico.com/PIV/Introduction/PIV_attestation.html
const yubikeySubjectCNPrefix = "YubiKey PIV Attestation "

// Attestation returns additional information about a key attested to be generated
// on a card. See https://developers.yubico.com/PIV/Introduction/PIV_attestation.html
// for more information.
type Attestation struct {
	// Version of the YubiKey's firmware.
	Version Version
	// Serial is the YubiKey's serial number.
	Serial uint32
	// Formfactor indicates the physical type of the YubiKey.
	//
	// Formfactor may be empty Formfactor(0) for some YubiKeys.
	Formfactor Formfactor

	// PINPolicy set on the slot.
	PINPolicy PINPolicy
	// TouchPolicy set on the slot.
	TouchPolicy TouchPolicy

	// Slot is the inferred slot the attested key resides in based on the
	// common name in the attestation. If the slot cannot be determined,
	// this field will be an empty struct.
	Slot Slot
}

func (a *Attestation) addExt(e pkix.Extension) error {
	if e.Id.Equal(extIDFirmwareVersion) {
		if len(e.Value) != 3 {
			return fmt.Errorf("expected 3 bytes for firmware version, got: %d", len(e.Value))
		}
		a.Version = Version{
			Major: int(e.Value[0]),
			Minor: int(e.Value[1]),
			Patch: int(e.Value[2]),
		}
	} else if e.Id.Equal(extIDSerialNumber) {
		var serial int64
		if _, err := asn1.Unmarshal(e.Value, &serial); err != nil {
			return fmt.Errorf("parsing serial number: %v", err)
		}
		if serial < 0 {
			return fmt.Errorf("serial number was negative: %d", serial)
		}
		a.Serial = uint32(serial)
	} else if e.Id.Equal(extIDKeyPolicy) {
		if len(e.Value) != 2 {
			return fmt.Errorf("expected 2 bytes from key policy, got: %d", len(e.Value))
		}
		switch e.Value[0] {
		case 0x01:
			a.PINPolicy = PINPolicyNever
		case 0x02:
			a.PINPolicy = PINPolicyOnce
		case 0x03:
			a.PINPolicy = PINPolicyAlways
		default:
			return fmt.Errorf("unrecognized pin policy: 0x%x", e.Value[0])
		}
		switch e.Value[1] {
		case 0x01:
			a.TouchPolicy = TouchPolicyNever
		case 0x02:
			a.TouchPolicy = TouchPolicyAlways
		case 0x03:
			a.TouchPolicy = TouchPolicyCached
		default:
			return fmt.Errorf("unrecognized touch policy: 0x%x", e.Value[1])
		}
	} else if e.Id.Equal(extIDFormFactor) {
		if len(e.Value) != 1 {
			return fmt.Errorf("expected 1 byte from formfactor, got: %d", len(e.Value))
		}
		a.Formfactor = Formfactor(e.Value[0])
	}
	return nil
}

// Verify proves that a key was generated on a YubiKey. It ensures the slot and
// YubiKey certificate chains up to the Yubico CA, parsing additional information
// out of the slot certificate, such as the touch and PIN policies of a key.
func Verify(attestationCert, slotCert *x509.Certificate) (*Attestation, error) {
	var v verifier
	return v.Verify(attestationCert, slotCert)
}

type verifier struct {
	Roots *x509.CertPool
}

func (v *verifier) Verify(attestationCert, slotCert *x509.Certificate) (*Attestation, error) {
	o := x509.VerifyOptions{KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny}}
	o.Roots = v.Roots
	if o.Roots == nil {
		cas, err := yubicoCAs()
		if err != nil {
			return nil, fmt.Errorf("failed to load yubico CAs: %v", err)
		}
		o.Roots = cas
	}

	o.Intermediates = x509.NewCertPool()

	// The attestation cert in some yubikey 4 does not encode X509v3 Basic Constraints.
	// This isn't valid as per https://datatracker.ietf.org/doc/html/rfc5280#section-4.2.1.9
	// (fourth paragraph) and thus makes x509.go validation fail.
	// Work around this by setting this constraint here.
	if !attestationCert.BasicConstraintsValid {
		attestationCert.BasicConstraintsValid = true
		attestationCert.IsCA = true
	}

	o.Intermediates.AddCert(attestationCert)

	_, err := slotCert.Verify(o)
	if err != nil {
		return nil, fmt.Errorf("error verifying attestation certificate: %v", err)
	}
	return parseAttestation(slotCert)
}

func parseAttestation(slotCert *x509.Certificate) (*Attestation, error) {
	var a Attestation
	for _, ext := range slotCert.Extensions {
		if err := a.addExt(ext); err != nil {
			return nil, fmt.Errorf("parsing extension: %v", err)
		}
	}

	slot, ok := parseSlot(slotCert.Subject.CommonName)
	if ok {
		a.Slot = slot
	}

	return &a, nil
}

func parseSlot(commonName string) (Slot, bool) {
	if !strings.HasPrefix(commonName, yubikeySubjectCNPrefix) {
		return Slot{}, false
	}

	slotName := strings.TrimPrefix(commonName, yubikeySubjectCNPrefix)
	key, err := strconv.ParseUint(slotName, 16, 32)
	if err != nil {
		return Slot{}, false
	}

	switch uint32(key) {
	case SlotAuthentication.Key:
		return SlotAuthentication, true
	case SlotSignature.Key:
		return SlotSignature, true
	case SlotCardAuthentication.Key:
		return SlotCardAuthentication, true
	case SlotKeyManagement.Key:
		return SlotKeyManagement, true
	}

	return RetiredKeyManagementSlot(uint32(key))
}

// yubicoPIVCAPEMAfter2018 is the PEM encoded attestation certificate used by Yubico.
//
// https://developers.yubico.com/PIV/Introduction/PIV_attestation.html
const yubicoPIVCAPEMAfter2018 = `-----BEGIN CERTIFICATE-----
MIIDFzCCAf+gAwIBAgIDBAZHMA0GCSqGSIb3DQEBCwUAMCsxKTAnBgNVBAMMIFl1
YmljbyBQSVYgUm9vdCBDQSBTZXJpYWwgMjYzNzUxMCAXDTE2MDMxNDAwMDAwMFoY
DzIwNTIwNDE3MDAwMDAwWjArMSkwJwYDVQQDDCBZdWJpY28gUElWIFJvb3QgQ0Eg
U2VyaWFsIDI2Mzc1MTCCASIwDQYJKoZIhvcNAQEBBQADggEPADCCAQoCggEBAMN2
cMTNR6YCdcTFRxuPy31PabRn5m6pJ+nSE0HRWpoaM8fc8wHC+Tmb98jmNvhWNE2E
ilU85uYKfEFP9d6Q2GmytqBnxZsAa3KqZiCCx2LwQ4iYEOb1llgotVr/whEpdVOq
joU0P5e1j1y7OfwOvky/+AXIN/9Xp0VFlYRk2tQ9GcdYKDmqU+db9iKwpAzid4oH
BVLIhmD3pvkWaRA2H3DA9t7H/HNq5v3OiO1jyLZeKqZoMbPObrxqDg+9fOdShzgf
wCqgT3XVmTeiwvBSTctyi9mHQfYd2DwkaqxRnLbNVyK9zl+DzjSGp9IhVPiVtGet
X02dxhQnGS7K6BO0Qe8CAwEAAaNCMEAwHQYDVR0OBBYEFMpfyvLEojGc6SJf8ez0
1d8Cv4O/MA8GA1UdEwQIMAYBAf8CAQEwDgYDVR0PAQH/BAQDAgEGMA0GCSqGSIb3
DQEBCwUAA4IBAQBc7Ih8Bc1fkC+FyN1fhjWioBCMr3vjneh7MLbA6kSoyWF70N3s
XhbXvT4eRh0hvxqvMZNjPU/VlRn6gLVtoEikDLrYFXN6Hh6Wmyy1GTnspnOvMvz2
lLKuym9KYdYLDgnj3BeAvzIhVzzYSeU77/Cupofj093OuAswW0jYvXsGTyix6B3d
bW5yWvyS9zNXaqGaUmP3U9/b6DlHdDogMLu3VLpBB9bm5bjaKWWJYgWltCVgUbFq
Fqyi4+JE014cSgR57Jcu3dZiehB6UtAPgad9L5cNvua/IWRmm+ANy3O2LH++Pyl8
SREzU8onbBsjMg9QDiSf5oJLKvd/Ren+zGY7
-----END CERTIFICATE-----`

// Yubikeys manufactured sometime in 2018 and prior to mid-2017
// were certified using the U2F root CA with serial number 457200631
// See https://github.com/Yubico/developers.yubico.com/pull/392/commits/a58f1003f003e04fc9baf09cad9f64f0c284fd47
// Cert available at https://developers.yubico.com/U2F/yubico-u2f-ca-certs.txt
const yubicoPIVCAPEMU2F = `-----BEGIN CERTIFICATE-----
MIIDHjCCAgagAwIBAgIEG0BT9zANBgkqhkiG9w0BAQsFADAuMSwwKgYDVQQDEyNZ
dWJpY28gVTJGIFJvb3QgQ0EgU2VyaWFsIDQ1NzIwMDYzMTAgFw0xNDA4MDEwMDAw
MDBaGA8yMDUwMDkwNDAwMDAwMFowLjEsMCoGA1UEAxMjWXViaWNvIFUyRiBSb290
IENBIFNlcmlhbCA0NTcyMDA2MzEwggEiMA0GCSqGSIb3DQEBAQUAA4IBDwAwggEK
AoIBAQC/jwYuhBVlqaiYWEMsrWFisgJ+PtM91eSrpI4TK7U53mwCIawSDHy8vUmk
5N2KAj9abvT9NP5SMS1hQi3usxoYGonXQgfO6ZXyUA9a+KAkqdFnBnlyugSeCOep
8EdZFfsaRFtMjkwz5Gcz2Py4vIYvCdMHPtwaz0bVuzneueIEz6TnQjE63Rdt2zbw
nebwTG5ZybeWSwbzy+BJ34ZHcUhPAY89yJQXuE0IzMZFcEBbPNRbWECRKgjq//qT
9nmDOFVlSRCt2wiqPSzluwn+v+suQEBsUjTGMEd25tKXXTkNW21wIWbxeSyUoTXw
LvGS6xlwQSgNpk2qXYwf8iXg7VWZAgMBAAGjQjBAMB0GA1UdDgQWBBQgIvz0bNGJ
hjgpToksyKpP9xv9oDAPBgNVHRMECDAGAQH/AgEAMA4GA1UdDwEB/wQEAwIBBjAN
BgkqhkiG9w0BAQsFAAOCAQEAjvjuOMDSa+JXFCLyBKsycXtBVZsJ4Ue3LbaEsPY4
MYN/hIQ5ZM5p7EjfcnMG4CtYkNsfNHc0AhBLdq45rnT87q/6O3vUEtNMafbhU6kt
hX7Y+9XFN9NpmYxr+ekVY5xOxi8h9JDIgoMP4VB1uS0aunL1IGqrNooL9mmFnL2k
LVVee6/VR6C5+KSTCMCWppMuJIZII2v9o4dkoZ8Y7QRjQlLfYzd3qGtKbw7xaF1U
sG/5xUb/Btwb2X2g4InpiB/yt/3CpQXpiWX/K4mBvUKiGn05ZsqeY1gx4g0xLBqc
U9psmyPzK+Vsgw2jeRQ5JlKDyqE0hebfC1tvFu0CCrJFcw==
-----END CERTIFICATE-----`

func yubicoCAs() (*x509.CertPool, error) {
	certPool := x509.NewCertPool()

	if !certPool.AppendCertsFromPEM([]byte(yubicoPIVCAPEMAfter2018)) {
		return nil, fmt.Errorf("failed to parse yubico cert")
	}

	bU2F, _ := pem.Decode([]byte(yubicoPIVCAPEMU2F))
	if bU2F == nil {
		return nil, fmt.Errorf("failed to decode yubico pem data")
	}

	certU2F, err := x509.ParseCertificate(bU2F.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse yubico cert: %v", err)
	}

	// The U2F root cert has pathlen x509 basic constraint set to 0.
	// As per RFC 5280 this means that no intermediate cert is allowed
	// in the validation path. This isn't really helpful since we do
	// want to use the device attestation cert as intermediate cert in
	// the chain. To make this work, set pathlen of the U2F root to 1.
	//
	// See https://datatracker.ietf.org/doc/html/rfc5280#section-4.2.1.9
	certU2F.MaxPathLen = 1
	certPool.AddCert(certU2F)

	return certPool, nil
}

// Slot combinations pre-defined by this package.
//
// Object IDs are specified in NIST 800-73-4 section 4.3:
// https://nvlpubs.nist.gov/nistpubs/SpecialPublications/NIST.SP.800-73-4.pdf#page=30
//
// Key IDs are specified in NIST 800-73-4 section 5.1:
// https://nvlpubs.nist.gov/nistpubs/SpecialPublications/NIST.SP.800-73-4.pdf#page=32
var (
	SlotAuthentication     = Slot{0x9a, 0x5fc105}
	SlotSignature          = Slot{0x9c, 0x5fc10a}
	SlotCardAuthentication = Slot{0x9e, 0x5fc101}
	SlotKeyManagement      = Slot{0x9d, 0x5fc10b}

	slotAttestation = Slot{0xf9, 0x5fff01}
)

var retiredKeyManagementSlots = map[uint32]Slot{
	0x82: {0x82, 0x5fc10d},
	0x83: {0x83, 0x5fc10e},
	0x84: {0x84, 0x5fc10f},
	0x85: {0x85, 0x5fc110},
	0x86: {0x86, 0x5fc111},
	0x87: {0x87, 0x5fc112},
	0x88: {0x88, 0x5fc113},
	0x89: {0x89, 0x5fc114},
	0x8a: {0x8a, 0x5fc115},
	0x8b: {0x8b, 0x5fc116},
	0x8c: {0x8c, 0x5fc117},
	0x8d: {0x8d, 0x5fc118},
	0x8e: {0x8e, 0x5fc119},
	0x8f: {0x8f, 0x5fc11a},
	0x90: {0x90, 0x5fc11b},
	0x91: {0x91, 0x5fc11c},
	0x92: {0x92, 0x5fc11d},
	0x93: {0x93, 0x5fc11e},
	0x94: {0x94, 0x5fc11f},
	0x95: {0x95, 0x5fc120},
}

// RetiredKeyManagementSlot provides access to "retired" slots. Slots meant for old Key Management
// keys that have been rotated. YubiKeys 4 and later support values between 0x82 and 0x95 (inclusive).
//
//     slot, ok := RetiredKeyManagementSlot(0x82)
//     if !ok {
//         // unrecognized slot
//     }
//     pub, err := yk.GenerateKey(managementKey, slot, key)
//
// https://developers.yubico.com/PIV/Introduction/Certificate_slots.html#_slot_82_95_retired_key_management
func RetiredKeyManagementSlot(key uint32) (Slot, bool) {
	slot, ok := retiredKeyManagementSlots[key]
	return slot, ok
}

// String returns the two-character hex representation of the slot
func (s Slot) String() string {
	return strconv.FormatUint(uint64(s.Key), 16)
}

// Algorithm represents a specific algorithm and bit size supported by the PIV
// specification.
type Algorithm int

// Algorithms supported by this package. Note that not all cards will support
// every algorithm.
//
// AlgorithmEd25519 is currently only implemented by SoloKeys.
//
// For algorithm discovery, see: https://github.com/ericchiang/piv-go/issues/1
const (
	AlgorithmEC256 Algorithm = iota + 1
	AlgorithmEC384
	AlgorithmEd25519
	AlgorithmRSA1024
	AlgorithmRSA2048
)

// PINPolicy represents PIN requirements when signing or decrypting with an
// asymmetric key in a given slot.
type PINPolicy int

// PIN policies supported by this package.
//
// BUG(ericchiang): Caching for PINPolicyOnce isn't supported on YubiKey
// versions older than 4.3.0 due to issues with verifying if a PIN is needed.
// If specified, a PIN will be required for every operation.
const (
	PINPolicyNever PINPolicy = iota + 1
	PINPolicyOnce
	PINPolicyAlways
)

// TouchPolicy represents proof-of-presence requirements when signing or
// decrypting with asymmetric key in a given slot.
type TouchPolicy int

// Touch policies supported by this package.
const (
	TouchPolicyNever TouchPolicy = iota + 1
	TouchPolicyAlways
	TouchPolicyCached
)

const (
	tagPINPolicy   = 0xaa
	tagTouchPolicy = 0xab
)

var pinPolicyMap = map[PINPolicy]byte{
	PINPolicyNever:  0x01,
	PINPolicyOnce:   0x02,
	PINPolicyAlways: 0x03,
}

var touchPolicyMap = map[TouchPolicy]byte{
	TouchPolicyNever:  0x01,
	TouchPolicyAlways: 0x02,
	TouchPolicyCached: 0x03,
}

var algorithmsMap = map[Algorithm]byte{
	AlgorithmEC256:   algECCP256,
	AlgorithmEC384:   algECCP384,
	AlgorithmEd25519: algEd25519,
	AlgorithmRSA1024: algRSA1024,
	AlgorithmRSA2048: algRSA2048,
}

// AttestationCertificate returns the YubiKey's attestation certificate, which
// is unique to the key and signed by Yubico.
func (yk *YubiKey) AttestationCertificate() (*x509.Certificate, error) {
	return yk.Certificate(slotAttestation)
}

// Attest generates a certificate for a key, signed by the YubiKey's attestation
// certificate. This can be used to prove a key was generate on a specific
// YubiKey.
//
// This method is only supported for YubiKey versions >= 4.3.0.
// https://developers.yubico.com/PIV/Introduction/PIV_attestation.html
//
// Certificates returned by this method MUST NOT be used for anything other than
// attestion or determining the slots public key. For example, the certificate
// is NOT suitable for TLS.
//
// If the slot doesn't have a key, the returned error wraps ErrNotFound.
func (yk *YubiKey) Attest(slot Slot) (*x509.Certificate, error) {
	cert, err := ykAttest(yk.tx, slot)
	if err == nil {
		return cert, nil
	}
	var e *apduErr
	if errors.As(err, &e) && e.sw1 == 0x6A && e.sw2 == 0x80 {
		return nil, ErrNotFound
	}
	return nil, err
}

func ykAttest(tx *scTx, slot Slot) (*x509.Certificate, error) {
	cmd := apdu{
		instruction: insAttest,
		param1:      byte(slot.Key),
	}
	resp, err := tx.Transmit(cmd)
	if err != nil {
		return nil, fmt.Errorf("command failed: %w", err)
	}
	if bytes.HasPrefix(resp, []byte{0x70}) {
		b, _, err := unmarshalASN1(resp, 0, 0x10) // tag 0x70
		if err != nil {
			return nil, fmt.Errorf("unmarshaling certificate: %v", err)
		}
		resp = b
	}
	cert, err := x509.ParseCertificate(resp)
	if err != nil {
		return nil, fmt.Errorf("parsing certificate: %v", err)
	}
	return cert, nil
}

// Certificate returns the certifiate object stored in a given slot.
//
// If a certificate hasn't been set in the provided slot, the returned error
// wraps ErrNotFound.
func (yk *YubiKey) Certificate(slot Slot) (*x509.Certificate, error) {
	cmd := apdu{
		instruction: insGetData,
		param1:      0x3f,
		param2:      0xff,
		data: []byte{
			0x5c, // Tag list
			0x03, // Length of tag
			byte(slot.Object >> 16),
			byte(slot.Object >> 8),
			byte(slot.Object),
		},
	}
	resp, err := yk.tx.Transmit(cmd)
	if err != nil {
		return nil, fmt.Errorf("command failed: %w", err)
	}
	// https://nvlpubs.nist.gov/nistpubs/SpecialPublications/NIST.SP.800-73-4.pdf#page=85
	obj, _, err := unmarshalASN1(resp, 1, 0x13) // tag 0x53
	if err != nil {
		return nil, fmt.Errorf("unmarshaling response: %v", err)
	}
	certDER, _, err := unmarshalASN1(obj, 1, 0x10) // tag 0x70
	if err != nil {
		return nil, fmt.Errorf("unmarshaling certificate: %v", err)
	}
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, fmt.Errorf("parsing certificate: %v", err)
	}
	return cert, nil
}

// marshalASN1Length encodes the length.
func marshalASN1Length(n uint64) []byte {
	var l []byte
	if n < 0x80 {
		l = []byte{byte(n)}
	} else if n < 0x100 {
		l = []byte{0x81, byte(n)}
	} else {
		l = []byte{0x82, byte(n >> 8), byte(n)}
	}

	return l
}

// marshalASN1 encodes a tag, length and data.
//
// TODO: clean this up and maybe switch to cryptobyte?
func marshalASN1(tag byte, data []byte) []byte {
	l := marshalASN1Length(uint64(len(data)))
	d := append([]byte{tag}, l...)
	return append(d, data...)
}

// SetCertificate stores a certificate object in the provided slot. Setting a
// certificate isn't required to use the associated key for signing or
// decryption.
func (yk *YubiKey) SetCertificate(key [24]byte, slot Slot, cert *x509.Certificate) error {
	if err := ykAuthenticate(yk.tx, key, yk.rand); err != nil {
		return fmt.Errorf("authenticating with management key: %w", err)
	}
	return ykStoreCertificate(yk.tx, slot, cert)
}

func ykStoreCertificate(tx *scTx, slot Slot, cert *x509.Certificate) error {
	// https://nvlpubs.nist.gov/nistpubs/SpecialPublications/NIST.SP.800-73-4.pdf#page=40
	data := marshalASN1(0x70, cert.Raw)
	// "for a certificate encoded in uncompressed form CertInfo shall be 0x00"
	data = append(data, marshalASN1(0x71, []byte{0x00})...)
	// Error Detection Code
	data = append(data, marshalASN1(0xfe, nil)...)
	// https://nvlpubs.nist.gov/nistpubs/SpecialPublications/NIST.SP.800-73-4.pdf#page=94
	data = append([]byte{
		0x5c, // Tag list
		0x03, // Length of tag
		byte(slot.Object >> 16),
		byte(slot.Object >> 8),
		byte(slot.Object),
	}, marshalASN1(0x53, data)...)
	cmd := apdu{
		instruction: insPutData,
		param1:      0x3f,
		param2:      0xff,
		data:        data,
	}
	if _, err := tx.Transmit(cmd); err != nil {
		return fmt.Errorf("command failed: %v", err)
	}
	return nil
}

// Key is used for key generation and holds different options for the key.
//
// While keys can have default PIN and touch policies, this package currently
// doesn't support this option, and all fields must be provided.
type Key struct {
	// Algorithm to use when generating the key.
	Algorithm Algorithm
	// PINPolicy for the key.
	//
	// BUG(ericchiang): some older YubiKeys (third generation) will silently
	// drop this value. If PINPolicyNever or PINPolicyOnce is supplied but the
	// key still requires a PIN every time, you may be using a buggy key and
	// should supply PINPolicyAlways. See https://github.com/go-piv/piv-go/issues/60
	PINPolicy PINPolicy
	// TouchPolicy for the key.
	TouchPolicy TouchPolicy
}

// GenerateKey generates an asymmetric key on the card, returning the key's
// public key.
func (yk *YubiKey) GenerateKey(key [24]byte, slot Slot, opts Key) (crypto.PublicKey, error) {
	if err := ykAuthenticate(yk.tx, key, yk.rand); err != nil {
		return nil, fmt.Errorf("authenticating with management key: %w", err)
	}
	return ykGenerateKey(yk.tx, slot, opts)
}

func ykGenerateKey(tx *scTx, slot Slot, o Key) (crypto.PublicKey, error) {
	alg, ok := algorithmsMap[o.Algorithm]
	if !ok {
		return nil, fmt.Errorf("unsupported algorithm")

	}
	tp, ok := touchPolicyMap[o.TouchPolicy]
	if !ok {
		return nil, fmt.Errorf("unsupported touch policy")
	}
	pp, ok := pinPolicyMap[o.PINPolicy]
	if !ok {
		return nil, fmt.Errorf("unsupported pin policy")
	}
	// https://nvlpubs.nist.gov/nistpubs/SpecialPublications/NIST.SP.800-73-4.pdf#page=95
	cmd := apdu{
		instruction: insGenerateAsymmetric,
		param2:      byte(slot.Key),
		data: []byte{
			0xac,
			0x09, // length of remaining data
			algTag, 0x01, alg,
			tagPINPolicy, 0x01, pp,
			tagTouchPolicy, 0x01, tp,
		},
	}
	resp, err := tx.Transmit(cmd)
	if err != nil {
		return nil, fmt.Errorf("command failed: %w", err)
	}

	var curve elliptic.Curve
	switch o.Algorithm {
	case AlgorithmRSA1024, AlgorithmRSA2048:
		pub, err := decodeRSAPublic(resp)
		if err != nil {
			return nil, fmt.Errorf("decoding rsa public key: %v", err)
		}
		return pub, nil
	case AlgorithmEC256:
		curve = elliptic.P256()
	case AlgorithmEC384:
		curve = elliptic.P384()
	case AlgorithmEd25519:
		pub, err := decodeEd25519Public(resp)
		if err != nil {
			return nil, fmt.Errorf("decoding ed25519 public key: %v", err)
		}
		return pub, nil
	default:
		return nil, fmt.Errorf("unsupported algorithm")
	}
	pub, err := decodeECPublic(resp, curve)
	if err != nil {
		return nil, fmt.Errorf("decoding ec public key: %v", err)
	}
	return pub, nil
}

// KeyAuth is used to authenticate against the YubiKey on each signing  and
// decryption request.
type KeyAuth struct {
	// PIN, if provided, is a static PIN used to authenticate against the key.
	// If provided, PINPrompt is ignored.
	PIN string
	// PINPrompt can be used to interactively request the PIN from the user. The
	// method is only called when needed. For example, if a key specifies
	// PINPolicyOnce, PINPrompt will only be called once per YubiKey struct.
	PINPrompt func() (pin string, err error)

	// PINPolicy can be used to specify the PIN caching strategy for the slot. If
	// not provided, this will be inferred from the attestation certificate.
	//
	// This field is required on older (<4.3.0) YubiKeys when using PINPrompt,
	// as well as for keys imported to the card.
	PINPolicy PINPolicy
}

func (k KeyAuth) authTx(yk *YubiKey, pp PINPolicy) error {
	// PINPolicyNever shouldn't require a PIN.
	if pp == PINPolicyNever {
		return nil
	}

	// PINPolicyAlways should always prompt a PIN even if the key says that
	// login isn't needed.
	// https://github.com/go-piv/piv-go/issues/49
	if pp != PINPolicyAlways && !ykLoginNeeded(yk.tx) {
		return nil
	}

	pin := k.PIN
	if pin == "" && k.PINPrompt != nil {
		p, err := k.PINPrompt()
		if err != nil {
			return fmt.Errorf("pin prompt: %v", err)
		}
		pin = p
	}
	if pin == "" {
		return fmt.Errorf("pin required but wasn't provided")
	}
	return ykLogin(yk.tx, pin)
}

func (k KeyAuth) do(yk *YubiKey, pp PINPolicy, f func(tx *scTx) ([]byte, error)) ([]byte, error) {
	if err := k.authTx(yk, pp); err != nil {
		return nil, err
	}
	return f(yk.tx)
}

func pinPolicy(yk *YubiKey, slot Slot) (PINPolicy, error) {
	cert, err := yk.Attest(slot)
	if err != nil {
		var e *apduErr
		if errors.As(err, &e) && e.sw1 == 0x6d && e.sw2 == 0x00 {
			// Attestation cert command not supported, probably an older YubiKey.
			// Guess PINPolicyAlways.
			//
			// See https://github.com/go-piv/piv-go/issues/55
			return PINPolicyAlways, nil
		}
		return 0, fmt.Errorf("get attestation cert: %v", err)
	}
	a, err := parseAttestation(cert)
	if err != nil {
		return 0, fmt.Errorf("parse attestation cert: %v", err)
	}
	if _, ok := pinPolicyMap[a.PINPolicy]; ok {
		return a.PINPolicy, nil
	}
	return PINPolicyOnce, nil
}

// PrivateKey is used to access signing and decryption options for the key
// stored in the slot. The returned key implements crypto.Signer and/or
// crypto.Decrypter depending on the key type.
//
// If the public key hasn't been stored externally, it can be provided by
// fetching the slot's attestation certificate:
//
//		cert, err := yk.Attest(slot)
//		if err != nil {
//			// ...
//		}
//		priv, err := yk.PrivateKey(slot, cert.PublicKey, auth)
//
func (yk *YubiKey) PrivateKey(slot Slot, public crypto.PublicKey, auth KeyAuth) (crypto.PrivateKey, error) {
	pp := PINPolicyNever
	if _, ok := pinPolicyMap[auth.PINPolicy]; ok {
		// If the PIN policy is manually specified, trust that value instead of
		// trying to use the attestation certificate.
		pp = auth.PINPolicy
	} else if auth.PIN != "" || auth.PINPrompt != nil {
		// Attempt to determine the key's PIN policy. This helps inform the
		// strategy for when to prompt for a PIN.
		policy, err := pinPolicy(yk, slot)
		if err != nil {
			return nil, err
		}
		pp = policy
	}

	switch pub := public.(type) {
	case *ecdsa.PublicKey:
		return &ECDSAPrivateKey{yk, slot, pub, auth, pp}, nil
	case ed25519.PublicKey:
		return &keyEd25519{yk, slot, pub, auth, pp}, nil
	case *rsa.PublicKey:
		return &keyRSA{yk, slot, pub, auth, pp}, nil
	default:
		return nil, fmt.Errorf("unsupported public key type: %T", public)
	}
}

// SetPrivateKeyInsecure is an insecure method which imports a private key into the slot.
// Users should almost always use GeneratePrivateKey() instead.
//
// Importing a private key breaks functionality provided by this package, including
// AttestationCertificate() and Attest(). There are no stability guarantees for other
// methods for imported private keys.
//
// Keys generated outside of the YubiKey should not be considered hardware-backed,
// as there's no way to prove the key wasn't copied, exfiltrated, or replaced with malicious
// material before being imported.
func (yk *YubiKey) SetPrivateKeyInsecure(key [24]byte, slot Slot, private crypto.PrivateKey, policy Key) error {
	// Reference implementation
	// https://github.com/Yubico/yubico-piv-tool/blob/671a5740ef09d6c5d9d33f6e5575450750b58bde/lib/ykpiv.c#L1812

	params := make([][]byte, 0)

	var paramTag byte
	var elemLen int

	switch priv := private.(type) {
	case *rsa.PrivateKey:
		paramTag = 0x01
		switch priv.N.BitLen() {
		case 1024:
			policy.Algorithm = AlgorithmRSA1024
			elemLen = 64
		case 2048:
			policy.Algorithm = AlgorithmRSA2048
			elemLen = 128
		default:
			return errUnsupportedKeySize
		}

		priv.Precompute()

		params = append(params, priv.Primes[0].Bytes())        // P
		params = append(params, priv.Primes[1].Bytes())        // Q
		params = append(params, priv.Precomputed.Dp.Bytes())   // dP
		params = append(params, priv.Precomputed.Dq.Bytes())   // dQ
		params = append(params, priv.Precomputed.Qinv.Bytes()) // Qinv
	case *ecdsa.PrivateKey:
		paramTag = 0x6
		size := priv.PublicKey.Params().BitSize
		switch size {
		case 256:
			policy.Algorithm = AlgorithmEC256
			elemLen = 32
		case 384:
			policy.Algorithm = AlgorithmEC384
			elemLen = 48
		default:
			return unsupportedCurveError{curve: size}
		}

		// S value
		privateKey := make([]byte, elemLen)
		valueBytes := priv.D.Bytes()
		padding := len(privateKey) - len(valueBytes)
		copy(privateKey[padding:], valueBytes)

		params = append(params, privateKey)
	default:
		return errors.New("unsupported private key type")
	}

	elemLenASN1 := marshalASN1Length(uint64(elemLen))

	tags := make([]byte, 0)
	for i, param := range params {
		tag := paramTag + byte(i)
		tags = append(tags, tag)
		tags = append(tags, elemLenASN1...)

		padding := elemLen - len(param)
		param = append(make([]byte, padding), param...)
		tags = append(tags, param...)
	}

	if err := ykAuthenticate(yk.tx, key, yk.rand); err != nil {
		return fmt.Errorf("authenticating with management key: %w", err)
	}

	return ykImportKey(yk.tx, tags, slot, policy)
}

func ykImportKey(tx *scTx, tags []byte, slot Slot, o Key) error {
	alg, ok := algorithmsMap[o.Algorithm]
	if !ok {
		return fmt.Errorf("unsupported algorithm")

	}
	tp, ok := touchPolicyMap[o.TouchPolicy]
	if !ok {
		return fmt.Errorf("unsupported touch policy")
	}
	pp, ok := pinPolicyMap[o.PINPolicy]
	if !ok {
		return fmt.Errorf("unsupported pin policy")
	}

	// This command is a Yubico PIV extension.
	// https://developers.yubico.com/PIV/Introduction/Yubico_extensions.html
	cmd := apdu{
		instruction: insImportKey,
		param1:      alg,
		param2:      byte(slot.Key),
		data: append(tags, []byte{
			tagPINPolicy, 0x01, pp,
			tagTouchPolicy, 0x01, tp,
		}...),
	}

	if _, err := tx.Transmit(cmd); err != nil {
		return fmt.Errorf("command failed: %w", err)
	}

	return nil
}

// ECDSAPrivateKey is a crypto.PrivateKey implementation for ECDSA
// keys. It implements crypto.Signer and the method SharedKey performs
// Diffie-Hellman key agreements.
//
// Keys returned by YubiKey.PrivateKey() may be type asserted to
// *ECDSAPrivateKey, if the slot contains an ECDSA key.
type ECDSAPrivateKey struct {
	yk   *YubiKey
	slot Slot
	pub  *ecdsa.PublicKey
	auth KeyAuth
	pp   PINPolicy
}

// Public returns the public key associated with this private key.
func (k *ECDSAPrivateKey) Public() crypto.PublicKey {
	return k.pub
}

var _ crypto.Signer = (*ECDSAPrivateKey)(nil)

// Sign implements crypto.Signer.
func (k *ECDSAPrivateKey) Sign(rand io.Reader, digest []byte, opts crypto.SignerOpts) ([]byte, error) {
	return k.auth.do(k.yk, k.pp, func(tx *scTx) ([]byte, error) {
		return ykSignECDSA(tx, k.slot, k.pub, digest)
	})
}

// SharedKey performs a Diffie-Hellman key agreement with the peer
// to produce a shared secret key.
//
// Peer's public key must use the same algorithm as the key in
// this slot, or an error will be returned.
//
// Length of the result depends on the types and sizes of the keys
// used for the operation. Callers should use a cryptographic key
// derivation function to extract the amount of bytes they need.
func (k *ECDSAPrivateKey) SharedKey(peer *ecdsa.PublicKey) ([]byte, error) {
	if peer.Curve.Params().BitSize != k.pub.Curve.Params().BitSize {
		return nil, errMismatchingAlgorithms
	}
	msg := elliptic.Marshal(peer.Curve, peer.X, peer.Y)
	return k.auth.do(k.yk, k.pp, func(tx *scTx) ([]byte, error) {
		var alg byte
		size := k.pub.Params().BitSize
		switch size {
		case 256:
			alg = algECCP256
		case 384:
			alg = algECCP384
		default:
			return nil, unsupportedCurveError{curve: size}
		}

		// https://nvlpubs.nist.gov/nistpubs/SpecialPublications/NIST.SP.800-73-4.pdf#page=118
		// https://nvlpubs.nist.gov/nistpubs/SpecialPublications/NIST.SP.800-73-4.pdf#page=93
		cmd := apdu{
			instruction: insAuthenticate,
			param1:      alg,
			param2:      byte(k.slot.Key),
			data: marshalASN1(0x7c,
				append([]byte{0x82, 0x00},
					marshalASN1(0x85, msg)...)),
		}
		resp, err := tx.Transmit(cmd)
		if err != nil {
			return nil, fmt.Errorf("command failed: %w", err)
		}
		sig, _, err := unmarshalASN1(resp, 1, 0x1c) // 0x7c
		if err != nil {
			return nil, fmt.Errorf("unmarshal response: %v", err)
		}
		rs, _, err := unmarshalASN1(sig, 2, 0x02) // 0x82
		if err != nil {
			return nil, fmt.Errorf("unmarshal response signature: %v", err)
		}
		return rs, nil
	})
}

type keyEd25519 struct {
	yk   *YubiKey
	slot Slot
	pub  ed25519.PublicKey
	auth KeyAuth
	pp   PINPolicy
}

func (k *keyEd25519) Public() crypto.PublicKey {
	return k.pub
}

func (k *keyEd25519) Sign(rand io.Reader, digest []byte, opts crypto.SignerOpts) ([]byte, error) {
	return k.auth.do(k.yk, k.pp, func(tx *scTx) ([]byte, error) {
		return skSignEd25519(tx, k.slot, k.pub, digest)
	})
}

type keyRSA struct {
	yk   *YubiKey
	slot Slot
	pub  *rsa.PublicKey
	auth KeyAuth
	pp   PINPolicy
}

func (k *keyRSA) Public() crypto.PublicKey {
	return k.pub
}

func (k *keyRSA) Sign(rand io.Reader, digest []byte, opts crypto.SignerOpts) ([]byte, error) {
	return k.auth.do(k.yk, k.pp, func(tx *scTx) ([]byte, error) {
		return ykSignRSA(tx, k.slot, k.pub, digest, opts)
	})
}

func (k *keyRSA) Decrypt(rand io.Reader, msg []byte, opts crypto.DecrypterOpts) ([]byte, error) {
	return k.auth.do(k.yk, k.pp, func(tx *scTx) ([]byte, error) {
		return ykDecryptRSA(tx, k.slot, k.pub, msg)
	})
}

func ykSignECDSA(tx *scTx, slot Slot, pub *ecdsa.PublicKey, digest []byte) ([]byte, error) {
	var alg byte
	size := pub.Params().BitSize
	switch size {
	case 256:
		alg = algECCP256
	case 384:
		alg = algECCP384
	default:
		return nil, unsupportedCurveError{curve: size}
	}

	// Same as the standard library
	// https://github.com/golang/go/blob/go1.13.5/src/crypto/ecdsa/ecdsa.go#L125-L128
	orderBytes := (size + 7) / 8
	if len(digest) > orderBytes {
		digest = digest[:orderBytes]
	}

	// https://nvlpubs.nist.gov/nistpubs/SpecialPublications/NIST.SP.800-73-4.pdf#page=118
	cmd := apdu{
		instruction: insAuthenticate,
		param1:      alg,
		param2:      byte(slot.Key),
		data: marshalASN1(0x7c,
			append([]byte{0x82, 0x00},
				marshalASN1(0x81, digest)...)),
	}
	resp, err := tx.Transmit(cmd)
	if err != nil {
		return nil, fmt.Errorf("command failed: %w", err)
	}
	sig, _, err := unmarshalASN1(resp, 1, 0x1c) // 0x7c
	if err != nil {
		return nil, fmt.Errorf("unmarshal response: %v", err)
	}
	rs, _, err := unmarshalASN1(sig, 2, 0x02) // 0x82
	if err != nil {
		return nil, fmt.Errorf("unmarshal response signature: %v", err)
	}
	return rs, nil
}

// This function only works on SoloKeys prototypes and other PIV devices that choose
// to implement Ed25519 signatures under alg 0x22.
func skSignEd25519(tx *scTx, slot Slot, pub ed25519.PublicKey, digest []byte) ([]byte, error) {
	// Adaptation of
	// https://nvlpubs.nist.gov/nistpubs/SpecialPublications/NIST.SP.800-73-4.pdf#page=118
	cmd := apdu{
		instruction: insAuthenticate,
		param1:      algEd25519,
		param2:      byte(slot.Key),
		data: marshalASN1(0x7c,
			append([]byte{0x82, 0x00},
				marshalASN1(0x81, digest)...)),
	}
	resp, err := tx.Transmit(cmd)
	if err != nil {
		return nil, fmt.Errorf("command failed: %w", err)
	}
	sig, _, err := unmarshalASN1(resp, 1, 0x1c) // 0x7c
	if err != nil {
		return nil, fmt.Errorf("unmarshal response: %v", err)
	}
	rs, _, err := unmarshalASN1(sig, 2, 0x02) // 0x82
	if err != nil {
		return nil, fmt.Errorf("unmarshal response signature: %v", err)
	}
	return rs, nil
}

func unmarshalASN1(b []byte, class, tag int) (obj, rest []byte, err error) {
	var v asn1.RawValue
	rest, err = asn1.Unmarshal(b, &v)
	if err != nil {
		return nil, nil, err
	}
	if v.Class != class || v.Tag != tag {
		return nil, nil, fmt.Errorf("unexpected class=%d and tag=0x%x", v.Class, v.Tag)
	}
	return v.Bytes, rest, nil
}

func decodeECPublic(b []byte, curve elliptic.Curve) (*ecdsa.PublicKey, error) {
	// https://nvlpubs.nist.gov/nistpubs/SpecialPublications/NIST.SP.800-73-4.pdf#page=95
	r, _, err := unmarshalASN1(b, 1, 0x49)
	if err != nil {
		return nil, fmt.Errorf("unmarshal response: %v", err)
	}
	p, _, err := unmarshalASN1(r, 2, 0x06)
	if err != nil {
		return nil, fmt.Errorf("unmarshal points: %v", err)
	}
	// https://nvlpubs.nist.gov/nistpubs/SpecialPublications/NIST.SP.800-73-4.pdf#page=96
	size := curve.Params().BitSize / 8
	if len(p) != (size*2)+1 {
		return nil, fmt.Errorf("unexpected points length: %d", len(p))
	}
	// Are points uncompressed?
	if p[0] != 0x04 {
		return nil, fmt.Errorf("points were not uncompressed")
	}
	p = p[1:]
	var x, y big.Int
	x.SetBytes(p[:size])
	y.SetBytes(p[size:])
	if !curve.IsOnCurve(&x, &y) {
		return nil, fmt.Errorf("resulting points are not on curve")
	}
	return &ecdsa.PublicKey{Curve: curve, X: &x, Y: &y}, nil
}

func decodeEd25519Public(b []byte) (ed25519.PublicKey, error) {
	// Adaptation of
	// https://nvlpubs.nist.gov/nistpubs/SpecialPublications/NIST.SP.800-73-4.pdf#page=95
	r, _, err := unmarshalASN1(b, 1, 0x49)
	if err != nil {
		return nil, fmt.Errorf("unmarshal response: %v", err)
	}
	p, _, err := unmarshalASN1(r, 2, 0x06)
	if err != nil {
		return nil, fmt.Errorf("unmarshal points: %v", err)
	}
	if len(p) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("unexpected points length: %d", len(p))
	}
	return ed25519.PublicKey(p), nil
}

func decodeRSAPublic(b []byte) (*rsa.PublicKey, error) {
	// https://nvlpubs.nist.gov/nistpubs/SpecialPublications/NIST.SP.800-73-4.pdf#page=95
	r, _, err := unmarshalASN1(b, 1, 0x49)
	if err != nil {
		return nil, fmt.Errorf("unmarshal response: %v", err)
	}
	mod, r, err := unmarshalASN1(r, 2, 0x01)
	if err != nil {
		return nil, fmt.Errorf("unmarshal modulus: %v", err)
	}
	exp, _, err := unmarshalASN1(r, 2, 0x02)
	if err != nil {
		return nil, fmt.Errorf("unmarshal exponent: %v", err)
	}
	var n, e big.Int
	n.SetBytes(mod)
	e.SetBytes(exp)
	if !e.IsInt64() {
		return nil, fmt.Errorf("returned exponent too large: %s", e.String())
	}
	return &rsa.PublicKey{N: &n, E: int(e.Int64())}, nil
}

func rsaAlg(pub *rsa.PublicKey) (byte, error) {
	size := pub.N.BitLen()
	switch size {
	case 1024:
		return algRSA1024, nil
	case 2048:
		return algRSA2048, nil
	default:
		return 0, fmt.Errorf("unsupported rsa key size: %d", size)
	}
}

func ykDecryptRSA(tx *scTx, slot Slot, pub *rsa.PublicKey, data []byte) ([]byte, error) {
	alg, err := rsaAlg(pub)
	if err != nil {
		return nil, err
	}
	cmd := apdu{
		instruction: insAuthenticate,
		param1:      alg,
		param2:      byte(slot.Key),
		data: marshalASN1(0x7c,
			append([]byte{0x82, 0x00},
				marshalASN1(0x81, data)...)),
	}
	resp, err := tx.Transmit(cmd)
	if err != nil {
		return nil, fmt.Errorf("command failed: %w", err)
	}

	sig, _, err := unmarshalASN1(resp, 1, 0x1c) // 0x7c
	if err != nil {
		return nil, fmt.Errorf("unmarshal response: %v", err)
	}
	decrypted, _, err := unmarshalASN1(sig, 2, 0x02) // 0x82
	if err != nil {
		return nil, fmt.Errorf("unmarshal response signature: %v", err)
	}
	// Decrypted blob contains a bunch of random data. Look for a NULL byte which
	// indicates where the plain text starts.
	for i := 2; i+1 < len(decrypted); i++ {
		if decrypted[i] == 0x00 {
			return decrypted[i+1:], nil
		}
	}
	return nil, fmt.Errorf("invalid pkcs#1 v1.5 padding")
}

// PKCS#1 v15 is largely informed by the standard library
// https://github.com/golang/go/blob/go1.13.5/src/crypto/rsa/pkcs1v15.go

func ykSignRSA(tx *scTx, slot Slot, pub *rsa.PublicKey, digest []byte, opts crypto.SignerOpts) ([]byte, error) {
	if _, ok := opts.(*rsa.PSSOptions); ok {
		return nil, fmt.Errorf("rsassa-pss signatures not supported")
	}

	alg, err := rsaAlg(pub)
	if err != nil {
		return nil, err
	}
	hash := opts.HashFunc()
	if hash.Size() != len(digest) {
		return nil, fmt.Errorf("input must be a hashed message")
	}
	prefix, ok := hashPrefixes[hash]
	if !ok {
		return nil, fmt.Errorf("unsupported hash algorithm: crypto.Hash(%d)", hash)
	}

	// https://tools.ietf.org/pdf/rfc2313.pdf#page=9
	d := make([]byte, len(prefix)+len(digest))
	copy(d[:len(prefix)], prefix)
	copy(d[len(prefix):], digest)

	paddingLen := pub.Size() - 3 - len(d)
	if paddingLen < 0 {
		return nil, fmt.Errorf("message too large")
	}

	padding := make([]byte, paddingLen)
	for i := range padding {
		padding[i] = 0xff
	}

	// https://tools.ietf.org/pdf/rfc2313.pdf#page=9
	data := append([]byte{0x00, 0x01}, padding...)
	data = append(data, 0x00)
	data = append(data, d...)

	// https://nvlpubs.nist.gov/nistpubs/SpecialPublications/NIST.SP.800-73-4.pdf#page=117
	cmd := apdu{
		instruction: insAuthenticate,
		param1:      alg,
		param2:      byte(slot.Key),
		data: marshalASN1(0x7c,
			append([]byte{0x82, 0x00},
				marshalASN1(0x81, data)...)),
	}
	resp, err := tx.Transmit(cmd)
	if err != nil {
		return nil, fmt.Errorf("command failed: %w", err)
	}

	sig, _, err := unmarshalASN1(resp, 1, 0x1c) // 0x7c
	if err != nil {
		return nil, fmt.Errorf("unmarshal response: %v", err)
	}
	pkcs1v15Sig, _, err := unmarshalASN1(sig, 2, 0x02) // 0x82
	if err != nil {
		return nil, fmt.Errorf("unmarshal response signature: %v", err)
	}
	return pkcs1v15Sig, nil
}

var hashPrefixes = map[crypto.Hash][]byte{
	crypto.MD5:       {0x30, 0x20, 0x30, 0x0c, 0x06, 0x08, 0x2a, 0x86, 0x48, 0x86, 0xf7, 0x0d, 0x02, 0x05, 0x05, 0x00, 0x04, 0x10},
	crypto.SHA1:      {0x30, 0x21, 0x30, 0x09, 0x06, 0x05, 0x2b, 0x0e, 0x03, 0x02, 0x1a, 0x05, 0x00, 0x04, 0x14},
	crypto.SHA224:    {0x30, 0x2d, 0x30, 0x0d, 0x06, 0x09, 0x60, 0x86, 0x48, 0x01, 0x65, 0x03, 0x04, 0x02, 0x04, 0x05, 0x00, 0x04, 0x1c},
	crypto.SHA256:    {0x30, 0x31, 0x30, 0x0d, 0x06, 0x09, 0x60, 0x86, 0x48, 0x01, 0x65, 0x03, 0x04, 0x02, 0x01, 0x05, 0x00, 0x04, 0x20},
	crypto.SHA384:    {0x30, 0x41, 0x30, 0x0d, 0x06, 0x09, 0x60, 0x86, 0x48, 0x01, 0x65, 0x03, 0x04, 0x02, 0x02, 0x05, 0x00, 0x04, 0x30},
	crypto.SHA512:    {0x30, 0x51, 0x30, 0x0d, 0x06, 0x09, 0x60, 0x86, 0x48, 0x01, 0x65, 0x03, 0x04, 0x02, 0x03, 0x05, 0x00, 0x04, 0x40},
	crypto.MD5SHA1:   {}, // A special TLS case which doesn't use an ASN1 prefix.
	crypto.RIPEMD160: {0x30, 0x20, 0x30, 0x08, 0x06, 0x06, 0x28, 0xcf, 0x06, 0x03, 0x00, 0x31, 0x04, 0x14},
}
