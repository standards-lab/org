package blobfs

import (
	"time"
	"uuid"
)

// Volume is one directory tree's name, a row of blobfs_volume. Its Name is
// the tree's one unique name: paths start at / inside a volume, and the
// volume's root directory carries no name of its own. Name is normalized
// with NormalizeName and checked with ValidateName, as a directory or file
// name is. The volume holds no owner and no unit; a consumer's own table
// binds it to whatever the consumer authorizes by. Version is the
// concurrency token the guarded commands check. The json tags are the scan
// and binding contract: the columns carry the same names.
type Volume struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Version   int64     `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Directory is one node of the directory hierarchy, a row of
// blobfs_directory. A root has a nil ParentID, a nil Name, and a VolumeID
// naming the volume it is the root of; every other directory has a
// ParentID and a Name and a nil VolumeID. The volume's name is what names
// the tree, so a root needs none. Version is the concurrency token the
// guarded commands check. The json tags are the scan and binding contract:
// the columns carry the same names, and a nil pointer binds or scans as
// NULL.
type Directory struct {
	ID        string    `json:"id"`
	ParentID  *string   `json:"parent_id"`
	VolumeID  *string   `json:"volume_id"`
	Name      *string   `json:"name"`
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
