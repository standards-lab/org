package blobfs_test

import (
	"errors"
	"testing"
	"uuid"

	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
)

func TestNewID(t *testing.T) {
	seen := make(map[string]bool)
	for range 1000 {
		id := blobfs.NewID()
		if seen[id] {
			t.Fatalf("NewID repeated %q", id)
		}
		seen[id] = true
		u, err := uuid.Parse(id)
		if err != nil {
			t.Fatalf("uuid.Parse(%q): %v", id, err)
		}
		if version := u[6] >> 4; version != 7 {
			t.Fatalf("NewID() = %q is version %d, want 7", id, version)
		}
		if len(id) != 36 {
			t.Fatalf("NewID() = %q has length %d, want the 36-character canonical form", id, len(id))
		}
	}
}

// TestSentinels fixes that each sentinel is distinct and matches only
// itself, so the persistence layer's mapping cannot alias two outcomes.
func TestSentinels(t *testing.T) {
	sentinels := []error{
		blobfs.ErrNotFound,
		blobfs.ErrNameTaken,
		blobfs.ErrInvalidName,
		blobfs.ErrInvalidKey,
		blobfs.ErrNotEmpty,
		blobfs.ErrInvalidTransition,
		blobfs.ErrDeleting,
		blobfs.ErrCycle,
	}
	for i, a := range sentinels {
		for j, b := range sentinels {
			if got := errors.Is(a, b); got != (i == j) {
				t.Errorf("errors.Is(%v, %v) = %v", a, b, got)
			}
		}
		if a.Error() == "" {
			t.Errorf("sentinel %d has an empty message", i)
		}
	}
}
