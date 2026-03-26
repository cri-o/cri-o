package supplychain_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	crierrors "k8s.io/cri-api/pkg/errors"

	"github.com/cri-o/cri-o/internal/supplychain"
)

const (
	warnMode = "warn"
	denyMode = "deny"
)

var _ = Describe("Verifier", func() {
	var cfg supplychain.Config

	BeforeEach(func() {
		cfg = supplychain.DefaultConfig()
	})

	Describe("NewVerifier", func() {
		It("should return a verifier when verification is disabled", func() {
			cfg.Verification = "disabled"
			v, err := supplychain.NewVerifier(&cfg)
			Expect(err).ToNot(HaveOccurred())
			Expect(v).ToNot(BeNil())
		})

		It("should return a verifier when verification is enabled", func() {
			cfg.Verification = warnMode
			v, err := supplychain.NewVerifier(&cfg)
			Expect(err).ToNot(HaveOccurred())
			Expect(v).ToNot(BeNil())
		})
	})

	Describe("Verify", func() {
		var (
			ctx    context.Context
			tmpDir string
		)

		BeforeEach(func() {
			ctx = context.Background()

			var err error

			tmpDir, err = os.MkdirTemp("", "supplychain-verify-test-*")
			Expect(err).ToNot(HaveOccurred())

			cfg.Verification = "enforce"
			cfg.PolicyDir = tmpDir
		})

		AfterEach(func() {
			os.RemoveAll(tmpDir)
		})

		It("should short-circuit when verification is disabled", func() {
			cfg.Verification = "disabled"
			v, err := supplychain.NewVerifier(&cfg)
			Expect(err).ToNot(HaveOccurred())

			result, err := v.Verify(ctx, "myimage:latest", "sha256:abc", "default")
			Expect(err).ToNot(HaveOccurred())
			Expect(result.Allowed).To(BeTrue())
			Expect(result.Reason).To(ContainSubstring("disabled"))
		})

		It("should allow excluded images", func() {
			Expect(os.WriteFile(
				filepath.Join(tmpDir, "default.json"),
				[]byte(`{"exclude": ["registry.k8s.io/pause:*"]}`),
				0o644,
			)).To(Succeed())

			v, err := supplychain.NewVerifier(&cfg)
			Expect(err).ToNot(HaveOccurred())

			result, err := v.Verify(ctx, "registry.k8s.io/pause:3.9", "sha256:abc", "default")
			Expect(err).ToNot(HaveOccurred())
			Expect(result.Allowed).To(BeTrue())
			Expect(result.Reason).To(ContainSubstring("excluded"))
		})

		It("should not exclude non-matching images", func() {
			Expect(os.WriteFile(
				filepath.Join(tmpDir, "default.json"),
				[]byte(`{"exclude": ["registry.k8s.io/pause:*"]}`),
				0o644,
			)).To(Succeed())

			v, err := supplychain.NewVerifier(&cfg)
			Expect(err).ToNot(HaveOccurred())

			result, err := v.Verify(ctx, "docker.io/library/nginx:latest", "sha256:def", "default")
			Expect(err).ToNot(HaveOccurred())
			Expect(result.Reason).ToNot(ContainSubstring("excluded"))
		})

		It("should support multiple exclude patterns", func() {
			Expect(os.WriteFile(
				filepath.Join(tmpDir, "default.json"),
				[]byte(`{"exclude": ["registry.k8s.io/pause:*", "gcr.io/internal/*"]}`),
				0o644,
			)).To(Succeed())

			v, err := supplychain.NewVerifier(&cfg)
			Expect(err).ToNot(HaveOccurred())

			result, err := v.Verify(ctx, "gcr.io/internal/sidecar", "sha256:xyz", "default")
			Expect(err).ToNot(HaveOccurred())
			Expect(result.Allowed).To(BeTrue())
			Expect(result.Reason).To(ContainSubstring("excluded"))
		})

		It("should use namespace-specific policy when available", func() {
			// Default policy: no builders (passes even with deny).
			Expect(os.WriteFile(
				filepath.Join(tmpDir, "default.json"),
				[]byte(`{}`),
				0o644,
			)).To(Succeed())

			// Production policy: has builders + deny, so missing provenance triggers deny.
			Expect(os.WriteFile(
				filepath.Join(tmpDir, "production.json"),
				[]byte(`{"trust": {"builders": [{"id": "https://example.com", "max_level": 3}]}, "provenance": {"missing_policy": "deny"}}`),
				0o644,
			)).To(Succeed())

			v, err := supplychain.NewVerifier(&cfg)
			Expect(err).ToNot(HaveOccurred())

			// Default namespace should pass (no builders configured).
			result, err := v.Verify(ctx, "myimage:latest", "sha256:abc", "default")
			Expect(err).ToNot(HaveOccurred())
			Expect(result.Allowed).To(BeTrue())

			// Production namespace should fail (has builders + deny policy).
			_, err = v.Verify(ctx, "myimage:latest", "sha256:def", "production")
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, crierrors.ErrSignatureValidationFailed)).To(BeTrue())
		})

		It("should return cached results on subsequent calls", func() {
			cfg.CacheTTL = 1 * time.Hour
			v, err := supplychain.NewVerifier(&cfg)
			Expect(err).ToNot(HaveOccurred())

			result1, err := v.Verify(ctx, "myimage:latest", "sha256:abc", "default")
			Expect(err).ToNot(HaveOccurred())

			result2, err := v.Verify(ctx, "myimage:latest", "sha256:abc", "default")
			Expect(err).ToNot(HaveOccurred())

			// Same pointer from cache.
			Expect(result1).To(BeIdenticalTo(result2))
		})

		It("should not return cached results after Reload", func() {
			cfg.CacheTTL = 1 * time.Hour
			v, err := supplychain.NewVerifier(&cfg)
			Expect(err).ToNot(HaveOccurred())

			result1, err := v.Verify(ctx, "myimage:latest", "sha256:abc", "default")
			Expect(err).ToNot(HaveOccurred())

			Expect(v.Reload(&cfg)).To(Succeed())

			result2, err := v.Verify(ctx, "myimage:latest", "sha256:abc", "default")
			Expect(err).ToNot(HaveOccurred())

			// Different pointer after reload.
			Expect(result1).ToNot(BeIdenticalTo(result2))
		})

		It("should reload config and policies on Reload", func() {
			v, err := supplychain.NewVerifier(&cfg)
			Expect(err).ToNot(HaveOccurred())

			// Without the new-ns policy, new-ns falls back to default (no builders), so passes.
			result, err := v.Verify(ctx, "myimage:latest", "sha256:abc", "new-ns")
			Expect(err).ToNot(HaveOccurred())
			Expect(result.Allowed).To(BeTrue())

			// Add a namespace policy with trusted builders + deny after initial creation.
			Expect(os.WriteFile(
				filepath.Join(tmpDir, "new-ns.json"),
				[]byte(`{"trust": {"builders": [{"id": "https://example.com", "max_level": 3}]}, "provenance": {"missing_policy": "deny"}}`),
				0o644,
			)).To(Succeed())

			Expect(v.Reload(&cfg)).To(Succeed())

			// After reload, new-ns has builders + deny policy, so verification fails.
			_, err = v.Verify(ctx, "myimage:latest", "sha256:def", "new-ns")
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, crierrors.ErrSignatureValidationFailed)).To(BeTrue())
		})

		It("should disable verification after Reload with disabled config", func() {
			v, err := supplychain.NewVerifier(&cfg)
			Expect(err).ToNot(HaveOccurred())

			disabledCfg := cfg
			disabledCfg.Verification = "disabled"
			Expect(v.Reload(&disabledCfg)).To(Succeed())

			result, err := v.Verify(ctx, "myimage:latest", "sha256:abc", "default")
			Expect(err).ToNot(HaveOccurred())
			Expect(result.Reason).To(ContainSubstring("disabled"))
		})

		It("should preserve verifier state when Reload fails", func() {
			v, err := supplychain.NewVerifier(&cfg)
			Expect(err).ToNot(HaveOccurred())

			// Verify works with the initial config.
			result, err := v.Verify(ctx, "myimage:latest", "sha256:abc", "default")
			Expect(err).ToNot(HaveOccurred())
			Expect(result.Allowed).To(BeTrue())

			// Reload with a config pointing to a directory containing a malformed policy.
			badDir, err := os.MkdirTemp("", "supplychain-bad-policy-*")
			Expect(err).ToNot(HaveOccurred())

			defer os.RemoveAll(badDir)

			Expect(os.WriteFile(filepath.Join(badDir, "default.json"), []byte("not json"), 0o644)).To(Succeed())

			badCfg := cfg
			badCfg.PolicyDir = badDir
			Expect(v.Reload(&badCfg)).To(HaveOccurred())

			// Verifier should still work with the original config after failed reload.
			result, err = v.Verify(ctx, "myimage:latest", "sha256:def", "default")
			Expect(err).ToNot(HaveOccurred())
			Expect(result.Allowed).To(BeTrue())
		})

		It("should return ErrSignatureValidationFailed in enforce mode with deny provenance policy", func() {
			Expect(os.WriteFile(
				filepath.Join(tmpDir, "default.json"),
				[]byte(`{"trust": {"builders": [{"id": "https://github.com/actions/runner", "max_level": 3}]}, "provenance": {"missing_policy": "deny"}}`),
				0o644,
			)).To(Succeed())

			v, err := supplychain.NewVerifier(&cfg)
			Expect(err).ToNot(HaveOccurred())

			result, err := v.Verify(ctx, "myimage:latest", "sha256:abc", "default")
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, crierrors.ErrSignatureValidationFailed)).To(BeTrue())
			Expect(result.Allowed).To(BeFalse())
		})

		It("should allow in warn mode even when provenance is denied", func() {
			cfg.Verification = warnMode

			Expect(os.WriteFile(
				filepath.Join(tmpDir, "default.json"),
				[]byte(`{"trust": {"builders": [{"id": "https://github.com/actions/runner", "max_level": 3}]}, "provenance": {"missing_policy": "deny"}}`),
				0o644,
			)).To(Succeed())

			v, err := supplychain.NewVerifier(&cfg)
			Expect(err).ToNot(HaveOccurred())

			result, err := v.Verify(ctx, "myimage:latest", "sha256:abc", "default")
			Expect(err).ToNot(HaveOccurred())
			Expect(result.Allowed).To(BeTrue())
		})

		It("should allow with provenance missing_policy=allow", func() {
			Expect(os.WriteFile(
				filepath.Join(tmpDir, "default.json"),
				[]byte(`{"trust": {"builders": [{"id": "https://github.com/actions/runner", "max_level": 3}]}, "provenance": {"missing_policy": "allow"}}`),
				0o644,
			)).To(Succeed())

			v, err := supplychain.NewVerifier(&cfg)
			Expect(err).ToNot(HaveOccurred())

			result, err := v.Verify(ctx, "myimage:latest", "sha256:abc", "default")
			Expect(err).ToNot(HaveOccurred())
			Expect(result.Allowed).To(BeTrue())
			Expect(result.CheckResults).To(HaveLen(1))
			Expect(result.CheckResults[0].Type).To(Equal("slsa_provenance"))
			Expect(result.CheckResults[0].Passed).To(BeTrue())
			Expect(result.CheckResults[0].Status).To(Equal("pass"))
		})

		It("should warn with provenance missing_policy=warn", func() {
			Expect(os.WriteFile(
				filepath.Join(tmpDir, "default.json"),
				[]byte(`{"trust": {"builders": [{"id": "https://github.com/actions/runner", "max_level": 3}]}, "provenance": {"missing_policy": "warn"}}`),
				0o644,
			)).To(Succeed())

			v, err := supplychain.NewVerifier(&cfg)
			Expect(err).ToNot(HaveOccurred())

			result, err := v.Verify(ctx, "myimage:latest", "sha256:abc", "default")
			Expect(err).ToNot(HaveOccurred())
			Expect(result.Allowed).To(BeTrue())
			Expect(result.CheckResults[0].Passed).To(BeTrue())
			Expect(result.CheckResults[0].Status).To(Equal("warn"))
			Expect(result.CheckResults[0].Detail).To(HavePrefix("warn:"))
		})

		It("should allow when no trusted builders are configured", func() {
			v, err := supplychain.NewVerifier(&cfg)
			Expect(err).ToNot(HaveOccurred())

			result, err := v.Verify(ctx, "myimage:latest", "sha256:abc", "default")
			Expect(err).ToNot(HaveOccurred())
			Expect(result.Allowed).To(BeTrue())
			Expect(result.CheckResults).To(HaveLen(1))
			Expect(result.CheckResults[0].Passed).To(BeTrue())
			Expect(result.CheckResults[0].Detail).To(ContainSubstring("no trusted builders configured"))
		})

		It("should allow when no trusted builders are configured even with deny provenance policy", func() {
			v, err := supplychain.NewVerifier(&cfg)
			Expect(err).ToNot(HaveOccurred())

			result, err := v.Verify(ctx, "myimage:latest", "sha256:abc", "default")
			Expect(err).ToNot(HaveOccurred())
			Expect(result.Allowed).To(BeTrue())
			Expect(result.CheckResults[0].Passed).To(BeTrue())
		})

		It("should allow in enforce mode with allow provenance policy and trusted builders", func() {
			Expect(os.WriteFile(
				filepath.Join(tmpDir, "default.json"),
				[]byte(`{"trust": {"builders": [{"id": "https://github.com/actions/runner", "max_level": 3}]}, "provenance": {"missing_policy": "allow"}}`),
				0o644,
			)).To(Succeed())

			v, err := supplychain.NewVerifier(&cfg)
			Expect(err).ToNot(HaveOccurred())

			result, err := v.Verify(ctx, "myimage:latest", "sha256:abc", "default")
			Expect(err).ToNot(HaveOccurred())
			Expect(result.Allowed).To(BeTrue())
			Expect(result.CheckResults[0].Detail).To(ContainSubstring("cosign integration pending"))
		})

		It("should cache the warn-adjusted result not the original denied result", func() {
			cfg.Verification = warnMode
			cfg.CacheTTL = 1 * time.Hour

			Expect(os.WriteFile(
				filepath.Join(tmpDir, "default.json"),
				[]byte(`{"trust": {"builders": [{"id": "https://github.com/actions/runner", "max_level": 3}]}, "provenance": {"missing_policy": "deny"}}`),
				0o644,
			)).To(Succeed())

			v, err := supplychain.NewVerifier(&cfg)
			Expect(err).ToNot(HaveOccurred())

			result1, err := v.Verify(ctx, "myimage:latest", "sha256:abc", "default")
			Expect(err).ToNot(HaveOccurred())
			Expect(result1.Allowed).To(BeTrue())

			// Second call should return cached result with Allowed=true.
			result2, err := v.Verify(ctx, "myimage:latest", "sha256:abc", "default")
			Expect(err).ToNot(HaveOccurred())
			Expect(result2.Allowed).To(BeTrue())
			Expect(result1).To(BeIdenticalTo(result2))
		})
	})
})
