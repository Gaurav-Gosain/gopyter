// Package table holds DataFrame outputs from golars (glr cells, and Go
// cells displaying a golars DataFrame or Series) and draws them as
// terminal tables in the style of polars: a header, a dtype row, numbers
// aligned right, nulls set apart, "…" where rows or columns were left out
// and a shape footer.
//
// The data comes from golars as JSON under MIME (see golars
// dataframe.TableMIME). A notebook output keeps it next to the HTML and
// plain text golars produced, so Jupyter shows the HTML while gopyter
// draws its own table.
package table

import (
	"encoding/json"
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// MIME is the media type of the JSON table in golars mime bundles.
const MIME = "application/vnd.golars.table+json"

// Table is a formatted window of a DataFrame or Series, as golars sends it.
type Table struct {
	// Kind is "series" for a Series.
	Kind    string   `json:"kind,omitempty"`
	Columns []string `json:"columns"`
	Dtypes  []string `json:"dtypes"`
	// Rows are the shown rows; a nil cell is a null.
	Rows [][]*string `json:"rows"`
	// Shape is the (height, width) of the whole frame.
	Shape [2]int `json:"shape"`
	// RowGap, when set, is the index in Rows where rows were left out.
	RowGap *int `json:"row_gap,omitempty"`
	// ColGap, when set, is the index in Columns where columns were left out.
	ColGap *int `json:"col_gap,omitempty"`
}

// Output is a table output of a cell: the table and the other forms of it
// saved in the notebook.
type Output struct {
	Table Table `json:"table"`
	// HTML is golars' text/html rendering, for Jupyter.
	HTML string `json:"html,omitempty"`
	// Plain is the text/plain form; Plain(Table) when golars gave none.
	Plain string `json:"plain,omitempty"`
}

// Encode returns the output as the text of a notebook output.
func (o Output) Encode() string {
	if o.Plain == "" {
		o.Plain = Plain(o.Table)
	}
	b, err := json.Marshal(o)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// Decode parses the text of a table output.
func Decode(s string) (Output, error) {
	var o Output
	err := json.Unmarshal([]byte(s), &o)
	return o, err
}

// FromBundle builds an output from a golars mime bundle; ok is false when
// the bundle has no table.
func FromBundle(bundle map[string]string) (Output, bool) {
	raw, ok := bundle[MIME]
	if !ok {
		return Output{}, false
	}
	var t Table
	if json.Unmarshal([]byte(raw), &t) != nil {
		return Output{}, false
	}
	return Output{Table: t, HTML: bundle["text/html"], Plain: bundle["text/plain"]}, true
}

// Styles are the styles a table is drawn with. The zero value draws plain
// text.
type Styles struct {
	Border lipgloss.Style
	Header lipgloss.Style
	Dtype  lipgloss.Style
	Text   lipgloss.Style
	Number lipgloss.Style
	Bool   lipgloss.Style
	Null   lipgloss.Style
	Muted  lipgloss.Style // "…" and the shape footer
}

// maxCell is the widest a cell is drawn, in columns, before it is cut.
const maxCell = 40

// column is a column as drawn; gap marks the "…" column.
type column struct {
	name, dtype string
	cells       []string // rows including the gap row
	nulls       []bool
	numeric     bool
	gap         bool
	width       int
}

// Render draws t in at most limit columns. Columns that don't fit are
// left out from the middle, polars style.
func Render(t Table, limit int, st Styles) []string {
	cols := columns(t)
	shape := shapeText(t)
	if len(cols) == 0 {
		return []string{st.Muted.Render(shape + " · empty")}
	}
	cols = fit(cols, limit)

	var lines []string
	border := func(l, mid, r, fill string) string {
		var b strings.Builder
		b.WriteString(l)
		for i, c := range cols {
			if i > 0 {
				b.WriteString(mid)
			}
			b.WriteString(strings.Repeat(fill, c.width+2))
		}
		b.WriteString(r)
		return st.Border.Render(b.String())
	}
	row := func(cell func(c column) string) string {
		var b strings.Builder
		b.WriteString(st.Border.Render("│"))
		for i, c := range cols {
			if i > 0 {
				b.WriteString(st.Border.Render("┆"))
			}
			b.WriteByte(' ')
			b.WriteString(cell(c))
			b.WriteByte(' ')
		}
		b.WriteString(st.Border.Render("│"))
		return b.String()
	}
	lines = append(lines, border("┌", "┬", "┐", "─"))
	lines = append(lines, row(func(c column) string {
		if c.gap {
			return st.Muted.Render(pad("…", c.width, false))
		}
		return st.Header.Render(pad(cut(c.name, c.width), c.width, false))
	}))
	lines = append(lines, row(func(c column) string {
		if c.gap {
			return st.Muted.Render(pad("…", c.width, false))
		}
		return st.Dtype.Render(pad(cut(c.dtype, c.width), c.width, false))
	}))
	lines = append(lines, border("╞", "╪", "╡", "═"))
	gapRow := rowGap(t)
	for r := range len(cols[0].cells) {
		lines = append(lines, row(func(c column) string {
			s := c.cells[r]
			switch {
			case c.gap || r == gapRow:
				return st.Muted.Render(pad("…", c.width, false))
			case c.nulls[r]:
				return st.Null.Render(pad("null", c.width, c.numeric))
			}
			style := st.Text
			switch {
			case c.numeric:
				style = st.Number
			case c.dtype == "bool":
				style = st.Bool
			}
			return style.Render(pad(cut(s, c.width), c.width, c.numeric))
		}))
	}
	lines = append(lines, border("└", "┴", "┘", "─"))
	footer := shape
	if shown := countData(cols); shown < t.Shape[1] && t.Kind != "series" {
		more := fmt.Sprintf(" · %d of %d columns shown", shown, t.Shape[1])
		if width(footer+more) <= limit {
			footer += more
		}
	}
	lines = append(lines, st.Muted.Render(footer))
	return lines
}

// Plain draws t as plain text, without a width limit.
func Plain(t Table) string {
	return strings.Join(Render(t, 1<<16, Styles{}), "\n")
}

func shapeText(t Table) string {
	if t.Kind == "series" {
		return fmt.Sprintf("shape: (%d,)", t.Shape[0])
	}
	return fmt.Sprintf("shape: (%d, %d)", t.Shape[0], t.Shape[1])
}

func rowGap(t Table) int {
	if t.RowGap == nil {
		return -1
	}
	return *t.RowGap
}

func countData(cols []column) int {
	n := 0
	for _, c := range cols {
		if !c.gap {
			n++
		}
	}
	return n
}

// columns lays t out as drawn columns, with the gap row and column.
func columns(t Table) []column {
	gapRow := rowGap(t)
	gapCol := -1
	if t.ColGap != nil {
		gapCol = *t.ColGap
	}
	var cols []column
	for i, name := range t.Columns {
		if i == gapCol {
			cols = append(cols, gapColumn(drawnRows(t)))
		}
		dt := ""
		if i < len(t.Dtypes) {
			dt = t.Dtypes[i]
		}
		c := column{name: name, dtype: dt, numeric: numeric(dt)}
		c.width = max(width(name), width(dt), 1)
		for r := 0; r <= len(t.Rows); r++ {
			if r == gapRow {
				c.cells = append(c.cells, "…")
				c.nulls = append(c.nulls, false)
			}
			if r == len(t.Rows) {
				break
			}
			var cell *string
			if i < len(t.Rows[r]) {
				cell = t.Rows[r][i]
			}
			s, null := "null", cell == nil
			if cell != nil {
				s = clean(*cell)
			}
			c.cells = append(c.cells, s)
			c.nulls = append(c.nulls, null)
			c.width = max(c.width, min(width(s), maxCell))
		}
		cols = append(cols, c)
	}
	if gapCol >= len(t.Columns) {
		cols = append(cols, gapColumn(drawnRows(t)))
	}
	return cols
}

// drawnRows is the number of body rows drawn, with the "…" row.
func drawnRows(t Table) int {
	if rowGap(t) >= 0 {
		return len(t.Rows) + 1
	}
	return len(t.Rows)
}

// gapColumn is the "…" column standing for left out columns.
func gapColumn(rows int) column {
	c := column{gap: true, width: 1, cells: make([]string, rows), nulls: make([]bool, rows)}
	for i := range c.cells {
		c.cells[i] = "…"
	}
	return c
}

// fit leaves columns out of the middle until the table fits width, and
// narrows the widest columns when even two don't.
func fit(cols []column, limit int) []column {
	total := func(cs []column) int {
		w := 1
		for _, c := range cs {
			w += c.width + 3
		}
		return w
	}
	if total(cols) <= limit {
		return cols
	}
	var data []column
	for _, c := range cols {
		if !c.gap {
			data = append(data, c)
		}
	}
	for k := len(data) - 1; k >= 1; k-- {
		head := (k + 1) / 2
		cand := append(append(append([]column{}, data[:head]...), gapColumn(len(data[0].cells))), data[len(data)-(k-head):]...)
		if total(cand) <= limit || k == 1 {
			cols = cand
			break
		}
	}
	// Still too wide: narrow the widest column until it fits.
	for total(cols) > limit {
		widest := 0
		for i, c := range cols {
			if c.width > cols[widest].width {
				widest = i
			}
		}
		if cols[widest].width <= 4 {
			break
		}
		cols[widest].width--
	}
	return cols
}

// numeric reports whether a golars dtype holds numbers, which are aligned
// right.
func numeric(dtype string) bool {
	switch {
	case len(dtype) < 2:
		return false
	case dtype[0] == 'i' || dtype[0] == 'u' || dtype[0] == 'f':
		// i64, u32, f64... but not "interval" and the like.
		return strings.Trim(dtype[1:], "0123456789") == ""
	case strings.HasPrefix(dtype, "decimal"):
		return true
	}
	return false
}

func width(s string) int { return ansi.StringWidth(s) }

// clean keeps a cell on one line.
func clean(s string) string {
	if !strings.ContainsAny(s, "\n\r\t") {
		return s
	}
	return strings.NewReplacer("\n", "↵", "\r", "", "\t", " ").Replace(s)
}

// cut shortens s to w columns, ending in "…".
func cut(s string, w int) string {
	if width(s) <= w {
		return s
	}
	if w <= 1 {
		return "…"
	}
	return ansi.Truncate(s, w, "…")
}

func pad(s string, w int, right bool) string {
	n := w - width(s)
	if n <= 0 {
		return s
	}
	if right {
		return strings.Repeat(" ", n) + s
	}
	return s + strings.Repeat(" ", n)
}
