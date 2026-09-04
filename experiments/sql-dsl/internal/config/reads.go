package config

import (
	"fmt"
	"os"
	"strconv"

	libconfig "github.com/standards-lab/go-core/config"
	"github.com/standards-lab/go-web-sdk"
)

// The service's paging policy defaults, applied by Finalize when the
// configuration leaves a field unset.
const (
	defaultReadsDefaultSize = 20
	defaultReadsMaxSize     = 100
)

// ReadsConfig is the service-owned read policy: the page size a request gets
// when it asks for none, and the largest size it may ask for. The SDK holds
// no policy numbers; this block is their single source, handed to each
// handler constructor as web.Limits at the composition root. Pointer fields
// distinguish unset from zero.
type ReadsConfig struct {
	DefaultSize *int `json:"default_size"`
	MaxSize     *int `json:"max_size"`
}

// Merge overlays src's set fields onto the receiver.
func (c *ReadsConfig) Merge(src *ReadsConfig) {
	if src == nil {
		return
	}
	if src.DefaultSize != nil {
		c.DefaultSize = src.DefaultSize
	}
	if src.MaxSize != nil {
		c.MaxSize = src.MaxSize
	}
}

// Finalize applies the defaults, reads the block's environment overrides
// when a prefix is given, and validates: DefaultSize at least 1 and MaxSize
// at least DefaultSize — the invariant web.ParseQuery panics on, caught here
// as configuration rather than at the first request.
func (c *ReadsConfig) Finalize(envPrefix string) error {
	if c.DefaultSize == nil {
		c.DefaultSize = new(defaultReadsDefaultSize)
	}
	if c.MaxSize == nil {
		c.MaxSize = new(defaultReadsMaxSize)
	}
	if envPrefix != "" {
		for key, dst := range map[string]*int{"reads_default_size": c.DefaultSize, "reads_max_size": c.MaxSize} {
			name := libconfig.EnvName(envPrefix, key)
			if v := os.Getenv(name); v != "" {
				n, err := strconv.Atoi(v)
				if err != nil {
					return fmt.Errorf("%s: %w", name, err)
				}
				*dst = n
			}
		}
	}
	if *c.DefaultSize < 1 {
		return fmt.Errorf("default_size must be at least 1, got %d", *c.DefaultSize)
	}
	if *c.MaxSize < *c.DefaultSize {
		return fmt.Errorf("max_size must be at least default_size (%d), got %d", *c.DefaultSize, *c.MaxSize)
	}
	return nil
}

// Limits hands the finalized policy to a handler constructor.
func (c ReadsConfig) Limits() web.Limits {
	return web.Limits{DefaultSize: *c.DefaultSize, MaxSize: *c.MaxSize}
}
