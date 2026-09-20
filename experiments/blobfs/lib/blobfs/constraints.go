package blobfs

// The names of the constraints and unique indexes blobfs's DDL declares and
// the persistence layer maps to sentinel errors. Constraint names are public
// API: a violation reaches a consumer as sqlate.ConstraintError.Constraint,
// and the scheme is blobfs_<kind>_<table>_<detail>. The constants live here
// rather than in the migrations package because the persistence layer
// imports this package and must not import the migrations package. The
// migrations package's tests check that every constant names a constraint
// or an index in the DDL.
const (
	// ConstraintUniqueDirectoryRoot is the partial unique index that allows
	// one directory row with no parent: the one root per install. A
	// violation is ErrRootDirectory.
	ConstraintUniqueDirectoryRoot = "blobfs_uq_directory_root"

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

	// ConstraintForeignKeyFileDirectory is the foreign key from
	// blobfs_file.directory_id to blobfs_directory.id. On an insert or a
	// move a violation is ErrNotFound; on a delete it means the directory
	// still has files.
	ConstraintForeignKeyFileDirectory = "blobfs_fk_file_directory"
)
