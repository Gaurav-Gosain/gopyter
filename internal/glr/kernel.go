// Package glr runs golars scripts (.glr, https://github.com/Gaurav-Gosain/golars)
// in notebook cells. A long-lived `golars kernel-host` process keeps the
// frames between cells, as with golars-kernel in Jupyter; cells are sent
// to it over its NDJSON protocol and its replies become kernel events:
// text, errors and tables. The package also holds glr's syntax
// highlighting, its completion (golars-lsp, or a built-in command list)
// and the import of .glr files as notebooks.
package glr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Gaurav-Gosain/gopyter/internal/kernel"
	"github.com/Gaurav-Gosain/gopyter/internal/table"
)

// Kernel runs glr cells. The zero value is ready to use; the host starts
// with the first cell.
type Kernel struct {
	// Bin is the golars binary; empty means FindGolars.
	Bin string
	// Dir is the host's working directory, which relative paths in
	// cells are resolved against (the notebook's directory).
	Dir string
	// BridgeDir holds the Arrow IPC files shared with Go cells (see
	// kernel.Kernel.BridgeDir). Empty disables %export and %import.
	BridgeDir string

	mu   sync.Mutex // serializes cells
	host *host
	// lost is set when a host with state was stopped, so the next cell
	// can say that earlier frames are gone.
	lost bool
}

// ErrNoGolars is wrapped by errors about a missing golars binary.
var ErrNoGolars = errors.New("golars not found")

// Execute runs a glr cell. name labels positions in errors ("In[3]").
// Output goes to emit, in order; emit is never called concurrently.
// A failing cell returns kernel.ErrFailed after emitting the error;
// cancelling ctx restarts the host and returns kernel.ErrInterrupted.
func (k *Kernel) Execute(ctx context.Context, cellID, name, src string, emit func(kernel.Event)) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	code, err := k.expand(src)
	if err != nil {
		emit(kernel.Event{Kind: kernel.Error, Text: err.Error()})
		return kernel.ErrFailed
	}
	if strings.TrimSpace(code) == "" {
		return nil
	}
	if k.host == nil {
		if err := k.start(); err != nil {
			return err
		}
		if k.lost {
			k.lost = false
			emit(kernel.Event{Kind: kernel.Info, Text: "golars restarted: frames from earlier cells are gone"})
		}
	}
	reply, err := k.host.run(ctx, code)
	if err != nil {
		k.host = nil
		k.lost = true
		if ctx.Err() != nil {
			return kernel.ErrInterrupted
		}
		emit(kernel.Event{Kind: kernel.Error, Text: err.Error()})
		return kernel.ErrFailed
	}
	k.emitReply(reply, name, emit)
	if reply.Error != "" {
		return kernel.ErrFailed
	}
	return nil
}

func (k *Kernel) start() error {
	bin := k.Bin
	if bin == "" {
		b, err := FindGolars()
		if err != nil {
			return fmt.Errorf("%w: %v", ErrNoGolars, err)
		}
		bin = b
	}
	dir := k.Dir
	if dir == "" {
		dir = "."
	}
	env := append(os.Environ(), "NO_COLOR=1")
	if k.BridgeDir != "" {
		env = append(env, "GOPYTER_BRIDGE_DIR="+k.BridgeDir)
	}
	h, err := startHost(bin, dir, env)
	if err != nil {
		return err
	}
	k.host = h
	return nil
}

// emitReply turns a host reply into events: stdout and tables in order,
// stderr, the error, then the auto-displayed frame.
func (k *Kernel) emitReply(r *Reply, name string, emit func(kernel.Event)) {
	text := func(s string) {
		if s = k.tidy(s); s != "" {
			emit(kernel.Event{Kind: kernel.Stdout, Text: s})
		}
	}
	if r.Outputs != nil {
		for _, o := range r.Outputs {
			switch o.Type {
			case "table":
				emitTable(o.Table, o.HTML, emit)
			default:
				text(o.Text)
			}
		}
	} else {
		text(r.Text)
	}
	if r.Stderr != "" {
		emit(kernel.Event{Kind: kernel.Stderr, Text: k.tidy(r.Stderr)})
	}
	if r.Error != "" {
		emit(kernel.Event{Kind: kernel.Error, Text: strings.ReplaceAll(k.tidy(r.Error), "<cell>", name)})
		return
	}
	switch {
	case len(r.Table) > 0:
		emitTable(r.Table, r.HTML, emit)
	case r.HTML != "":
		// An older golars: no table data, so show its HTML.
		emit(kernel.Event{Kind: kernel.HTML, Text: r.HTML})
	}
}

func emitTable(raw json.RawMessage, html string, emit func(kernel.Event)) {
	var t table.Table
	if err := json.Unmarshal(raw, &t); err != nil {
		emit(kernel.Event{Kind: kernel.Error, Text: "golars table: " + err.Error()})
		return
	}
	emit(kernel.Event{Kind: kernel.Table, Text: table.Output{Table: t, HTML: html}.Encode()})
}

// tidy shortens paths into the bridge directory to "bridge:".
func (k *Kernel) tidy(s string) string {
	if k.BridgeDir == "" {
		return s
	}
	return strings.ReplaceAll(s, k.BridgeDir+string(filepath.Separator), "bridge:")
}

// expand replaces gopyter's directives with glr: `%export NAME` saves the
// focused frame for Go cells (nb.BridgePath(NAME)), and `%import NAME`
// loads one a Go cell wrote, as the named frame NAME. Line numbers stay
// the same.
func (k *Kernel) expand(src string) (string, error) {
	lines := strings.Split(src, "\n")
	for i, l := range lines {
		t := strings.TrimSpace(l)
		if !strings.HasPrefix(t, "%") {
			continue
		}
		fields := strings.Fields(t)
		switch fields[0] {
		case "%export", "%import":
			if len(fields) != 2 || !validName(fields[1]) {
				return "", fmt.Errorf("line %d: usage: %s NAME (a letter or _, then letters, digits or _)", i+1, fields[0])
			}
			path, err := k.bridgePath(fields[1])
			if err != nil {
				return "", fmt.Errorf("line %d: %w", i+1, err)
			}
			if fields[0] == "%export" {
				lines[i] = "save " + path
			} else {
				lines[i] = "load " + path + " as " + fields[1]
			}
		default:
			return "", fmt.Errorf("line %d: unknown directive %s (glr cells know %%export NAME and %%import NAME)", i+1, fields[0])
		}
	}
	return strings.Join(lines, "\n"), nil
}

func (k *Kernel) bridgePath(name string) (string, error) {
	if k.BridgeDir == "" {
		return "", errors.New("no bridge directory: %export and %import need the Go kernel's workspace")
	}
	if strings.ContainsAny(k.BridgeDir, " \t") {
		return "", fmt.Errorf("the bridge directory %q has spaces, which glr paths can't: use a --workdir without them", k.BridgeDir)
	}
	return filepath.Join(k.BridgeDir, name+".arrow"), nil
}

func validName(s string) bool {
	for i, r := range s {
		switch {
		case r == '_', r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9' && i > 0:
		default:
			return false
		}
	}
	return s != ""
}

// Restart stops the host. The next cell starts a new one, without the
// frames of earlier cells.
func (k *Kernel) Restart() {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.host != nil {
		k.host.kill()
		k.host = nil
		k.lost = false // a deliberate restart needs no notice
	}
}

// Close stops the host.
func (k *Kernel) Close() error {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.host != nil {
		k.host.close()
		k.host = nil
	}
	return nil
}
