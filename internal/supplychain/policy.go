package supplychain

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// Policy defines the trust roots and per-namespace verification settings.
// Loaded from JSON files in the policy directory.
type Policy struct {
	// Trust contains trust roots for verification (builders, verifiers, issuers, etc.).
	Trust *TrustPolicy `json:"trust,omitempty"`

	// Exclude is a list of glob patterns for images that skip verification.
	Exclude []string `json:"exclude,omitempty"`

	// Provenance contains SLSA provenance verification settings.
	Provenance *ProvenancePolicy `json:"provenance,omitempty"`
	// VEX contains VEX verification settings.
	VEX *VEXPolicy `json:"vex,omitempty"`
	// VSA contains Verification Summary Attestation settings.
	VSA *VSAPolicy `json:"vsa,omitempty"`
	// Signatures contains attestation signature verification settings.
	Signatures *SignaturesPolicy `json:"signatures,omitempty"`
}

// TrustPolicy contains trust roots for verification.
type TrustPolicy struct {
	// Builders is the list of trusted SLSA provenance builders.
	Builders []TrustedBuilder `json:"builders"`
	// Verifiers is the list of trusted VSA verifiers.
	Verifiers []TrustedVerifier `json:"verifiers"`
	// Issuers is the list of trusted signing identity issuers (Fulcio/OIDC).
	Issuers []string `json:"issuers"`
	// Sources is a list of allowed source repository patterns.
	Sources []string `json:"sources"`
	// BuildTypes is a list of accepted build type URIs.
	BuildTypes []string `json:"build_types"`
}

// ProvenancePolicy contains SLSA provenance verification settings.
type ProvenancePolicy struct {
	// MissingPolicy controls behavior when no provenance attestation is found.
	// Valid values: "allow" (default), "warn", "deny".
	MissingPolicy string `json:"missing_policy,omitempty"`
	// RejectUnknownParameters rejects provenance with unrecognized externalParameters fields.
	RejectUnknownParameters bool `json:"reject_unknown_parameters,omitempty"`
}

// VEXPolicy contains VEX verification settings.
type VEXPolicy struct {
	// SeverityThreshold is the minimum severity at which an "affected" VEX status
	// triggers rejection. Valid values: "low", "medium", "high", "critical".
	// Defaults to "critical" if empty.
	SeverityThreshold string `json:"severity_threshold,omitempty"`
	// MissingPolicy controls behavior when no VEX attestation is found.
	// Valid values: "allow" (default), "warn", "deny".
	MissingPolicy string `json:"missing_policy,omitempty"`
	// UnderInvestigationPolicy controls behavior for VEX "under_investigation" status.
	// Valid values: "allow" (default), "warn", "deny".
	UnderInvestigationPolicy string `json:"under_investigation_policy,omitempty"`
}

// VSAPolicy contains Verification Summary Attestation settings.
type VSAPolicy struct {
	// MinimumLevel is the minimum SLSA level required in VSA verifiedLevels (0-3).
	MinimumLevel int `json:"minimum_level,omitempty"`
	// MaxAge is the maximum age of a VSA's timeVerified before it's considered stale.
	// Uses Go duration format (e.g., "168h"). Defaults to "168h" if empty.
	MaxAge string `json:"max_age,omitempty"`
	// Policy is the expected policy URI in the VSA.
	Policy string `json:"policy,omitempty"`
}

// SignaturesPolicy contains attestation signature verification settings.
type SignaturesPolicy struct {
	// RequireTransparencyLog requires Rekor transparency log inclusion.
	RequireTransparencyLog bool `json:"require_transparency_log,omitempty"`
}

// TrustedBuilder represents a trusted SLSA provenance builder.
type TrustedBuilder struct {
	// ID is the builder identity URI (e.g., "https://github.com/actions/runner").
	ID string `json:"id"`
	// MaxLevel is the maximum SLSA level this builder can attest to (0-3).
	MaxLevel int `json:"max_level"`
}

// TrustedVerifier represents a trusted VSA verifier.
type TrustedVerifier struct {
	// ID is the verifier identity URI.
	ID string `json:"id"`
	// Key is the path to the verifier's public key file.
	Key string `json:"key"`
}

// ProvenanceMissingPolicy returns the effective provenance missing policy,
// defaulting to "allow" if not set.
func (p *Policy) ProvenanceMissingPolicy() string {
	if p.Provenance != nil && p.Provenance.MissingPolicy != "" {
		return p.Provenance.MissingPolicy
	}

	return "allow"
}

// Builders returns the trusted builders list, or nil if trust is not configured.
func (p *Policy) Builders() []TrustedBuilder {
	if p.Trust != nil {
		return p.Trust.Builders
	}

	return nil
}

// Validate checks the policy for invalid values.
func (p *Policy) Validate() error {
	if p.Trust != nil {
		for i, b := range p.Trust.Builders {
			if b.ID == "" {
				return fmt.Errorf("trust.builders[%d]: id is required", i)
			}

			if b.MaxLevel < 0 || b.MaxLevel > 3 {
				return fmt.Errorf("trust.builders[%d] %q: max_level must be 0-3, got %d", i, b.ID, b.MaxLevel)
			}
		}

		for i, v := range p.Trust.Verifiers {
			if v.ID == "" {
				return fmt.Errorf("trust.verifiers[%d]: id is required", i)
			}

			if v.Key == "" {
				return fmt.Errorf("trust.verifiers[%d] %q: key is required", i, v.ID)
			}

			if !filepath.IsAbs(v.Key) {
				return fmt.Errorf("trust.verifiers[%d] %q: key must be an absolute path, got %q", i, v.ID, v.Key)
			}
		}
	}

	for _, pattern := range p.Exclude {
		if _, err := path.Match(pattern, ""); err != nil {
			return fmt.Errorf("invalid exclude pattern %q: %w", pattern, err)
		}
	}

	if p.Provenance != nil {
		if p.Provenance.MissingPolicy != "" {
			if err := validatePolicyValue("provenance.missing_policy", p.Provenance.MissingPolicy); err != nil {
				return err
			}
		}
	}

	if p.VEX != nil {
		if p.VEX.SeverityThreshold != "" {
			switch p.VEX.SeverityThreshold {
			case "low", "medium", "high", "critical":
			default:
				return fmt.Errorf("invalid vex.severity_threshold %q", p.VEX.SeverityThreshold)
			}
		}

		if p.VEX.MissingPolicy != "" {
			if err := validatePolicyValue("vex.missing_policy", p.VEX.MissingPolicy); err != nil {
				return err
			}
		}

		if p.VEX.UnderInvestigationPolicy != "" {
			if err := validatePolicyValue("vex.under_investigation_policy", p.VEX.UnderInvestigationPolicy); err != nil {
				return err
			}
		}
	}

	if p.VSA != nil {
		if p.VSA.MinimumLevel < 0 || p.VSA.MinimumLevel > 3 {
			return fmt.Errorf("invalid vsa.minimum_level %d, must be 0-3", p.VSA.MinimumLevel)
		}

		if p.VSA.MaxAge != "" {
			if _, err := time.ParseDuration(p.VSA.MaxAge); err != nil {
				return fmt.Errorf("invalid vsa.max_age %q: %w", p.VSA.MaxAge, err)
			}
		}
	}

	return nil
}

func validatePolicyValue(name, value string) error {
	switch value {
	case "allow", "warn", "deny":
		return nil
	default:
		return fmt.Errorf("invalid %s %q", name, value)
	}
}

// LoadPolicy loads and validates a policy file from disk.
func LoadPolicy(policyPath string) (*Policy, error) {
	data, err := os.ReadFile(policyPath)
	if err != nil {
		return nil, fmt.Errorf("reading policy file %q: %w", policyPath, err)
	}

	var p Policy

	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()

	if err := dec.Decode(&p); err != nil {
		return nil, fmt.Errorf("parsing policy file %q: %w", policyPath, err)
	}

	if err := dec.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("parsing policy file %q: unexpected trailing content after JSON object", policyPath)
		}

		return nil, fmt.Errorf("parsing policy file %q: unexpected trailing content after JSON object: %w", policyPath, err)
	}

	if err := p.Validate(); err != nil {
		return nil, fmt.Errorf("invalid policy file %q: %w", policyPath, err)
	}

	return &p, nil
}

// LoadPolicies loads all policy files from the given directory.
// Returns a map keyed by namespace (empty string for default.json).
func LoadPolicies(policyDir string) (map[string]*Policy, error) {
	policies := make(map[string]*Policy)

	if policyDir == "" {
		return policies, nil
	}

	entries, err := os.ReadDir(policyDir)
	if err != nil {
		if os.IsNotExist(err) {
			return policies, nil
		}

		return nil, fmt.Errorf("reading policy directory %q: %w", policyDir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		fullPath := filepath.Join(policyDir, entry.Name())

		policy, err := LoadPolicy(fullPath)
		if err != nil {
			return nil, err
		}

		// "default.json" maps to empty string key (the fallback).
		namespace := strings.TrimSuffix(entry.Name(), ".json")
		if namespace == "default" {
			namespace = ""
		}

		policies[namespace] = policy
	}

	return policies, nil
}
