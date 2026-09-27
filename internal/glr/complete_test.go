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

func TestBasicCompletion(t *testing.T) {
	res := basic(complete.Request{Src: "load x.csv\ngro", Row: 1, Col: 3})
	if res.Replace != 3 || len(res.Items) == 0 || res.Items[0].Label != "groupby" || res.Items[0].Detail == "" {
		t.Fatalf("%+v", res)
	}
	res = basic(complete.Request{Src: "join b o", Row: 0, Col: 8})
	if len(res.Items) != 2 || res.Items[0].Label != "on" || res.Items[1].Label != "or" {
		t.Fatalf("keywords %+v", res)
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
