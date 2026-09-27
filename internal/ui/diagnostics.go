package ui

import (
	"context"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/Gaurav-Gosain/gopyter/internal/complete"
	"github.com/Gaurav-Gosain/gopyter/internal/glr"
	"github.com/Gaurav-Gosain/gopyter/internal/notebook"
	"github.com/charmbracelet/x/ansi"
)

// Diagnoser is optionally implemented by completers that report problems
// in a cell (golars-lsp for glr cells).
type Diagnoser interface {
	Diagnose(ctx context.Context, req complete.Request) ([]glr.Diagnostic, error)
}

const (
	diagDebounce = 350 * time.Millisecond
	// maxDiagLines caps the diagnostics listed under a cell.
	maxDiagLines = 4
)

type diagTickMsg struct{ seq int }

type diagResultMsg struct {
	cellID, src string
	diags       []glr.Diagnostic
}

// scheduleDiagnose checks the current glr cell after a pause in typing.
func (m *Model) scheduleDiagnose() tea.Cmd {
	c := m.cur()
	if _, ok := m.completerFor(c).(Diagnoser); !ok || c.kind != notebook.Code ||
		c.runLang(c.ed.Value()) != notebook.GLR || c.diagSrc == c.ed.Value() {
		return nil
	}
	m.diagSeq++
	seq := m.diagSeq
	return tea.Tick(diagDebounce, func(time.Time) tea.Msg { return diagTickMsg{seq: seq} })
}

func (m *Model) requestDiagnose(msg diagTickMsg) tea.Cmd {
	if msg.seq != m.diagSeq {
		return nil
	}
	c := m.cur()
	d, ok := m.completerFor(c).(Diagnoser)
	if !ok {
		return nil
	}
	req := m.completionRequest(c, 0, 0, false, 0)
	id, src := c.id, c.ed.Value()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		diags, err := d.Diagnose(ctx, req)
		if err != nil {
			return nil // diagnostics are a hint; try again on the next edit
		}
		return diagResultMsg{cellID: id, src: src, diags: diags}
	}
}

func (m *Model) handleDiagResult(msg diagResultMsg) {
	if _, c := m.cellByID(msg.cellID); c != nil && c.ed.Value() == msg.src {
		c.diags, c.diagSrc = msg.diags, msg.src
	}
}

// diagLines renders a cell's diagnostics, if they are for its current
// source.
func (m *Model) diagLines(c *Cell, width int) []string {
	if len(c.diags) == 0 || c.diagSrc != c.ed.Value() {
		return nil
	}
	t := m.theme
	var out []string
	for i, d := range c.diags {
		if i == maxDiagLines {
			out = append(out, t.muted.Render(fmt.Sprintf("  … %d more", len(c.diags)-i)))
			break
		}
		mark, style := "▲", t.stderr
		if d.Error {
			mark, style = "●", t.errorText
		}
		line := fmt.Sprintf("%s %d:%d %s", mark, d.Row+1, d.Col+1, d.Message)
		out = append(out, style.Render(ansi.Truncate(line, width, "…")))
	}
	return out
}
