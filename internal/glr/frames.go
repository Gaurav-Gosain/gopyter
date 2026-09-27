package glr

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/Gaurav-Gosain/gopyter/internal/kernel"
)

// The glr side of shared frames (kernel/frames.go). Before a cell, the Go
// frames it mentions that glr hasn't seen are staged as named frames
// (op=import). After it, the host lists its frames (op=frames) and those
// the cell changed become visible to Go cells, with the focus as the Go
// variable kernel.FocusName. Go cells read them through ExportFrame.

// NewKernel returns a glr kernel that shares frames with the Go kernel
// gk: its bridge directory for %export and %import, and its frames.
func NewKernel(gk *kernel.Kernel) *Kernel {
	k := &Kernel{BridgeDir: gk.BridgeDir(), Frames: gk.Frames}
	if gk.Frames != nil {
		gk.Frames.SetGLR(k)
	}
	return k
}

// glrWords returns the words of src that are frame names: paths
// (orders.csv), numbers and strings are not.
func glrWords(src string) []string {
	var out []string
	seen := map[string]bool{}
	isPart := func(r rune) bool {
		return r == '_' || r == '.' || r == '/' || r == '\\' || r == '-' || r == ':' || r == '"' || r == '\'' ||
			r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9'
	}
	for w := range strings.FieldsFuncSeq(src, func(r rune) bool { return !isPart(r) }) {
		if validName(w) && !seen[w] {
			seen[w] = true
			out = append(out, w)
		}
	}
	return out
}

// errOldGolars marks a host without the frame ops.
var errOldGolars = errors.New("this golars can't share frames with Go cells: update golars (or build it from source) to share them automatically; %export and %import still work")

// frameOp runs a frame request on the host.
func (k *Kernel) frameOp(ctx context.Context, req Request) (*Reply, error) {
	if k.host == nil {
		return nil, errHostExited
	}
	r, err := k.host.do(ctx, req)
	if err != nil {
		// The host is gone, with its frames.
		k.host, k.lost = nil, true
		return nil, err
	}
	if r.Error != "" {
		if strings.Contains(r.Error, "unknown op") {
			k.noFrames = true
			return nil, errOldGolars
		}
		return nil, errors.New(r.Error)
	}
	return r, nil
}

// pushFrames stages the Go frames code mentions that glr doesn't have
// yet. It returns their names.
func (k *Kernel) pushFrames(ctx context.Context, code string, emit func(kernel.Event)) []string {
	if k.Frames == nil || k.noFrames {
		return nil
	}
	var read []string
	for _, name := range k.Frames.ForGLR(glrWords(code)) {
		path := k.Frames.FramePath(name)
		if strings.ContainsAny(path, " \t") {
			emit(kernel.Event{Kind: kernel.Info, Text: fmt.Sprintf("Go frame %s is not visible to glr: its path %q has spaces", name, path)})
			continue
		}
		r, err := k.frameOp(ctx, Request{Op: "import", Name: name, Path: path})
		if err != nil {
			if k.noFrames {
				emit(kernel.Event{Kind: kernel.Info, Text: err.Error()})
				return nil
			}
			emit(kernel.Event{Kind: kernel.Info, Text: fmt.Sprintf("Go frame %s not read: %v", name, err)})
			if k.host == nil {
				return read
			}
			continue
		}
		if r.Gen == 0 {
			// A host that ignored the op ran it as an empty cell.
			k.noFrames = true
			emit(kernel.Event{Kind: kernel.Info, Text: errOldGolars.Error()})
			return nil
		}
		k.Frames.GLRImported(name, r.Gen)
		read = append(read, name)
	}
	return read
}

// syncFrames lists the host's frames after a cell and returns the named
// frames the cell changed.
func (k *Kernel) syncFrames(ctx context.Context, emit func(kernel.Event)) []string {
	if k.Frames == nil || k.noFrames || k.host == nil {
		return nil
	}
	r, err := k.frameOp(ctx, Request{Op: "frames"})
	if err != nil {
		if k.noFrames {
			emit(kernel.Event{Kind: kernel.Info, Text: err.Error()})
		}
		return nil
	}
	names := map[string]bool{}
	var shared []string
	var focus *HostFrame
	for i, f := range r.Frames {
		if f.Focus {
			focus = &r.Frames[i]
			continue
		}
		names[f.Name] = true
		if k.Frames.GLRSeen(f.Name, f.Gen, f.Rows, f.Cols, false) {
			shared = append(shared, f.Name)
		}
	}
	if focus != nil && !names[kernel.FocusName] {
		if k.Frames.GLRSeen(kernel.FocusName, focus.Gen, focus.Rows, focus.Cols, true) || k.Frames.IsFocus(kernel.FocusName) {
			names[kernel.FocusName] = true
		}
	}
	k.Frames.GLRKeep(names)
	return shared
}

// ExportFrame writes the glr frame name ("" for the focus) to path as
// Arrow IPC, for a Go cell. It implements kernel.FrameExporter.
func (k *Kernel) ExportFrame(ctx context.Context, name, path string) (rows, cols int, err error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.host == nil {
		return 0, 0, errors.New("golars isn't running (it was restarted): run the glr cells that make it again")
	}
	tmp := path + ".tmp"
	r, err := k.frameOp(ctx, Request{Op: "export", Name: name, Path: tmp})
	if err != nil {
		return 0, 0, err
	}
	if r.Shape != nil {
		rows, cols = r.Shape[0], r.Shape[1]
	}
	return rows, cols, os.Rename(tmp, path)
}
