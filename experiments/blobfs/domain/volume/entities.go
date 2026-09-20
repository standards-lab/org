package volume

import (
	"time"

	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
)

// Owner is one row of volume_owner: the consumer's ownership record, which
// binds a blobfs volume to the unit that owns it. It is the join table the
// auth strategy's scope predicate filters, at the volume grain; UnitID
// stands in for the scope's unit. Version is the concurrency token the
// guarded commands check. The json tags are the scan and binding contract:
// the columns carry the same names.
type Owner struct {
	VolumeID  string    `json:"volume_id"`
	UnitID    string    `json:"unit_id"`
	Version   int64     `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Bookmark is one row of volume_bookmark: a volume joined to one of its
// files. At most one bookmark per volume is active, which the set's partial
// unique index enforces. The json tags are the scan and binding contract:
// the columns carry the same names.
type Bookmark struct {
	VolumeID  string    `json:"volume_id"`
	FileID    string    `json:"file_id"`
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// VolumeEntry is one row of the consumer's volume read model,
// volume_view: a blobfs.Volume's columns and the unit that owns it. The
// fields are flat rather than an embedded blobfs.Volume because the
// struct-tag mapper matches columns to a struct's own fields by tag and
// does not flatten an embedded struct. The json tags are the scan
// contract: the view's columns carry the same names.
type VolumeEntry struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Version   int64     `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	UnitID    string    `json:"unit_id"`
}

// FileEntry is one row of the consumer's file read model, file_view: a
// blobfs.File's columns, the file's path inside its volume, the volume, and
// the unit that owns the volume. Size and ETag are nil until the file is
// available, as on blobfs.File. The fields are flat for the same reason
// VolumeEntry's are. The json tags are the scan contract: the view's
// columns carry the same names.
type FileEntry struct {
	ID          string        `json:"id"`
	DirectoryID string        `json:"directory_id"`
	Name        string        `json:"name"`
	Status      blobfs.Status `json:"status"`
	Key         string        `json:"key"`
	Size        *int64        `json:"size"`
	ContentType string        `json:"content_type"`
	ETag        *string       `json:"etag"`
	Version     int64         `json:"version"`
	CreatedAt   time.Time     `json:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at"`
	Path        string        `json:"path"`
	VolumeID    string        `json:"volume_id"`
	UnitID      string        `json:"unit_id"`
}

// Listing is one page request of a listing command, as the command line
// states it: the 1-based page and its size, the sort terms in order, and
// the unit to filter by when Unit is not empty. It is the consumer's own
// shape of a read request; database.go lowers it to the query library's
// directives, since no other file of the package names that library.
type Listing struct {
	Page int
	Size int
	Sort []Sort
	Unit string
}

// Sort is one sort term of a Listing: a declared field of the read model
// and its direction.
type Sort struct {
	Field      string
	Descending bool
}

// Contents is what ls returns for one directory: the directories under it
// and the files in it, each one page under the same Listing with its own
// total. Directories carry no path; the command composes one from the
// address it listed.
type Contents struct {
	Directories    []blobfs.Directory
	DirectoryTotal int
	Files          []FileEntry
	FileTotal      int
}
