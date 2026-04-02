package supplychain_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/cri-o/cri-o/internal/supplychain"
)

var _ = Describe("Policy", func() {
	var tmpDir string

	BeforeEach(func() {
		var err error

		tmpDir, err = os.MkdirTemp("", "supplychain-policy-test-*")
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		os.RemoveAll(tmpDir)
	})

	writePolicy := func(name string, content string) {
		path := filepath.Join(tmpDir, name+".json")
		Expect(os.WriteFile(path, []byte(content), 0o644)).To(Succeed())
	}

	Describe("LoadPolicy", func() {
		It("should load a valid policy file", func() {
			policyJSON := `{
				"trust": {
					"builders": [
						{"id": "https://github.com/actions/runner", "max_level": 3},
						{"id": "https://tekton.dev/chains/v2", "max_level": 2}
					],
					"verifiers": [
						{"id": "https://verify.example.com", "key": "/etc/keys/v.pub"}
					],
					"issuers": ["https://accounts.google.com"],
					"sources": ["github.com/my-org/*"],
					"build_types": ["https://slsa.dev/container-based-build/v0.1"]
				}
			}`

			path := filepath.Join(tmpDir, "default.json")
			Expect(os.WriteFile(path, []byte(policyJSON), 0o644)).To(Succeed())

			policy, err := supplychain.LoadPolicy(path)
			Expect(err).ToNot(HaveOccurred())
			Expect(policy.Trust.Builders).To(HaveLen(2))
			Expect(policy.Trust.Builders[0].ID).To(Equal("https://github.com/actions/runner"))
			Expect(policy.Trust.Builders[0].MaxLevel).To(Equal(3))
			Expect(policy.Trust.Builders[1].MaxLevel).To(Equal(2))
			Expect(policy.Trust.Verifiers).To(HaveLen(1))
			Expect(policy.Trust.Verifiers[0].Key).To(Equal("/etc/keys/v.pub"))
			Expect(policy.Trust.Issuers).To(ConsistOf("https://accounts.google.com"))
			Expect(policy.Trust.Sources).To(ConsistOf("github.com/my-org/*"))
			Expect(policy.Trust.BuildTypes).To(ConsistOf("https://slsa.dev/container-based-build/v0.1"))
		})

		It("should return an error for a non-existent file", func() {
			_, err := supplychain.LoadPolicy("/does/not/exist.json")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("reading policy file"))
		})

		It("should return an error for invalid JSON", func() {
			path := filepath.Join(tmpDir, "bad.json")
			Expect(os.WriteFile(path, []byte("not json"), 0o644)).To(Succeed())

			_, err := supplychain.LoadPolicy(path)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("parsing policy file"))
		})

		It("should handle an empty policy file", func() {
			path := filepath.Join(tmpDir, "empty.json")
			Expect(os.WriteFile(path, []byte("{}"), 0o644)).To(Succeed())

			policy, err := supplychain.LoadPolicy(path)
			Expect(err).ToNot(HaveOccurred())
			Expect(policy.Trust).To(BeNil())
			Expect(policy.Builders()).To(BeEmpty())
		})

		It("should reject a builder with empty id", func() {
			writePolicy("bad-builder", `{"trust": {"builders": [{"id": "", "max_level": 1}]}}`)

			_, err := supplychain.LoadPolicy(filepath.Join(tmpDir, "bad-builder.json"))
			Expect(err).To(MatchError(ContainSubstring("id is required")))
		})

		It("should reject a builder with invalid max_level", func() {
			writePolicy("bad-level", `{"trust": {"builders": [{"id": "https://example.com", "max_level": 5}]}}`)

			_, err := supplychain.LoadPolicy(filepath.Join(tmpDir, "bad-level.json"))
			Expect(err).To(MatchError(ContainSubstring("max_level must be 0-3")))
		})

		It("should reject a verifier with empty key", func() {
			writePolicy("bad-verifier", `{"trust": {"verifiers": [{"id": "https://example.com", "key": ""}]}}`)

			_, err := supplychain.LoadPolicy(filepath.Join(tmpDir, "bad-verifier.json"))
			Expect(err).To(MatchError(ContainSubstring("key is required")))
		})

		It("should reject a verifier with relative key path", func() {
			writePolicy("bad-key-path", `{"trust": {"verifiers": [{"id": "https://example.com", "key": "relative/path.pub"}]}}`)

			_, err := supplychain.LoadPolicy(filepath.Join(tmpDir, "bad-key-path.json"))
			Expect(err).To(MatchError(ContainSubstring("key must be an absolute path")))
		})

		It("should reject invalid provenance.missing_policy", func() {
			writePolicy("bad-prov", `{"provenance": {"missing_policy": "bogus"}}`)

			_, err := supplychain.LoadPolicy(filepath.Join(tmpDir, "bad-prov.json"))
			Expect(err).To(MatchError(ContainSubstring("provenance.missing_policy")))
		})

		It("should accept valid provenance.missing_policy values", func() {
			for _, val := range []string{"allow", "warn", "deny"} {
				writePolicy("prov-valid", `{"provenance": {"missing_policy": "`+val+`"}}`)

				_, err := supplychain.LoadPolicy(filepath.Join(tmpDir, "prov-valid.json"))
				Expect(err).ToNot(HaveOccurred())
			}
		})

		It("should accept valid vex.severity_threshold values", func() {
			for _, sev := range []string{"low", "medium", "high", "critical"} {
				writePolicy("vex-sev", `{"vex": {"severity_threshold": "`+sev+`"}}`)

				_, err := supplychain.LoadPolicy(filepath.Join(tmpDir, "vex-sev.json"))
				Expect(err).ToNot(HaveOccurred())
			}
		})

		It("should reject invalid vex.severity_threshold", func() {
			writePolicy("vex-bad-sev", `{"vex": {"severity_threshold": "bogus"}}`)

			_, err := supplychain.LoadPolicy(filepath.Join(tmpDir, "vex-bad-sev.json"))
			Expect(err).To(MatchError(ContainSubstring("vex.severity_threshold")))
		})

		It("should reject invalid vex.missing_policy", func() {
			writePolicy("vex-bad-missing", `{"vex": {"missing_policy": "bogus"}}`)

			_, err := supplychain.LoadPolicy(filepath.Join(tmpDir, "vex-bad-missing.json"))
			Expect(err).To(MatchError(ContainSubstring("vex.missing_policy")))
		})

		It("should reject invalid vex.under_investigation_policy", func() {
			writePolicy("vex-bad-ui", `{"vex": {"under_investigation_policy": "bogus"}}`)

			_, err := supplychain.LoadPolicy(filepath.Join(tmpDir, "vex-bad-ui.json"))
			Expect(err).To(MatchError(ContainSubstring("vex.under_investigation_policy")))
		})

		It("should accept empty nested fields as defaults", func() {
			writePolicy("empty-nested", `{}`)

			policy, err := supplychain.LoadPolicy(filepath.Join(tmpDir, "empty-nested.json"))
			Expect(err).ToNot(HaveOccurred())
			Expect(policy.Trust).To(BeNil())
			Expect(policy.VEX).To(BeNil())
			Expect(policy.Provenance).To(BeNil())
			Expect(policy.VSA).To(BeNil())
			Expect(policy.Signatures).To(BeNil())
		})

		It("should reject invalid vsa.minimum_level", func() {
			writePolicy("vsa-bad-level", `{"vsa": {"minimum_level": 5}}`)

			_, err := supplychain.LoadPolicy(filepath.Join(tmpDir, "vsa-bad-level.json"))
			Expect(err).To(MatchError(ContainSubstring("vsa.minimum_level")))
		})

		It("should reject invalid vsa.max_age", func() {
			writePolicy("vsa-bad-age", `{"vsa": {"max_age": "not-a-duration"}}`)

			_, err := supplychain.LoadPolicy(filepath.Join(tmpDir, "vsa-bad-age.json"))
			Expect(err).To(MatchError(ContainSubstring("vsa.max_age")))
		})

		It("should accept valid vsa.max_age", func() {
			writePolicy("vsa-good-age", `{"vsa": {"max_age": "168h"}}`)

			policy, err := supplychain.LoadPolicy(filepath.Join(tmpDir, "vsa-good-age.json"))
			Expect(err).ToNot(HaveOccurred())
			Expect(policy.VSA.MaxAge).To(Equal("168h"))
		})

		It("should accept all per-namespace fields", func() {
			writePolicy("full", `{
				"trust": {
					"builders": [{"id": "https://example.com", "max_level": 3}],
					"verifiers": [{"id": "https://verify.example.com", "key": "/etc/keys/v.pub"}],
					"issuers": ["https://accounts.google.com"],
					"sources": ["github.com/my-org/*"],
					"build_types": ["https://slsa.dev/container-based-build/v0.1"]
				},
				"exclude": ["registry.k8s.io/pause:*"],
				"provenance": {
					"missing_policy": "deny",
					"reject_unknown_parameters": true
				},
				"vex": {
					"severity_threshold": "high",
					"missing_policy": "deny",
					"under_investigation_policy": "warn"
				},
				"vsa": {
					"minimum_level": 2,
					"max_age": "72h",
					"policy": "https://example.com/policy"
				},
				"signatures": {
					"require_transparency_log": true
				}
			}`)

			policy, err := supplychain.LoadPolicy(filepath.Join(tmpDir, "full.json"))
			Expect(err).ToNot(HaveOccurred())
			Expect(policy.Trust.Builders).To(HaveLen(1))
			Expect(policy.Trust.Verifiers).To(HaveLen(1))
			Expect(policy.Trust.Issuers).To(ConsistOf("https://accounts.google.com"))
			Expect(policy.Trust.Sources).To(ConsistOf("github.com/my-org/*"))
			Expect(policy.Trust.BuildTypes).To(ConsistOf("https://slsa.dev/container-based-build/v0.1"))
			Expect(policy.Provenance.MissingPolicy).To(Equal("deny"))
			Expect(policy.Provenance.RejectUnknownParameters).To(BeTrue())
			Expect(policy.VEX.SeverityThreshold).To(Equal("high"))
			Expect(policy.VEX.MissingPolicy).To(Equal("deny"))
			Expect(policy.VEX.UnderInvestigationPolicy).To(Equal("warn"))
			Expect(policy.VSA.MinimumLevel).To(Equal(2))
			Expect(policy.VSA.MaxAge).To(Equal("72h"))
			Expect(policy.VSA.Policy).To(Equal("https://example.com/policy"))
			Expect(policy.Signatures.RequireTransparencyLog).To(BeTrue())
			Expect(policy.Exclude).To(ConsistOf("registry.k8s.io/pause:*"))
		})

		It("should reject malformed exclude patterns", func() {
			writePolicy("bad-exempt", `{"exclude": ["[invalid"]}`)

			_, err := supplychain.LoadPolicy(filepath.Join(tmpDir, "bad-exempt.json"))
			Expect(err).To(MatchError(ContainSubstring("exclude pattern")))
		})

		It("should reject trailing content after JSON object", func() {
			writePolicy("trailing", `{"exclude": ["a:*"]}{"exclude": ["b:*"]}`)

			_, err := supplychain.LoadPolicy(filepath.Join(tmpDir, "trailing.json"))
			Expect(err).To(MatchError(ContainSubstring("unexpected trailing content")))
		})

		It("should reject trailing delimiters after JSON object", func() {
			writePolicy("trailing-delim", `{"exclude": ["a:*"]}]`)

			_, err := supplychain.LoadPolicy(filepath.Join(tmpDir, "trailing-delim.json"))
			Expect(err).To(MatchError(ContainSubstring("unexpected trailing content")))
		})

		It("should reject unknown fields", func() {
			writePolicy("unknown", `{"unknown_field": true}`)

			_, err := supplychain.LoadPolicy(filepath.Join(tmpDir, "unknown.json"))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("parsing policy file"))
		})
	})

	Describe("LoadPolicies", func() {
		It("should return empty map for empty policy dir", func() {
			policies, err := supplychain.LoadPolicies("")
			Expect(err).ToNot(HaveOccurred())
			Expect(policies).To(BeEmpty())
		})

		It("should return empty map for non-existent directory", func() {
			policies, err := supplychain.LoadPolicies("/nonexistent/path")
			Expect(err).ToNot(HaveOccurred())
			Expect(policies).To(BeEmpty())
		})

		It("should load default.json as empty-string key", func() {
			writePolicy("default", `{"trust": {"issuers": ["issuer-default"]}}`)

			policies, err := supplychain.LoadPolicies(tmpDir)
			Expect(err).ToNot(HaveOccurred())
			Expect(policies).To(HaveKey(""))
			Expect(policies[""].Trust.Issuers).To(ConsistOf("issuer-default"))
		})

		It("should load namespace-specific policies", func() {
			writePolicy("default", `{"trust": {"issuers": ["issuer-default"]}}`)
			writePolicy("production", `{"trust": {"issuers": ["issuer-prod"]}}`)

			policies, err := supplychain.LoadPolicies(tmpDir)
			Expect(err).ToNot(HaveOccurred())
			Expect(policies).To(HaveLen(2))
			Expect(policies[""].Trust.Issuers).To(ConsistOf("issuer-default"))
			Expect(policies["production"].Trust.Issuers).To(ConsistOf("issuer-prod"))
		})

		It("should skip non-json files", func() {
			writePolicy("default", `{"trust": {"issuers": ["issuer-default"]}}`)
			Expect(os.WriteFile(filepath.Join(tmpDir, "README.md"), []byte("hi"), 0o644)).To(Succeed())

			policies, err := supplychain.LoadPolicies(tmpDir)
			Expect(err).ToNot(HaveOccurred())
			Expect(policies).To(HaveLen(1))
		})

		It("should return an error for malformed policy file", func() {
			writePolicy("broken", "not json at all")

			_, err := supplychain.LoadPolicies(tmpDir)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("parsing policy file"))
		})
	})

	Describe("ProvenanceMissingPolicy", func() {
		It("should return allow by default", func() {
			p := &supplychain.Policy{}
			Expect(p.ProvenanceMissingPolicy()).To(Equal("allow"))
		})

		It("should return the configured value", func() {
			p := &supplychain.Policy{
				Provenance: &supplychain.ProvenancePolicy{MissingPolicy: "deny"},
			}
			Expect(p.ProvenanceMissingPolicy()).To(Equal("deny"))
		})
	})

	Describe("Builders", func() {
		It("should return nil when trust is not configured", func() {
			p := &supplychain.Policy{}
			Expect(p.Builders()).To(BeEmpty())
		})

		It("should return builders when configured", func() {
			p := &supplychain.Policy{
				Trust: &supplychain.TrustPolicy{
					Builders: []supplychain.TrustedBuilder{
						{ID: "https://example.com", MaxLevel: 3},
					},
				},
			}
			Expect(p.Builders()).To(HaveLen(1))
		})
	})
})
