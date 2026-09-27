package glr

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Gaurav-Gosain/gopyter/internal/kernel"
	"github.com/Gaurav-Gosain/gopyter/internal/table"
)

// TestMain doubles as a fake `golars kernel-host` when the test binary
// is started with GLR_FAKE_HOST=1, so the host protocol is tested
// without golars.
func TestMain(m *testing.M) {
	if os.Getenv("GLR_FAKE_HOST") == "1" && len(os.Args) > 1 && os.Args[len(os.Args)-1] == "kernel-host" {
		fakeHost()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

const fakeTable = `{"columns":["a","b"],"dtypes":["i64","str"],"rows":[["1",null]],"shape":[1,2]}`

// fakeHost answers requests by their code.
func fakeHost() {
	sc := bufio.NewScanner(os.Stdin)
	out := json.NewEncoder(os.Stdout)
	state := ""
	for sc.Scan() {
		var req Request
		if err := json.Unmarshal(sc.Bytes(), &req); err != nil {
			_ = out.Encode(Reply{Error: "invalid request"})
			continue
		}
		r := Reply{ID: req.ID}
		switch code := strings.TrimSpace(req.Code); {
		case code == "table":
			if !req.Structured {
				r.Error = "not structured"
				break
			}
			r.Outputs = []Output{
				{Type: "stdout", Text: "before\n"},
				{Type: "table", Table: json.RawMessage(fakeTable), HTML: "<table>t</table>"},
				{Type: "stdout", Text: "after\n"},
			}
			r.Table, r.HTML = json.RawMessage(fakeTable), "<table>auto</table>"
		case code == "old":
			r.Text, r.HTML = "text\n", "<table>old</table>"
		case code == "fail":
			r.Text, r.Error = "partial\n", "<cell>:2: unknown command .frob"
		case code == "sleep":
			time.Sleep(time.Minute)
		case code == "crash":
			fmt.Fprintln(os.Stderr, "boom")
			os.Exit(3)
		case strings.HasPrefix(code, "set "):
			state = strings.TrimPrefix(code, "set ")
		case code == "get":
			r.Text = "state=" + state + "\n"
		default:
			r.Text = "echo:" + req.Code + "\n"
		}
		if err := out.Encode(r); err != nil {
			return
		}
	}
}

func newFakeKernel(t *testing.T) *Kernel {
	t.Helper()
	t.Setenv("GLR_FAKE_HOST", "1")
	k := &Kernel{Bin: os.Args[0], Dir: t.TempDir(), BridgeDir: t.TempDir()}
	t.Cleanup(func() { _ = k.Close() })
	return k
}

func execute(t *testing.T, k *Kernel, src string) ([]kernel.Event, error) {
	t.Helper()
	var evs []kernel.Event
	err := k.Execute(context.Background(), "c", "In[1]", src, func(e kernel.Event) { evs = append(evs, e) })
	return evs, err
}

func TestHostTablesInOrder(t *testing.T) {
	k := newFakeKernel(t)
	evs, err := execute(t, k, "table")
	if err != nil {
		t.Fatal(err)
	}
	kinds := []kernel.EventKind{kernel.Stdout, kernel.Table, kernel.Stdout, kernel.Table}
	if len(evs) != len(kinds) {
		t.Fatalf("events %+v", evs)
	}
	for i, want := range kinds {
		if evs[i].Kind != want {
			t.Fatalf("event %d: %+v", i, evs[i])
		}
	}
	out, err := table.Decode(evs[1].Text)
	if err != nil || out.HTML != "<table>t</table>" || out.Table.Columns[1] != "b" || out.Table.Rows[0][1] != nil {
		t.Fatalf("table %+v %v", out, err)
	}
	if auto, _ := table.Decode(evs[3].Text); auto.HTML != "<table>auto</table>" {
		t.Fatalf("auto display %+v", auto)
	}
}

func TestHostOldReply(t *testing.T) {
	k := newFakeKernel(t)
	evs, err := execute(t, k, "old")
	if err != nil || len(evs) != 2 || evs[0].Text != "text\n" || evs[1].Kind != kernel.HTML {
		t.Fatalf("%v %+v", err, evs)
	}
}

func TestHostError(t *testing.T) {
	k := newFakeKernel(t)
	evs, err := execute(t, k, "fail")
	if !errors.Is(err, kernel.ErrFailed) {
		t.Fatalf("err %v", err)
	}
	if len(evs) != 2 || evs[0].Text != "partial\n" || evs[1].Kind != kernel.Error || evs[1].Text != "In[1]:2: unknown command .frob" {
		t.Fatalf("events %+v", evs)
	}
}

func TestHostStateAndRestart(t *testing.T) {
	k := newFakeKernel(t)
	if _, err := execute(t, k, "set x"); err != nil {
		t.Fatal(err)
	}
	if evs, _ := execute(t, k, "get"); len(evs) != 1 || evs[0].Text != "state=x\n" {
		t.Fatalf("state not kept: %+v", evs)
	}
	k.Restart()
	if evs, _ := execute(t, k, "get"); len(evs) != 1 || evs[0].Text != "state=\n" {
		t.Fatalf("restart kept state: %+v", evs)
	}
}

func TestHostInterrupt(t *testing.T) {
	k := newFakeKernel(t)
	if _, err := execute(t, k, "set x"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := k.Execute(ctx, "c", "In[2]", "sleep", func(kernel.Event) {})
	if !errors.Is(err, kernel.ErrInterrupted) || time.Since(start) > 10*time.Second {
		t.Fatalf("err %v after %v", err, time.Since(start))
	}
	// The next cell runs on a new host, and says the frames are gone.
	evs, err := execute(t, k, "get")
	if err != nil || len(evs) != 2 || evs[0].Kind != kernel.Info || evs[1].Text != "state=\n" {
		t.Fatalf("%v %+v", err, evs)
	}
}

func TestHostCrash(t *testing.T) {
	k := newFakeKernel(t)
	evs, err := execute(t, k, "crash")
	if !errors.Is(err, kernel.ErrFailed) || len(evs) != 1 || !strings.Contains(evs[0].Text, "boom") {
		t.Fatalf("%v %+v", err, evs)
	}
	if evs, err := execute(t, k, "get"); err != nil || evs[len(evs)-1].Text != "state=\n" {
		t.Fatalf("no new host: %v %+v", err, evs)
	}
}

func TestDirectives(t *testing.T) {
	k := newFakeKernel(t)
	evs, err := execute(t, k, "load a.csv\n%export sales\n  %import other\n")
	if err != nil {
		t.Fatal(err)
	}
	want := "echo:load a.csv\nsave bridge:sales.arrow\nload bridge:other.arrow as other\n\n"
	if len(evs) != 1 || evs[0].Text != want {
		t.Fatalf("got %q, want %q", evs[0].Text, want)
	}
	for _, bad := range []string{"%export", "%export 1x", "%nope x", "%import a b"} {
		evs, err := execute(t, k, bad)
		if !errors.Is(err, kernel.ErrFailed) || len(evs) != 1 || evs[0].Kind != kernel.Error {
			t.Errorf("%q: %v %+v", bad, err, evs)
		}
	}
	k.BridgeDir = filepath.Join(t.TempDir(), "with space")
	if _, err := execute(t, k, "%export x"); !errors.Is(err, kernel.ErrFailed) {
		t.Errorf("spaces in the bridge dir: %v", err)
	}
}

func TestMissingGolars(t *testing.T) {
	k := &Kernel{Bin: "", Dir: t.TempDir()}
	t.Setenv("GOLARS_BIN", filepath.Join(t.TempDir(), "nope"))
	_, err := execute(t, k, "show")
	if !errors.Is(err, ErrNoGolars) {
		t.Fatalf("err %v", err)
	}
}

func TestLintErrors(t *testing.T) {
	out := "/t/cell.glr:1: stash \"a\" is never used\n/t/cell.glr:3: unknown command \"frob\"\n/t/cell.glr:5: use \"x\" with no prior stash or load as\nother noise\n"
	got := lintErrors(out, "/t/cell.glr", "In[2]", 2, 2)
	if got != "In[2]:1: unknown command \"frob\"" {
		t.Fatalf("got %q", got)
	}
}

func TestLintJSONErrors(t *testing.T) {
	out := `[{"line":1,"column":1,"severity":"warning","code":"unused-stash","message":"stash \"a\" is never used"},
{"line":3,"column":8,"severity":"error","code":"unknown-column","message":"unknown column \"amout\"","hint":"did you mean \"amount\"?"},
{"line":4,"column":6,"severity":"error","code":"missing-file","message":"file \"x.csv\" does not exist"}]`
	got, ok := lintJSONErrors(out, "In[2]", 1, 5)
	if !ok || got != `In[2]:2: unknown column "amout" (did you mean "amount"?)` {
		t.Fatalf("got %q %v", got, ok)
	}
	if _, ok := lintJSONErrors("usage: golars lint", "In[1]", 0, 1); ok {
		t.Fatal("plain output parsed as JSON")
	}
}

// Check lints with the real golars when it is installed.
func TestCheck(t *testing.T) {
	if _, err := FindGolars(); err != nil || testing.Short() {
		t.Skip("golars not found")
	}
	k := &Kernel{BridgeDir: t.TempDir()}
	r, err := k.Check(context.Background(), "load a.csv\nstash base\n", "In[2]", "use base\nfrob")
	if err != nil {
		t.Fatal(err)
	}
	// Newer golars versions add a suggestion after the message.
	if !strings.HasPrefix(r.Errors, `In[2]:2: unknown command "frob"`) || strings.Contains(r.Errors, "\n") {
		t.Fatalf("errors %q", r.Errors)
	}
	if r, err := k.Check(context.Background(), "", "In[1]", "load a.csv\n%export a"); err != nil || !r.OK() {
		t.Fatalf("%+v %v", r, err)
	}
}
