package kernel

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// The runtime packages cell programs import: gopyter's own API (nb) and
// the GoNB compatibility layer, adapted from GoNB's gonbui packages
// (https://github.com/janpfeifer/gonb, MIT license: runtime/gonb/LICENSE). They are part of this repository (so they
// are built, vetted and tested with it) and written into the kernel
// workspace as two local modules, which its go.mod uses through replace
// directives. That keeps them offline and in sync with gopyter.
//
//go:embed runtime
var runtimeFS embed.FS

// RuntimeDirName is the workspace directory holding the runtime modules.
const RuntimeDirName = "gopyter_runtime"

// zeroVersion is the version required for the replaced runtime modules.
const zeroVersion = "v0.0.0-00010101000000-000000000000"

// runtimeModule is a module written from runtime/<dir>.
type runtimeModule struct {
	dir, path, gomod string
}

var runtimeModules = []runtimeModule{
	{"gopyter", "github.com/Gaurav-Gosain/gopyter", "module github.com/Gaurav-Gosain/gopyter\n\ngo 1.23\n"},
	{"gonb", "github.com/janpfeifer/gonb", "module github.com/janpfeifer/gonb\n\ngo 1.23\n\nrequire github.com/Gaurav-Gosain/gopyter " + zeroVersion + "\n"},
}

// repoRuntime is the import path of the runtime packages in this
// repository; in the workspace, runtime/<dir>/ is module <path>.
const repoRuntime = "github.com/Gaurav-Gosain/gopyter/internal/kernel/runtime/"

// Paths of the runtime packages, as cells import them.
const (
	NBPath      = "github.com/Gaurav-Gosain/gopyter/nb"
	GonbuiPath  = "github.com/janpfeifer/gonb/gonbui"
	WidgetsPath = GonbuiPath + "/widgets"
)

// runtimePackages maps the package names cells use to their paths, so
// that e.g. nb.Display works without an import.
var runtimePackages = map[string]string{
	"nb":       NBPath,
	"gonbui":   GonbuiPath,
	"widgets":  WidgetsPath,
	"comms":    GonbuiPath + "/comms",
	"dom":      GonbuiPath + "/dom",
	"protocol": GonbuiPath + "/protocol",
}

// rewriteImports maps the repository's import paths of the runtime
// packages to the ones they have in the workspace.
func rewriteImports(src string) string {
	for _, m := range runtimeModules {
		src = strings.ReplaceAll(src, `"`+repoRuntime+m.dir+"/", `"`+m.path+"/")
	}
	return src
}

// setupRuntime writes the runtime modules into the workspace and points
// its go.mod at them.
func (k *Kernel) setupRuntime(ctx context.Context) error {
	root := filepath.Join(k.Dir, RuntimeDirName)
	if err := os.RemoveAll(root); err != nil {
		return err
	}
	err := fs.WalkDir(runtimeFS, "runtime", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || strings.HasSuffix(path, "_test.go") {
			return err
		}
		b, err := runtimeFS.ReadFile(path)
		if err != nil {
			return err
		}
		dst := filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(path, "runtime/")))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		return os.WriteFile(dst, []byte(rewriteImports(string(b))), 0o644)
	})
	if err != nil {
		return err
	}
	args := []string{"mod", "edit"}
	for _, m := range runtimeModules {
		if err := os.WriteFile(filepath.Join(root, m.dir, "go.mod"), []byte(m.gomod), 0o644); err != nil {
			return err
		}
		args = append(args,
			"-require="+m.path+"@"+zeroVersion,
			"-replace="+m.path+"=./"+RuntimeDirName+"/"+m.dir)
	}
	if out, err := k.goCmd(ctx, args...).CombinedOutput(); err != nil {
		return fmt.Errorf("go mod edit: %v: %s", err, out)
	}
	if dir := os.Getenv("GOLARS_DIR"); dir != "" {
		return k.linkGolars(ctx, dir)
	}
	return nil
}

// GolarsPath is the module path of golars.
const GolarsPath = "github.com/Gaurav-Gosain/golars"

// golarsHint says what to set when golars (pkg is a package path) can't
// be fetched and GOLARS_DIR isn't set.
func golarsHint(pkg string) string {
	if os.Getenv("GOLARS_DIR") != "" || !strings.HasPrefix(pkg, GolarsPath) {
		return ""
	}
	return "\n\ngolars could not be fetched. Build against a local checkout instead: set GOLARS_DIR to it and restart gopyter:\n" +
		"  git clone https://github.com/Gaurav-Gosain/golars ~/src/golars\n" +
		"  GOLARS_DIR=~/src/golars gopyter notebook.ipynb"
}

// linkGolars points the workspace at a local golars checkout (GOLARS_DIR),
// so cells can import golars without fetching it: a replace directive,
// plus golars' go.sum, whose checksums let its dependencies resolve from
// the module cache without the network. The module is only required once
// a cell imports it (see missingModules).
func (k *Kernel) linkGolars(ctx context.Context, dir string) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	mod, err := os.ReadFile(filepath.Join(abs, "go.mod"))
	if err != nil {
		return fmt.Errorf("GOLARS_DIR: %w", err)
	}
	if !strings.Contains(string(mod), "module "+GolarsPath+"\n") {
		return fmt.Errorf("GOLARS_DIR: %s is not the %s module", abs, GolarsPath)
	}
	if out, err := k.goCmd(ctx, "mod", "edit", "-replace="+GolarsPath+"="+abs).CombinedOutput(); err != nil {
		return fmt.Errorf("go mod edit: %v: %s", err, out)
	}
	sums, err := os.ReadFile(filepath.Join(abs, "go.sum"))
	if err != nil {
		return nil // no dependencies to check
	}
	path := filepath.Join(k.Dir, "go.sum")
	have, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	seen := map[string]bool{}
	for l := range strings.SplitSeq(string(have), "\n") {
		seen[l] = true
	}
	var b strings.Builder
	b.Write(have)
	for l := range strings.SplitSeq(string(sums), "\n") {
		if l != "" && !seen[l] {
			seen[l] = true
			b.WriteString(l)
			b.WriteByte('\n')
		}
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}
