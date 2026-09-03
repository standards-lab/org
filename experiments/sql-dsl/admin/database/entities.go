package database

import (
	"errors"
	"time"
)

// ErrValidation classifies a rejected administrative request.
var ErrValidation = errors.New("validation failed")

// Diagnostics is one read of the database's health, with the pattern
// namespaces the catalog registered.
type Diagnostics struct {
	Dialect       string        `json:"dialect"`
	Ping          time.Duration `json:"ping"`
	ServerVersion string        `json:"server_version"`
	Pool          Pool          `json:"pool"`
	Namespaces    []string      `json:"namespaces"`
}

// Catalog is the pattern catalog as an operator reads it: every namespace
// the composition root registered and every pattern under them, in
// namespace then name order. It is build-time state; the read has no
// write.
type Catalog struct {
	Namespaces []string  `json:"namespaces"`
	Patterns   []Pattern `json:"patterns"`
}

// Pattern is one catalog entry: its namespace and name, its tier and
// native note, the slots its body declares, and the body as the library
// composes or splices it.
type Pattern struct {
	Namespace string   `json:"namespace"`
	Name      string   `json:"name"`
	Tier      string   `json:"tier"`
	Native    string   `json:"native,omitempty"`
	Slots     []string `json:"slots"`
	Text      string   `json:"text"`
}

// Pool is the connection pool's counters.
type Pool struct {
	Open         int           `json:"open"`
	InUse        int           `json:"in_use"`
	Idle         int           `json:"idle"`
	MaxOpen      int           `json:"max_open"`
	WaitCount    int64         `json:"wait_count"`
	WaitDuration time.Duration `json:"wait_duration"`
}

// Status is the schema's state against the embedded set: the applied head,
// whether it is dirty, the versions still pending, and whether the service
// reports ready (a clean, complete history).
type Status struct {
	Version    int             `json:"version"`
	Dirty      bool            `json:"dirty"`
	Pending    []int           `json:"pending"`
	Ready      bool            `json:"ready"`
	Migrations []MigrationInfo `json:"migrations"`
}

// MigrationInfo describes one migration of the embedded set.
type MigrationInfo struct {
	Version       int    `json:"version"`
	Name          string `json:"name"`
	Transactional bool   `json:"transactional"`
	Applied       bool   `json:"applied"`
}

// Steps is the body of a steps or down request.
type Steps struct {
	Steps int `json:"steps"`
}

// Force is the body of a force request.
type Force struct {
	Version int `json:"version"`
}
