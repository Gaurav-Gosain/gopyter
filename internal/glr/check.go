package glr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Gaurav-Gosain/gopyter/internal/kernel"
)

// Check lints a glr cell with `golars lint`, without running it, for the
// AI assistant: before, the notebook's earlier glr cells, comes first so
// the frames they stash and load are known. Problems in the cell are
// reported like errors of Execute ("In[3]:2: ..."). Warnings about stashes
// that are never used are left out, since later cells may use them.
func (k *Kernel) Check(ctx context.Context, before, name, src string) (kernel.CheckResult, error) {
	bin := k.Bin
	if bin == "" {
		b, err := FindGolars()
		if err != nil {
			return kernel.CheckResult{}, fmt.Errorf("%w: %v", ErrNoGolars, err)
		}
		bin = b
	}
	code, err := k.expand(src)
	if err != nil {
		return kernel.CheckResult{Errors: name + ":" + strings.TrimPrefix(err.Error(), "line ")}, nil
	}
	doc := NewDoc(k.expandLoose(before), code)
	dir, err := os.MkdirTemp("", "gopyter-glr-check-*")
	if err != nil {
		return kernel.CheckResult{}, err
	}
	defer func() { _ = os.RemoveAll(dir) }() // a leftover temp file is harmless
	path := filepath.Join(dir, "cell.glr")
	if err := os.WriteFile(path, []byte(doc.Content), 0o644); err != nil {
		return kernel.CheckResult{}, err
	}
	lines := strings.Count(code, "\n") + 1
	// golars lint --json reports columns, dtypes and bad calls with
	// positions; older golars versions only have the plain format.
	out, err := runLint(ctx, bin, "lint", "--json", path)
	if err != nil {
		return kernel.CheckResult{}, err
	}
	if msgs, ok := lintJSONErrors(out, name, doc.Offset, lines); ok {
		return kernel.CheckResult{Errors: msgs}, nil
	}
	out, err = runLint(ctx, bin, "lint", path)
	if err != nil {
		return kernel.CheckResult{}, err
	}
	return kernel.CheckResult{Errors: lintErrors(out, path, name, doc.Offset, lines)}, nil
}

func runLint(ctx context.Context, bin string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = append(os.Environ(), "NO_COLOR=1")
	out, err := cmd.Output()
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if ee := (*exec.ExitError)(nil); err != nil && !errors.As(err, &ee) {
		return "", err
	}
	return string(out), nil
}

// lintFinding is one entry of `golars lint --json`.
type lintFinding struct {
	Line     int    `json:"line"`
	Column   int    `json:"column"`
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Message  string `json:"message"`
	Hint     string `json:"hint"`
}

// lintJSONErrors formats the errors of `golars lint --json` output that
// fall inside the checked cell. It reports false when out is not that
// JSON. Warnings (an unused stash) and missing files are left to the
// run: the check runs away from the notebook's data.
func lintJSONErrors(out, name string, offset, lines int) (string, bool) {
	var fs []lintFinding
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &fs); err != nil {
		return "", false
	}
	var errs []string
	for _, f := range fs {
		if f.Severity != "error" || f.Code == "missing-file" || f.Code == "unreadable-file" {
			continue
		}
		row := f.Line - offset
		if row < 1 || row > lines {
			continue
		}
		msg := fmt.Sprintf("%s:%d: %s", name, row, f.Message)
		if f.Hint != "" {
			msg += " (" + f.Hint + ")"
		}
		errs = append(errs, msg)
	}
	return strings.Join(errs, "\n"), true
}

// lintErrors keeps the lint messages about the cell's lines, positioned in
// the cell.
func lintErrors(out, path, name string, offset, lines int) string {
	var errs []string
	for l := range strings.SplitSeq(out, "\n") {
		rest, ok := strings.CutPrefix(l, path+":")
		if !ok {
			continue
		}
		num, msg, ok := strings.Cut(rest, ": ")
		n, err := strconv.Atoi(num)
		if !ok || err != nil || strings.HasSuffix(msg, "is never used") {
			continue
		}
		if row := n - offset; row >= 1 && row <= lines {
			errs = append(errs, fmt.Sprintf("%s:%d: %s", name, row, msg))
		}
	}
	return strings.Join(errs, "\n")
}

// expandLoose is expand for context: bad directives become blank lines.
func (k *Kernel) expandLoose(src string) string {
	lines := strings.Split(src, "\n")
	for i, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), "%") {
			if x, err := k.expand(l); err == nil {
				lines[i] = x
			} else {
				lines[i] = ""
			}
		}
	}
	return strings.Join(lines, "\n")
}
