package runner

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Gaurav-Gosain/gopyter/internal/glr"
	"github.com/Gaurav-Gosain/gopyter/internal/kernel"
	"github.com/Gaurav-Gosain/gopyter/internal/notebook"
)

// copyData copies the CSV files next to the examples into dir.
func copyData(t *testing.T, from, dir string) {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(from, "*.csv"))
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, filepath.Base(p)), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// needs reports what an example needs besides Go: golars (for glr
// cells) and a golars checkout (GOLARS_DIR, for Go cells importing it).
func needs(nb *notebook.Notebook) (golars, golarsModule bool) {
	for _, c := range nb.Cells {
		if c.Type != notebook.Code {
			continue
		}
		if l, _ := notebook.Resolve(c.Source, notebook.CellLang(c.Metadata, nb.Lang())); l == notebook.GLR {
			golars = true
		}
		golarsModule = golarsModule || strings.Contains(c.Source, kernel.GolarsPath)
	}
	return golars, golarsModule
}

// TestExamples runs every example notebook headlessly: they must run
// without failures or notes about lost variables. They only use the
// standard library and gopyter's runtime, so no network is needed. The
// golars examples also need golars (found like gopyter finds it) and,
// for Go cells importing golars, a checkout in GOLARS_DIR; without them
// they are skipped.
func TestExamples(t *testing.T) {
	if testing.Short() {
		t.Skip("builds every cell of the examples")
	}
	paths, err := filepath.Glob("../../examples/*.ipynb")
	if err != nil || len(paths) == 0 {
		t.Fatalf("no examples: %v", err)
	}
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			t.Parallel()
			nb, err := notebook.Load(path)
			if err != nil {
				t.Fatal(err)
			}
			wantGolars, wantModule := needs(nb)
			var g *glr.Kernel
			if wantGolars {
				if _, err := glr.FindGolars(); err != nil {
					t.Skipf("needs golars: %v", err)
				}
			}
			if wantModule && os.Getenv("GOLARS_DIR") == "" {
				t.Skip("needs a golars checkout in GOLARS_DIR")
			}
			k, err := kernel.New("")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := k.Close(); err != nil {
					t.Error(err)
				}
			})
			// Files the notebooks write go to a temporary directory, with
			// the data files the examples read.
			k.RunDir = t.TempDir()
			copyData(t, filepath.Dir(path), k.RunDir)
			if wantGolars {
				g = glr.NewKernel(k)
				g.Dir = k.RunDir
				t.Cleanup(func() { _ = g.Close() })
			}
			var out bytes.Buffer
			// Guesses for the terminal notebook's game, then a password.
			stdin := strings.NewReader("50\n25\n75\nsecret\n")
			if failed := Run(context.Background(), k, g, nb, &out, stdin, false); failed != 0 {
				t.Fatalf("%d cells failed:\n%s", failed, out.String())
			}
			if strings.Contains(out.String(), "not kept") {
				t.Fatalf("variables not kept:\n%s", out.String())
			}
		})
	}
}
