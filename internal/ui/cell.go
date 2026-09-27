package ui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/Gaurav-Gosain/gopyter/internal/glr"
	"github.com/Gaurav-Gosain/gopyter/internal/htmlview"
	"github.com/Gaurav-Gosain/gopyter/internal/notebook"
	"github.com/charmbracelet/x/ansi"
)

type cellStatus int

const (
	statusIdle cellStatus = iota
	statusQueued
	statusRunning
	statusOK
	statusFailed
	statusInterrupted
)

const maxOutputLines = 40

// Cell is a notebook cell in the UI.
type Cell struct {
	id       string
	kind     notebook.CellType
	ed       *Editor
	outputs  []notebook.Output
	count    int
	status   cellStatus
	errMsg   string
	started  time.Time
	duration time.Duration
	expanded bool
	metadata map[string]any
	// lang is the language of a code cell, before any %%go/%%glr magic
	// (see runLang).
	lang notebook.Lang
	// diags are golars-lsp's diagnostics for diagSrc, the source they
	// were computed for (glr cells).
	diags   []glr.Diagnostic
	diagSrc string
	// ran is set once the cell has run in the current kernel session. The
	// execution count may come from the saved notebook instead.
	ran bool

	mdSrc, mdOut string
	mdWidth      int

	outKey      outputKey
	outLines    []string
	outResultAt int
	outWidgets  []htmlview.Widget // widgets of HTML outputs, positioned in outLines
	// outRev counts changes to outputs, which aren't always appends:
	// DisplayID replaces an output in place.
	outRev int
}

// outputKey fingerprints the outputs, and how their widgets are drawn.
type outputKey struct {
	width, n, last, rev int
	kind                notebook.OutputKind
	focus               string
	live                bool
}

func (c *Cell) outputFingerprint(width int, focus string, live bool) outputKey {
	k := outputKey{width: width, n: len(c.outputs), rev: c.outRev, focus: focus, live: live}
	if k.n > 0 {
		k.last = len(c.outputs[k.n-1].Text)
		k.kind = c.outputs[k.n-1].Kind
	}
	return k
}

// langFor is the highlighting language of a cell.
func langFor(kind notebook.CellType, lang notebook.Lang) string {
	if kind == notebook.Markdown {
		return "markdown"
	}
	if kind == notebook.Raw {
		return "plaintext"
	}
	return string(lang)
}

// newCell returns a Go cell; see Model.newCodeCell for the notebook's
// language.
func newCell(kind notebook.CellType, src string) *Cell {
	return newLangCell(kind, notebook.Go, src)
}

func newLangCell(kind notebook.CellType, lang notebook.Lang, src string) *Cell {
	c := &Cell{id: notebook.NewID(), kind: kind, lang: lang}
	c.ed = NewEditor(langFor(kind, c.runLang(src)), src)
	return c
}

// fromNotebook converts a notebook cell; def is the notebook's language.
func fromNotebook(c *notebook.Cell, def notebook.Lang) *Cell {
	cell := &Cell{
		id: c.ID, kind: c.Type, lang: notebook.CellLang(c.Metadata, def),
		outputs: c.Outputs, count: c.ExecutionCount, metadata: c.Metadata,
		status: diskStatus(c),
	}
	cell.ed = NewEditor(langFor(c.Type, cell.runLang(c.Source)), c.Source)
	return cell
}

// runLang is the language the cell runs in: its own, unless the source
// starts with a %%go or %%glr magic.
func (c *Cell) runLang(src string) notebook.Lang {
	l, _ := notebook.Resolve(src, c.baseLang())
	return l
}

// baseLang is the cell's language, ignoring magics.
func (c *Cell) baseLang() notebook.Lang {
	if c.lang == "" {
		return notebook.Go
	}
	return c.lang
}

// syncLang updates the highlighting after the language or a magic
// changed.
func (c *Cell) syncLang() {
	if want := langFor(c.kind, c.runLang(c.ed.Value())); c.ed.lang != want {
		c.ed.SetLang(want)
	}
}

// diskStatus derives a cell's run status from its saved outputs.
func diskStatus(c *notebook.Cell) cellStatus {
	if c.ExecutionCount == 0 {
		return statusIdle
	}
	for _, o := range c.Outputs {
		if o.Kind == notebook.Error {
			return statusFailed
		}
	}
	return statusOK
}

// toNotebook converts the cell; def is the notebook's language.
func (c *Cell) toNotebook(def notebook.Lang) *notebook.Cell {
	md := c.metadata
	if c.kind == notebook.Code && c.lang != "" {
		md = notebook.SetCellLang(md, c.lang, def)
		c.metadata = md
	}
	return &notebook.Cell{ID: c.id, Type: c.kind, Source: c.ed.Value(), ExecutionCount: c.count, Outputs: c.outputs, Metadata: md}
}

func (c *Cell) clone() *Cell {
	n := newLangCell(c.kind, c.lang, c.ed.Value())
	n.outputs = append([]notebook.Output(nil), c.outputs...)
	return n
}

func (c *Cell) setKind(kind notebook.CellType) {
	c.kind = kind
	c.ed.SetLang(langFor(kind, c.runLang(c.ed.Value())))
	if kind != notebook.Code {
		c.outputs, c.count, c.status = nil, 0, statusIdle
		c.outRev++
	}
}

func (c *Cell) appendOutput(kind notebook.OutputKind, text string) {
	c.outRev++
	if n := len(c.outputs); n > 0 && (kind == notebook.Stdout || kind == notebook.Stderr) && c.outputs[n-1].Kind == kind {
		c.outputs[n-1].Text += text
		// Keep memory bounded for chatty programs.
		if len(c.outputs[n-1].Text) > 4<<20 {
			t := c.outputs[n-1].Text
			c.outputs[n-1].Text = t[len(t)-(2<<20):]
		}
		return
	}
	c.outputs = append(c.outputs, notebook.Output{Kind: kind, Text: text})
}

// setOutput shows an updatable output (DisplayID): it replaces the
// output with the same id, or is appended the first time.
func (c *Cell) setOutput(kind notebook.OutputKind, text, id string) {
	c.outRev++
	for i := range c.outputs {
		if c.outputs[i].ID == id {
			c.outputs[i].Kind, c.outputs[i].Text = kind, text
			return
		}
	}
	c.outputs = append(c.outputs, notebook.Output{Kind: kind, Text: text, ID: id})
}

// cellRender is the rendered form of a cell plus layout metadata.
type cellRender struct {
	lines   []string
	edTop   int // first line of the editor within lines, -1 if none
	edLeft  int // column where the editor starts
	ev      editorView
	hasEdit bool
	zones   []zone // clickable regions; y is relative to the cell's first line
	// ownSpacer is set when the last line doubles as the inter-cell spacer.
	ownSpacer bool
}

// normalizeOutput turns raw program output into printable lines.
func normalizeOutput(s string) []string {
	s = ansi.Strip(s)
	s = strings.TrimSuffix(s, "\n")
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		l = strings.TrimSuffix(l, "\r")
		if j := strings.LastIndex(l, "\r"); j >= 0 {
			l = l[j+1:]
		}
		l = expandTabs(l)
		l = strings.Map(func(r rune) rune {
			if r < 0x20 || r == 0x7f {
				return -1
			}
			return r
		}, l)
		lines[i] = l
	}
	return lines
}

func expandTabs(s string) string {
	if !strings.ContainsRune(s, '\t') {
		return s
	}
	var b strings.Builder
	x := 0
	for _, r := range s {
		if r == '\t' {
			n := tabWidth - x%tabWidth
			b.WriteString(strings.Repeat(" ", n))
			x += n
			continue
		}
		b.WriteRune(r)
		x += runeWidth(r, x)
	}
	return b.String()
}

func wrapLines(lines []string, width int) []string {
	var out []string
	for _, l := range lines {
		if lipgloss.Width(l) <= width {
			out = append(out, l)
			continue
		}
		out = append(out, strings.Split(ansi.Wrap(l, width, ""), "\n")...)
	}
	return out
}

func fmtDuration(d time.Duration) string {
	switch {
	case d < time.Millisecond:
		return "<1ms"
	case d < time.Second:
		return fmt.Sprintf("%dms", d.Milliseconds())
	case d < time.Minute:
		return fmt.Sprintf("%.2fs", d.Seconds())
	default:
		return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
	}
}

// boxTop draws a rounded top border with a left title and a right badge.
// The title is inserted verbatim after "╭─" (callers include any padding).
func boxTop(width int, border lipgloss.Style, title, badge string) string {
	left := border.Render("╭─") + title
	right := border.Render("╮")
	if badge != "" {
		right = " " + badge + " " + border.Render("─╮")
	}
	fill := width - lipgloss.Width(left) - lipgloss.Width(right)
	if fill < 0 {
		return border.Render("╭" + strings.Repeat("─", max(width-2, 0)) + "╮")
	}
	return left + border.Render(strings.Repeat("─", fill)) + right
}

func boxBottom(width int, border lipgloss.Style) string {
	return border.Render("╰" + strings.Repeat("─", max(width-2, 0)) + "╯")
}

func padRight(s string, w int) string {
	if n := lipgloss.Width(s); n < w {
		return s + strings.Repeat(" ", w-n)
	}
	return s
}
