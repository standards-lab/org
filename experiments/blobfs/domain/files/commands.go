package files

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"uuid"

	"github.com/spf13/cobra"

	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
	"github.com/standards-lab/org/experiments/blobfs/output"
)

// deps is what every leaf command needs: the store constructor and the
// output to render its result through. Bundled once so a leaf builder is a
// method taking neither.
type deps struct {
	newStore func() (*Store, error)
	out      *output.Output
}

// Commands builds the domain's root-level commands: mkdir, ls, put, cat,
// stat, cp, mv, rm, rmdir, and bookmark with its add, ls, and rm subcommands. A leaf's
// RunE calls newStore when it runs, never when the tree is built: the
// composition root closes newStore over its persistent flags, which cobra
// parses during execution, so the DSN is unknown until then. The store is
// verified against the database before the leaf's first use, so a schema
// that is not applied fails with ErrVerify and does no work. Every leaf
// renders through out and returns its error unrendered; the root prints
// it.
func Commands(newStore func() (*Store, error), out *output.Output) []*cobra.Command {
	d := deps{newStore: newStore, out: out}
	return []*cobra.Command{
		d.mkdir(),
		d.list(),
		d.put(),
		d.cat(),
		d.stat(),
		d.copy(),
		d.move(),
		d.remove(),
		d.removeDirectory(),
		d.bookmark(),
	}
}

// store constructs the store and verifies it against the database, so a
// leaf works over statements the schema satisfies.
func (d deps) store(ctx context.Context) (*Store, error) {
	s, err := d.newStore()
	if err != nil {
		return nil, err
	}
	if err := s.Verify(ctx); err != nil {
		return nil, err
	}
	return s, nil
}

// mkdir is mkdir <path> [--unit <uuid>]: the last segment created under
// its existing parent, and with --unit, at a top-level path, the
// ownership row in the same transaction. There is no -p; a missing parent
// is an error.
func (d deps) mkdir() *cobra.Command {
	var unit string
	cmd := &cobra.Command{
		Use:   "mkdir <path>",
		Short: "Create a directory under an existing parent",
		Long: "mkdir creates the directory at an absolute path such as /reports/2026 under its\n" +
			"parent, which must exist. With --unit, the path must name a top-level directory,\n" +
			"and the unit becomes its owner in the same transaction.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			unit, err := parseUnit(unit)
			if err != nil {
				return err
			}
			s, err := d.store(cmd.Context())
			if err != nil {
				return err
			}
			dir, err := s.Mkdir(cmd.Context(), args[0], unit)
			if err != nil {
				return err
			}
			line := fmt.Sprintf("mkdir: %s (id %s)", args[0], dir.ID)
			if unit != "" {
				line = fmt.Sprintf("mkdir: %s (id %s, unit %s)", args[0], dir.ID, unit)
			}
			d.out.Line(line)
			return nil
		},
	}
	cmd.Flags().StringVar(&unit, "unit", "", "the id of the unit that owns the directory, a UUID; top-level paths only")
	return cmd
}

// list is ls <path|id:<uuid>>: the directories under the directory first,
// then its files, each one page, with a line per half stating the page
// and the total, and the cursor lines only under --cursors.
func (d deps) list() *cobra.Command {
	var f listingFlags
	cmd := &cobra.Command{
		Use:   "ls <path|id:<uuid>>",
		Short: "List a directory: its directories, then its files, one page each",
		Long: "ls lists the directory at an absolute path such as /reports, or the directory\n" +
			"with an id written as id:<uuid>: the directories under it, then the files in it,\n" +
			"one page of each, with each row's id in the last column. --sort applies to both\n" +
			"halves; a field only files have sorts the files and leaves the directories in\n" +
			"name order.\n" +
			"\n" +
			"--filter <field>:<op>:<value> keeps the rows the predicate holds for, and it is\n" +
			"repeatable. The file half takes every filter, and the directory half takes those\n" +
			"naming a field both halves have: id, name, version, created_at, and updated_at.\n" +
			"A filter on a field only files have (status, size, content_type, etag, and\n" +
			"directory_id) leaves the directory half unfiltered, and a field neither half has\n" +
			"is refused. The operators are eq, ne, gt, ge, lt, le, like, and in, whose value\n" +
			"is a comma-separated list, and null and notnull, which take no value:\n" +
			"<field>:null. The value is text the engine reads as the field's type, so\n" +
			"size:gt:100, name:like:a%, and created_at:ge:2026-01-01 all work.\n" +
			"\n" +
			"Each half prints more: yes or more: no, whether rows remain after its page. With\n" +
			"--cursors, a half with a next page prints next-dirs: or next-files: with a\n" +
			"cursor; pass it back as --after-dirs or --after-files to continue that half,\n" +
			"which then ignores --page and carries no total. With --unit, the unit must own\n" +
			"the path's top-level directory, and at / the listing is the unit's own top-level\n" +
			"directories; --unit takes a path, not an id.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref, err := ParseRef(args[0])
			if err != nil {
				return err
			}
			l, err := f.listing()
			if err != nil {
				return err
			}
			if ref.ID != "" && l.Unit != "" {
				return fmt.Errorf("ls %s --unit: a listing by id has no path to derive the unit's scope from; list the path instead", args[0])
			}
			s, err := d.store(cmd.Context())
			if err != nil {
				return err
			}
			var c Contents
			if ref.ID != "" {
				c, err = s.ListDirectory(cmd.Context(), ref.ID, l, Scope{})
			} else {
				c, err = s.List(cmd.Context(), ref.Path, l)
			}
			if err != nil {
				return err
			}
			entries := make([]output.Entry, 0, len(c.Directories.Rows)+len(c.Files.Rows))
			for _, dir := range c.Directories.Rows {
				entries = append(entries, output.Entry{Kind: "dir", Name: dir.Name, ID: dir.ID, Updated: dir.UpdatedAt})
			}
			for _, file := range c.Files.Rows {
				entries = append(entries, output.Entry{Kind: "file", Name: file.Name, ID: file.ID, Size: file.Size, Status: string(file.Status), Updated: file.UpdatedAt})
			}
			dirs, files := pageOf(l, l.After.Directories, c.Directories), pageOf(l, l.After.Files, c.Files)
			if !f.cursors {
				dirs.Next, files.Next = "", ""
			}
			d.out.Listing(entries, dirs, files)
			return nil
		},
	}
	f.bind(cmd)
	return cmd
}

// put is put <local-file|-> <path|id:<uuid>> [--content-type <type>]
// [--fail-after <step>]: the local file, or stdin for -, uploaded as the
// file at the path, or into the directory with the id under the local
// file's base name. The content type comes from the flag, else from the
// local file's extension, else application/octet-stream. --fail-after
// stops the write after the named step with a non-zero exit, leaving the
// pending row for a later put of the same path to complete.
func (d deps) put() *cobra.Command {
	var contentType, failAfter string
	cmd := &cobra.Command{
		Use:   "put <local-file|-> <path|id:<uuid>>",
		Short: "Upload a local file, or stdin, as the file at a path",
		Long: "put uploads a local file (or stdin, for -) as the file at an absolute path such\n" +
			"as /reports/2026/q1.pdf, whose parent must exist. When the destination is a\n" +
			"directory's id written as id:<uuid>, the file lands in that directory under the\n" +
			"local file's base name; stdin has no name, so - needs a path. The write is two\n" +
			"steps around the upload: the row is inserted as pending and committed, the\n" +
			"object is stored, and the row is completed as available. --fail-after insert or\n" +
			"write stops after that step and exits non-zero; ls and stat then show the row\n" +
			"pending, and a put of the same path resumes it. A name held by an available\n" +
			"file is refused.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			step, err := ParseStep(failAfter)
			if err != nil {
				return err
			}
			ref, err := ParseRef(args[1])
			if err != nil {
				return err
			}
			if ref.ID != "" && args[0] == "-" {
				return fmt.Errorf("put - %s: stdin has no name to store under; give the destination as a path", args[1])
			}
			body, size, closeBody, err := openLocal(cmd.InOrStdin(), args[0])
			if err != nil {
				return err
			}
			defer closeBody()
			s, err := d.store(cmd.Context())
			if err != nil {
				return err
			}
			req := PutRequest{Path: ref.Path, ContentType: declaredType(contentType, args[0]), Body: body, Size: size, StopAfter: step}
			var res PutResult
			label := args[1]
			if ref.ID != "" {
				name := filepath.Base(args[0])
				label = name + " in " + args[1]
				res, err = s.PutFile(cmd.Context(), ref.ID, name, req, Scope{})
			} else {
				res, err = s.Put(cmd.Context(), req)
			}
			if err != nil {
				return err
			}
			f := res.File
			line := fmt.Sprintf("put: %s (id %s, %d bytes, etag %s)", label, f.ID, sizeOf(f), etagOf(f))
			if res.Resumed {
				line = fmt.Sprintf("put: %s (id %s, %d bytes, etag %s, resumed the pending row)", label, f.ID, sizeOf(f), etagOf(f))
			}
			d.out.Line(line)
			return nil
		},
	}
	cmd.Flags().StringVar(&contentType, "content-type", "", "the media type to store with the object; the default is derived from the local file's extension")
	cmd.Flags().StringVar(&failAfter, "fail-after", "", "stop after this step of the write, insert or write, and exit non-zero")
	return cmd
}

// cat is cat <path|id:<uuid>>: the file's content streamed to stdout as
// it is.
func (d deps) cat() *cobra.Command {
	return &cobra.Command{
		Use:   "cat <path|id:<uuid>>",
		Short: "Write a file's content to stdout",
		Long: "cat streams the content of the file at an absolute path, or of the file with an\n" +
			"id written as id:<uuid>, to stdout. A pending or deleting file has no content to\n" +
			"read and is refused.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref, err := ParseRef(args[0])
			if err != nil {
				return err
			}
			s, err := d.store(cmd.Context())
			if err != nil {
				return err
			}
			var body io.ReadCloser
			if ref.ID != "" {
				body, _, err = s.OpenFile(cmd.Context(), ref.ID, Scope{})
			} else {
				body, _, err = s.Open(cmd.Context(), ref.Path)
			}
			if err != nil {
				return err
			}
			defer func() { _ = body.Close() }()
			if _, err := d.out.Copy(body); err != nil {
				return fmt.Errorf("cat %s: %w", args[0], err)
			}
			return nil
		},
	}
}

// stat is stat <path|id:<uuid>>: the file's row, or the directory's when
// no file is at the path or has the id, one field per line.
func (d deps) stat() *cobra.Command {
	return &cobra.Command{
		Use:   "stat <path|id:<uuid>>",
		Short: "Show a file's or a directory's row, one field per line",
		Long: "stat prints the row of the file at an absolute path, whatever its status: a\n" +
			"pending file shows as pending with no size or etag. When no file is at the\n" +
			"path, stat prints the row of the directory there: its id, its parent's id, its\n" +
			"name, its version, and its timestamps; / is the root. An id written as\n" +
			"id:<uuid> is looked up the same way, the file first and then the directory,\n" +
			"and the record then carries no path. The object store is not consulted.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref, err := ParseRef(args[0])
			if err != nil {
				return err
			}
			s, err := d.store(cmd.Context())
			if err != nil {
				return err
			}
			if ref.ID != "" {
				kind, f, dir, err := entry(cmd.Context(), s, ref.ID)
				if err != nil {
					return err
				}
				if kind == EntryFile {
					d.out.Record(fileRecord("", f))
				} else {
					d.out.Record(directoryRecord("", dir))
				}
				return nil
			}
			f, err := s.Stat(cmd.Context(), ref.Path)
			if err == nil {
				d.out.Record(fileRecord(ref.Path, f))
				return nil
			}
			if !errors.Is(err, blobfs.ErrNotFound) && !errors.Is(err, blobfs.ErrRootDirectory) {
				return err
			}
			dir, dirErr := s.Resolve(cmd.Context(), ref.Path)
			if errors.Is(dirErr, blobfs.ErrNotFound) {
				return err
			}
			if dirErr != nil {
				return dirErr
			}
			d.out.Record(directoryRecord(ref.Path, dir))
			return nil
		},
	}
}

// copy is cp <src> <dst> [--fail-after <step>]: the available file at
// src copied into the existing directory dst under its own name, or to
// the new path dst, through the same two steps around the object write
// as put, with the bytes streamed through this process. With two ids,
// the file with the first is copied into the directory with the second
// under its own name. --fail-after stops the copy after the named step
// with a non-zero exit, leaving the pending row for a later cp of the
// same paths to complete.
func (d deps) copy() *cobra.Command {
	var failAfter string
	cmd := &cobra.Command{
		Use:   "cp <src> <dst>",
		Short: "Copy a file into an existing directory or to a new path",
		Long: "cp copies the file at an absolute path to a new file with the same bytes and\n" +
			"content type. When the destination names an existing directory the copy lands in\n" +
			"it under the source's name; otherwise the destination is the copy's path, whose\n" +
			"parent must exist. With ids written as id:<uuid>, the first is the file to copy\n" +
			"and the second the directory it is copied into under its own name; a path and\n" +
			"an id are not mixed. The source must be an available file: a directory, a\n" +
			"pending file, and a deleting file are refused. A destination name a file holds\n" +
			"is refused, so nothing is overwritten. The bytes stream through this process\n" +
			"around the same two steps as put: the row is inserted as pending and committed,\n" +
			"the object is stored, and the row is completed as available. --fail-after insert\n" +
			"or write stops after that step and exits non-zero; a cp of the same paths\n" +
			"resumes the pending row. A copy may cross top-level directories, and neither\n" +
			"bookmarks nor ownership follow it.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			step, err := ParseStep(failAfter)
			if err != nil {
				return err
			}
			src, dst, err := parsePair("cp", args[0], args[1])
			if err != nil {
				return err
			}
			s, err := d.store(cmd.Context())
			if err != nil {
				return err
			}
			var res CopyResult
			if src.ID != "" {
				res, err = s.CopyFile(cmd.Context(), src.ID, dst.ID, "", step, Scope{})
				res.From, res.To = args[0], args[1]
			} else {
				res, err = s.Copy(cmd.Context(), CopyRequest{Source: src.Path, Destination: dst.Path, StopAfter: step})
			}
			if err != nil {
				return err
			}
			f := res.File
			line := fmt.Sprintf("cp: %s -> %s (id %s, %d bytes, etag %s)", res.From, res.To, f.ID, sizeOf(f), etagOf(f))
			if res.Resumed {
				line = fmt.Sprintf("cp: %s -> %s (id %s, %d bytes, etag %s, resumed the pending row)", res.From, res.To, f.ID, sizeOf(f), etagOf(f))
			}
			d.out.Line(line)
			return nil
		},
	}
	cmd.Flags().StringVar(&failAfter, "fail-after", "", "stop after this step of the copy, insert or write, and exit non-zero")
	return cmd
}

// move is mv <src> <dst>: the directory or file at src moved into the
// existing directory dst, or to the new path dst, in one transaction.
// With two ids, the entry with the first, a file or else a directory,
// moves into the directory with the second and keeps its name.
func (d deps) move() *cobra.Command {
	return &cobra.Command{
		Use:   "mv <src> <dst>",
		Short: "Move or rename a directory or a file",
		Long: "mv moves the directory or file at an absolute path. When the destination names\n" +
			"an existing directory the source moves into it and keeps its name; otherwise the\n" +
			"destination is the new path, whose parent must exist and whose last segment is\n" +
			"the new name, so mv renames as well. With ids written as id:<uuid>, the first is\n" +
			"the entry to move, a file or else a directory, and the second the directory it\n" +
			"moves into under its own name; a path and an id are not mixed. A directory\n" +
			"moves with everything under it, in one transaction under the tree lock, and a\n" +
			"move into its own subtree is refused. A file's object stays where it is; only\n" +
			"its row changes. A move stays under one top-level directory: a top-level\n" +
			"directory may be renamed but not moved below another, and nothing moves up to\n" +
			"the top level or across two top-level directories, because the ownership row\n" +
			"binds a top-level directory. The root cannot be moved.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			src, dst, err := parsePair("mv", args[0], args[1])
			if err != nil {
				return err
			}
			s, err := d.store(cmd.Context())
			if err != nil {
				return err
			}
			var res MoveResult
			if src.ID != "" {
				kind, _, _, kindErr := entry(cmd.Context(), s, src.ID)
				if kindErr != nil {
					return kindErr
				}
				res, err = s.MoveEntry(cmd.Context(), MoveRequest{Kind: kind, ID: src.ID, DirectoryID: dst.ID}, Scope{})
			} else {
				res, err = s.Move(cmd.Context(), src.Path, dst.Path)
			}
			if err != nil {
				return err
			}
			d.out.Line(fmt.Sprintf("mv: %s -> %s (id %s)", res.From, res.To, res.ID))
			return nil
		},
	}
}

// remove is rm <path|id:<uuid>> [--fail-after <step>] and rm -r <path>: a
// file deleted through the three steps, or a directory tree removed,
// files and then directories, deepest first, with a line per removal.
// --fail-after stops the file delete after the named step with a
// non-zero exit, leaving the row deleting for a later rm of the same
// path to finish.
func (d deps) remove() *cobra.Command {
	var recursive bool
	var failAfter string
	cmd := &cobra.Command{
		Use:   "rm <path|id:<uuid>>",
		Short: "Delete a file, or with -r a directory and everything under it",
		Long: "rm deletes the file at an absolute path such as /reports/2026/q1.pdf, or the\n" +
			"file with an id written as id:<uuid>. The delete is two steps around the object\n" +
			"delete: the row is marked deleting and committed, the object is deleted from\n" +
			"the store, and the row is removed. A file a unit has bookmarked is refused\n" +
			"before anything is touched. --fail-after begin or object stops after that step\n" +
			"and exits non-zero; ls and stat then show the row deleting, and an rm of the\n" +
			"same path finishes the delete. A pending file (an abandoned put) is deleted the\n" +
			"same way.\n" +
			"\n" +
			"rm -r removes the directory at the path and everything under it: each file\n" +
			"through the same steps and each directory once it is empty, deepest first, the\n" +
			"target last, printing a line per removal. It takes no lock; a row inserted\n" +
			"meanwhile is removed by a later pass or refuses the directory's removal, and a\n" +
			"rerun continues. It takes a path, not an id. The root cannot be removed.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			step, err := ParseRemoveStep(failAfter)
			if err != nil {
				return err
			}
			if recursive && step != "" {
				return errors.New("--fail-after applies to a single file's delete; rm -r takes no step")
			}
			ref, err := ParseRef(args[0])
			if err != nil {
				return err
			}
			if recursive && ref.ID != "" {
				return fmt.Errorf("rm -r %s: a tree is removed by path, not by id", args[0])
			}
			s, err := d.store(cmd.Context())
			if err != nil {
				return err
			}
			if !recursive {
				var f blobfs.File
				if ref.ID != "" {
					f, err = s.RemoveFile(cmd.Context(), ref.ID, 0, step, Scope{})
				} else {
					f, err = s.Remove(cmd.Context(), ref.Path, step)
				}
				if err != nil {
					return err
				}
				d.out.Line(fmt.Sprintf("rm: %s (id %s)", args[0], f.ID))
				return nil
			}
			res, err := s.RemoveTree(cmd.Context(), args[0], func(ev RemovalEvent) {
				switch ev.Kind {
				case RemovedFile:
					d.out.Line(fmt.Sprintf("rm: %s (id %s)", ev.Path, ev.ID))
				case RemovedDirectory:
					d.out.Line(fmt.Sprintf("rmdir: %s (id %s)", ev.Path, ev.ID))
				}
			})
			if err != nil {
				return err
			}
			d.out.Line(fmt.Sprintf("rm -r: %s (%d files, %d directories)", args[0], res.Files, res.Directories))
			return nil
		},
	}
	cmd.Flags().BoolVarP(&recursive, "recursive", "r", false, "remove the directory at the path and everything under it")
	cmd.Flags().StringVar(&failAfter, "fail-after", "", "stop after this step of a file's delete, begin or object, and exit non-zero")
	return cmd
}

// removeDirectory is rmdir <path>: an empty directory removed, with its
// ownership row when it has one. A directory that still has contents is
// refused, and so is the root.
func (d deps) removeDirectory() *cobra.Command {
	return &cobra.Command{
		Use:   "rmdir <path>",
		Short: "Remove an empty directory",
		Long: "rmdir removes the directory at an absolute path such as /reports/2026, which must\n" +
			"be empty: a directory that still has directories or files under it is refused,\n" +
			"and rm -r removes a whole tree. A top-level directory owned by a unit goes with\n" +
			"its ownership row, in one transaction. The root cannot be removed.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := d.store(cmd.Context())
			if err != nil {
				return err
			}
			dir, err := s.RemoveDirectory(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			d.out.Line(fmt.Sprintf("rmdir: %s (id %s)", args[0], dir.ID))
			return nil
		},
	}
}

// bookmark is the bookmark command group: add, ls, and rm, each under a
// unit, the file-grain ownership rehearsal.
func (d deps) bookmark() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bookmark",
		Short: "Bookmark files for a unit: add, ls, rm",
		Long: "bookmark records which files a unit bookmarks, at most one of them active. add\n" +
			"bookmarks a file at an absolute path, ls lists the unit's bookmarks with their\n" +
			"paths, and rm removes one. Every subcommand takes --unit.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(d.bookmarkAdd(), d.bookmarkList(), d.bookmarkRemove())
	return cmd
}

// bookmarkAdd is bookmark add <path> --unit <uuid> [--active]: the unit's
// bookmark of the file at the path, active when asked, and refused when
// another bookmark of the unit is active.
func (d deps) bookmarkAdd() *cobra.Command {
	var unit string
	var active bool
	cmd := &cobra.Command{
		Use:   "add <path> --unit <uuid>",
		Short: "Bookmark the file at a path for a unit",
		Long: "add records that the unit bookmarks the file at an absolute path such as\n" +
			"/reports/2026/plan.txt. With --active the bookmark becomes the unit's one active\n" +
			"bookmark, and the add is refused while another is active; remove that one\n" +
			"first. A file bookmarked by the unit already is refused, and so is a file whose\n" +
			"delete is under way. A pending file can be bookmarked.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			unit, err := parseUnit(unit)
			if err != nil {
				return err
			}
			s, err := d.store(cmd.Context())
			if err != nil {
				return err
			}
			f, err := s.AddBookmark(cmd.Context(), args[0], unit, active)
			if err != nil {
				return err
			}
			state := "inactive"
			if active {
				state = "active"
			}
			d.out.Line(fmt.Sprintf("bookmark add: %s (file %s, unit %s, %s)", args[0], f.ID, unit, state))
			return nil
		},
	}
	cmd.Flags().StringVar(&unit, "unit", "", "the id of the unit that bookmarks the file, a UUID")
	cmd.Flags().BoolVar(&active, "active", false, "make this bookmark the unit's one active bookmark")
	_ = cmd.MarkFlagRequired("unit")
	return cmd
}

// bookmarkList is bookmark ls --unit <uuid>: the unit's bookmarks with
// their files' paths, one page, and a line stating the page and the total.
func (d deps) bookmarkList() *cobra.Command {
	var f pageFlags
	var unit string
	cmd := &cobra.Command{
		Use:   "ls --unit <uuid>",
		Short: "List a unit's bookmarks with their files' paths, one page",
		Long: "ls lists the files the unit bookmarks, each at its full path, in path order unless\n" +
			"--sort says otherwise, one page at a time. The total comes from a count statement\n" +
			"run in the same read-only snapshot as the page, so the two agree; --total none\n" +
			"omits it from the output. The listing pages by number only.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			unit, err := parseUnit(unit)
			if err != nil {
				return err
			}
			l, err := f.listing()
			if err != nil {
				return err
			}
			// The command prints paths, so it asks for them; the read
			// model computes none by default.
			l.Paths = true
			s, err := d.store(cmd.Context())
			if err != nil {
				return err
			}
			p, err := s.ListBookmarks(cmd.Context(), unit, l)
			if err != nil {
				return err
			}
			entries := make([]output.BookmarkEntry, 0, len(p.Rows))
			for _, b := range p.Rows {
				entries = append(entries, output.BookmarkEntry{Path: b.Path, Size: b.Size, Status: string(b.Status), Active: b.Active, Updated: b.UpdatedAt})
			}
			d.out.Bookmarks(entries, pageOf(l, "", p))
			return nil
		},
	}
	f.bind(cmd)
	cmd.Flags().StringVar(&unit, "unit", "", "the id of the unit whose bookmarks to list, a UUID")
	_ = cmd.MarkFlagRequired("unit")
	return cmd
}

// bookmarkRemove is bookmark rm <path> --unit <uuid>: the unit's bookmark
// of the file at the path removed, active or not.
func (d deps) bookmarkRemove() *cobra.Command {
	var unit string
	cmd := &cobra.Command{
		Use:   "rm <path> --unit <uuid>",
		Short: "Remove a unit's bookmark of the file at a path",
		Long: "rm removes the unit's bookmark of the file at an absolute path, whether or not\n" +
			"it is the active one. A file the unit has not bookmarked is refused.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			unit, err := parseUnit(unit)
			if err != nil {
				return err
			}
			s, err := d.store(cmd.Context())
			if err != nil {
				return err
			}
			f, err := s.RemoveBookmark(cmd.Context(), args[0], unit)
			if err != nil {
				return err
			}
			d.out.Line(fmt.Sprintf("bookmark rm: %s (file %s, unit %s)", args[0], f.ID, unit))
			return nil
		},
	}
	cmd.Flags().StringVar(&unit, "unit", "", "the id of the unit whose bookmark to remove, a UUID")
	_ = cmd.MarkFlagRequired("unit")
	return cmd
}

// fileRecord lays a file row out as the fields stat prints, in order. The
// path line is left out when path is empty, as it is for a stat by id.
func fileRecord(path string, f blobfs.File) []output.Field {
	size := "-"
	if f.Size != nil {
		size = strconv.FormatInt(*f.Size, 10)
	}
	fields := []output.Field{
		{Name: "id", Value: f.ID},
		{Name: "name", Value: f.Name},
		{Name: "status", Value: string(f.Status)},
		{Name: "size", Value: size},
		{Name: "content-type", Value: f.ContentType},
		{Name: "etag", Value: etagOf(f)},
		{Name: "key", Value: f.Key},
		{Name: "version", Value: strconv.FormatInt(f.Version, 10)},
		{Name: "created", Value: f.CreatedAt.UTC().Format(time.RFC3339)},
		{Name: "updated", Value: f.UpdatedAt.UTC().Format(time.RFC3339)},
	}
	if path != "" {
		fields = append([]output.Field{{Name: "path", Value: path}}, fields...)
	}
	return fields
}

// directoryRecord lays a directory row out as the fields stat prints, in
// the file record's order for the fields the two share; the parent is -
// for the root. The path line is left out when path is empty.
func directoryRecord(path string, dir blobfs.Directory) []output.Field {
	parent := "-"
	if dir.ParentID != nil {
		parent = *dir.ParentID
	}
	fields := []output.Field{
		{Name: "id", Value: dir.ID},
		{Name: "parent", Value: parent},
		{Name: "name", Value: dir.Name},
		{Name: "version", Value: strconv.FormatInt(dir.Version, 10)},
		{Name: "created", Value: dir.CreatedAt.UTC().Format(time.RFC3339)},
		{Name: "updated", Value: dir.UpdatedAt.UTC().Format(time.RFC3339)},
	}
	if path != "" {
		fields = append([]output.Field{{Name: "path", Value: path}}, fields...)
	}
	return fields
}

// entry reads the row with id as stat and mv find it by id: the file
// first, then the directory when no file has the id, each with no scope.
// The kind says which of the two rows is set. An id neither table holds
// is blobfs.ErrNotFound.
func entry(ctx context.Context, s *Store, id string) (EntryKind, blobfs.File, blobfs.Directory, error) {
	f, err := s.StatFile(ctx, id, Scope{})
	if err == nil {
		return EntryFile, f, blobfs.Directory{}, nil
	}
	if !errors.Is(err, blobfs.ErrNotFound) {
		return "", blobfs.File{}, blobfs.Directory{}, err
	}
	dir, err := s.StatDirectory(ctx, id, Scope{})
	if errors.Is(err, blobfs.ErrNotFound) {
		return "", blobfs.File{}, blobfs.Directory{}, fmt.Errorf("files: id %s: no file or directory has it: %w", id, blobfs.ErrNotFound)
	}
	if err != nil {
		return "", blobfs.File{}, blobfs.Directory{}, err
	}
	return EntryDirectory, blobfs.File{}, dir, nil
}

// Ref is one argument that names an entry: an absolute path, or a row's
// id written as id:<uuid>. Exactly one of Path and ID is set. A path
// starts with /, so the two forms never collide.
type Ref struct {
	Path string
	ID   string
}

// ParseRef reads one path-or-id argument. Text after an id: prefix must
// be a UUID other than the root's, checked by blobfs.ParseID before any
// I/O, and is returned in canonical form; anything else is taken as a
// path, which the store validates.
func ParseRef(arg string) (Ref, error) {
	rest, ok := strings.CutPrefix(arg, "id:")
	if !ok {
		return Ref{Path: arg}, nil
	}
	id, err := blobfs.ParseID(rest)
	if err != nil {
		return Ref{}, err
	}
	return Ref{ID: id}, nil
}

// parsePair reads the two arguments of cp and mv, which take two paths or
// two ids and not one of each, since the id forms of the store take a
// source id and a destination directory id together.
func parsePair(command, first, second string) (Ref, Ref, error) {
	src, err := ParseRef(first)
	if err != nil {
		return Ref{}, Ref{}, err
	}
	dst, err := ParseRef(second)
	if err != nil {
		return Ref{}, Ref{}, err
	}
	if (src.ID != "") != (dst.ID != "") {
		return Ref{}, Ref{}, fmt.Errorf("%s %s %s: give two paths, or two ids as id:<uuid> for the source and the destination directory", command, first, second)
	}
	return src, dst, nil
}

// sizeOf returns a file's size, 0 when the row has none.
func sizeOf(f blobfs.File) int64 {
	if f.Size == nil {
		return 0
	}
	return *f.Size
}

// etagOf returns a file's etag, or - when the row has none.
func etagOf(f blobfs.File) string {
	if f.ETag == nil {
		return "-"
	}
	return *f.ETag
}

// openLocal opens the body a put uploads: stdin for -, with its length
// unknown, or the local file named, with its length from the file system
// so the store can hold the body to it. The close function releases what
// was opened.
func openLocal(stdin io.Reader, name string) (body io.Reader, size int64, closeBody func(), err error) {
	if name == "-" {
		return stdin, 0, func() {}, nil
	}
	file, err := os.Open(name)
	if err != nil {
		return nil, 0, nil, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, 0, nil, err
	}
	return file, info.Size(), func() { _ = file.Close() }, nil
}

// declaredType is the content type a put declares: the flag when given,
// else the type registered for the local file's extension, else
// application/octet-stream, which is also what stdin gets.
func declaredType(flag, local string) string {
	if flag != "" {
		return flag
	}
	if local != "-" {
		if t := mime.TypeByExtension(filepath.Ext(local)); t != "" {
			return t
		}
	}
	return "application/octet-stream"
}

// pageOf describes one half's page for the output: the request's page and
// size, whether the half was read after a cursor (after not empty), the
// rows listed, the total as the half reported it, marked counted when the
// listing asked for one and the half was read by number, whether rows
// remain, and the cursor of the next page.
func pageOf[T any](l Listing, after string, p Page[T]) output.Page {
	out := output.Page{Number: l.Page, Size: l.Size, Listed: len(p.Rows), Counted: l.Total == TotalExact && after == "", Cursor: after != "", More: p.More, Next: p.Next}
	if p.Total == NoTotal {
		out.Total = output.NoTotal
	} else {
		out.Total = p.Total
	}
	return out
}

// pageFlags is the flag set every paged listing takes: the page and its
// size, the repeatable sort term, and the total mode.
type pageFlags struct {
	page, size int
	sort       []string
	total      string
}

// bind registers the paging flags on cmd.
func (f *pageFlags) bind(cmd *cobra.Command) {
	cmd.Flags().IntVar(&f.page, "page", 1, "the 1-based page to list")
	cmd.Flags().IntVar(&f.size, "size", 20, "the number of rows per page")
	cmd.Flags().StringArrayVar(&f.sort, "sort", nil, "a sort term, <field> or <field>:desc; repeatable, applied in order")
	cmd.Flags().StringVar(&f.total, "total", "exact", "exact to count the total, none to omit it")
}

// listing builds the Listing the paging flags state, validating the
// total mode and parsing each sort term.
func (f *pageFlags) listing() (Listing, error) {
	l := Listing{Page: f.page, Size: f.size}
	switch f.total {
	case "exact":
		l.Total = TotalExact
	case "none":
		l.Total = TotalNone
	default:
		return Listing{}, fmt.Errorf("--total %q: the mode is exact or none", f.total)
	}
	for _, term := range f.sort {
		s, err := ParseSort(term)
		if err != nil {
			return Listing{}, err
		}
		l.Sort = append(l.Sort, s)
	}
	return l, nil
}

// listingFlags is the flag set ls takes: the paging flags, the repeatable
// filter term, the cursor of each half and whether to print the next
// ones, and the unit.
type listingFlags struct {
	pageFlags
	filter                []string
	afterDirs, afterFiles string
	cursors               bool
	unit                  string
}

// bind registers the listing flags on cmd.
func (f *listingFlags) bind(cmd *cobra.Command) {
	f.pageFlags.bind(cmd)
	cmd.Flags().StringArrayVar(&f.filter, "filter", nil, "a filter term, <field>:<op>:<value>, or <field>:null and <field>:notnull; repeatable")
	cmd.Flags().StringVar(&f.afterDirs, "after-dirs", "", "continue the directory half after this cursor, from an earlier next-dirs: line")
	cmd.Flags().StringVar(&f.afterFiles, "after-files", "", "continue the file half after this cursor, from an earlier next-files: line")
	cmd.Flags().BoolVar(&f.cursors, "cursors", false, "print the next-dirs: and next-files: lines with the cursors that continue each half")
	cmd.Flags().StringVar(&f.unit, "unit", "", "list as the unit with this id, a UUID; it must own the path's top-level directory")
}

// listing builds the Listing the flags state, validating the unit and
// the total mode and parsing each filter and sort term.
func (f *listingFlags) listing() (Listing, error) {
	unit, err := parseUnit(f.unit)
	if err != nil {
		return Listing{}, err
	}
	l, err := f.pageFlags.listing()
	if err != nil {
		return Listing{}, err
	}
	for _, term := range f.filter {
		filter, err := ParseFilter(term)
		if err != nil {
			return Listing{}, err
		}
		l.Filters = append(l.Filters, filter)
	}
	l.Unit = unit
	l.After = After{Directories: f.afterDirs, Files: f.afterFiles}
	return l, nil
}

// ParseFilter reads one --filter term: <field>:<op>:<value>, where the
// value is the rest of the term and may hold colons, as a timestamp
// does; <field>:<op> alone for null and notnull; and for in, a value
// that is a comma-separated list. The field and the operator are checked
// by the library against the listing's declared fields and the query
// library's operators, so an unknown one is refused there, before the
// statement runs.
func ParseFilter(term string) (Filter, error) {
	field, rest, ok := strings.Cut(term, ":")
	if field == "" || !ok {
		return Filter{}, fmt.Errorf("--filter %q: write <field>:<op>:<value>, or <field>:null or <field>:notnull", term)
	}
	op, value, hasValue := strings.Cut(rest, ":")
	if op == "" {
		return Filter{}, fmt.Errorf("--filter %q: names no operator; write <field>:<op>:<value>", term)
	}
	switch op {
	case "null", "notnull":
		if hasValue {
			return Filter{}, fmt.Errorf("--filter %q: %s takes no value; write %s:%s", term, op, field, op)
		}
		return Filter{Field: field, Op: op}, nil
	case "in":
		if !hasValue {
			return Filter{}, fmt.Errorf("--filter %q: in takes a comma-separated list; write %s:in:<value>,<value>", term, field)
		}
		parts := strings.Split(value, ",")
		values := make([]any, len(parts))
		for i, p := range parts {
			values[i] = p
		}
		return Filter{Field: field, Op: op, Value: values}, nil
	}
	if !hasValue {
		return Filter{}, fmt.Errorf("--filter %q: names no value; write <field>:<op>:<value>, or <field>:null or <field>:notnull", term)
	}
	return Filter{Field: field, Op: op, Value: value}, nil
}

// ParseSort reads one --sort term: a field name, or a field name and
// :desc for descending order; :asc is accepted and means the default.
func ParseSort(term string) (Sort, error) {
	field, direction, hasDirection := strings.Cut(term, ":")
	if field == "" {
		return Sort{}, fmt.Errorf("--sort %q: names no field; write <field> or <field>:desc", term)
	}
	switch {
	case !hasDirection, direction == "asc":
		return Sort{Field: field}, nil
	case direction == "desc":
		return Sort{Field: field, Descending: true}, nil
	}
	return Sort{}, fmt.Errorf("--sort %q: the direction is asc or desc", term)
}

// parseUnit checks that a --unit value, when given, is a UUID, so a
// malformed id fails before the store is constructed, and returns it in
// canonical form, which is the form the engine returns a uuid column in,
// so the scope check compares like with like.
func parseUnit(unit string) (string, error) {
	if unit == "" {
		return "", nil
	}
	id, err := uuid.Parse(unit)
	if err != nil {
		return "", errors.Join(fmt.Errorf("--unit %q is not a UUID", unit), err)
	}
	return id.String(), nil
}
