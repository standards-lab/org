package database

import (
	"errors"
	"time"
)

// ErrValidation classifies a rejected administrative request.
var ErrValidation = errors.New("validation failed")

// Diagnostics is one read of the database's health.
type Diagnostics struct {
	Dialect       string        `json:"dialect"`
	Ping          time.Duration `json:"ping"`
	ServerVersion string        `json:"server_version"`
	Pool          Pool          `json:"pool"`
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
