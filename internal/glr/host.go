package glr

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Request is one cell for `golars kernel-host`, as a line of JSON.
type Request struct {
	ID   string `json:"id"`
	Code string `json:"code"`
	// Structured asks for tables as data: the reply then carries Outputs
	// and Table. Hosts that predate it ignore the field.
	Structured bool `json:"structured,omitempty"`
	// Op, Name and Path make a request that runs no code: "frames" lists
	// the frames, "export" writes frame Name ("" for the focus) to Path
	// as Arrow IPC and "import" stages Path as frame Name. Hosts that
	// predate them reply with an error.
	Op   string `json:"op,omitempty"`
	Name string `json:"name,omitempty"`
	Path string `json:"path,omitempty"`
}

// HostFrame is a frame listed by an op=frames reply.
type HostFrame struct {
	Name  string `json:"name"`
	Focus bool   `json:"focus,omitempty"`
	Rows  int    `json:"rows"`
	Cols  int    `json:"cols"`
	Lazy  bool   `json:"lazy,omitempty"`
	Gen   int    `json:"gen"`
}

// Reply is the host's answer to a Request.
type Reply struct {
	ID string `json:"id"`
	// Text is the cell's stdout. For structured requests it lacks the
	// tables, which are in Outputs.
	Text   string  `json:"text"`
	Stderr string  `json:"stderr"`
	HTML   string  `json:"html"`
	Error  string  `json:"error,omitempty"`
	Shape  *[2]int `json:"shape,omitempty"`
	// Table is the auto-displayed frame (table.Table JSON).
	Table json.RawMessage `json:"table,omitempty"`
	// Outputs are stdout text and tables in the order the cell printed
	// them.
	Outputs []Output `json:"outputs,omitempty"`
	// Frames answers op=frames; Gen is the generation of the frame an
	// op=import staged.
	Frames []HostFrame `json:"frames,omitempty"`
	Gen    int         `json:"gen,omitempty"`
}

// Output is an entry of Reply.Outputs.
type Output struct {
	Type  string          `json:"type"` // "stdout" or "table"
	Text  string          `json:"text,omitempty"`
	Table json.RawMessage `json:"table,omitempty"`
	HTML  string          `json:"html,omitempty"`
}

// errHostExited is returned when the host process ends mid-request.
var errHostExited = errors.New("golars kernel-host exited")

// host is a running `golars kernel-host` process. It runs one request at
// a time.
type host struct {
	cmd    *exec.Cmd
	in     io.WriteCloser
	out    *bufio.Reader
	stderr *tailBuffer
	seq    int
	exited chan struct{}
}

// startHost runs `bin kernel-host` in dir.
func startHost(bin, dir string, env []string) (*host, error) {
	cmd := exec.Command(bin, "kernel-host")
	cmd.Dir = dir
	cmd.Env = env
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	h := &host{cmd: cmd, in: stdin, out: bufio.NewReaderSize(stdout, 1<<20), stderr: &tailBuffer{max: 8 << 10}, exited: make(chan struct{})}
	cmd.Stderr = h.stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting %s kernel-host: %w", bin, err)
	}
	go func() {
		_ = cmd.Wait() // the exit is reported by the next read
		close(h.exited)
	}()
	return h, nil
}

// run sends code and waits for the reply. Cancelling ctx kills the host,
// which can't stop a cell otherwise; run then returns ctx.Err().
func (h *host) run(ctx context.Context, code string) (*Reply, error) {
	return h.do(ctx, Request{Code: code, Structured: true})
}

// do sends a request and waits for the reply, like run.
func (h *host) do(ctx context.Context, req Request) (*Reply, error) {
	h.seq++
	req.ID = strconv.Itoa(h.seq)
	line, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	type result struct {
		reply *Reply
		err   error
	}
	done := make(chan result, 1)
	go func() {
		if _, err := h.in.Write(append(line, '\n')); err != nil {
			done <- result{err: errHostExited}
			return
		}
		for {
			b, err := h.out.ReadBytes('\n')
			if err != nil {
				done <- result{err: errHostExited}
				return
			}
			var r Reply
			if err := json.Unmarshal(b, &r); err != nil {
				done <- result{err: fmt.Errorf("golars kernel-host: bad reply: %w", err)}
				return
			}
			// A reply without our id answers an earlier, broken line.
			if r.ID == req.ID {
				done <- result{reply: &r}
				return
			}
		}
	}()
	select {
	case r := <-done:
		if errors.Is(r.err, errHostExited) {
			h.kill() // wait for it, so its stderr is complete
			if msg := strings.TrimSpace(h.stderr.String()); msg != "" {
				return nil, fmt.Errorf("%w: %s", errHostExited, msg)
			}
		}
		return r.reply, r.err
	case <-ctx.Done():
		h.kill()
		<-done
		return nil, ctx.Err()
	}
}

// kill stops the host at once.
func (h *host) kill() {
	if h.cmd.Process != nil {
		_ = h.cmd.Process.Kill() // it may have exited already
	}
	<-h.exited
}

// close ends the host: closing its input ends its loop.
func (h *host) close() {
	_ = h.in.Close()
	select {
	case <-h.exited:
	case <-time.After(2 * time.Second):
		h.kill()
	}
}

// tailBuffer keeps the last max bytes written to it.
type tailBuffer struct {
	mu  sync.Mutex
	max int
	b   []byte
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.b = append(t.b, p...)
	if len(t.b) > t.max {
		t.b = t.b[len(t.b)-t.max:]
	}
	return len(p), nil
}

func (t *tailBuffer) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.b)
}

// FindGolars locates the golars binary the way golars-kernel does:
// $GOLARS_BIN, then next to the gopyter binary, then on $PATH.
func FindGolars() (string, error) {
	return find("golars", os.Getenv("GOLARS_BIN"))
}

// FindLSP locates golars-lsp: $GOLARS_LSP, next to $GOLARS_BIN, next to
// the gopyter binary, then on $PATH.
func FindLSP() (string, error) {
	if env := os.Getenv("GOLARS_LSP"); env != "" {
		return find("golars-lsp", env)
	}
	if bin := os.Getenv("GOLARS_BIN"); bin != "" {
		cand := filepath.Join(filepath.Dir(bin), "golars-lsp")
		if isFile(cand) {
			return cand, nil
		}
	}
	return find("golars-lsp", "")
}

func find(name, env string) (string, error) {
	if env != "" {
		if isFile(env) {
			return env, nil
		}
		return "", fmt.Errorf("%s: %s not found", name, env)
	}
	if exe, err := os.Executable(); err == nil {
		if cand := filepath.Join(filepath.Dir(exe), name); isFile(cand) {
			return cand, nil
		}
	}
	p, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("%s not found: install golars (https://github.com/Gaurav-Gosain/golars) or set GOLARS_BIN", name)
	}
	return p, nil
}

func isFile(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}
