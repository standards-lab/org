package blobfs

// The names of the constraints blobfs's DDL declares and the persistence
// layer maps to sentinel errors. Constraint names are public API: a
// violation reaches a consumer as sqlate.ConstraintError.Constraint, and the
// scheme is blobfs_<kind>_<table>_<detail>. The constants live here rather
// than in the migrations package because the persistence layer imports this
// package and must not import the migrations package. The migrations
// package's tests check that every constant names a constraint in the DDL.
const (
	// ConstraintUniqueVolumeName is the unique constraint on
	// blobfs_volume.name. A violation is ErrNameTaken.
	ConstraintUniqueVolumeName = "blobfs_uq_volume_name"

	// ConstraintUniqueDirectoryVolume is the unique constraint on
	// blobfs_directory.volume_id: one root per volume.
	ConstraintUniqueDirectoryVolume = "blobfs_uq_directory_volume"

	// ConstraintUniqueDirectoryParentName is the unique constraint on
	// blobfs_directory (parent_id, name). A violation is ErrNameTaken.
	ConstraintUniqueDirectoryParentName = "blobfs_uq_directory_parent_name"

	// ConstraintUniqueFileDirectoryName is the unique constraint on
	// blobfs_file (directory_id, name). A violation is ErrNameTaken.
	ConstraintUniqueFileDirectoryName = "blobfs_uq_file_directory_name"

	// ConstraintForeignKeyDirectoryParent is the foreign key from
	// blobfs_directory.parent_id to blobfs_directory.id. On an insert or a
	// move a violation is ErrNotFound; on a delete it means the directory
	// still has child directories.
	ConstraintForeignKeyDirectoryParent = "blobfs_fk_directory_parent"

	// ConstraintForeignKeyDirectoryVolume is the foreign key from
	// blobfs_directory.volume_id to blobfs_volume.id. On a delete a
	// violation means the volume's root still exists.
	ConstraintForeignKeyDirectoryVolume = "blobfs_fk_directory_volume"

	// ConstraintForeignKeyFileDirectory is the foreign key from
	// blobfs_file.directory_id to blobfs_directory.id. On an insert or a
	// move a violation is ErrNotFound; on a delete it means the directory
	// still has files.
	ConstraintForeignKeyFileDirectory = "blobfs_fk_file_directory"
)
