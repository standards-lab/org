package files

import (
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

// Contents is what ls returns for one directory: the path it listed, the
// directories under it, and the files in it, each one page under the same
// Listing with its own total. Both halves were read in one read-only
// repeatable-read transaction, so they agree with each other.
type Contents struct {
	Path        string
	Directories Page[blobfs.Directory]
	Files       Page[blobfs.File]
}
