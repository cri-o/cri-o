package supplychain_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/cri-o/cri-o/internal/supplychain"
)

const invalidValue = "invalid"

var _ = Describe("Config", func() {
	var cfg supplychain.Config

	BeforeEach(func() {
		cfg = supplychain.DefaultConfig()
	})

	Describe("DefaultConfig", func() {
		It("should have verification disabled", func() {
			Expect(cfg.Verification).To(Equal("disabled"))
			Expect(cfg.Enabled()).To(BeFalse())
		})
	})

	Describe("Validate", func() {
		It("should pass with defaults (disabled)", func() {
			Expect(cfg.Validate(false)).To(Succeed())
		})

		It("should accept valid verification modes", func() {
			for _, mode := range []string{"disabled", "warn", "enforce"} {
				cfg.Verification = mode
				Expect(cfg.Validate(false)).To(Succeed())
			}
		})

		It("should reject invalid verification mode", func() {
			cfg.Verification = invalidValue
			Expect(cfg.Validate(false)).To(MatchError(ContainSubstring("invalid supply chain verification mode")))
		})

		Context("when verification is enabled", func() {
			BeforeEach(func() {
				cfg.Verification = "enforce"
			})

			It("should pass with valid defaults", func() {
				Expect(cfg.Validate(false)).To(Succeed())
			})

			It("should reject invalid fetch_failure_policy", func() {
				cfg.FetchFailurePolicy = invalidValue
				Expect(cfg.Validate(false)).To(MatchError(ContainSubstring("fetch_failure_policy")))
			})

			It("should reject non-positive fetch_timeout", func() {
				cfg.FetchTimeout = 0
				Expect(cfg.Validate(false)).To(MatchError(ContainSubstring("fetch_timeout must be positive")))
			})

			It("should reject negative cache_ttl", func() {
				cfg.CacheTTL = -1
				Expect(cfg.Validate(false)).To(MatchError(ContainSubstring("cache_ttl must be non-negative")))
			})

			It("should reject relative policy_dir", func() {
				cfg.PolicyDir = "relative/path"
				Expect(cfg.Validate(false)).To(MatchError(ContainSubstring("not absolute")))
			})

			It("should reject empty policy_dir", func() {
				cfg.PolicyDir = ""
				Expect(cfg.Validate(false)).To(MatchError(ContainSubstring("policy_dir must not be empty")))
			})
		})

		Context("when verification is disabled", func() {
			It("should skip all further validation", func() {
				cfg.Verification = "disabled"
				cfg.FetchFailurePolicy = invalidValue
				Expect(cfg.Validate(false)).To(Succeed())
			})
		})
	})
})
