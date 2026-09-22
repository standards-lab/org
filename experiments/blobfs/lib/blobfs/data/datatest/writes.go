package datatest

import (
	"errors"
	"testing"

	"github.com/standards-lab/sqlate"
	"github.com/standards-lab/sqlate/query"

	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs/data"
)

// acceptAll is the key validator the write groups pass: every key is
// accepted, since no object store is involved.
type acceptAll struct{}

func (acceptAll) ValidateKey(string) error { return nil }

// object is what the write groups report as the stored object.
var object = blobfs.Object{Size: 42, ContentType: "text/plain", ETag: `"etag"`}

// fileWrite checks the write protocol's begin and complete steps through
// the variant against the baseline: the same rows created and the same
// refusals, in text, for the same inputs.
func (s *suite) fileWrite(t *testing.T) {
	dir := s.mkdir(t, "write-"+t.Name())

	t.Run("BeginCreatesThePendingRow", func(t *testing.T) {
		id := blobfs.NewID()
		got, err := s.store.BeginFileWrite(s.ctx, s.db, acceptAll{}, dir.ID, "new.txt", "text/plain", data.WithID(id))
		if err != nil {
			t.Fatalf("BeginFileWrite: %v", err)
		}
		if got.ID != id || got.DirectoryID != dir.ID || got.Name != "new.txt" || got.Status != blobfs.StatusPending ||
			got.Key != id+"/new.txt" || got.Size != nil || got.ContentType != "text/plain" || got.ETag != nil || got.Version != 1 ||
			got.CreatedAt.IsZero() || !got.UpdatedAt.Equal(got.CreatedAt) {
			t.Errorf("BeginFileWrite returned %+v, want the pending row at version 1 with the table's defaults", got)
		}
		if stored := s.file(t, id); !equalFile(got, stored) {
			t.Errorf("BeginFileWrite returned\n%+v\nbut the database holds\n%+v", got, stored)
		}
		base, err := s.standard.BeginFileWrite(s.ctx, s.db, acceptAll{}, dir.ID, "base.txt", "text/plain")
		if err != nil {
			t.Fatalf("the baseline's BeginFileWrite: %v", err)
		}
		if !sameFileShape(got, base) {
			t.Errorf("the variant's row\n%+v\ndiffers from the baseline's\n%+v", got, base)
		}
	})
	t.Run("BeginRefusals", func(t *testing.T) {
		taken := s.insertFile(t, dir.ID, "taken.txt", blobfs.StatusAvailable)
		for _, c := range []struct {
			name       string
			dir, file  string
			opts       []data.WriteOption
			want       error
			constraint string
		}{
			{"NameTaken", dir.ID, "taken.txt", nil, blobfs.ErrNameTaken, blobfs.ConstraintUniqueFileDirectoryName},
			{"MissingDirectory", blobfs.NewID(), "orphan.txt", nil, blobfs.ErrNotFound, blobfs.ConstraintForeignKeyFileDirectory},
			{"IDTaken", dir.ID, "twin.txt", []data.WriteOption{data.WithID(taken)}, blobfs.ErrIDTaken, blobfs.ConstraintPrimaryKeyFile},
		} {
			t.Run(c.name, func(t *testing.T) {
				_, err := s.store.BeginFileWrite(s.ctx, s.db, acceptAll{}, c.dir, c.file, "text/plain", c.opts...)
				s.wantViolation(t, err, c.want, c.constraint)
				_, base := s.standard.BeginFileWrite(s.ctx, s.db, acceptAll{}, c.dir, c.file, "text/plain", c.opts...)
				s.wantSameError(t, err, base)
				_, _, err = s.store.BeginOrResumeFileWrite(s.ctx, s.db, acceptAll{}, c.dir, c.file, "text/plain", c.opts...)
				_, _, base = s.standard.BeginOrResumeFileWrite(s.ctx, s.db, acceptAll{}, c.dir, c.file, "text/plain", c.opts...)
				if c.name == "NameTaken" {
					// The taken name is found, not refused: the lookup runs first.
					if err != nil || base != nil {
						t.Errorf("BeginOrResumeFileWrite of a taken name = %v and %v on the baseline, want the row found", err, base)
					}
					return
				}
				s.wantViolation(t, err, c.want, c.constraint)
				s.wantSameError(t, err, base)
			})
		}
	})
	t.Run("BeginOrResume", func(t *testing.T) {
		for _, tier := range []struct {
			name  string
			store *data.Store
		}{{"variant", s.store}, {"baseline", s.standard}} {
			t.Run(tier.name, func(t *testing.T) {
				name := "resumed-" + tier.name + ".txt"
				created, outcome, err := tier.store.BeginOrResumeFileWrite(s.ctx, s.db, acceptAll{}, dir.ID, name, "text/plain")
				if err != nil || outcome != data.WriteCreated || created.Status != blobfs.StatusPending {
					t.Fatalf("first BeginOrResumeFileWrite = %+v, %s, %v; want the pending row created", created, outcome, err)
				}
				resumed, outcome, err := tier.store.BeginOrResumeFileWrite(s.ctx, s.db, acceptAll{}, dir.ID, name, "text/plain", data.WithID(blobfs.NewID()))
				if err != nil || outcome != data.WriteResumed || !equalFile(created, resumed) {
					t.Fatalf("second BeginOrResumeFileWrite = %+v, %s, %v; want the same row resumed", resumed, outcome, err)
				}
				completed, err := tier.store.CompleteFileWrite(s.ctx, s.db, created.ID, created.Version, object)
				if err != nil {
					t.Fatalf("CompleteFileWrite: %v", err)
				}
				exists, outcome, err := tier.store.BeginOrResumeFileWrite(s.ctx, s.db, acceptAll{}, dir.ID, name, "text/plain")
				if err != nil || outcome != data.WriteExists || !equalFile(completed, exists) {
					t.Fatalf("BeginOrResumeFileWrite of a completed write = %+v, %s, %v; want the available row as it is", exists, outcome, err)
				}
			})
		}
	})
	t.Run("CompleteMovesTheRowToAvailable", func(t *testing.T) {
		pending, err := s.store.BeginFileWrite(s.ctx, s.db, acceptAll{}, dir.ID, "complete.txt", "application/octet-stream")
		if err != nil {
			t.Fatalf("BeginFileWrite: %v", err)
		}
		got, err := s.store.CompleteFileWrite(s.ctx, s.db, pending.ID, pending.Version, object)
		if err != nil {
			t.Fatalf("CompleteFileWrite: %v", err)
		}
		if got.Status != blobfs.StatusAvailable || got.Version != pending.Version+1 || got.Size == nil || *got.Size != object.Size ||
			got.ContentType != object.ContentType || got.ETag == nil || *got.ETag != object.ETag || !got.UpdatedAt.After(pending.UpdatedAt) ||
			!got.CreatedAt.Equal(pending.CreatedAt) || got.Key != pending.Key || got.Name != pending.Name {
			t.Errorf("CompleteFileWrite returned %+v, want the available row at the next version carrying the object", got)
		}
		if stored := s.file(t, pending.ID); !equalFile(got, stored) {
			t.Errorf("CompleteFileWrite returned\n%+v\nbut the database holds\n%+v", got, stored)
		}
		basePending, err := s.standard.BeginFileWrite(s.ctx, s.db, acceptAll{}, dir.ID, "complete-base.txt", "application/octet-stream")
		if err != nil {
			t.Fatalf("the baseline's BeginFileWrite: %v", err)
		}
		base, err := s.standard.CompleteFileWrite(s.ctx, s.db, basePending.ID, basePending.Version, object)
		if err != nil {
			t.Fatalf("the baseline's CompleteFileWrite: %v", err)
		}
		if !sameFileShape(got, base) {
			t.Errorf("the variant's completed row\n%+v\ndiffers from the baseline's\n%+v", got, base)
		}
	})
	t.Run("CompleteRefusals", func(t *testing.T) {
		available, err := s.store.BeginFileWrite(s.ctx, s.db, acceptAll{}, dir.ID, "refused.txt", "text/plain")
		if err != nil {
			t.Fatalf("BeginFileWrite: %v", err)
		}
		if available, err = s.store.CompleteFileWrite(s.ctx, s.db, available.ID, available.Version, object); err != nil {
			t.Fatalf("CompleteFileWrite: %v", err)
		}
		deleting := s.insertFile(t, dir.ID, "deleting.txt", blobfs.StatusPending)
		s.begin(t, deleting)
		missing := blobfs.NewID()
		for _, c := range []struct {
			name    string
			id      string
			version int64
			want    error
			not     error
		}{
			{"NotFound", missing, 1, blobfs.ErrNotFound, query.ErrVersionMismatch},
			{"VersionMismatch", available.ID, available.Version - 1, query.ErrVersionMismatch, blobfs.ErrInvalidTransition},
			{"AlreadyAvailable", available.ID, available.Version, blobfs.ErrInvalidTransition, blobfs.ErrDeleting},
			{"Deleting", deleting, 2, blobfs.ErrDeleting, query.ErrVersionMismatch},
		} {
			t.Run(c.name, func(t *testing.T) {
				before := blobfs.File{}
				if c.id != missing {
					before = s.file(t, c.id)
				}
				_, err := s.store.CompleteFileWrite(s.ctx, s.db, c.id, c.version, object)
				if !errors.Is(err, c.want) || errors.Is(err, c.not) {
					t.Errorf("CompleteFileWrite = %v, want %v and not %v", err, c.want, c.not)
				}
				_, base := s.standard.CompleteFileWrite(s.ctx, s.db, c.id, c.version, object)
				s.wantSameError(t, err, base)
				if c.id != missing {
					if after := s.file(t, c.id); !equalFile(before, after) {
						t.Errorf("the refused completes changed the row to\n%+v\nfrom\n%+v", after, before)
					}
				}
			})
		}
	})
	t.Run("InsideATransaction", func(t *testing.T) {
		// The begin composes into a caller's transaction and a rollback
		// leaves no row, on every variant.
		id := blobfs.NewID()
		tx := s.beginTx(t)
		if _, err := s.store.BeginFileWrite(s.ctx, tx, acceptAll{}, dir.ID, "rolled-back.txt", "text/plain", data.WithID(id)); err != nil {
			_ = tx.Rollback()
			t.Fatalf("BeginFileWrite in a transaction: %v", err)
		}
		if err := tx.Rollback(); err != nil {
			t.Fatalf("Rollback: %v", err)
		}
		s.wantGone(t, id)
	})
}

// mkdir checks Mkdir and EnsureDirectory through the variant against the
// baseline: the same rows created and the same refusals, in text, for
// the same inputs.
func (s *suite) mkdirGroup(t *testing.T) {
	parent := s.mkdir(t, "mkdir-"+t.Name())

	t.Run("CreatesTheRow", func(t *testing.T) {
		id := blobfs.NewID()
		got, err := s.store.Mkdir(s.ctx, s.db, parent.ID, "child", data.WithID(id))
		if err != nil {
			t.Fatalf("Mkdir: %v", err)
		}
		if got.ID != id || got.ParentID == nil || *got.ParentID != parent.ID || got.Name != "child" || got.Version != 1 ||
			got.CreatedAt.IsZero() || !got.UpdatedAt.Equal(got.CreatedAt) {
			t.Errorf("Mkdir returned %+v, want the row at version 1 with the table's defaults", got)
		}
		if stored := s.directory(t, id); !equalDirectory(got, stored) {
			t.Errorf("Mkdir returned\n%+v\nbut the database holds\n%+v", got, stored)
		}
		base, err := s.standard.Mkdir(s.ctx, s.db, parent.ID, "base")
		if err != nil {
			t.Fatalf("the baseline's Mkdir: %v", err)
		}
		if *base.ParentID != *got.ParentID || base.Version != got.Version {
			t.Errorf("the variant's row\n%+v\ndiffers from the baseline's\n%+v", got, base)
		}
	})
	t.Run("Refusals", func(t *testing.T) {
		taken := s.mkdirUnder(t, parent.ID, "taken")
		for _, c := range []struct {
			name       string
			parent     string
			dir        string
			opts       []data.WriteOption
			want       error
			constraint string
		}{
			{"NameTaken", parent.ID, "taken", nil, blobfs.ErrNameTaken, blobfs.ConstraintUniqueDirectoryParentName},
			{"MissingParent", blobfs.NewID(), "orphan", nil, blobfs.ErrNotFound, blobfs.ConstraintForeignKeyDirectoryParent},
			{"IDTaken", parent.ID, "twin", []data.WriteOption{data.WithID(taken.ID)}, blobfs.ErrIDTaken, blobfs.ConstraintPrimaryKeyDirectory},
		} {
			t.Run(c.name, func(t *testing.T) {
				_, err := s.store.Mkdir(s.ctx, s.db, c.parent, c.dir, c.opts...)
				s.wantViolation(t, err, c.want, c.constraint)
				_, base := s.standard.Mkdir(s.ctx, s.db, c.parent, c.dir, c.opts...)
				s.wantSameError(t, err, base)
				_, _, err = s.store.EnsureDirectory(s.ctx, s.db, c.parent, c.dir, c.opts...)
				_, _, base = s.standard.EnsureDirectory(s.ctx, s.db, c.parent, c.dir, c.opts...)
				if c.name == "NameTaken" {
					if err != nil || base != nil {
						t.Errorf("EnsureDirectory of a taken name = %v and %v on the baseline, want the row found", err, base)
					}
					return
				}
				s.wantViolation(t, err, c.want, c.constraint)
				s.wantSameError(t, err, base)
			})
		}
	})
	t.Run("Ensure", func(t *testing.T) {
		for _, tier := range []struct {
			name  string
			store *data.Store
		}{{"variant", s.store}, {"baseline", s.standard}} {
			t.Run(tier.name, func(t *testing.T) {
				name := "ensured-" + tier.name
				created, made, err := tier.store.EnsureDirectory(s.ctx, s.db, parent.ID, name)
				if err != nil || !made || created.Version != 1 {
					t.Fatalf("first EnsureDirectory = %+v, %v, %v; want the row created", created, made, err)
				}
				found, made, err := tier.store.EnsureDirectory(s.ctx, s.db, parent.ID, name, data.WithID(blobfs.NewID()))
				if err != nil || made || !equalDirectory(created, found) {
					t.Fatalf("second EnsureDirectory = %+v, %v, %v; want the same row found", found, made, err)
				}
			})
		}
	})
}

// sameFileShape compares two file rows on every column that does not
// identify the row or carry a clock.
func sameFileShape(a, b blobfs.File) bool {
	return a.DirectoryID == b.DirectoryID && a.Status == b.Status && equalInt(a.Size, b.Size) &&
		a.ContentType == b.ContentType && equalString(a.ETag, b.ETag) && a.Version == b.Version
}

// wantViolation checks err is want as a blobfs.ViolationError over the
// named constraint, with the sqlate.ConstraintError reachable.
func (s *suite) wantViolation(t *testing.T, err, want error, constraint string) {
	t.Helper()
	var ve *blobfs.ViolationError
	var ce *sqlate.ConstraintError
	if !errors.Is(err, want) || !errors.As(err, &ve) || ve.Constraint != constraint || !errors.As(err, &ce) || ce.Constraint != constraint {
		t.Errorf("got %v, want %v over the constraint %s", err, want, constraint)
	}
}

// wantSameError checks the baseline refused the same way, in text, as
// the variant did.
func (s *suite) wantSameError(t *testing.T, got, base error) {
	t.Helper()
	if got == nil || base == nil {
		t.Errorf("the variant returned %v and the baseline %v; want both refused", got, base)
		return
	}
	if got.Error() != base.Error() {
		t.Errorf("the variant refused with\n%v\nand the baseline with\n%v", got, base)
	}
}
