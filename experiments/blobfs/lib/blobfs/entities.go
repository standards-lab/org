package blobfs

import (
	"time"
	"uuid"
)

// RootID is the id of the one root directory of an install: the nil UUID,
// seeded by the directory migration. Every path starts at the root, and a
// consumer that needs the root reads it by this id instead of searching
// for the row with no parent.
const RootID = "00000000-0000-0000-0000-000000000000"

// Directory is one node of the directory hierarchy, a row of
// blobfs_directory. The root has a nil ParentID and a nil Name, and it is
// the only row with either; every other directory has a ParentID and a
// Name. Version is the concurrency token the guarded commands check. The
// json tags are the scan and binding contract: the columns carry the same
// names, and a nil pointer binds or scans as NULL.
type Directory struct {
	ID        string    `json:"id"`
	ParentID  *string   `json:"parent_id"`
	Name      *string   `json:"name"`
	Version   int64     `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// IsRoot reports whether d is the root directory: the row with no parent.
func (d Directory) IsRoot() bool {
	return d.ParentID == nil
}

// File is one file's metadata, a row of blobfs_file. DirectoryID is never
// nil: every file sits in a directory. Key is the object's key in the store,
// built once by NewKey when the pending row is inserted and never parsed
// back. Size and ETag are nil until the row is available, because the store
// reports them only once the object exists; ContentType is the media type
// the consumer declares at upload, so it is known from the pending row on.
// Version is the concurrency token the guarded commands check. The json
// tags are the scan and binding contract: the columns carry the same names.
type File struct {
	ID          string    `json:"id"`
	DirectoryID string    `json:"directory_id"`
	Name        string    `json:"name"`
	Status      Status    `json:"status"`
	Key         string    `json:"key"`
	Size        *int64    `json:"size"`
	ContentType string    `json:"content_type"`
	ETag        *string   `json:"etag"`
	Version     int64     `json:"version"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// NewID mints a new row id: a version 7 UUID in its canonical string form,
// so ids sort in insertion order and bind to a uuid column as text.
func NewID() string {
	return uuid.NewV7().String()
}
