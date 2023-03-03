// Code generated
// This file is a generated precompile contract config with stubbed abstract functions.
// The file is generated by a template. Please inspect every code and comment in this file before use.

package warp

import (
	"math/big"

	"github.com/ava-labs/subnet-evm/precompile/precompileconfig"
)

var _ precompileconfig.Config = &Config{}

// Config implements the precompileconfig.Config interface and
// adds specific configuration for WarpMessenger.
type Config struct {
	precompileconfig.Upgrade
	QuorumNumerator   *big.Int `json:"quorumNumerator,omitempty"`
	QuorumDenominator *big.Int `json:"quorumDenominator,omitempty"`
}

// NewConfig returns a config for a network upgrade at [blockTimestamp] that enables
// WarpMessenger.
func NewConfig(blockTimestamp *big.Int) *Config {
	return &Config{
		Upgrade: precompileconfig.Upgrade{BlockTimestamp: blockTimestamp},
	}
}

// NewDisableConfig returns config for a network upgrade at [blockTimestamp]
// that disables WarpMessenger.
func NewDisableConfig(blockTimestamp *big.Int) *Config {
	return &Config{
		Upgrade: precompileconfig.Upgrade{
			BlockTimestamp: blockTimestamp,
			Disable:        true,
		},
	}
}

// Key returns the key for the WarpMessenger precompileconfig.
// This should be the same key as used in the precompile module.
func (*Config) Key() string { return ConfigKey }

// Verify tries to verify Config and returns an error accordingly.
func (c *Config) Verify() error {
	switch {
	case c.QuorumNumerator == nil && c.QuorumDenominator == nil:
		return nil
	case c.QuorumNumerator == nil:
		return ErrQuorumNilCheck
	case c.QuorumDenominator == nil:
		return ErrQuorumNilCheck
	case c.QuorumDenominator.Cmp(big.NewInt(0)) == 0:
		return ErrInvalidQuorumDenominator
	case c.QuorumNumerator.Cmp(c.QuorumDenominator) == 1:
		return ErrGreaterQuorumNumerator
	default:
		return nil
	}
}

// Equal returns true if [s] is a [*Config] and it has been configured identical to [c].
func (c *Config) Equal(s precompileconfig.Config) bool {
	// typecast before comparison
	other, ok := (s).(*Config)
	if !ok {
		return false
	}
	// CUSTOM CODE STARTS HERE
	// modify this boolean accordingly with your custom Config, to check if [other] and the current [c] are equal
	// if Config contains only Upgrade you can skip modifying it.
	equals := c.Upgrade.Equal(&other.Upgrade)
	if !equals {
		return false
	}

	if (c.QuorumNumerator == nil && other.QuorumNumerator != nil) || (other.QuorumNumerator == nil && c.QuorumNumerator != nil) {
		return false
	}

	if (c.QuorumDenominator == nil && other.QuorumDenominator != nil) || (other.QuorumDenominator == nil && c.QuorumDenominator != nil) {
		return false
	}

	if c.QuorumNumerator != nil && c.QuorumNumerator.Cmp(other.QuorumNumerator) != 0 {
		return false
	}

	if c.QuorumDenominator != nil && c.QuorumDenominator.Cmp(other.QuorumDenominator) != 0 {
		return false
	}

	return true
}