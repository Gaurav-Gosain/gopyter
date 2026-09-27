package kernel

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

type fakeExporter struct{ calls []string }

func (e *fakeExporter) ExportFrame(_ context.Context, name, path string) (int, int, error) {
	e.calls = append(e.calls, name)
	return 3, 2, os.WriteFile(path, []byte("arrow"), 0o644)
}

func TestFramesVersions(t *testing.T) {
	f := NewFrames(t.TempDir())
	exp := &fakeExporter{}
	f.SetGLR(exp)

	// Go writes df: glr should read it when a cell mentions it, once.
	f.GoWrote("df", 10, 3)
	if got := f.ForGLR([]string{"use", "df", "df"}); !slices.Equal(got, []string{"df"}) {
		t.Fatalf("ForGLR = %v", got)
	}
	if got := f.ForGo([]string{"df"}); got != nil {
		t.Fatalf("ForGo = %v, Go has the latest", got)
	}
	f.GLRImported("df", 7)
	if got := f.ForGLR([]string{"df"}); got != nil {
		t.Fatalf("ForGLR after import = %v", got)
	}
	// The host lists df with the generation it got from the import: no change.
	if f.GLRSeen("df", 7, 10, 3, false) {
		t.Fatal("imported frame reported as changed")
	}
	// glr changes df: Go reads the new version when it mentions it.
	if !f.GLRSeen("df", 8, 5, 3, false) {
		t.Fatal("changed frame not reported")
	}
	if got := f.ForGo([]string{"x", "df"}); !slices.Equal(got, []string{"df"}) {
		t.Fatalf("ForGo = %v", got)
	}
	if err := f.exportForGo(context.Background(), "df"); err != nil {
		t.Fatal(err)
	}
	if got := f.ForGo([]string{"df"}); got != nil {
		t.Fatalf("ForGo after export = %v", got)
	}
	if _, err := os.Stat(f.FramePath("df")); err != nil {
		t.Fatal(err)
	}
	if f.Shape("df") != "3 x 2" {
		t.Fatalf("shape %s", f.Shape("df"))
	}

	// The focus is exported as "" and never replaces a Go frame of that name.
	if !f.GLRSeen(FocusName, 1, -1, 4, true) || !f.IsFocus(FocusName) {
		t.Fatal("focus not recorded")
	}
	if err := f.exportForGo(context.Background(), FocusName); err != nil {
		t.Fatal(err)
	}
	if exp.calls[len(exp.calls)-1] != "" {
		t.Fatalf("focus exported as %q", exp.calls[len(exp.calls)-1])
	}
	f.GoWrote(FocusName, 1, 1)
	if f.GLRSeen(FocusName, 2, 1, 1, true) || f.IsFocus(FocusName) {
		t.Fatal("focus replaced a Go frame")
	}

	// Frames glr no longer lists are forgotten unless Go has them.
	f.GLRSeen("tmp", 3, 1, 1, false)
	f.GLRKeep(map[string]bool{"df": true})
	names := func() []string {
		var out []string
		for _, i := range f.List() {
			out = append(out, i.Name)
		}
		return out
	}
	if got := names(); !slices.Equal(got, []string{"df", FocusName}) {
		t.Fatalf("frames %v", got)
	}
	f.GoDropped(FocusName)
	f.ResetGo()
	if got := names(); !slices.Equal(got, []string{"df"}) {
		t.Fatalf("after reset %v", got)
	}

	if s := f.Status("glr", []string{"df"}, nil); s != "frames shared with glr: df (3 x 2)" {
		t.Fatalf("status %q", s)
	}
	if s := f.Status("glr", nil, nil); s != "" {
		t.Fatalf("empty status %q", s)
	}
	if !f.Once("k", "a") || f.Once("k", "a") || !f.Once("k", "b") {
		t.Fatal("Once")
	}
}

func TestFramesNil(t *testing.T) {
	var f *Frames
	if f.List() != nil || f.PendingForGo() != nil {
		t.Fatal("nil registry lists frames")
	}
}

func TestWordsAndGoName(t *testing.T) {
	got := Words(`df.Head(3) // top 2x
x := "df" + y_1`)
	if !slices.Equal(got, []string{"df", "Head", "top", "x", "y_1"}) {
		t.Fatalf("Words = %v", got)
	}
	for name, ok := range map[string]bool{"df": true, "_x": true, "type": false, "len": false, "_": false, "a-b": false, "é": false, "1a": false} {
		if GoName(name) != ok {
			t.Errorf("GoName(%q) = %v", name, !ok)
		}
	}
}

func TestFramesUsedAndFile(t *testing.T) {
	decls := []*Decl{
		frameDecl("df"),
		frameDecl("other"),
		{Names: []string{"f"}, Src: "func f() int { return other.Height() }"},
	}
	used := framesUsed("df.Head(1)", decls)
	if !used["df"] || !used["other"] || len(used) != 2 {
		t.Fatalf("used %v", used)
	}
	if used := framesUsed("x := 1", decls[:2]); len(used) != 0 {
		t.Fatalf("used %v", used)
	}
	dir := t.TempDir()
	if err := writeVars(dir, decls, used); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, framesFile))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `var gopyterFramesUsed = map[string]bool{"df": true, "other": true}`) {
		t.Fatalf("frames file:\n%s", b)
	}
	v, err := os.ReadFile(filepath.Join(dir, varsFile))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(v), `gopyterSaveFrame("df", df)`) {
		t.Fatalf("vars file:\n%s", v)
	}
	// Without frames the file goes away, since golars may not be there.
	if err := writeVars(dir, nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, framesFile)); !os.IsNotExist(err) {
		t.Fatalf("frames file kept: %v", err)
	}
}
