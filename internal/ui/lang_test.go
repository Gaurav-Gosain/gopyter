package ui

import (
	"context"
	"strings"
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/Gaurav-Gosain/gopyter/internal/complete"
	"github.com/Gaurav-Gosain/gopyter/internal/glr"
	"github.com/Gaurav-Gosain/gopyter/internal/kernel"
	"github.com/Gaurav-Gosain/gopyter/internal/notebook"
	"github.com/Gaurav-Gosain/gopyter/internal/table"
	"github.com/charmbracelet/x/ansi"
)

func glrNotebook(cells ...*notebook.Cell) *notebook.Notebook {
	nb := notebook.NewLang(notebook.GLR)
	nb.Cells = cells
	return nb
}

func TestSwitchLanguage(t *testing.T) {
	m := New(Options{Notebook: glrNotebook(&notebook.Cell{ID: "a", Type: notebook.Code, Source: "load x.csv"}), Kernel: &kernel.Kernel{}})
	m.width, m.height = 100, 30
	c := m.cur()
	if c.lang != notebook.GLR || c.ed.lang != "glr" {
		t.Fatalf("lang %q, highlighting %q", c.lang, c.ed.lang)
	}
	m.handleKey(tea.KeyPressMsg{Code: 'l', Text: "l"})
	if c.lang != notebook.Go || c.ed.lang != "go" {
		t.Fatalf("after l: lang %q, highlighting %q", c.lang, c.ed.lang)
	}
	md := m.toNotebook().Cells[0].Metadata
	if g, _ := md["gopyter"].(map[string]any); g["language"] != "go" {
		t.Fatalf("metadata %v", md)
	}
	// New cells take the selected cell's language.
	m.handleKey(tea.KeyPressMsg{Code: 'b', Text: "b"})
	if m.cur().lang != notebook.Go {
		t.Fatalf("new cell lang %q", m.cur().lang)
	}
	m.leaveEdit()
	m.sel = 0
	m.handleKey(tea.KeyPressMsg{Code: 'l', Text: "l"})
	if md := m.toNotebook().Cells[0].Metadata; md["gopyter"] != nil {
		t.Fatalf("back to the default, metadata %v", md)
	}
	if !strings.Contains(ansi.Strip(m.View().Content), "╭─ glr ") {
		t.Fatal("border should show glr")
	}
}

func TestMagicSetsLanguage(t *testing.T) {
	m := New(Options{Notebook: glrNotebook(&notebook.Cell{ID: "a", Type: notebook.Code, Source: "%%go\nx := 1"}), Kernel: &kernel.Kernel{}})
	c := m.cur()
	if c.lang != notebook.GLR || c.runLang(c.ed.Value()) != notebook.Go || c.ed.lang != "go" {
		t.Fatalf("lang %q run %q hl %q", c.lang, c.runLang(c.ed.Value()), c.ed.lang)
	}
}

type recordingCompleter struct {
	mu   sync.Mutex
	reqs []complete.Request
}

func (r *recordingCompleter) Complete(_ context.Context, req complete.Request) (complete.Result, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reqs = append(r.reqs, req)
	return complete.Result{Source: "glr", Items: []complete.Item{{Label: "region", Insert: "region"}}}, nil
}

func (r *recordingCompleter) Diagnose(context.Context, complete.Request) ([]glr.Diagnostic, error) {
	return []glr.Diagnostic{{Row: 0, Col: 0, Message: "unknown command: frob", Error: true}}, nil
}

func TestGLRCompletionContext(t *testing.T) {
	rec := &recordingCompleter{}
	nb := glrNotebook(
		&notebook.Cell{ID: "a", Type: notebook.Code, Source: "load orders.csv"},
		&notebook.Cell{ID: "b", Type: notebook.Code, Source: "x := 1", Metadata: map[string]any{"gopyter": map[string]any{"language": "go"}}},
		&notebook.Cell{ID: "c", Type: notebook.Markdown, Source: "# notes"},
		&notebook.Cell{ID: "d", Type: notebook.Code, Source: "%%glr\nstash base"},
		&notebook.Cell{ID: "e", Type: notebook.Code, Source: ""},
	)
	m := New(Options{Notebook: nb, Kernel: &kernel.Kernel{}, Completer: fakeCompleter{}, GLRCompleter: rec})
	m.width, m.height = 100, 40
	m.sel = 4
	m.enterEdit()
	typeText(m, "sort re")
	if len(rec.reqs) == 0 {
		t.Fatal("the glr completer wasn't asked")
	}
	req := rec.reqs[len(rec.reqs)-1]
	if req.Lang != "glr" || req.Before != "load orders.csv\n\nstash base\n" || req.Src != "sort re" {
		t.Fatalf("request %+v", req)
	}
	if !m.comp.open || m.comp.items[0].Label != "region" {
		t.Fatalf("popup %+v", m.comp)
	}

	// Diagnostics show under the cell once computed for its source.
	m.closeCompletion()
	c := m.cur()
	m.handleDiagResult(diagResultMsg{cellID: c.id, src: c.ed.Value(), diags: []glr.Diagnostic{{Row: 0, Message: "unknown column", Error: true}}})
	if !strings.Contains(ansi.Strip(m.View().Content), "● 1:1 unknown column") {
		t.Fatal("diagnostic not shown")
	}
	typeText(m, "g")
	if strings.Contains(ansi.Strip(m.View().Content), "unknown column") {
		t.Fatal("stale diagnostic shown")
	}
}

func TestTableOutput(t *testing.T) {
	str := func(s string) *string { return &s }
	tb := table.Table{Shape: [2]int{20, 12}}
	row := []*string{}
	for i := range 12 {
		tb.Columns = append(tb.Columns, "column_"+string(rune('a'+i)))
		tb.Dtypes = append(tb.Dtypes, "f64")
		row = append(row, str("1.5"))
	}
	row[3] = nil
	tb.Rows = [][]*string{row}
	out := table.Output{Table: tb, HTML: "<table></table>"}.Encode()
	nb := glrNotebook(&notebook.Cell{ID: "a", Type: notebook.Code, Source: "show", ExecutionCount: 1,
		Outputs: []notebook.Output{{Kind: notebook.TableOut, Text: out}}})
	for _, width := range []int{80, 160} {
		m := New(Options{Notebook: nb, Kernel: &kernel.Kernel{}})
		m.width, m.height = width, 40
		m.mode = modeCommand
		view := ansi.Strip(m.View().Content)
		for l := range strings.SplitSeq(view, "\n") {
			if ansi.StringWidth(l) > width {
				t.Fatalf("width %d: line too wide: %q", width, l)
			}
		}
		if !strings.Contains(view, "Out[1]") || !strings.Contains(view, "column_a") || !strings.Contains(view, "shape: (20, 12)") {
			t.Fatalf("width %d:\n%s", width, view)
		}
		narrow := strings.Contains(view, "┆ … ┆")
		if narrow != (width == 80) {
			t.Fatalf("width %d: columns left out = %v\n%s", width, narrow, view)
		}
	}
}
