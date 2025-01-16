package config_test

import (
	"crypto/tls"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/cri-o/cri-o/pkg/config"
)

var _ = t.Describe("CipherSuitesFromConfig", func() {
	It("returns their corresponding IDs", func() {
		exampleCiphers := []string{
			"TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA",
		}
		result := config.CipherSuitesFromConfig(exampleCiphers)

		Expect(result).To(HaveLen(1))
		Expect(result[0]).To(Equal(tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA))
	})

	It("handles multiple valid cipher suites", func() {
		exampleCiphers := []string{
			"TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA",
			"TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA",
		}
		result := config.CipherSuitesFromConfig(exampleCiphers)

		Expect(result).To(HaveLen(2))
		Expect(result[0]).To(Equal(tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA))
		Expect(result[1]).To(Equal(tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA))
	})

	It("includes insecure cipher suites", func() {
		insecureCiphers := []string{
			"TLS_RSA_WITH_AES_128_CBC_SHA256",
		}

		result := config.CipherSuitesFromConfig(insecureCiphers)
		Expect(result).To(HaveLen(1))
		Expect(result[0]).To(Equal(tls.TLS_RSA_WITH_AES_128_CBC_SHA256))
	})

	It("skips unknown cipher suites", func() {
		invalidCipher := "INVALID_CIPHER"

		result := config.CipherSuitesFromConfig([]string{invalidCipher})
		Expect(result).To(BeEmpty())
	})
})

var _ = Describe("ValidateTLSVersion", func() {
	DescribeTable("should return nil for a supported TLS version",
		func(version string) {
			err := config.ValidateTLSVersion(version)
			Expect(err).ToNot(HaveOccurred())
		},
		Entry("VersionTLS10", config.VersionTLS10),
		Entry("VersionTLS11", config.VersionTLS11),
		Entry("VersionTLS12", config.VersionTLS12),
		Entry("VersionTLS13", config.VersionTLS13),
	)

	It("should return an error for an unsupported TLS version (invalid version)", func() {
		err := config.ValidateTLSVersion("InvalidVersion")
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("ValidateCipherSuites", func() {
	It("should return nil for valid cipher suites", func() {
		// Assume these are supported cipher suite names
		validCipherSuites := []string{"TLS_RSA_WITH_AES_128_CBC_SHA", "TLS_RSA_WITH_AES_256_CBC_SHA"}

		err := config.ValidateCipherSuites(validCipherSuites)
		Expect(err).ToNot(HaveOccurred())
	})

	It("should return an error with the unsupported cipher suite names", func() {
		unsupportedCipherSuites := []string{"UNSUPPORTED_CIPHER_SUITE"}

		err := config.ValidateCipherSuites(unsupportedCipherSuites)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("UNSUPPORTED_CIPHER_SUITE"))
	})

	It("should return an error for the unsupported cipher suites", func() {
		mixedCipherSuites := []string{
			"TLS_RSA_WITH_AES_128_CBC_SHA", // supported
			"UNSUPPORTED_CIPHER_SUITE",     // unsupported
		}

		err := config.ValidateCipherSuites(mixedCipherSuites)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("UNSUPPORTED_CIPHER_SUITE"))
	})

	It("should return nil without error", func() {
		err := config.ValidateCipherSuites([]string{})
		Expect(err).ToNot(HaveOccurred())
	})
})
