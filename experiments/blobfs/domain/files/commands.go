package files

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"uuid"

	"github.com/spf13/cobra"

	"github.com/standards-lab/org/experiments/blobfs/output"
)

// deps is what every leaf command needs: the store constructor and the
// output to render its result through. Bundled once so a leaf builder is a
// method taking neither.
type deps struct {
	newStore func() (*Store, error)
	out      *output.Output
}

// Commands builds the domain's root-level commands, mkdir and ls. A leaf's
// RunE calls newStore when it runs, never when the tree is built: the
// composition root closes newStore over its persistent flags, which cobra
// parses during execution, so the DSN is unknown until then. The store is
// verified against the database before the leaf's first use, so a schema
// that is not applied fails with ErrVerify and does no work. Every leaf
// renders through out and returns its error unrendered; the root prints
// it.
func Commands(newStore func() (*Store, error), out *output.Output) []*cobra.Command {
	d := deps{newStore: newStore, out: out}
	return []*cobra.Command{d.mkdir(), d.list()}
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

// listingFlags is the flag set ls takes: the page and its size, the
// repeatable sort term, the total mode, the cursor of each half, and the
// unit.
type listingFlags struct {
	page, size            int
	sort                  []string
	total                 string
	afterDirs, afterFiles string
	unit                  string
}

// bind registers the listing flags on cmd.
func (f *listingFlags) bind(cmd *cobra.Command) {
	cmd.Flags().IntVar(&f.page, "page", 1, "the 1-based page to list")
	cmd.Flags().IntVar(&f.size, "size", 20, "the number of rows per page, for each half")
	cmd.Flags().StringArrayVar(&f.sort, "sort", nil, "a sort term, <field> or <field>:desc; repeatable, applied in order")
	cmd.Flags().StringVar(&f.total, "total", "exact", "exact to count every page's total in the page statement, none to omit it")
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
	l := Listing{Page: f.page, Size: f.size, Unit: unit, After: After{Directories: f.afterDirs, Files: f.afterFiles}}
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
