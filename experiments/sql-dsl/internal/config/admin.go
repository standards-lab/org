package config

import (
	"fmt"
	"os"
	"strconv"

	libconfig "github.com/standards-lab/go-core/config"
)

// AdminConfig is the administrative layer's block: the switches that decide
// what an environment's admin services may do.
type AdminConfig struct {
	// Seed enables seeding at startup and on demand. Tri-state: nil is
	// unset and takes the default, off.
	Seed *bool `json:"seed"`
	// Env records the environment-variable names Finalize composed and read.
	Env AdminEnv `json:"-"`
}

// AdminEnv is the environment-variable names of the admin block.
type AdminEnv struct {
	Seed string
}

// SeedEnabled reports the finalized switch.
func (c *AdminConfig) SeedEnabled() bool { return c.Seed != nil && *c.Seed }

// Merge overlays src's set fields onto the receiver.
func (c *AdminConfig) Merge(src *AdminConfig) {
	if src == nil {
		return
	}
	if src.Seed != nil {
		v := *src.Seed
		c.Seed = &v
	}
}

// Finalize applies the default and reads the environment override under
// envPrefix; an empty prefix disables the override.
func (c *AdminConfig) Finalize(envPrefix string) error {
	if c.Seed == nil {
		c.Seed = new(bool)
	}
	if envPrefix == "" {
		return nil
	}
	c.Env.Seed = libconfig.EnvName(envPrefix, "admin_seed")
	if v := os.Getenv(c.Env.Seed); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return fmt.Errorf("%s: %w", c.Env.Seed, err)
		}
		c.Seed = &b
	}
	return nil
}
