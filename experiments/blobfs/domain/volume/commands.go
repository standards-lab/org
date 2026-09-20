package volume

import (
	"context"
	"errors"
	"fmt"
	"strconv"
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

// Commands builds the domain's root-level commands: volume, a container
// with create, ls, and rename; mkdir; and ls. A leaf's RunE calls newStore
// when it runs, never when the tree is built: the composition root closes
// newStore over its persistent flags, which cobra parses during execution,
// so the DSN is unknown until then. The store is verified against the
// database before the leaf's first use, so a schema that is not applied
// fails with ErrVerify and does no work. Every leaf renders through out
// and returns its error unrendered; the root prints it.
func Commands(newStore func() (*Store, error), out *output.Output) []*cobra.Command {
	d := deps{newStore: newStore, out: out}
	volume := &cobra.Command{
		Use:   "volume",
		Short: "Create, list, and rename volumes",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	volume.AddCommand(d.volumeCreate(), d.volumeList(), d.volumeRename())
	return []*cobra.Command{volume, d.mkdir(), d.list()}
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

// volumeCreate is volume create <name> --unit <uuid>: blobfs's volume and
// root and the consumer's owner row, in one transaction.
func (d deps) volumeCreate() *cobra.Command {
	var unit string
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a volume owned by a unit",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validUnit(unit); err != nil {
				return err
			}
			s, err := d.store(cmd.Context())
			if err != nil {
				return err
			}
			v, err := s.CreateVolume(cmd.Context(), args[0], unit)
			if err != nil {
				return err
			}
			d.out.Line(fmt.Sprintf("volume create: %s (id %s, unit %s)", v.Name, v.ID, v.UnitID))
			return nil
		},
	}
	cmd.Flags().StringVar(&unit, "unit", "", "the id of the unit that owns the volume, a UUID")
	_ = cmd.MarkFlagRequired("unit")
	return cmd
}

// volumeList is volume ls: one page of volume_view, with --unit as a
// filter on the owner and the paging and sort flags.
func (d deps) volumeList() *cobra.Command {
	var f listingFlags
	cmd := &cobra.Command{
		Use:   "ls",
		Short: "List volumes with their owners, one page at a time",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			l, err := f.listing()
			if err != nil {
				return err
			}
			s, err := d.store(cmd.Context())
			if err != nil {
				return err
			}
			volumes, total, err := s.Volumes(cmd.Context(), l)
			if err != nil {
				return err
			}
			rows := make([][]string, 0, len(volumes))
			for _, v := range volumes {
				rows = append(rows, []string{v.Name, strconv.FormatInt(v.Version, 10), v.UnitID, v.CreatedAt.UTC().Format("2006-01-02 15:04:05")})
			}
			d.out.Rows([]string{"NAME", "VERSION", "UNIT", "CREATED"}, rows)
			d.out.Total(total)
			return nil
		},
	}
	f.bind(cmd)
	return cmd
}

// volumeRename is volume rename <name> <new-name>, under blobfs's version
// guard at the version the lookup saw.
func (d deps) volumeRename() *cobra.Command {
	return &cobra.Command{
		Use:   "rename <name> <new-name>",
		Short: "Rename a volume",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := d.store(cmd.Context())
			if err != nil {
				return err
			}
			v, err := s.RenameVolume(cmd.Context(), args[0], args[1])
			if err != nil {
				return err
			}
			d.out.Line(fmt.Sprintf("volume rename: %s -> %s (version %d)", args[0], v.Name, v.Version))
			return nil
		},
	}
}

// mkdir is mkdir <volume>:<path>: the last segment created under its
// existing parent. There is no -p; a missing parent is an error.
func (d deps) mkdir() *cobra.Command {
	return &cobra.Command{
		Use:   "mkdir <volume>:<path>",
		Short: "Create a directory under an existing parent",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			addr, err := ParseAddress(args[0])
			if err != nil {
				return err
			}
			if _, _, err := addr.Split(); err != nil {
				return err
			}
			s, err := d.store(cmd.Context())
			if err != nil {
				return err
			}
			if _, err := s.Mkdir(cmd.Context(), addr); err != nil {
				return err
			}
			d.out.Line("mkdir: " + addr.String())
			return nil
		},
	}
}

// list is ls <volume>:<path>: the directories under the address first,
// then its files, each one page, with a total line for both.
func (d deps) list() *cobra.Command {
	var f listingFlags
	cmd := &cobra.Command{
		Use:   "ls <volume>:<path>",
		Short: "List a directory: its directories, then its files, one page each",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			addr, err := ParseAddress(args[0])
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
			c, err := s.List(cmd.Context(), addr, l)
			if err != nil {
				return err
			}
			rows := make([][]string, 0, len(c.Directories)+len(c.Files))
			for _, dir := range c.Directories {
				name := ""
				if dir.Name != nil {
					name = *dir.Name
				}
				rows = append(rows, []string{"dir", name, "-", "-", addr.Join(name).Path})
			}
			for _, file := range c.Files {
				size := "-"
				if file.Size != nil {
					size = strconv.FormatInt(*file.Size, 10)
				}
				rows = append(rows, []string{"file", file.Name, size, string(file.Status), file.Path})
			}
			d.out.Rows([]string{"KIND", "NAME", "SIZE", "STATUS", "PATH"}, rows)
			d.out.Line(fmt.Sprintf("total: %d directories, %d files", c.DirectoryTotal, c.FileTotal))
			return nil
		},
	}
	f.bind(cmd)
	return cmd
}

// listingFlags is the flag set the two listing commands share: the page
// and its size, the repeatable sort term, and the unit filter.
type listingFlags struct {
	page, size int
	sort       []string
	unit       string
}

// bind registers the listing flags on cmd.
func (f *listingFlags) bind(cmd *cobra.Command) {
	cmd.Flags().IntVar(&f.page, "page", 1, "the 1-based page to list")
	cmd.Flags().IntVar(&f.size, "size", 20, "the number of rows per page")
	cmd.Flags().StringArrayVar(&f.sort, "sort", nil, "a sort term, <field> or <field>:desc; repeatable, applied in order")
	cmd.Flags().StringVar(&f.unit, "unit", "", "list only what the unit with this id owns, a UUID")
}

// listing builds the Listing the flags state, validating the unit and
// parsing each sort term.
func (f *listingFlags) listing() (Listing, error) {
	if err := validUnit(f.unit); err != nil {
		return Listing{}, err
	}
	l := Listing{Page: f.page, Size: f.size, Unit: f.unit}
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

// validUnit checks that a --unit value, when given, is a UUID, so a
// malformed id fails before the store is constructed.
func validUnit(unit string) error {
	if unit == "" {
		return nil
	}
	if _, err := uuid.Parse(unit); err != nil {
		return errors.Join(fmt.Errorf("--unit %q is not a UUID", unit), err)
	}
	return nil
}
