package output

import (
	"fmt"
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
type Page struct {
	Number  int
	Size    int
	Listed  int
	Total   int
	Counted bool
}

// Listing writes a directory listing to stdout: the entries as aligned
// columns, directories then files as the caller ordered them, and one line
// per half saying what the page holds and the total or its absence.
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
	o.page("directories", directories)
	o.page("files", files)
}

// page writes one half's line: the rows on the page, the page number and
// size, and the total as counted, not counted, or unknown.
func (o *Output) page(half string, p Page) {
	total := "not counted"
	switch {
	case p.Counted && p.Total == NoTotal:
		total = "unknown (the page is empty)"
	case p.Counted:
		total = strconv.Itoa(p.Total)
	}
	_, _ = fmt.Fprintf(o.stdout, "%s: %d on page %d of size %d, total %s\n", half, p.Listed, p.Number, p.Size, total)
}
