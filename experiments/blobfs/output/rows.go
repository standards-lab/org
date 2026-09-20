package output

import (
	"fmt"
	"strings"
	"text/tabwriter"
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
