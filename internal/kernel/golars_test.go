package kernel

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Gaurav-Gosain/gopyter/internal/table"
)

func collect(t *testing.T, k *Kernel, id, src string) ([]Event, error) {
	t.Helper()
	var evs []Event
	err := k.Execute(context.Background(), id, "In["+id+"]", src, func(e Event) { evs = append(evs, e) })
	return evs, err
}

// A value with a MimeBundle method (as golars DataFrames have) is shown
// in its richest form: a golars table, else HTML.
func TestDisplayMimeBundle(t *testing.T) {
	k := newTestKernel(t)
	src := "type frame struct{}\n\n" +
		"func (frame) MimeBundle() map[string]string {\n" +
		"\treturn map[string]string{\"text/plain\": \"plain\", \"text/html\": \"<b>x</b>\",\n" +
		"\t\t\"" + table.MIME + "\": `{\"columns\":[\"a\"],\"dtypes\":[\"i64\"],\"rows\":[[\"1\"]],\"shape\":[1,1]}`}\n}\n\n" +
		"type page struct{}\n\nfunc (page) HTML() string { return \"<i>p</i>\" }\n\n" +
		"nb.Display(page{})\nframe{}"
	evs, err := collect(t, k, "a", src)
	if err != nil {
		t.Fatalf("%v %+v", err, evs)
	}
	if len(evs) != 2 || evs[0].Kind != HTML || evs[0].Text != "<i>p</i>" || evs[1].Kind != Table {
		t.Fatalf("events %+v", evs)
	}
	out, err := table.Decode(evs[1].Text)
	if err != nil || out.HTML != "<b>x</b>" || out.Plain != "plain" || out.Table.Columns[0] != "a" {
		t.Fatalf("table %+v %v", out, err)
	}
}

func TestBundleEvent(t *testing.T) {
	cases := []struct {
		bundle map[string]string
		kind   EventKind
	}{
		{map[string]string{"text/markdown": "*x*", "text/plain": "x"}, Markdown},
		{map[string]string{"image/png": "AAAA"}, Image},
		{map[string]string{"text/plain": "x"}, Result},
		{map[string]string{table.MIME: "not json", "text/html": "<b>"}, HTML},
	}
	for _, c := range cases {
		if e := BundleEvent(c.bundle, "id"); e.Kind != c.kind || e.ID != "id" {
			t.Errorf("%v: got %+v", c.bundle, e)
		}
	}
}

func TestBridgePath(t *testing.T) {
	k := newTestKernel(t)
	out, err := run(t, k, "a", `fmt.Println(nb.BridgePath("sales"))`)
	if err != nil {
		t.Fatal(out, err)
	}
	if want := filepath.Join(k.BridgeDir(), "sales.arrow"); strings.TrimSpace(out) != want {
		t.Fatalf("got %q, want %q", out, want)
	}
	if st, err := os.Stat(k.BridgeDir()); err != nil || !st.IsDir() {
		t.Fatalf("bridge dir: %v", err)
	}
}

// With GOLARS_DIR pointing at a golars checkout, cells import golars from
// it and DataFrames display as tables.
func TestGolarsDir(t *testing.T) {
	dir := os.Getenv("GOLARS_DIR")
	if dir == "" || testing.Short() {
		t.Skip("set GOLARS_DIR to a golars checkout to run")
	}
	k := newTestKernel(t)
	src := `import "github.com/Gaurav-Gosain/golars"

df, err := golars.FromMap(map[string]any{"a": []int64{1, 2}}, []string{"a"})
if err != nil {
	panic(err)
}
df`
	evs, err := collect(t, k, "a", src)
	if err != nil {
		t.Fatalf("%v %+v", err, evs)
	}
	for _, e := range evs {
		if e.Kind == Table {
			return
		}
	}
	t.Fatalf("no table in %+v", evs)
}
