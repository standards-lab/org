package output

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
)

// Rows writes a listing to stdout as aligned columns: the header line,
// then one line per row, cells separated by two spaces at least. A listing
// with no rows still writes its header, so an empty page is visible as one.
func (o *Output) Rows(header []string, rows [][]string) {
	w := tabwriter.NewWriter(o.stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, strings.Join(header, "\t"))
	for _, row := range rows {
		_, _ = fmt.Fprintln(w, strings.Join(row, "\t"))
	}
	_ = w.Flush()
}

// Total writes the total line of a listing to stdout: the count of rows
// under the listing's filters, all pages together.
func (o *Output) Total(n int) {
	_, _ = fmt.Fprintf(o.stdout, "total: %d\n", n)
}

// Entry is one line of a directory listing: a directory or a file at the
// listed path. Kind is dir or file. Size is nil for a directory and for a
// file whose size is not known yet, and Status is empty for a directory.
type Entry struct {
	Kind    string
	Name    string
	Size    *int64
	Status  string
	Updated time.Time
}

// NoTotal is the Total of a Page whose total is not known: the page is
// empty and is not the first, so nothing on it carried the count.
const NoTotal = -1

// Page describes one half of a directory listing: the page number and
// size the listing asked for, how many rows the page holds, and the total
// of all pages, which is NoTotal when the page carries none and is not
// reported at all when the listing did not ask for one (Counted false).
// Cursor says the half was read after a cursor rather than by number, so
// the number is not shown, and Next is the cursor of the following page,
// written on its own line when it is not empty.
type Page struct {
	Number  int
	Size    int
	Listed  int
	Total   int
	Counted bool
	Cursor  bool
	Next    string
}

// Listing writes a directory listing to stdout: the entries as aligned
// columns, directories then files as the caller ordered them, one line per
// half saying what the page holds and the total or its absence, and after
// a half that has a next page, the line next-dirs: or next-files: with the
// cursor that continues it.
func (o *Output) Listing(entries []Entry, directories, files Page) {
	rows := make([][]string, 0, len(entries))
	for _, e := range entries {
		size, status := "-", "-"
		if e.Size != nil {
			size = strconv.FormatInt(*e.Size, 10)
		}
		if e.Status != "" {
			status = e.Status
		}
		rows = append(rows, []string{e.Kind, e.Name, size, status, e.Updated.UTC().Format("2006-01-02 15:04:05")})
	}
	o.Rows([]string{"KIND", "NAME", "SIZE", "STATUS", "UPDATED"}, rows)
	o.page("directories", "next-dirs", directories)
	o.page("files", "next-files", files)
}

// page writes one half's line: the rows on the page, the page number and
// size (or the cursor and the size), and the total as counted, not
// counted, or unknown; then the next cursor under label when there is one.
func (o *Output) page(half, label string, p Page) {
	total := "not counted"
	switch {
	case p.Counted && p.Total == NoTotal:
		total = "unknown (the page is empty)"
	case p.Counted:
		total = strconv.Itoa(p.Total)
	}
	if p.Cursor {
		_, _ = fmt.Fprintf(o.stdout, "%s: %d after the cursor, size %d, total %s\n", half, p.Listed, p.Size, total)
	} else {
		_, _ = fmt.Fprintf(o.stdout, "%s: %d on page %d of size %d, total %s\n", half, p.Listed, p.Number, p.Size, total)
	}
	if p.Next != "" {
		_, _ = fmt.Fprintf(o.stdout, "%s: %s\n", label, p.Next)
	}
}

// BookmarkEntry is one line of a bookmark listing: a file the unit
// bookmarked, at its full path. Size is nil for a file whose size is not
// known yet, and Active marks the unit's one active bookmark.
type BookmarkEntry struct {
	Path    string
	Size    *int64
	Status  string
	Active  bool
	Updated time.Time
}

// Bookmarks writes a bookmark listing to stdout: the entries as aligned
// columns in the order given, then one line saying what the page holds
// and the total or its absence, in the form the halves of a directory
// listing use. The read model pages by number only, so no cursor line is
// written.
func (o *Output) Bookmarks(entries []BookmarkEntry, p Page) {
	rows := make([][]string, 0, len(entries))
	for _, e := range entries {
		size, active := "-", "-"
		if e.Size != nil {
			size = strconv.FormatInt(*e.Size, 10)
		}
		if e.Active {
			active = "active"
		}
		rows = append(rows, []string{e.Path, size, e.Status, active, e.Updated.UTC().Format("2006-01-02 15:04:05")})
	}
	o.Rows([]string{"PATH", "SIZE", "STATUS", "ACTIVE", "UPDATED"}, rows)
	o.page("bookmarks", "next", p)
}

// Field is one line of a record: a label and its value.
type Field struct {
	Name  string
	Value string
}

// Record writes one record to stdout as aligned label and value lines,
// one per field in the order given, the label followed by a colon. It is
// how a command shows one row, where a listing would show many.
func (o *Output) Record(fields []Field) {
	w := tabwriter.NewWriter(o.stdout, 0, 0, 1, ' ', 0)
	for _, f := range fields {
		_, _ = fmt.Fprintf(w, "%s:\t%s\n", f.Name, f.Value)
	}
	_ = w.Flush()
}

// Copy writes r's bytes to stdout as they are, with nothing added, and
// returns the count written. It is how a command streams a file's
// content.
func (o *Output) Copy(r io.Reader) (int64, error) {
	return io.Copy(o.stdout, r)
}
