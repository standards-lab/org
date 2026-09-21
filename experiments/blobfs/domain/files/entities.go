package files

import (
	"fmt"
	"io"
	"time"

	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
)

// DirectoryOwner is one row of directory_owner: the consumer's ownership
// record, which binds a depth-one blobfs directory to the unit that owns
// it. It is the join table the auth strategy's scope predicate filters, at
// the directory grain; UnitID stands in for the scope's unit. Version is
// the concurrency token the guarded commands check. The json tags are the
// scan and binding contract: the columns carry the same names.
type DirectoryOwner struct {
	DirectoryID string    `json:"directory_id"`
	UnitID      string    `json:"unit_id"`
	Version     int64     `json:"version"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// OwnedDirectory is one row of the consumer's owner read model,
// owned_directories: a blobfs.Directory's columns and the unit that owns
// it. It is what ls / --unit lists. The fields are flat rather than an
// embedded blobfs.Directory because the struct-tag mapper matches columns
// to a struct's own fields by tag and does not flatten an embedded struct,
// so the read model restates every library column, ParentID included,
// which it never uses. The json tags are the scan contract: the
// projection's columns carry the same names.
type OwnedDirectory struct {
	ID        string    `json:"id"`
	ParentID  *string   `json:"parent_id"`
	Name      string    `json:"name"`
	Version   int64     `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	UnitID    string    `json:"unit_id"`
}

// Directory returns the row as the library's Directory type, so a listing
// through the owner read model and one through the library's own listing
// return the same shape.
func (o OwnedDirectory) Directory() blobfs.Directory {
	name := o.Name
	return blobfs.Directory{ID: o.ID, ParentID: o.ParentID, Name: &name, Version: o.Version, CreatedAt: o.CreatedAt, UpdatedAt: o.UpdatedAt}
}

// BookmarkedFile is one row of the consumer's bookmark read model,
// bookmarks: a unit's bookmark of a file, the file's columns the listing
// shows, and the file's full path, computed by the read model from the
// file's directory upward. It is what bookmark ls lists. Active says the
// bookmark is the unit's one active bookmark. Size is nil for a file whose
// write has not completed. CreatedAt and UpdatedAt are the bookmark's, not
// the file's. The fields are flat, as in OwnedDirectory, because the
// struct-tag mapper does not flatten an embedded struct; the read model
// restates the library columns it uses and no others. The json tags are
// the scan contract: the projection's columns carry the same names.
type BookmarkedFile struct {
	UnitID      string        `json:"unit_id"`
	FileID      string        `json:"file_id"`
	Active      bool          `json:"active"`
	Path        string        `json:"path"`
	Name        string        `json:"name"`
	Status      blobfs.Status `json:"status"`
	Size        *int64        `json:"size"`
	ContentType string        `json:"content_type"`
	CreatedAt   time.Time     `json:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at"`
}

// TotalMode says whether a listing asks for its total.
type TotalMode int

const (
	// TotalExact, the default, asks for the total in the same statement as
	// the page.
	TotalExact TotalMode = iota

	// TotalNone omits the total. The page's Total is NoTotal.
	TotalNone
)

// NoTotal is the Total of a page that carries none: the listing asked for
// TotalNone, or the page is empty and is not the first, so no row carried
// the count.
const NoTotal = -1

// Listing is one page request of ls, as the command line states it: the
// 1-based page and its size, the sort terms in order, the total mode, the
// cursors to continue each half from, and the unit whose scope the
// listing is checked against when Unit is not empty. It is the consumer's
// own shape of a read request; database.go lowers it to the library's
// listing and to the query library's directives, since no other file of
// the package names those.
type Listing struct {
	Page  int
	Size  int
	Sort  []Sort
	Total TotalMode
	After After
	Unit  string
}

// After holds the cursors a listing continues from, one per half, each
// the Next of an earlier page of that half under the same sort. A half
// whose cursor is empty is read by page number. A half read by cursor
// ignores Page and carries no total, whatever Total says.
type After struct {
	Directories string
	Files       string
}

// Sort is one sort term of a Listing: a declared field of a listing and
// its direction.
type Sort struct {
	Field      string
	Descending bool
}

// Page is one page of one half of a listing: its rows, its total, which
// is NoTotal when the page carries none, and Next, the cursor that
// continues the half after this page, empty on the last page and when the
// half cannot be continued by cursor.
type Page[T any] struct {
	Rows  []T
	Total int
	Next  string
}

// Step names a step of a two-phase command that --fail-after may stop
// after. For put: insert, once the pending row is committed and before
// the object is stored, and write, once the object is stored and before
// the row is completed. For rm: begin, once the row is committed as
// deleting and before the object is deleted, and object, once the object
// is deleted and before the row is removed. There is no stop after the
// last step of either, because nothing follows it.
type Step string

const (
	// StepInsert is the pending row's insert, committed on its own.
	StepInsert Step = "insert"

	// StepWrite is the object's write to the store.
	StepWrite Step = "write"

	// StepBegin is the file delete's begin, committed on its own with the
	// bookmark check that precedes it.
	StepBegin Step = "begin"

	// StepObject is the object's delete from the store.
	StepObject Step = "object"
)

// ParseStep reads put's --fail-after value: insert, write, or empty for
// no stop.
func ParseStep(s string) (Step, error) {
	switch Step(s) {
	case "", StepInsert, StepWrite:
		return Step(s), nil
	}
	return "", fmt.Errorf("--fail-after %q: the step is insert or write", s)
}

// ParseRemoveStep reads rm's --fail-after value: begin, object, or empty
// for no stop.
func ParseRemoveStep(s string) (Step, error) {
	switch Step(s) {
	case "", StepBegin, StepObject:
		return Step(s), nil
	}
	return "", fmt.Errorf("--fail-after %q: the step is begin or object", s)
}

// TreeRemoval is what rm -r returns: how many files and directories it
// removed, the target directory included when it was removed. On an error
// the counts are what was removed before it.
type TreeRemoval struct {
	Files       int
	Directories int
}

// RemovalEvent is one step of a recursive delete, reported to the
// observer rm -r takes as it happens: a file removed, a directory
// removed, or a directory found empty and about to be removed. Path is
// the entry's path; ID is the row's id, empty for the emptied event.
type RemovalEvent struct {
	Kind RemovalKind
	Path string
	ID   string
}

// RemovalKind names what a RemovalEvent reports.
type RemovalKind string

const (
	// RemovedFile reports a file removed through the full delete steps.
	RemovedFile RemovalKind = "file"

	// RemovedDirectory reports a directory removed after its contents.
	RemovedDirectory RemovalKind = "directory"

	// DirectoryEmptied reports a directory whose listing came back empty,
	// before its own removal runs. The removal is refused if a row was
	// inserted meanwhile, and the walk then empties the directory again.
	DirectoryEmptied RemovalKind = "emptied"
)

// PutRequest is one upload as the command line states it: the file's
// path, the content type to declare, the body and its length when known
// (0 asserts nothing), and the step to stop after, empty for a full write.
type PutRequest struct {
	Path        string
	ContentType string
	Body        io.Reader
	Size        int64
	StopAfter   Step
}

// PutResult is what a put returns: the row as it stands when the put
// returned, available after a full write and pending after a stop, and
// whether the put resumed a pending row an earlier put left instead of
// inserting one.
type PutResult struct {
	File    blobfs.File
	Resumed bool
}

// MoveResult is what mv returns: what kind of entry moved, its id, the
// path it was at, and the path it is at now, which is the destination
// itself when the destination named a new path and the destination with
// the source's name appended when it named an existing directory.
type MoveResult struct {
	Kind EntryKind
	ID   string
	From string
	To   string
}

// EntryKind names what an entry of the tree is.
type EntryKind string

const (
	// EntryDirectory is a directory.
	EntryDirectory EntryKind = "directory"

	// EntryFile is a file.
	EntryFile EntryKind = "file"
)

// Contents is what ls returns for one directory: the path it listed, the
// directories under it, and the files in it, each one page under the same
// Listing with its own total. Both halves were read in one read-only
// repeatable-read transaction, so they agree with each other.
type Contents struct {
	Path        string
	Directories Page[blobfs.Directory]
	Files       Page[blobfs.File]
}
