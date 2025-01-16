package config

import (
	"context"
	"crypto/tls"
	"fmt"

	"github.com/cri-o/cri-o/internal/log"
)

const (
	VersionTLS10 = "VersionTLS10"
	VersionTLS11 = "VersionTLS11"
	VersionTLS12 = "VersionTLS12"
	VersionTLS13 = "VersionTLS13"
)

var (
	TLSVersions = map[string]uint16{
		VersionTLS10: tls.VersionTLS10,
		VersionTLS11: tls.VersionTLS11,
		VersionTLS12: tls.VersionTLS12,
		VersionTLS13: tls.VersionTLS13,
	}
	cipherSuites         = map[string]uint16{}
	insecureCipherSuites = map[string]uint16{}
)

func loadCipherSuites() {
	for _, cs := range tls.CipherSuites() {
		cipherSuites[cs.Name] = cs.ID
	}
	for _, cs := range tls.InsecureCipherSuites() {
		insecureCipherSuites[cs.Name] = cs.ID
	}
}

func CipherSuitesFromConfig(c []string) []uint16 {
	var ret []uint16
	if len(cipherSuites) == 0 {
		loadCipherSuites()
	}
	for _, name := range c {
		if id, ok := cipherSuites[name]; ok {
			ret = append(ret, id)
			continue
		}
		if id, ok := insecureCipherSuites[name]; ok {
			log.Warnf(context.Background(), "Insecure cipher suite might be used: %s", name)
			ret = append(ret, id)
			continue
		}
		log.Errorf(context.Background(), "Cipher suite not found: %s", name)
	}
	return ret
}

func ValidateTLSVersion(version string) error {
	if _, ok := TLSVersions[version]; !ok {
		return fmt.Errorf("unsupported TLS Version %s: Supported versions are VersionTLS10, VersionTLS11, VersionTLS12 and VersionTLS13", version)
	}
	return nil
}

func ValidateCipherSuites(csuites []string) error {
	if len(cipherSuites) == 0 {
		loadCipherSuites()
	}
	var unsupported []string
	for _, cs := range csuites {
		_, ok := cipherSuites[cs]
		_, oki := insecureCipherSuites[cs]
		if !ok && !oki {
			unsupported = append(unsupported, cs)
		}
	}
	if len(unsupported) != 0 {
		return fmt.Errorf("unsupported cipher suites %+v: For supported cipher suites, see https://pkg.go.dev/crypto/tls#pkg-constants", unsupported)
	}
	return nil
}
