package table

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func str(s string) *string { return &s }

func sample() Table {
	return Table{
		Columns: []string{"region", "total", "note"},
		Dtypes:  []string{"str", "f64", "str"},
		Rows: [][]*string{
			{str("eu"), str("12.5"), nil},
			{str("us"), str("7"), str("two\nlines")},
		},
		Shape: [2]int{2, 3},
	}
}

func TestPlain(t *testing.T) {
	got := Plain(sample())
	want := strings.Join([]string{
		"┌────────┬───────┬───────────┐",
		"│ region ┆ total ┆ note      │",
		"│ str    ┆ f64   ┆ str       │",
		"╞════════╪═══════╪═══════════╡",
		"│ eu     ┆  12.5 ┆ null      │",
		"│ us     ┆     7 ┆ two↵lines │",
		"└────────┴───────┴───────────┘",
		"shape: (2, 3)",
	}, "\n")
	if got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
}

func TestGaps(t *testing.T) {
	tbl := sample()
	two, one := 1, 2
	tbl.RowGap, tbl.ColGap = &two, &one
	tbl.Shape = [2]int{100, 9}
	lines := strings.Split(Plain(tbl), "\n")
	if !strings.Contains(lines[1], "total ┆ … ┆ note") {
		t.Errorf("gap column: %q", lines[1])
	}
	if strings.Count(lines[5], "…") != 4 || !strings.Contains(lines[4], "eu") || !strings.Contains(lines[6], "us") {
		t.Errorf("gap row:\n%s", strings.Join(lines, "\n"))
	}
	if last := lines[len(lines)-1]; last != "shape: (100, 9) · 3 of 9 columns shown" {
		t.Errorf("footer %q", last)
	}
}

func TestFitWidth(t *testing.T) {
	tbl := Table{Shape: [2]int{1, 12}}
	row := []*string{}
	for i := range 12 {
		tbl.Columns = append(tbl.Columns, "column_"+string(rune('a'+i)))
		tbl.Dtypes = append(tbl.Dtypes, "i64")
		row = append(row, str("123456"))
	}
	tbl.Rows = [][]*string{row}
	for _, w := range []int{80, 40, 20} {
		lines := Render(tbl, w, Styles{})
		for _, l := range lines {
			if lw := ansi.StringWidth(l); lw > w {
				t.Fatalf("width %d: line %q is %d wide", w, l, lw)
			}
		}
		if !strings.Contains(lines[1], "column_a") || !strings.Contains(lines[1], "…") {
			t.Fatalf("width %d: %q", w, lines[1])
		}
	}
	if lines := Render(tbl, 80, Styles{}); !strings.Contains(lines[1], "column_l") {
		t.Fatalf("last column should stay: %q", lines[1])
	}
}

func TestSeriesAndEmpty(t *testing.T) {
	s := Table{Kind: "series", Columns: []string{"x"}, Dtypes: []string{"i64"}, Rows: [][]*string{{str("1")}}, Shape: [2]int{1, 1}}
	if p := Plain(s); !strings.HasSuffix(p, "shape: (1,)") {
		t.Fatalf("series footer: %s", p)
	}
	if p := Plain(Table{Shape: [2]int{0, 0}}); p != "shape: (0, 0) · empty" {
		t.Fatalf("empty: %q", p)
	}
}

func TestStyles(t *testing.T) {
	st := Styles{Null: lipgloss.NewStyle().Italic(true), Number: lipgloss.NewStyle().Bold(true)}
	lines := Render(sample(), 80, st)
	if !strings.Contains(lines[4], "\x1b[3mnull") || !strings.Contains(lines[4], "\x1b[1m 12.5") {
		t.Fatalf("styles not applied: %q", lines[4])
	}
}

func TestOutputRoundTrip(t *testing.T) {
	o, ok := FromBundle(map[string]string{
		MIME:        `{"columns":["a"],"dtypes":["i64"],"rows":[["1"],[null]],"shape":[2,1]}`,
		"text/html": "<table></table>",
	})
	if !ok || o.HTML != "<table></table>" || o.Table.Rows[1][0] != nil {
		t.Fatalf("%+v", o)
	}
	back, err := Decode(o.Encode())
	if err != nil || back.Plain == "" || back.Table.Shape != [2]int{2, 1} {
		t.Fatalf("%+v %v", back, err)
	}
	if _, ok := FromBundle(map[string]string{"text/plain": "x"}); ok {
		t.Fatal("bundle without a table")
	}
}
