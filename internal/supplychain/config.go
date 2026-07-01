package supplychain

import (
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/cri-o/cri-o/utils"
)

// Config represents the "crio.image.supply_chain" TOML config table.
type Config struct {
	// Verification is the master toggle for supply chain verification.
	// Valid values: "disabled" (default), "warn" (log-only), "enforce" (reject on failure).
	Verification string `toml:"verification"`
	// FetchTimeout is the per-fetch timeout for retrieving attestations.
	// SLSA and VEX fetches run in parallel, each gets this timeout independently.
	FetchTimeout time.Duration `toml:"fetch_timeout"`
	// FetchFailurePolicy controls behavior when attestation fetch fails due to
	// network errors. Valid values: "allow", "warn" (default), "deny".
	FetchFailurePolicy string `toml:"fetch_failure_policy"`
	// CacheTTL is how long verification results are cached per image digest + namespace.
	CacheTTL time.Duration `toml:"cache_ttl"`
	// PolicyDir is the path to the directory containing JSON policy files for
	// per-namespace verification settings and trust roots. <dir>/default.json is the
	// base policy, <dir>/<namespace>.json overrides it for that namespace.
	PolicyDir string `toml:"policy_dir"`
}

// DefaultConfig returns the default supply chain verification config.
func DefaultConfig() Config {
	return Config{
		Verification:       "disabled",
		FetchTimeout:       30 * time.Second,
		FetchFailurePolicy: "warn",
		CacheTTL:           24 * time.Hour,
		PolicyDir:          "/etc/crio/supply-chain-policies",
	}
}

// Enabled returns true if supply chain verification is not disabled.
func (c *Config) Enabled() bool {
	return c.Verification != "disabled"
}

// Validate checks the Config for invalid values.
func (c *Config) Validate(onExecution bool) error {
	switch c.Verification {
	case "disabled", "warn", "enforce":
	default:
		return fmt.Errorf("invalid supply chain verification mode %q", c.Verification)
	}

	if !c.Enabled() {
		return nil
	}

	if err := validatePolicyValue("fetch_failure_policy", c.FetchFailurePolicy); err != nil {
		return err
	}

	if c.FetchTimeout <= 0 {
		return fmt.Errorf("supply chain fetch_timeout must be positive, got %s", c.FetchTimeout)
	}

	if c.CacheTTL < 0 {
		return fmt.Errorf("supply chain cache_ttl must be non-negative, got %s", c.CacheTTL)
	}

	if c.PolicyDir == "" {
		return errors.New("supply chain policy_dir must not be empty when verification is enabled")
	}

	if !filepath.IsAbs(c.PolicyDir) {
		return fmt.Errorf("supply chain policy_dir %q is not absolute", c.PolicyDir)
	}

	if onExecution {
		if err := utils.IsDirectory(c.PolicyDir); err != nil {
			return fmt.Errorf("invalid supply chain policy_dir %q: %w", c.PolicyDir, err)
		}
	}

	return nil
}
