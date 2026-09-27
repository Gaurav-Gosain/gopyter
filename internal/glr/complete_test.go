package glr

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Gaurav-Gosain/gopyter/internal/complete"
)

func TestDocMapping(t *testing.T) {
	before := "load a.csv\nstash base"
	src := "use base\nfilter naïve > 1\nshow"
	doc := NewDoc(before, src)
	if doc.Offset != 2 || doc.Content != before+"\n"+src {
		t.Fatalf("doc %+v", doc)
	}
	// Row 1, after "filter naïve" (12 runes): UTF-16 units match runes
	// here, since ï is in the BMP.
	if p := doc.Position(src, 1, 12); p.Line != 3 || p.Character != 12 {
		t.Fatalf("position %+v", p)
	}
	// Out of range positions are clamped to the cell.
	if p := doc.Position(src, 9, 99); p.Line != 4 || p.Character != 4 {
		t.Fatalf("clamped %+v", p)
	}
	emoji := NewDoc("", "filter \U0001F600x > 1")
	if p := emoji.Position("filter \U0001F600x > 1", 0, 9); p.Character != 10 {
		t.Fatalf("utf-16 position %+v", p)
	}

	mk := func(line, char, sev int, msg string) lspDiagnostic {
		var d lspDiagnostic
		d.Range.Start = lspPosition{Line: line, Character: char}
		d.Severity, d.Message = sev, msg
		return d
	}
	diags := doc.CellDiagnostics(src, []lspDiagnostic{
		mk(0, 0, 1, "in an earlier cell"),
		mk(4, 0, 2, "last line"),
		mk(3, 7, 1, "unknown column"),
		mk(9, 0, 1, "past the end"),
		// Other cells, Go ones too, may use a stash.
		mk(3, 0, 2, `stash "big" is never used (use it with use, join or join_asof, or remove the stash)`),
	})
	if len(diags) != 2 {
		t.Fatalf("diags %+v", diags)
	}
	if d := diags[0]; d.Row != 1 || d.Col != 7 || !d.Error || d.Message != "unknown column" {
		t.Fatalf("diag %+v", d)
	}
	if d := diags[1]; d.Row != 2 || d.Error {
		t.Fatalf("diag %+v", d)
	}
}

func TestBasicCompletionFrames(t *testing.T) {
	res := basic(complete.Request{Src: "use d", Row: 0, Col: 5, Frames: []string{"df", "big"}})
	if len(res.Items) == 0 || res.Items[0].Label != "df" || res.Items[0].Kind != complete.KindVar {
		t.Fatalf("items %+v", res.Items)
	}
}

func TestBasicCompletion(t *testing.T) {
	res := basic(complete.Request{Src: "load x.csv\ngro", Row: 1, Col: 3})
	if res.Replace != 3 || len(res.Items) == 0 || res.Items[0].Label != "groupby" || res.Items[0].Detail == "" {
		t.Fatalf("%+v", res)
	}
	res = basic(complete.Request{Src: "join b o", Row: 0, Col: 8})
	if len(res.Items) < 2 || res.Items[0].Label != "on" || res.Items[1].Label != "or" ||
		res.Items[0].Kind != complete.KindKeyword {
		t.Fatalf("keywords %+v", res)
	}
	labels := func(res complete.Result) map[string]bool {
		out := map[string]bool{}
		for _, it := range res.Items {
			out[it.Label] = true
		}
		return out
	}
	// Expression functions, namespaced functions and methods.
	for _, tc := range []struct {
		src      string
		want     []string
		unwanted []string
	}{
		{"with y = coal", []string{"coalesce"}, nil},
		{"with y = dt.ye", []string{"year"}, []string{"round"}},
		{"with y = ts.dt.ye", []string{"year"}, nil},
		{"with y = x.ro", []string{"round"}, []string{"year"}},
		{"with y = x.", []string{"round", "str", "dt"}, nil},
		{"with y = str.to_upper", []string{"to_uppercase"}, nil},
		{"filter x no", []string{"not", "not_like"}, nil},
		{"filter a whe", []string{"when"}, nil},
		{"load data/x.", nil, []string{"round"}},
	} {
		got := labels(basic(complete.Request{Src: tc.src, Row: 0, Col: len(tc.src)}))
		for _, w := range tc.want {
			if !got[w] {
				t.Errorf("%q: no %q in %v", tc.src, w, got)
			}
		}
		for _, w := range tc.unwanted {
			if got[w] {
				t.Errorf("%q: unexpected %q", tc.src, w)
			}
		}
	}
	for _, name := range []string{"join_asof", "group_by_dynamic", "to_dummies"} {
		if !labels(basic(complete.Request{Src: name[:5], Row: 0, Col: 5}))[name] {
			t.Errorf("no command %q", name)
		}
	}
	if md := basicHover(complete.Request{Src: "groupby_dynamic ts every 1h", Row: 0, Col: 2}); !strings.Contains(md, "group_by_dynamic") {
		t.Fatalf("alias hover %q", md)
	}
	if md := basicHover(complete.Request{Src: "  .groupby a b:sum", Row: 0, Col: 12}); !strings.Contains(md, "groupby <k1") {
		t.Fatalf("hover %q", md)
	}
}

// With golars-lsp installed (or GOLARS_LSP set), completion knows the
// columns of files loaded by earlier cells.
func TestGolarsLSP(t *testing.T) {
	if _, err := FindLSP(); err != nil || testing.Short() {
		t.Skip("golars-lsp not found")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "orders.csv"), []byte("region,amount\neu,1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := NewCompleter(func() string { return dir })
	t.Cleanup(func() { _ = c.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	req := complete.Request{Before: "load orders.csv\n", Src: "sort re", Row: 0, Col: 7, Manual: true, Lang: "glr"}
	res, err := c.Complete(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, it := range res.Items {
		found = found || it.Label == "region"
	}
	if !found || res.Source != "golars-lsp" {
		t.Fatalf("no region column in %+v", res)
	}
	diags, err := c.Diagnose(ctx, complete.Request{Before: "load orders.csv\n", Src: "show\nfrob x", Lang: "glr"})
	if err != nil {
		t.Fatal(err)
	}
	if len(diags) != 1 || diags[0].Row != 1 {
		t.Fatalf("diagnostics %+v", diags)
	}
	md, err := c.Hover(ctx, complete.Request{Src: "groupby region amount:sum", Row: 0, Col: 2})
	if err != nil || !strings.Contains(md, "groupby") {
		t.Fatalf("hover %q %v", md, err)
	}
}
