// Package schema is the service side of the schema lifecycle: the embedded
// migration set and reference data, the startup stage that verifies or
// applies them under the configured mode, and the diagnostics the one-shot
// -schema mode and a future management surface read. It is what the
// template would scaffold; the mechanisms it triggers live under lib/.
package schema
