package config

import (
	"fmt"
	"os"
	"strconv"

	libconfig "github.com/standards-lab/go-core/config"
)

// Schema modes: what startup does about the embedded migration set.
const (
	// SchemaVerify checks that the applied version is the embedded set's
	// head and that every statement prepares; it never changes the schema.
	SchemaVerify = "verify"
	// SchemaApply applies pending migrations under the lock, then verifies.
	SchemaApply = "apply"
	// SchemaNone skips the schema stage entirely.
	SchemaNone = "none"
)

// SchemaConfig is the service-owned schema policy: the startup mode, and
// whether reference data is loaded after the schema is current. Seed is a
// tri-state pointer so an overlay can turn it off explicitly.
type SchemaConfig struct {
	Mode string    `json:"mode"`
	Seed *bool     `json:"seed"`
	Env  SchemaEnv `json:"-"`
}

// SchemaEnv records the environment-variable names Finalize composed.
type SchemaEnv struct {
	Mode string
	Seed string
}

// Seeding reports whether reference data loads at startup.
func (c *SchemaConfig) Seeding() bool { return c.Seed != nil && *c.Seed }

// Merge overlays src's set fields onto the receiver.
func (c *SchemaConfig) Merge(src *SchemaConfig) {
	if src == nil {
		return
	}
	if src.Mode != "" {
		c.Mode = src.Mode
	}
	if src.Seed != nil {
		c.Seed = src.Seed
	}
}

// Finalize applies the verify default, reads the environment overrides under
// envPrefix when one is given, and validates the mode.
func (c *SchemaConfig) Finalize(envPrefix string) error {
	if c.Mode == "" {
		c.Mode = SchemaVerify
	}
	if envPrefix != "" {
		c.Env.Mode = libconfig.EnvName(envPrefix, "schema", "mode")
		c.Env.Seed = libconfig.EnvName(envPrefix, "schema", "seed")
		if v := os.Getenv(c.Env.Mode); v != "" {
			c.Mode = v
		}
		if v := os.Getenv(c.Env.Seed); v != "" {
			b, err := strconv.ParseBool(v)
			if err != nil {
				return fmt.Errorf("%s: %w", c.Env.Seed, err)
			}
			c.Seed = &b
		}
	}
	switch c.Mode {
	case SchemaVerify, SchemaApply, SchemaNone:
		return nil
	default:
		return fmt.Errorf("mode must be verify, apply, or none, got %q", c.Mode)
	}
}
