package notebook

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Gaurav-Gosain/gopyter/internal/table"
)

func TestGolarsNotebookRoundTrip(t *testing.T) {
	nb := NewLang(GLR)
	nb.Cells[0].Source = "load data.csv"
	goCell := &Cell{ID: "g", Type: Code, Source: "x := 1"}
	goCell.Metadata = SetCellLang(goCell.Metadata, Go, nb.Lang())
	nb.Cells = append(nb.Cells, goCell)
	b, err := nb.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	// The kernelspec is golars-kernel's, so JupyterLab opens it there.
	var raw struct {
		Metadata struct {
			Kernelspec   map[string]string `json:"kernelspec"`
			LanguageInfo map[string]any    `json:"language_info"`
		} `json:"metadata"`
		Cells []struct {
			Metadata map[string]any `json:"metadata"`
		} `json:"cells"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	ks := raw.Metadata.Kernelspec
	if ks["name"] != "golars" || ks["language"] != "golars" || ks["display_name"] != "golars (.glr)" {
		t.Fatalf("kernelspec %v", ks)
	}
	if raw.Metadata.LanguageInfo["mimetype"] != "text/x-glr" || raw.Metadata.LanguageInfo["file_extension"] != ".glr" {
		t.Fatalf("language_info %v", raw.Metadata.LanguageInfo)
	}
	if len(raw.Cells[0].Metadata) != 0 {
		t.Fatalf("default-language cell has metadata %v", raw.Cells[0].Metadata)
	}
	if g, _ := raw.Cells[1].Metadata["gopyter"].(map[string]any); g["language"] != "go" {
		t.Fatalf("go cell metadata %v", raw.Cells[1].Metadata)
	}

	back, err := Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	if back.Lang() != GLR || CellLang(back.Cells[0].Metadata, back.Lang()) != GLR || CellLang(back.Cells[1].Metadata, back.Lang()) != Go {
		t.Fatalf("languages lost: %v %v", back.Lang(), back.Cells[1].Metadata)
	}
	// Back to the default: the entry goes away.
	md := SetCellLang(back.Cells[1].Metadata, GLR, GLR)
	if _, ok := md["gopyter"]; ok {
		t.Fatalf("metadata %v", md)
	}
	if New().Lang() != Go {
		t.Fatal("plain notebooks are Go")
	}
}

func TestResolveMagic(t *testing.T) {
	cases := []struct {
		src      string
		def      Lang
		wantLang Lang
		wantSrc  string
	}{
		{"load x.csv", GLR, GLR, "load x.csv"},
		{"%%go\nfmt.Println(1)", GLR, Go, "\nfmt.Println(1)"},
		{"  %%glr\nshow\n", Go, GLR, "  \nshow\n"},
		{"%%golars", Go, GLR, ""},
		{"%% -n=3\nx", Go, Go, "%% -n=3\nx"},
		{"%%writefile a.txt\nx", Go, Go, "%%writefile a.txt\nx"},
	}
	for _, c := range cases {
		l, src := Resolve(c.src, c.def)
		if l != c.wantLang || src != c.wantSrc {
			t.Errorf("Resolve(%q) = %q, %q; want %q, %q", c.src, l, src, c.wantLang, c.wantSrc)
		}
		if strings.Count(src, "\n") != strings.Count(c.src, "\n") {
			t.Errorf("line count changed for %q", c.src)
		}
	}
}

func TestTableOutputRoundTrip(t *testing.T) {
	out := table.Output{
		Table: table.Table{Columns: []string{"a"}, Dtypes: []string{"i64"}, Rows: [][]*string{{nil}}, Shape: [2]int{1, 1}},
		HTML:  "<table>x</table>",
	}
	nb := &Notebook{Cells: []*Cell{{ID: "a", Type: Code, ExecutionCount: 1, Outputs: []Output{{Kind: TableOut, Text: out.Encode()}}}}}
	b, err := nb.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{`"application/vnd.golars.table+json": {`, `"text/html": [`, `"text/plain": [`} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %s in\n%s", want, s)
		}
	}
	back, err := Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	o := back.Cells[0].Outputs
	if len(o) != 1 || o[0].Kind != TableOut {
		t.Fatalf("outputs %+v", o)
	}
	got, err := table.Decode(o[0].Text)
	if err != nil || got.HTML != out.HTML || got.Table.Rows[0][0] != nil || !strings.Contains(got.Plain, "null") {
		t.Fatalf("%+v %v", got, err)
	}
}
