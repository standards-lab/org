package blobfs

import (
	"time"
	"uuid"
)

// Directory is one node of the directory hierarchy, a row of
// blobfs_directory. ParentID is nil at a root; a root is anchored by the
// consumer's own table and needs no constraint of its own. Version is the
// concurrency token the guarded commands check. The json tags are the scan
// and binding contract: the columns carry the same names.
type Directory struct {
	ID        string    `json:"id"`
	ParentID  *string   `json:"parent_id"`
	Name      string    `json:"name"`
	Version   int64     `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
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
