package files

import (
	"fmt"
	"strings"

	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs/data"
)

// This file is the translation over the library under test: it constructs
// blobfs's persistence against the consumer's catalog and composes the
// consumer's operations from blobfs's methods and the consumer's own
// statements. The consumer uses blobfs.Directory and blobfs.File as the
// library defines them.

// compileLibrary compiles blobfs's statements against the store's catalog
// for its dialect, over the variant build returns when one was given and
// over the standard baseline otherwise. It is a method rather than a
// constructor because the catalog's type belongs to the query library,
// which only database.go and the database_<concern>.go files name; the
// field is reached without naming it.
func (s *Store) compileLibrary(build VariantConstructor) error {
	var opts []data.Option
	if build != nil {
		v, err := build(s.catalog, s.db.Dialect())
		if err != nil {
			return fmt.Errorf("files: variant: %w", err)
		}
		opts = append(opts, data.WithVariant(v))
	}
	lib, err := data.New(s.catalog, s.db.Dialect(), opts...)
	if err != nil {
		return fmt.Errorf("files: %w", err)
	}
	s.blobfs = lib
	return nil
}

// splitParent splits the path of a directory or file to create into its
// parent's path, the name of the new row, and the new row's depth (1 for
// a top-level directory). The root itself is blobfs.ErrRootDirectory, a
// relative path or one ending with a slash blobfs.ErrInvalidPath. The
// parent's segments are validated when the parent is resolved and the
// name when the row is written.
func splitParent(path string) (parent, name string, depth int, err error) {
	if !strings.HasPrefix(path, "/") {
		return "", "", 0, fmt.Errorf("%w: %q does not start with /", blobfs.ErrInvalidPath, path)
	}
	if path == "/" {
		return "", "", 0, blobfs.ErrRootDirectory
	}
	at := strings.LastIndex(path, "/")
	parent, name = path[:at], path[at+1:]
	if name == "" {
		return "", "", 0, fmt.Errorf("%w: %q ends with a slash", blobfs.ErrInvalidPath, path)
	}
	if parent == "" {
		parent = "/"
	}
	return parent, name, strings.Count(path, "/"), nil
}
