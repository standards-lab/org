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
// stat, and bookmark with its add, ls, and rm subcommands. A leaf's
// RunE calls newStore when it runs, never when the tree is built: the
// composition root closes newStore over its persistent flags, which cobra
// parses during execution, so the DSN is unknown until then. The store is
// verified against the database before the leaf's first use, so a schema
// that is not applied fails with ErrVerify and does no work. Every leaf
// renders through out and returns its error unrendered; the root prints
// it.
func Commands(newStore func() (*Store, error), out *output.Output) []*cobra.Command {
	d := deps{newStore: newStore, out: out}
	return []*cobra.Command{d.mkdir(), d.list(), d.put(), d.cat(), d.stat(), d.bookmark()}
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

// list is ls <path>: the directories under the path first, then its files,
// each one page, with a line per half stating the page and the total.
func (d deps) list() *cobra.Command {
	var f listingFlags
	cmd := &cobra.Command{
		Use:   "ls <path>",
		Short: "List a directory: its directories, then its files, one page each",
		Long: "ls lists the directory at an absolute path such as /reports: the directories\n" +
			"under it, then the files in it, one page of each. --sort applies to both halves;\n" +
			"a field only files have sorts the files and leaves the directories in name order.\n" +
			"A half with a next page prints next-dirs: or next-files: with a cursor; pass it\n" +
			"back as --after-dirs or --after-files to continue that half, which then ignores\n" +
			"--page and carries no total. With --unit, the unit must own the path's top-level\n" +
			"directory, and at / the listing is the unit's own top-level directories.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			l, err := f.listing()
			if err != nil {
				return err
			}
			s, err := d.store(cmd.Context())
			if err != nil {
				return err
			}
			c, err := s.List(cmd.Context(), args[0], l)
			if err != nil {
				return err
			}
			entries := make([]output.Entry, 0, len(c.Directories.Rows)+len(c.Files.Rows))
			for _, dir := range c.Directories.Rows {
				name := ""
				if dir.Name != nil {
					name = *dir.Name
				}
				entries = append(entries, output.Entry{Kind: "dir", Name: name, Updated: dir.UpdatedAt})
			}
			for _, file := range c.Files.Rows {
				entries = append(entries, output.Entry{Kind: "file", Name: file.Name, Size: file.Size, Status: string(file.Status), Updated: file.UpdatedAt})
			}
			d.out.Listing(entries, pageOf(l, l.After.Directories, c.Directories), pageOf(l, l.After.Files, c.Files))
			return nil
		},
	}
	f.bind(cmd)
	return cmd
}

// put is put <local-file|-> <path> [--content-type <type>] [--fail-after
// <step>]: the local file, or stdin for -, uploaded as the file at the
// path. The content type comes from the flag, else from the local file's
// extension, else application/octet-stream. --fail-after stops the write
// after the named step with a non-zero exit, leaving the pending row for
// a later put of the same path to complete.
func (d deps) put() *cobra.Command {
	var contentType, failAfter string
	cmd := &cobra.Command{
		Use:   "put <local-file|-> <path>",
		Short: "Upload a local file, or stdin, as the file at a path",
		Long: "put uploads a local file (or stdin, for -) as the file at an absolute path such\n" +
			"as /reports/2026/q1.pdf, whose parent must exist. The write is two steps around\n" +
			"the upload: the row is inserted as pending and committed, the object is stored,\n" +
			"and the row is completed as available. --fail-after insert or write stops after\n" +
			"that step and exits non-zero; ls and stat then show the row pending, and a put\n" +
			"of the same path resumes it. A name held by an available file is refused.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			step, err := ParseStep(failAfter)
			if err != nil {
				return err
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
			req := PutRequest{Path: args[1], ContentType: declaredType(contentType, args[0]), Body: body, Size: size, StopAfter: step}
			res, err := s.Put(cmd.Context(), req)
			if err != nil {
				return err
			}
			f := res.File
			line := fmt.Sprintf("put: %s (id %s, %d bytes, etag %s)", args[1], f.ID, sizeOf(f), etagOf(f))
			if res.Resumed {
				line = fmt.Sprintf("put: %s (id %s, %d bytes, etag %s, resumed the pending row)", args[1], f.ID, sizeOf(f), etagOf(f))
			}
			d.out.Line(line)
			return nil
		},
	}
	cmd.Flags().StringVar(&contentType, "content-type", "", "the media type to store with the object; the default is derived from the local file's extension")
	cmd.Flags().StringVar(&failAfter, "fail-after", "", "stop after this step of the write, insert or write, and exit non-zero")
	return cmd
}

// cat is cat <path>: the file's content streamed to stdout as it is.
func (d deps) cat() *cobra.Command {
	return &cobra.Command{
		Use:   "cat <path>",
		Short: "Write a file's content to stdout",
		Long: "cat streams the content of the file at an absolute path to stdout. A pending or\n" +
			"deleting file has no content to read and is refused.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := d.store(cmd.Context())
			if err != nil {
				return err
			}
			body, _, err := s.Open(cmd.Context(), args[0])
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

// stat is stat <path>: the file's row, one field per line.
func (d deps) stat() *cobra.Command {
	return &cobra.Command{
		Use:   "stat <path>",
		Short: "Show a file's row: status, size, content type, etag, and timestamps",
		Long: "stat prints the row of the file at an absolute path, whatever its status: a\n" +
			"pending file shows as pending with no size or etag. The object store is not\n" +
			"consulted.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := d.store(cmd.Context())
			if err != nil {
				return err
			}
			f, err := s.Stat(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			d.out.Record(fileRecord(args[0], f))
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

// fileRecord lays a file row out as the fields stat prints, in order.
func fileRecord(path string, f blobfs.File) []output.Field {
	size := "-"
	if f.Size != nil {
		size = strconv.FormatInt(*f.Size, 10)
	}
	return []output.Field{
		{Name: "path", Value: path},
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
// listing asked for one and the half was read by number, and the cursor
// of the next page.
func pageOf[T any](l Listing, after string, p Page[T]) output.Page {
	out := output.Page{Number: l.Page, Size: l.Size, Listed: len(p.Rows), Counted: l.Total == TotalExact && after == "", Cursor: after != "", Next: p.Next}
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

// listingFlags is the flag set ls takes: the paging flags, the cursor of
// each half, and the unit.
type listingFlags struct {
	pageFlags
	afterDirs, afterFiles string
	unit                  string
}

// bind registers the listing flags on cmd.
func (f *listingFlags) bind(cmd *cobra.Command) {
	f.pageFlags.bind(cmd)
	cmd.Flags().StringVar(&f.afterDirs, "after-dirs", "", "continue the directory half after this cursor, from an earlier next-dirs: line")
	cmd.Flags().StringVar(&f.afterFiles, "after-files", "", "continue the file half after this cursor, from an earlier next-files: line")
	cmd.Flags().StringVar(&f.unit, "unit", "", "list as the unit with this id, a UUID; it must own the path's top-level directory")
}

// listing builds the Listing the flags state, validating the unit and
// the total mode and parsing each sort term.
func (f *listingFlags) listing() (Listing, error) {
	unit, err := parseUnit(f.unit)
	if err != nil {
		return Listing{}, err
	}
	l, err := f.pageFlags.listing()
	if err != nil {
		return Listing{}, err
	}
	l.Unit = unit
	l.After = After{Directories: f.afterDirs, Files: f.afterFiles}
	return l, nil
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
