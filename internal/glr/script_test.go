package glr

import (
	"testing"

	"github.com/Gaurav-Gosain/gopyter/internal/notebook"
)

func TestImportScript(t *testing.T) {
	src := "# Orders\n# by region\n\nload orders.csv\nfilter a > 1 # keep\n\n# Group them\ngroupby region a:sum\n# ^?\nshow\n\n\n# ^?\n"
	nb := ImportScript(src)
	if nb.Lang() != notebook.GLR {
		t.Fatal("not a glr notebook")
	}
	want := []struct {
		kind notebook.CellType
		src  string
	}{
		{notebook.Markdown, "Orders\nby region"},
		{notebook.Code, "load orders.csv\nfilter a > 1 # keep"},
		{notebook.Markdown, "Group them"},
		{notebook.Code, "groupby region a:sum\n# ^?\nshow"},
	}
	if len(nb.Cells) != len(want) {
		for _, c := range nb.Cells {
			t.Logf("%s %q", c.Type, c.Source)
		}
		t.Fatalf("got %d cells", len(nb.Cells))
	}
	for i, w := range want {
		if c := nb.Cells[i]; c.Type != w.kind || c.Source != w.src {
			t.Errorf("cell %d: %s %q, want %s %q", i, c.Type, c.Source, w.kind, w.src)
		}
	}
	if empty := ImportScript("\n\n"); len(empty.Cells) != 1 {
		t.Fatalf("empty script: %d cells", len(empty.Cells))
	}
}
