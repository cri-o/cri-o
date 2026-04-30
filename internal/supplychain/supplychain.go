package supplychain

import (
	"context"
	"fmt"
	"path"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	crierrors "k8s.io/cri-api/pkg/errors"

	"github.com/cri-o/cri-o/internal/log"
)

// defaultEmptyPolicy is a shared zero-value policy for namespaces without a
// dedicated policy. Safe to share because Verify never mutates policies.
var defaultEmptyPolicy = &Policy{}

// Result represents the outcome of a supply chain verification.
type Result struct {
	// Allowed indicates whether the image passed verification.
	Allowed bool
	// Reason provides details about the verification decision.
	Reason string
	// CheckResults contains per-check outcomes for audit logging.
	CheckResults []CheckResult
}

// CheckResult represents the outcome of an individual verification check.
type CheckResult struct {
	// Type is the check type (e.g., "slsa_provenance", "vex", "vsa").
	Type string
	// Passed indicates whether this check passed.
	Passed bool
	// Status is the check outcome: "pass", "warn", or "fail".
	Status string
	// Detail provides additional information about the check result.
	Detail string
}

// verifierSnapshot holds the config, policies, and cache snapshotted atomically
// from Verifier under a single RLock, so that Verify uses a consistent triple.
// Including the cache ensures that in-flight Verify calls don't write stale
// results into a freshly created cache after Reload.
type verifierSnapshot struct {
	config   *Config
	policies map[string]*Policy
	cache    *Cache
}

// Verifier performs supply chain attestation verification on container images.
type Verifier struct {
	mu       sync.RWMutex
	config   *Config
	cache    *Cache
	policies map[string]*Policy // namespace -> policy, "" key = default
}

// NewVerifier creates a new supply chain Verifier with the given configuration.
func NewVerifier(cfg *Config) (*Verifier, error) {
	cfgCopy := *cfg

	v := &Verifier{
		config: &cfgCopy,
		cache:  NewCache(cfgCopy.CacheTTL),
	}

	if cfgCopy.Enabled() {
		policies, err := LoadPolicies(cfgCopy.PolicyDir)
		if err != nil {
			return nil, fmt.Errorf("loading supply chain policies: %w", err)
		}

		v.policies = policies
	}

	return v, nil
}

// snapshot returns a consistent copy of config, policies, and cache under a
// single lock. All three are safe to use after unlock because Reload replaces
// them atomically rather than mutating in-place.
func (v *Verifier) snapshot() verifierSnapshot {
	v.mu.RLock()
	defer v.mu.RUnlock()

	return verifierSnapshot{
		config:   v.config,
		policies: v.policies,
		cache:    v.cache,
	}
}

// Verify performs supply chain verification for the given image.
// The imageRef is the image reference string, digest is the resolved image digest,
// and namespace is the Kubernetes namespace (for per-namespace policy lookup).
// Returns nil error on success (allowed or warn-mode), or a wrapped
// ErrSignatureValidationFailed on enforce-mode rejection.
func (v *Verifier) Verify(ctx context.Context, imageRef, digest, namespace string) (*Result, error) {
	snap := v.snapshot()

	if !snap.config.Enabled() {
		return &Result{Allowed: true, Reason: "supply chain verification disabled"}, nil
	}

	log.Debugf(ctx, "Supply chain verification for image %s (digest: %s, namespace: %s)", imageRef, digest, namespace)

	// Look up policy for namespace first (cheap map lookup).
	policy := policyForNamespace(snap.policies, namespace)

	// Check excluded images before the cache. Exclusions depend on imageRef,
	// but the cache is keyed by (digest, namespace). Checking exclusions
	// first avoids a cached non-excluded result blocking an excluded ref and
	// vice versa. Exclusion results are not cached because the check is
	// cheap and caching them could leak across different image refs that
	// share the same digest.
	if isExcluded(policy.Exclude, imageRef) {
		log.Infof(ctx, "Image %s is excluded from supply chain verification", imageRef)

		return &Result{Allowed: true, Reason: "image is excluded"}, nil
	}

	// Check cache after exclusions.
	if cached := snap.cache.Get(digest, namespace); cached != nil {
		CacheHitsTotal.Inc()
		logResult(ctx, imageRef, digest, namespace, cached)

		return cached, nil
	}

	// Run verification checks.
	result := runChecks(ctx, snap.config, policy, imageRef, digest)

	// Audit log the result.
	logResult(ctx, imageRef, digest, namespace, result)

	// Record metrics.
	recordMetrics(result)

	if !result.Allowed {
		if snap.config.Verification == "enforce" {
			// Don't cache denied results so that retries can succeed
			// immediately after the issue is fixed (e.g., provenance added).
			// Operators can SIGHUP to clear the cache if needed for other cases.
			return result, fmt.Errorf("%w: supply chain verification failed for %s: %s",
				crierrors.ErrSignatureValidationFailed, imageRef, result.Reason)
		}

		// Warn mode: log but allow. Create a new Result with Allowed=true
		// so that the cached entry reflects the warn-adjusted outcome.
		// If the mode later changes to "enforce" via SIGHUP, Reload creates
		// a fresh cache, so stale warn-adjusted entries won't leak.
		log.Warnf(ctx, "Supply chain verification failed for %s (warn mode, allowing): %s", imageRef, result.Reason)
		result = &Result{
			Allowed:      true,
			Reason:       result.Reason,
			CheckResults: result.CheckResults,
		}
	}

	// Cache the final (possibly warn-adjusted) result.
	snap.cache.Set(digest, namespace, result)

	return result, nil
}

// runChecks executes all configured verification checks for the image.
func runChecks(ctx context.Context, cfg *Config, policy *Policy, imageRef, digest string) *Result {
	result := &Result{Allowed: true}

	// SLSA provenance verification.
	slsaResult := verifySLSAProvenance(ctx, cfg, policy, imageRef, digest)
	result.CheckResults = append(result.CheckResults, slsaResult)

	if !slsaResult.Passed {
		result.Allowed = false
		result.Reason = slsaResult.Detail
	} else if slsaResult.Status == "warn" {
		result.Reason = slsaResult.Detail
	}

	return result
}

// verifySLSAProvenance verifies SLSA provenance attestations for the given image.
// The digest parameter will be used once cosign integration lands to match the subject digest.
func verifySLSAProvenance(_ context.Context, _ *Config, policy *Policy, imageRef, _ string) CheckResult {
	start := time.Now()

	defer func() {
		VerificationDuration.WithLabelValues("slsa_provenance").Observe(time.Since(start).Seconds())
	}()

	// If no trusted builders are configured, provenance verification is a no-op.
	// This is distinct from "missing attestation": the policy simply has no
	// builder trust roots, so there is nothing to verify.
	if len(policy.Builders()) == 0 {
		return CheckResult{
			Type:   "slsa_provenance",
			Passed: true,
			Status: "pass",
			Detail: "no trusted builders configured for image " + imageRef,
		}
	}

	// TODO: Fetch attestations via cosign API (OCI Referrers) and verify:
	// 1. Envelope signature against trusted issuers
	// 2. subject digest matches pulled image digest
	// 3. predicateType is SLSA provenance (v1 or v0.2)
	// 4. builder.id is in trusted builders list with sufficient level
	// 5. buildType matches trusted build types
	// 6. source repo matches trusted sources
	// 7. reject unknown externalParameters (if configured)
	//
	// For now, treat as "no provenance found" and apply the missing policy.
	return handleMissingAttestation(policy.ProvenanceMissingPolicy(), "slsa_provenance",
		fmt.Sprintf("provenance attestation not found for image %s (cosign integration pending)", imageRef))
}

// handleMissingAttestation applies the configured policy for a missing attestation type.
func handleMissingAttestation(policy, checkType, detail string) CheckResult {
	switch policy {
	case "deny":
		return CheckResult{Type: checkType, Passed: false, Status: "fail", Detail: detail}
	case "warn":
		return CheckResult{Type: checkType, Passed: true, Status: "warn", Detail: "warn: " + detail}
	case "allow":
		return CheckResult{Type: checkType, Passed: true, Status: "pass", Detail: detail}
	default:
		// Policy values are validated at config load time, so this is unreachable.
		// Default to deny as the safe fallback for a security-relevant decision.
		return CheckResult{Type: checkType, Passed: false, Status: "fail", Detail: detail}
	}
}

// logResult emits structured audit log entries for the verification decision.
func logResult(ctx context.Context, imageRef, digest, namespace string, result *Result) {
	for _, cr := range result.CheckResults {
		log.WithFields(ctx, logrus.Fields{
			"image":     imageRef,
			"digest":    digest,
			"namespace": namespace,
			"check":     cr.Type,
			"status":    cr.Status,
			"detail":    cr.Detail,
		}).Info("Supply chain audit")
	}
}

// recordMetrics updates prometheus counters for verification results.
func recordMetrics(result *Result) {
	for _, cr := range result.CheckResults {
		VerificationTotal.WithLabelValues(cr.Type, cr.Status).Inc()
	}
}

// Reload reloads the verifier's configuration and policies. Called on SIGHUP.
// Policies are loaded outside the lock to avoid blocking concurrent Verify calls
// during disk I/O.
func (v *Verifier) Reload(cfg *Config) error {
	cfgCopy := *cfg

	var policies map[string]*Policy

	if cfgCopy.Enabled() {
		var err error

		policies, err = LoadPolicies(cfgCopy.PolicyDir)
		if err != nil {
			return fmt.Errorf("reloading supply chain policies: %w", err)
		}
	}

	v.mu.Lock()
	defer v.mu.Unlock()

	v.config = &cfgCopy
	v.cache = NewCache(cfgCopy.CacheTTL)
	v.policies = policies

	return nil
}

// policyForNamespace returns the policy for the given namespace.
// Falls back to the default policy if no namespace-specific one exists.
func policyForNamespace(policies map[string]*Policy, namespace string) *Policy {
	if p, ok := policies[namespace]; ok {
		return p
	}

	if p, ok := policies[""]; ok {
		return p
	}

	return defaultEmptyPolicy
}

// isExcluded checks whether the image matches any exclude glob pattern.
// Note: path.Match treats '*' as matching a single path segment (not crossing '/').
// For example, "gcr.io/org/*" matches "gcr.io/org/app" but not "gcr.io/org/team/app".
func isExcluded(excludedImages []string, imageRef string) bool {
	for _, pattern := range excludedImages {
		matched, err := path.Match(pattern, imageRef)
		if err != nil {
			logrus.Debugf("Malformed exclude pattern %q for image %s: %v", pattern, imageRef, err)

			continue
		}

		if matched {
			return true
		}
	}

	return false
}
