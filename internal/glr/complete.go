package glr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/Gaurav-Gosain/gopyter/internal/complete"
	"github.com/Gaurav-Gosain/gopyter/internal/lsp"
)

// Diagnostic is a problem golars-lsp found in a cell.
type Diagnostic struct {
	Row, Col int // 0-based, in the cell; Col counts runes
	Message  string
	Error    bool // an error rather than a warning
}

// Completer completes glr cells and shows their symbol info and
// diagnostics with golars-lsp. golars-lsp sees one virtual document: the
// notebook's earlier glr cells, then the current one, so the frames and
// columns they load are known; positions are mapped back to the cell.
// Without golars-lsp it completes command names and keywords.
type Completer struct {
	// Dir is the directory relative paths in cells are resolved against
	// (the notebook's); it is read when golars-lsp starts.
	Dir func() string

	once     sync.Once
	started  atomic.Bool
	ready    chan struct{}
	startErr error
	client   *lsp.Client
	uri      string

	syncMu  sync.Mutex // orders document versions
	version int
	opened  bool

	diagMu   sync.Mutex
	diagWait chan []lspDiagnostic
}

// NewCompleter returns a completer; golars-lsp starts on first use.
func NewCompleter(dir func() string) *Completer {
	return &Completer{Dir: dir, ready: make(chan struct{})}
}

func (c *Completer) start() {
	c.once.Do(func() {
		c.started.Store(true)
		go func() {
			c.startErr = c.launch()
			close(c.ready)
		}()
	})
}

// client waits for golars-lsp; it is nil when it isn't available.
func (c *Completer) lspClient(ctx context.Context) *lsp.Client {
	c.start()
	select {
	case <-c.ready:
	case <-ctx.Done():
		return nil
	}
	if c.client == nil {
		return nil
	}
	select {
	case <-c.client.Done():
		return nil
	default:
		return c.client
	}
}

func (c *Completer) launch() error {
	bin, err := FindLSP()
	if err != nil {
		return err
	}
	dir := "."
	if c.Dir != nil && c.Dir() != "" {
		dir = c.Dir()
	}
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	cmd := exec.Command(bin)
	cmd.Dir = dir
	client, err := lsp.StartNotify(cmd, nil, c.notified)
	if err != nil {
		return err
	}
	root := fileURI(dir)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err = client.Call(ctx, "initialize", map[string]any{
		"processId": os.Getpid(),
		"rootUri":   root,
		"capabilities": map[string]any{
			"textDocument": map[string]any{
				"completion": map[string]any{"completionItem": map[string]any{"snippetSupport": false}},
				"hover":      map[string]any{"contentFormat": []string{"markdown", "plaintext"}},
			},
			"general": map[string]any{"positionEncodings": []string{"utf-16"}},
		},
	}, nil)
	if err == nil {
		err = client.Notify("initialized", map[string]any{})
	}
	if err != nil {
		_ = client.Close()
		return fmt.Errorf("starting golars-lsp: %w", err)
	}
	c.client = client
	// The document lives next to the notebook, where golars-lsp looks
	// for the files cells load; it is never written.
	c.uri = fileURI(filepath.Join(dir, ".gopyter-cells.glr"))
	return nil
}

func fileURI(path string) string {
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(path)}).String()
}

type lspPosition struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type lspDiagnostic struct {
	Range struct {
		Start lspPosition `json:"start"`
		End   lspPosition `json:"end"`
	} `json:"range"`
	Severity int    `json:"severity"`
	Message  string `json:"message"`
}

func (c *Completer) notified(method string, params json.RawMessage) {
	if method != "textDocument/publishDiagnostics" {
		return
	}
	var p struct {
		URI         string          `json:"uri"`
		Diagnostics []lspDiagnostic `json:"diagnostics"`
	}
	if json.Unmarshal(params, &p) != nil || p.URI != c.uri {
		return
	}
	c.diagMu.Lock()
	defer c.diagMu.Unlock()
	if c.diagWait != nil {
		select {
		case c.diagWait <- p.Diagnostics:
		default:
		}
	}
}

// Doc is the virtual document for a cell: the source before it, then the
// cell. Offset is the document line of the cell's first line.
type Doc struct {
	Content string
	Offset  int
}

// NewDoc builds the document for a cell with the given earlier source.
func NewDoc(before, src string) Doc {
	if before != "" && !strings.HasSuffix(before, "\n") {
		before += "\n"
	}
	return Doc{Content: before + src, Offset: strings.Count(before, "\n")}
}

// Position maps a cell position (row, rune column) to an LSP position
// (UTF-16 code units).
func (d Doc) Position(src string, row, col int) lspPosition {
	lines := strings.Split(src, "\n")
	row = min(max(row, 0), len(lines)-1)
	r := []rune(lines[row])
	col = min(max(col, 0), len(r))
	return lspPosition{Line: d.Offset + row, Character: utf16Len(string(r[:col]))}
}

// CellDiagnostics keeps the diagnostics inside the cell (src) and maps
// them to cell positions.
func (d Doc) CellDiagnostics(src string, diags []lspDiagnostic) []Diagnostic {
	lines := strings.Split(src, "\n")
	var out []Diagnostic
	for _, x := range diags {
		row := x.Range.Start.Line - d.Offset
		if row < 0 || row >= len(lines) {
			continue
		}
		if strings.HasPrefix(x.Message, "stash ") && strings.Contains(x.Message, "is never used") {
			// Later cells, in either language, may use it.
			continue
		}
		col := utf8.RuneCountInString(lines[row][:utf16ToByte(lines[row], x.Range.Start.Character)])
		out = append(out, Diagnostic{Row: row, Col: col, Message: x.Message, Error: x.Severity == 1 || x.Severity == 0})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Row < out[j].Row })
	return out
}

// sync sends the document for req, returning it.
func (c *Completer) sync(client *lsp.Client, req complete.Request) (Doc, error) {
	doc := NewDoc(req.Before, req.Src)
	c.syncMu.Lock()
	defer c.syncMu.Unlock()
	c.version++
	td := map[string]any{"uri": c.uri, "version": c.version}
	var err error
	if !c.opened {
		c.opened = true
		td["languageId"] = "golars"
		td["text"] = doc.Content
		err = client.Notify("textDocument/didOpen", map[string]any{"textDocument": td})
	} else {
		err = client.Notify("textDocument/didChange", map[string]any{
			"textDocument":   td,
			"contentChanges": []any{map[string]string{"text": doc.Content}},
		})
	}
	return doc, err
}

// Status reports whether golars-lsp is in use, and why not otherwise.
// It starts golars-lsp in the background, without waiting.
func (c *Completer) Status() (bool, string) {
	c.start()
	select {
	case <-c.ready:
		if c.startErr != nil {
			return false, c.startErr.Error()
		}
		return true, ""
	default:
		return false, "golars-lsp starting"
	}
}

// Complete completes a glr cell.
func (c *Completer) Complete(ctx context.Context, req complete.Request) (complete.Result, error) {
	client := c.lspClient(ctx)
	if client == nil {
		return basic(req), nil
	}
	doc, err := c.sync(client, req)
	if err != nil {
		return basic(req), nil
	}
	pos := doc.Position(req.Src, req.Row, req.Col)
	var raw json.RawMessage
	err = client.Call(ctx, "textDocument/completion", map[string]any{
		"textDocument": map[string]string{"uri": c.uri},
		"position":     pos,
	}, &raw)
	if err != nil {
		return complete.Result{}, err
	}
	var items []struct {
		Label         string          `json:"label"`
		Kind          int             `json:"kind"`
		Detail        string          `json:"detail"`
		Documentation json.RawMessage `json:"documentation"`
		InsertText    string          `json:"insertText"`
	}
	if len(raw) > 0 && raw[0] == '[' {
		_ = json.Unmarshal(raw, &items) // bad items just complete nothing
	} else {
		var list struct {
			Items json.RawMessage `json:"items"`
		}
		_ = json.Unmarshal(raw, &list)
		_ = json.Unmarshal(list.Items, &items)
	}
	res := complete.Result{Source: "golars-lsp", Replace: wordBefore(req)}
	for _, it := range items {
		insert := it.InsertText
		if insert == "" {
			insert = it.Label
		}
		res.Items = append(res.Items, complete.Item{
			Label: it.Label, Detail: it.Detail, Doc: markupText(it.Documentation),
			Insert: insert, Kind: kindOf(it.Kind), Replace: res.Replace,
		})
	}
	return res, nil
}

// Hover returns golars-lsp's documentation for the word at the cursor as
// markdown, or the command's summary without golars-lsp.
func (c *Completer) Hover(ctx context.Context, req complete.Request) (string, error) {
	client := c.lspClient(ctx)
	if client == nil {
		return basicHover(req), nil
	}
	doc, err := c.sync(client, req)
	if err != nil {
		return "", err
	}
	var res struct {
		Contents json.RawMessage `json:"contents"`
	}
	err = client.Call(ctx, "textDocument/hover", map[string]any{
		"textDocument": map[string]string{"uri": c.uri},
		"position":     doc.Position(req.Src, req.Row, req.Col),
	}, &res)
	if err != nil {
		return "", err
	}
	if s := strings.TrimSpace(markupText(res.Contents)); s != "" {
		return s, nil
	}
	return basicHover(req), nil
}

// Diagnose returns golars-lsp's diagnostics for a cell. Without
// golars-lsp there are none.
func (c *Completer) Diagnose(ctx context.Context, req complete.Request) ([]Diagnostic, error) {
	client := c.lspClient(ctx)
	if client == nil {
		return nil, nil
	}
	wait := make(chan []lspDiagnostic, 1)
	c.diagMu.Lock()
	c.diagWait = wait
	c.diagMu.Unlock()
	defer func() {
		c.diagMu.Lock()
		if c.diagWait == wait {
			c.diagWait = nil
		}
		c.diagMu.Unlock()
	}()
	doc, err := c.sync(client, req)
	if err != nil {
		return nil, err
	}
	select {
	case d := <-wait:
		return doc.CellDiagnostics(req.Src, d), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Close stops golars-lsp.
func (c *Completer) Close() error {
	if !c.started.Load() {
		return nil
	}
	<-c.ready
	if c.client == nil {
		return nil
	}
	err := c.client.Close()
	if errors.Is(err, os.ErrProcessDone) {
		return nil
	}
	return err
}

func markupText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var mc struct {
		Value string `json:"value"`
	}
	_ = json.Unmarshal(raw, &mc) // anything else shows nothing
	return mc.Value
}

func kindOf(k int) complete.Kind {
	switch k {
	case 3, 2:
		return complete.KindFunc
	case 5, 10:
		return complete.KindField
	case 6, 12:
		return complete.KindVar
	case 14:
		return complete.KindKeyword
	case 17, 19:
		return complete.KindPackage // files and folders
	case 21, 20:
		return complete.KindConst
	}
	return complete.KindOther
}

func utf16Len(s string) int {
	n := 0
	for _, r := range s {
		n += utf16.RuneLen(r)
	}
	return n
}

func utf16ToByte(s string, units int) int {
	n := 0
	for i, r := range s {
		if n >= units {
			return i
		}
		n += utf16.RuneLen(r)
	}
	return len(s)
}

func isWord(r rune) bool {
	return r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9'
}

// cursorLine returns the cell line at the cursor and the runes before it.
func cursorLine(req complete.Request) (line, before []rune) {
	lines := strings.Split(req.Src, "\n")
	row := min(max(req.Row, 0), len(lines)-1)
	line = []rune(lines[row])
	col := min(max(req.Col, 0), len(line))
	return line, line[:col]
}

// wordBefore is the number of word runes before the cursor.
func wordBefore(req complete.Request) int {
	_, before := cursorLine(req)
	n := 0
	for n < len(before) && isWord(before[len(before)-1-n]) {
		n++
	}
	return n
}

// basic completes command names at the start of a line; after it,
// keywords and expression functions, and after a `.` the methods of a
// value (x.ro -> round) or of a namespace (dt.ye, ts.dt.ye -> year).
func basic(req complete.Request) complete.Result {
	_, before := cursorLine(req)
	n := wordBefore(req)
	prefix := string(before[len(before)-n:])
	res := complete.Result{Source: "basic", Replace: n}
	head := strings.TrimLeft(string(before[:len(before)-n]), " \t.")
	if head == "" {
		if prefix == "" && !req.Manual {
			return res
		}
		for _, c := range Commands {
			if strings.HasPrefix(c.Name, prefix) {
				res.Items = append(res.Items, complete.Item{
					Label: c.Name, Detail: c.Signature, Doc: c.Summary,
					Insert: c.Name, Kind: complete.KindFunc, Replace: n,
				})
			}
		}
		return res
	}
	add := func(names []string, detail string, kind complete.Kind) {
		for _, name := range names {
			if strings.HasPrefix(name, prefix) {
				res.Items = append(res.Items, complete.Item{Label: name, Detail: detail, Insert: name, Kind: kind, Replace: n})
			}
		}
	}
	rest := before[:len(before)-n]
	if cmd := findCommand(strings.Fields(head)[0]); cmd != nil && cmd.ArgKind == "path" {
		// A path such as data/orders.csv: no member completion.
	} else if ns, isMember := memberContext(rest); isMember {
		if ns != "" {
			add(Functions[ns], ns+" function", complete.KindMethod)
		} else {
			add(namespaces, "namespace", complete.KindPackage)
			add(Functions[""], "method", complete.KindMethod)
		}
		return res
	}
	if prefix == "" && !req.Manual {
		return res
	}
	add(req.Frames, "shared frame", complete.KindVar)
	add(keywords, "", complete.KindKeyword)
	add(Functions["free"], "function", complete.KindFunc)
	add(namespaces, "namespace", complete.KindPackage)
	add(Functions[""], "function", complete.KindFunc)
	return res
}

// memberContext reports whether the text before the current word ends
// in a member access `.`, and when the value before the dot is a
// namespace (dt. or ts.dt.), which one.
func memberContext(rest []rune) (ns string, ok bool) {
	if len(rest) == 0 || rest[len(rest)-1] != '.' {
		return "", false
	}
	end := len(rest) - 1
	start := end
	for start > 0 && isWord(rest[start-1]) {
		start--
	}
	if start == end {
		// `).` or `].`: a method of an expression.
		return "", end > 0 && (rest[end-1] == ')' || rest[end-1] == ']')
	}
	word := string(rest[start:end])
	if word[0] >= '0' && word[0] <= '9' {
		return "", false
	}
	// dt. and ts.dt. both lead to the dt functions. name. may also be
	// a column, but its namespace functions are the likelier completion.
	if slices.Contains(namespaces, word) {
		return word, true
	}
	return "", true
}

// basicHover documents the command of the cursor's line.
func basicHover(req complete.Request) string {
	line, _ := cursorLine(req)
	fields := strings.Fields(strings.TrimLeft(string(line), " \t."))
	if len(fields) == 0 {
		return ""
	}
	if c := findCommand(fields[0]); c != nil {
		md := "```glr\n" + c.Signature + "\n```\n\n" + c.Summary
		if len(c.Aliases) > 0 {
			md += "\n\nAliases: " + strings.Join(c.Aliases, ", ")
		}
		return md
	}
	return ""
}
