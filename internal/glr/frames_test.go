package glr

import (
	"slices"
	"testing"

	"github.com/Gaurav-Gosain/gopyter/internal/kernel"
)

func TestGLRWords(t *testing.T) {
	got := glrWords("load orders.csv as sales\njoin df on id\nfilter name == \"top\" and qty > 2\nsave out/x.parquet")
	want := []string{"load", "as", "sales", "join", "df", "on", "id", "filter", "name", "and", "qty", "save"}
	if !slices.Equal(got, want) {
		t.Fatalf("glrWords = %v, want %v", got, want)
	}
}

// Without a registry, or with a host that has no frame ops, cells run as
// before.
func TestFramesDisabled(t *testing.T) {
	k := newFakeKernel(t)
	if _, err := execute(t, k, "set a"); err != nil {
		t.Fatal(err)
	}
	k.Frames = kernel.NewFrames(t.TempDir())
	k.Frames.GoWrote("df", 1, 1)
	evs, err := execute(t, k, "use df")
	if err != nil {
		t.Fatal(err)
	}
	var notes int
	for _, e := range evs {
		if e.Kind == kernel.Info {
			notes++
			if e.Text != errOldGolars.Error() {
				t.Fatalf("note %q", e.Text)
			}
		}
	}
	if notes != 1 || !k.noFrames {
		t.Fatalf("events %+v", evs)
	}
	// Said once: later cells skip sharing.
	evs, err = execute(t, k, "get")
	if err != nil || len(evs) != 1 || evs[0].Text != "state=a\n" {
		t.Fatalf("events %+v, %v", evs, err)
	}
}
