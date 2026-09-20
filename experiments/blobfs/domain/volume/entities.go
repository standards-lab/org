package volume

import "time"

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
