package kernel

import (
	"context"
	"encoding/hex"
	"fmt"
	"go/token"
	"go/types"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"sync"
)

// Shared frames: golars DataFrames that Go and glr cells see by the same
// name. Frames records, for every shared name, which version each side
// holds. A side that changes a frame publishes a new version; the other
// side reads it lazily, when one of its cells mentions the name, from an
// Arrow IPC file in Dir (FramePath). So a frame nobody reads is written
// once and never read, and an unchanged frame is never written again.
//
//	Go cell                              glr cell
//	df := golars.ReadCSV(...)            (mentions df)
//	  saved to frames/<df>.arrow  ---->    op=import df before the cell
//	(mentions sales)                     stash sales
//	  op=export sales before the  <----    listed by op=frames after
//	  cell, declared as a Go var           the cell
//
// Names are one namespace, like variables: the last cell that changes a
// frame wins, and since a cell reads the latest version of every frame it
// mentions first, it always builds on what the other side did. Each cell
// ends with a line saying which frames it shared.

// FrameType is the Go type of a shared frame, qualified by package path.
const FrameType = "*" + DataFramePkg + ".DataFrame"

// DataFramePkg is golars' dataframe package.
const DataFramePkg = "github.com/Gaurav-Gosain/golars/dataframe"

// FocusName is the Go name of glr's focused frame.
const FocusName = "glr"

// FrameExporter writes a glr frame ("" for the focus) to an Arrow IPC
// file and returns its shape. glr.Kernel implements it.
type FrameExporter interface {
	ExportFrame(ctx context.Context, name, path string) (rows, cols int, err error)
}

// Frames is the registry of shared frames. Its methods are safe for
// concurrent use.
type Frames struct {
	// Dir holds the Arrow IPC files.
	Dir string

	mu   sync.Mutex
	glr  FrameExporter
	ver  int
	m    map[string]*sharedFrame
	note map[string]string // one-time notes already shown, by key
}

type sharedFrame struct {
	ver         int // latest version
	goAt, glrAt int // version each side holds, 0 for none
	glrGen      int // the host's generation of the frame glr holds
	rows, cols  int
	focus       bool // the glr focus, exported as FocusName
}

// NewFrames returns a registry whose files live in dir.
func NewFrames(dir string) *Frames {
	return &Frames{Dir: dir, m: map[string]*sharedFrame{}, note: map[string]string{}}
}

// SetGLR sets where glr frames come from.
func (f *Frames) SetGLR(e FrameExporter) {
	f.mu.Lock()
	f.glr = e
	f.mu.Unlock()
}

// FramePath is the file of a shared frame. Hex keeps names distinct on
// case-insensitive file systems and the path free of spaces.
func (f *Frames) FramePath(name string) string {
	return filepath.Join(f.Dir, hex.EncodeToString([]byte(name))+".arrow")
}

func (f *Frames) entry(name string) *sharedFrame {
	e := f.m[name]
	if e == nil {
		e = &sharedFrame{}
		f.m[name] = e
	}
	return e
}

// FrameInfo describes a shared frame.
type FrameInfo struct {
	Name       string
	Rows, Cols int // Rows is -1 for a lazy glr focus
	// InGo and InGLR report whether that side holds the latest version.
	InGo, InGLR bool
	// Focus marks glr's focused frame.
	Focus bool
}

// List returns the shared frames by name.
func (f *Frames) List() []FrameInfo {
	if f == nil {
		return nil
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]FrameInfo, 0, len(f.m))
	for _, n := range slices.Sorted(maps.Keys(f.m)) {
		e := f.m[n]
		out = append(out, FrameInfo{Name: n, Rows: e.rows, Cols: e.cols, InGo: e.goAt == e.ver, InGLR: e.glrAt == e.ver, Focus: e.focus})
	}
	return out
}

func shape(rows, cols int) string {
	if rows < 0 {
		return fmt.Sprintf("lazy, %d cols", cols)
	}
	return fmt.Sprintf("%d x %d", rows, cols)
}

// GoWrote records that a Go cell saved a new version of name.
func (f *Frames) GoWrote(name string, rows, cols int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	e := f.entry(name)
	f.ver++
	e.ver, e.goAt, e.rows, e.cols, e.focus = f.ver, f.ver, rows, cols, false
}

// GoDropped records that Go no longer has a frame called name.
func (f *Frames) GoDropped(name string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if e := f.m[name]; e != nil {
		e.goAt = 0
		if e.glrAt == 0 {
			delete(f.m, name)
		}
	}
}

// GLRSeen records a named glr frame (or the focus, with focus set) listed
// by the host after a cell, with its generation. It reports whether glr
// changed it since it was last seen. The focus never takes the place of
// a frame of that name from Go.
func (f *Frames) GLRSeen(name string, gen, rows, cols int, focus bool) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if e := f.m[name]; focus && e != nil && !e.focus && e.goAt != 0 {
		return false
	}
	e := f.entry(name)
	if e.glrAt != 0 && e.glrGen == gen {
		return false
	}
	f.ver++
	e.ver, e.glrAt, e.glrGen, e.rows, e.cols, e.focus = f.ver, f.ver, gen, rows, cols, focus
	return true
}

// IsFocus reports whether name is glr's focus.
func (f *Frames) IsFocus(name string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	e := f.m[name]
	return e != nil && e.focus
}

// GLRImported records that glr now holds the latest version of name,
// staged by op=import with generation gen.
func (f *Frames) GLRImported(name string, gen int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if e := f.m[name]; e != nil {
		e.glrAt, e.glrGen = e.ver, gen
	}
}

// GLRKeep drops glr's claim on every frame not in names (the frames the
// host listed), as after drop_frame or a restart.
func (f *Frames) GLRKeep(names map[string]bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for n, e := range f.m {
		if e.glrAt != 0 && !names[n] {
			e.glrAt, e.glrGen = 0, 0
			if e.goAt == 0 {
				delete(f.m, n)
			}
		}
	}
}

// ForGLR returns the frames among words (a glr cell's words) whose latest
// version glr doesn't have yet, with their files.
func (f *Frames) ForGLR(words []string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, w := range words {
		if e := f.m[w]; e != nil && e.goAt == e.ver && e.glrAt != e.ver && !slices.Contains(out, w) {
			out = append(out, w)
		}
	}
	return out
}

// ForGo returns the frames among words whose latest version Go doesn't
// have yet.
func (f *Frames) ForGo(words []string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, w := range words {
		if e := f.m[w]; e != nil && e.glrAt == e.ver && e.goAt != e.ver && !slices.Contains(out, w) {
			out = append(out, w)
		}
	}
	return out
}

// PendingForGo returns the frames glr holds that Go hasn't read, for
// completion.
func (f *Frames) PendingForGo() []string {
	if f == nil {
		return nil
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for n, e := range f.m {
		if e.glrAt == e.ver && e.goAt != e.ver {
			out = append(out, n)
		}
	}
	slices.Sort(out)
	return out
}

// exportForGo writes glr's latest version of name for Go and records that
// Go has it.
func (f *Frames) exportForGo(ctx context.Context, name string) error {
	f.mu.Lock()
	e, glr := f.m[name], f.glr
	var focus bool
	if e != nil {
		focus = e.focus
	}
	f.mu.Unlock()
	if e == nil || glr == nil {
		return fmt.Errorf("no glr frame %s", name)
	}
	src := name
	if focus {
		src = ""
	}
	rows, cols, err := glr.ExportFrame(ctx, src, f.FramePath(name))
	if err != nil {
		return err
	}
	f.mu.Lock()
	e.goAt, e.rows, e.cols = e.ver, rows, cols
	f.mu.Unlock()
	return nil
}

// Shape returns the size of a shared frame for status lines.
func (f *Frames) Shape(name string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if e := f.m[name]; e != nil {
		return shape(e.rows, e.cols)
	}
	return "?"
}

// Once reports whether the note with this key hasn't been shown yet, and
// marks it shown.
func (f *Frames) Once(key, text string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.note[key] == text {
		return false
	}
	f.note[key] = text
	return true
}

// ResetGo forgets Go's frames, after the Go kernel's state was cleared.
func (f *Frames) ResetGo() {
	f.mu.Lock()
	defer f.mu.Unlock()
	for n, e := range f.m {
		e.goAt = 0
		if e.glrAt == 0 {
			delete(f.m, n)
		}
	}
}

// Status formats the line shown after a cell: the frames it shared with
// the other language and those it read from it.
func (f *Frames) Status(other string, shared, read []string) string {
	var parts []string
	list := func(names []string) string {
		var b strings.Builder
		for i, n := range names {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(n)
			b.WriteString(" (")
			b.WriteString(f.Shape(n))
			b.WriteString(")")
		}
		return b.String()
	}
	if len(read) > 0 {
		parts = append(parts, "read from "+other+": "+list(read))
	}
	if len(shared) > 0 {
		parts = append(parts, "shared with "+other+": "+list(shared))
	}
	if len(parts) == 0 {
		return ""
	}
	return "frames " + strings.Join(parts, "; ")
}

// Words returns the identifier-like words of src, in order, without
// duplicates. Words in strings and comments count too: reading a frame
// that isn't needed costs little, missing one would show a stale frame.
func Words(src string) []string {
	var out []string
	seen := map[string]bool{}
	start := -1
	flush := func(end int) {
		if start >= 0 {
			w := src[start:end]
			if !seen[w] && (w[0] < '0' || w[0] > '9') {
				seen[w] = true
				out = append(out, w)
			}
			start = -1
		}
	}
	for i := 0; i < len(src); i++ {
		c := src[i]
		if c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' {
			if start < 0 {
				start = i
			}
			continue
		}
		flush(i)
	}
	flush(len(src))
	return out
}

// GoName reports whether name can be a Go variable holding a shared
// frame: an ASCII identifier that is neither a keyword nor predeclared.
func GoName(name string) bool {
	if !token.IsIdentifier(name) || types.Universe.Lookup(name) != nil || name == "_" {
		return false
	}
	for _, r := range name {
		if r > 127 {
			return false
		}
	}
	return true
}
