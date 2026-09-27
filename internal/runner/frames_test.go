package runner

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/Gaurav-Gosain/gopyter/internal/glr"
	"github.com/Gaurav-Gosain/gopyter/internal/kernel"
	"github.com/Gaurav-Gosain/gopyter/internal/notebook"
)

// Frames are shared by name between Go and glr cells. Needs golars (with
// the kernel-host frame ops) and a golars checkout in GOLARS_DIR.
func TestSharedFrames(t *testing.T) {
	if _, err := glr.FindGolars(); err != nil || testing.Short() {
		t.Skip("golars not found")
	}
	if os.Getenv("GOLARS_DIR") == "" {
		t.Skip("needs a golars checkout in GOLARS_DIR")
	}
	k, err := kernel.New("")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = k.Close() })
	k.RunDir = t.TempDir()
	copyData(t, "../../examples", k.RunDir)
	g := glr.NewKernel(k)
	g.Dir = k.RunDir
	t.Cleanup(func() { _ = g.Close() })

	goCell := func(id, src string) *notebook.Cell {
		return &notebook.Cell{ID: id, Type: notebook.Code, Source: src}
	}
	glrCell := func(id, src string) *notebook.Cell {
		c := goCell(id, src)
		c.Metadata = notebook.SetCellLang(c.Metadata, notebook.GLR, notebook.Go)
		return c
	}
	nb := notebook.New()
	nb.Cells = []*notebook.Cell{
		goCell("load", "import \"github.com/Gaurav-Gosain/golars\"\n\ndf, err := golars.ReadCSV(\"orders.csv\")\nif err != nil {\n\tpanic(err)\n}"),
		goCell("persist", "df.Height()"),
		glrCell("glr", "load customers.csv as customers\nuse df\nfilter qty > 2\nstash big"),
		goCell("use", "big.Height()"),
		goCell("focus", "glr.Width()"),
		goCell("clash", "customers := 5\n_ = customers"),
		goCell("clash2", "customers"),
		goCell("lazy", "lf := golars.Lazy(df)\n_ = lf"),
		glrCell("back", "use big\nselect order_id qty"),
	}
	var out bytes.Buffer
	if failed := Run(context.Background(), k, g, nb, &out, nil, false); failed != 0 {
		t.Fatalf("%d cells failed:\n%s", failed, out.String())
	}
	if strings.Contains(out.String(), "can't share frames") {
		t.Skip("this golars has no kernel-host frame ops")
	}
	// Status lines aren't saved in the notebook: read them from the
	// printed run, cell by cell.
	sections := strings.Split(out.String(), "In[")[1:]
	if len(sections) != len(nb.Cells) {
		t.Fatalf("%d cells printed:\n%s", len(sections), out.String())
	}
	text := func(i int) string { return sections[i] }
	for i, want := range []string{
		"frames shared with glr: df (14 x 7)",
		"14",
		"read from go: df (14 x 7); shared with go: big (6 x 7), customers (8 x 3)",
		"frames read from glr: big (6 x 7)",
		"frames read from glr: glr (6 x 7)",
		"",
		"glr frame customers is not visible in Go cells",
		"a golars LazyFrame is a query plan",
		"order_id",
	} {
		if !strings.Contains(text(i), want) {
			t.Errorf("cell %d: want %q in:\n%s", i, want, text(i))
		}
	}
	if strings.Contains(text(5), "frames") {
		t.Errorf("a cell that redeclares a frame name reads it:\n%s", text(5))
	}
	if strings.Contains(text(1), "frames") {
		t.Errorf("an unchanged frame was shared again:\n%s", text(1))
	}
}
