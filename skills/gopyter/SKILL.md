---
name: gopyter
description: Create, edit, run and validate Go and golars notebooks (.ipynb) with gopyter, a Jupyter-style notebook for Go that runs in the terminal and is compatible with GoNB and golars-kernel. Use when the user wants a Go notebook, a golars (.glr) DataFrame notebook, asks to explore or prototype Go code cell by cell, mentions gopyter, GoNB or golars, or needs to execute a .ipynb with Go or glr cells headlessly (for example in CI).
---

# gopyter

gopyter is a Jupyter-style notebook for **Go**. It stores notebooks as standard
`.ipynb` files (nbformat v4) with a [GoNB](https://github.com/janpfeifer/gonb)
kernelspec. Each cell is compiled with the real Go toolchain. Cells can also
be **glr** cells: [golars](https://github.com/Gaurav-Gosain/golars) DataFrame
scripts (see [golars notebooks](#golars-notebooks)). It has two entry points:

- `gopyter notes.ipynb`: the interactive terminal UI, for humans.
- `gopyter run notes.ipynb`: headless execution, for agents, scripts and CI.

**As an agent, write the `.ipynb` JSON directly and check it with
`gopyter run`.** The TUI needs an interactive terminal. Open it only if the
user asks, and then tell them the command to run instead of starting it
yourself.

## Install and check

```sh
gopyter --version || curl -fsSL https://raw.githubusercontent.com/Gaurav-Gosain/gopyter/master/install.sh | bash
# or: go install github.com/Gaurav-Gosain/gopyter@latest
go version            # required: cells are built with the Go toolchain on PATH
```

`gopls` is optional. It's only used for completion in the TUI.

## Commands

| Command | Purpose |
|---|---|
| `gopyter [file.ipynb]` | open (or create) a notebook in the TUI |
| `gopyter run file.ipynb` | run every code cell in order and print the outputs |
| `gopyter run file.ipynb --save` | also write outputs and execution counts into the file |
| `gopyter run file.ipynb --fail-fast` | stop at the first failing cell |
| `gopyter new.ipynb --lang glr` | create a golars notebook (glr cells by default) |
| `gopyter run script.glr` | run a golars script as a notebook of its blank-line separated blocks |
| `--workdir DIR` | keep the kernel workspace (a Go module) in `DIR` instead of a temporary directory |
| `gopyter themes`, `gopyter model` | list themes / show or set the optional AI model (TUI features) |

`gopyter run` exits non-zero if any cell fails. Compile errors are reported
as `In[n]:line:col: message`, where `n` is the position of the code cell
(markdown cells aren't counted) and `line` is the line within that cell.
Output looks like this:

```
In[2]
  │ p := Point{3, 4}
  │ p.Dist()
=> 5
✓ 294ms
```

`=>` is the cell's result, `✓`/`✗` its status. stdout and stderr are printed as is.

## How cells execute

Knowing these rules is the key to writing cells that work:

1. **Top-level declarations persist** across cells: `func`, `type`, `var`,
   `const` and `import`. Re-running a cell replaces what it declared.
2. **All other statements run inside a generated `func main()`**, and each cell
   runs in a **fresh process**. Goroutines, open files and connections don't
   survive between cells.
3. **Top-level `:=` variables carry over** to later cells. They're saved with
   `encoding/gob` when a cell ends and restored when the next one starts, so
   only gob-encodable values carry over (for structs, only exported fields).
   Channels, mutexes, `*regexp.Regexp` and funcs don't carry over. Pointers
   come back as copies. Variables declared inside blocks stay local.
4. **A persisted `var x = expensive()` is re-evaluated in every later cell.**
   Compute it once with `nb.Cache("key", func() T {...})` or
   `nb.CacheErr("key", func() (T, error) {...})`.
5. **A trailing bare expression is displayed** as the cell's result (`=> ...`).
6. **Imports are automatic**, for the standard library (`fmt`, `strings`,
   `math`, `slices`, `os`, `time`...) and for `nb`. An explicit `import` is
   only needed for packages whose name is ambiguous or for third-party modules.
   Third-party modules are fetched on first use (`go get`). Unused imports are
   not an error.
7. **Don't write `package main` or `func main()`.** A cell can't define
   `func main()` and also contain top-level statements. `func init()` is allowed.
8. Programs, `%%writefile` and `%%sh` run in **the notebook's directory**, so
   relative paths resolve next to the `.ipynb`. `!cmd` runs in the kernel
   workspace (a Go module), which is where `!go get pkg@version` belongs.
9. Tests go in a cell that starts with `%test`. That cell is built with
   `go test`, and its `TestXxx`/`BenchmarkXxx` functions run.

## Rich output: package `nb` (no import needed)

| Call | Shows |
|---|---|
| `nb.Display(v...)` | values; an `image.Image` is drawn |
| `nb.DisplayMarkdown(s)` | rendered markdown (good for tables and reports) |
| `nb.DisplayPNG(data)` | PNG bytes |
| `nb.DisplayID(id, v...)`, `nb.DisplayMarkdownID(id, s)` | replace the earlier output with the same id (animation, progress) |
| `nb.Cache`, `nb.CacheErr` | compute a value once per kernel (`%cache clear key` resets it) |

GoNB's packages also work: `github.com/janpfeifer/gonb/gonbui` (`DisplayHTML`,
`UpdateHTML`...), `.../gonbui/widgets` (`Slider`, `Button`, `Select`),
`.../gonbui/dom` and `.../gonbui/comms`. They ship with gopyter, so there's
nothing to download. In `gopyter run` nobody can operate the widgets, so they
report "done" immediately. Keep widget cells out of notebooks meant for CI.

## Cell commands and magics

Commands are lines within a code cell. A cell magic (`%%...`) must be the
first line and takes the rest of the cell.

| Command | Effect |
|---|---|
| `!cmd` | shell command in the kernel workspace (`!go get pkg@v1`); a trailing `\` continues the line |
| `%%writefile [-a] path` | write the rest of the cell to a file |
| `%%sh` / `%%bash` / `%%script cmd` | run the rest of the cell as a script |
| `%env K=V`, `%args a b` | environment variables / program arguments |
| `%% [args]` | call `flag.Parse()` first, with these arguments. Variables in that cell stay local |
| `%exec fn [args]` | run `fn()` after parsing flags |
| `%test [flags]` | run this cell's tests and benchmarks with `go test` |
| `%goflags [flags]` | extra `go build` flags (`-race`, `-tags=x`); `%goflags ""` clears them |
| `%capture [-a] file` | also write the cell's output to a file |
| `%ls`, `%rm name...`, `%reset` | list, forget or reset all declarations |
| `%cache`, `%cache clear [key]` | list or clear cached values |
| `%workspace`, `%version`, `%help` | info |

GoNB spellings also work: `//gonb:%...` and `!*cmd`.

## Creating a notebook

Write valid nbformat v4 JSON. Use this template and keep the `metadata`
block so the file also opens in Jupyter with the GoNB kernel:

```json
{
 "cells": [
  {
   "cell_type": "markdown",
   "id": "intro",
   "metadata": {},
   "source": [
    "# Points\n",
    "\n",
    "Distances in the plane."
   ]
  },
  {
   "cell_type": "code",
   "id": "types",
   "execution_count": null,
   "metadata": {},
   "outputs": [],
   "source": [
    "type Point struct{ X, Y float64 }\n",
    "\n",
    "func (p Point) Dist() float64 {\n",
    "\treturn math.Hypot(p.X, p.Y)\n",
    "}"
   ]
  },
  {
   "cell_type": "code",
   "id": "use",
   "execution_count": null,
   "metadata": {},
   "outputs": [],
   "source": [
    "pts := []Point{{3, 4}, {1, 1}}\n",
    "for _, p := range pts {\n",
    "\tfmt.Printf(\"%v -> %.2f\\n\", p, p.Dist())\n",
    "}\n",
    "len(pts)"
   ]
  }
 ],
 "metadata": {
  "kernelspec": {"display_name": "Go (gonb)", "language": "go", "name": "gonb"},
  "language_info": {"file_extension": ".go", "mimetype": "text/x-go", "name": "go"}
 },
 "nbformat": 4,
 "nbformat_minor": 5
}
```

Format rules:

- `source` is a list of lines. Every line ends in `\n` except the last. A
  single string is accepted too.
- Code cells need `"execution_count": null` and `"outputs": []`. Markdown
  cells have neither.
- Give every cell a unique short `id` (for example 12 hex characters). gopyter
  uses ids to keep the user's place when the file changes while it's open.
- Escape JSON properly: tabs as `\t` (use tabs for indentation, as gofmt
  does), quotes as `\"`, and backslashes as `\\`.
- Prefer generating the file with a script (such as Python's `json.dump`) over
  hand-writing JSON for anything but small notebooks.

Suggested structure:

1. A markdown title cell that says what the notebook does.
2. Declarations (types, helpers) in their own cells, before the cells that use them.
3. Short cells that each do one step and end with an expression, so every step
   shows a result.
4. `nb.DisplayMarkdown` for tables and summaries. Markdown cells between steps
   for the narrative.
5. Keep cells deterministic and offline where possible, so `gopyter run` works in CI.

## Workflow: create or change a notebook

1. Write or edit the `.ipynb` JSON. To change one cell, edit only its `source`
   and keep its `id`.
2. Validate it: `gopyter run file.ipynb --fail-fast`.
3. Fix the reported `In[n]:line:col` errors in the `n`-th code cell, then run again.
4. When everything passes, save the outputs if the user wants them stored:
   `gopyter run file.ipynb --save`. Otherwise leave outputs empty to keep
   diffs small.
5. Tell the user how to open it: `gopyter file.ipynb`.

If the user has the notebook open in gopyter, it reloads external edits within
about a second, so editing the file on disk is safe. If they have unsaved
changes, gopyter asks them which version to keep.

## Common mistakes

- `declared and not used`: Go's rules apply inside blocks and functions. Only
  top-level `:=` variables are exempt, because they're moved to package level.
- Relying on state that doesn't survive between processes (goroutines,
  channels, open handles). Recreate it in the cell that uses it.
- A `var` whose initializer has side effects or is slow runs again in every
  later cell. Use `nb.Cache`.
- Unexported struct fields are lost when a `:=` variable carries over. Export
  them, or re-derive the value.
- Interactive stdin or widgets in a notebook meant for `gopyter run`. The
  input ends immediately (headless `run` passes its own stdin), and widgets
  report done at once.

## golars notebooks

glr cells run [golars](https://github.com/Gaurav-Gosain/golars) scripts in one
long-lived `golars kernel-host` process, so frames persist across cells. They
need the `golars` binary: `$GOLARS_BIN`, next to the gopyter binary, or on
`PATH`. Completion and diagnostics in the TUI use `golars-lsp` when found
(`$GOLARS_LSP`, next to `$GOLARS_BIN` or gopyter, or on `PATH`).

- **Notebook language.** A notebook whose kernelspec is golars-kernel's
  defaults to glr cells, and opens in JupyterLab with golars-kernel:

  ```json
  "metadata": {
   "kernelspec": {"display_name": "golars (.glr)", "language": "golars", "name": "golars"},
   "language_info": {"codemirror_mode": {"name": "shell"}, "file_extension": ".glr",
                     "mimetype": "text/x-glr", "name": "golars", "pygments_lexer": "text"}
  }
  ```

  Any other kernelspec (GoNB's) means Go cells.
- **Per-cell language.** A cell in the other language says so in its
  metadata: `"metadata": {"gopyter": {"language": "glr"}}` (or `"go"`). A
  first line of `%%glr` or `%%go` also sets it, and wins over the metadata.
- **glr cells** are one command per line: `load orders.csv`, `filter qty > 1
  and discount is_not_null`, `with revenue = unit_price * qty`, `groupby
  region revenue:sum:total`, `sort total desc`, `join customers on customer`,
  `select a b c`, `stash NAME` / `use NAME`, `head 5`, `show`. Expressions
  call golars functions by snake_case name, as methods or functions
  (`ts.dt.year()` or `dt.year(ts)`, `x.round(2)`), with lists, keyword
  arguments, `//`, `%`, `**`, `in [...]` and `when ... then ... otherwise`.
  See golars' docs/scripting.md for the full language. A cell that
  changes the focused frame displays it as a table. Relative paths resolve in
  the notebook's directory. Errors look like `In[n]:line: message`, and a
  failing glr cell stops `gopyter run`.
- **Go cells with golars** import `github.com/Gaurav-Gosain/golars`. A trailing
  DataFrame or Series, or `nb.Display(df)`, is drawn as a table and saved as
  `text/html` (golars' `MimeBundle` method). Set `GOLARS_DIR` to a golars
  checkout to build against it offline. DataFrames don't carry over as `:=`
  variables; declare them as top-level `var`s (recomputed per cell).
- **Moving frames.** In a glr cell, `%export NAME` saves the focused frame and
  `%import NAME` loads one as the named frame `NAME`. In Go,
  `golars.ReadIPC(nb.BridgePath("NAME"))` and
  `golars.WriteIPC(df, nb.BridgePath("NAME"))` read and write the same Arrow
  IPC files, in the kernel workspace.

See `examples/golars.ipynb` and `examples/golars-go.ipynb`.

## TUI quick reference (for telling users)

`enter`/`esc` switch between edit and command mode. `ctrl+r` (or
`shift+enter`) runs the cell and moves to the next one; `A` runs all cells.
`a`/`b` insert a cell above/below, `m`/`y` convert to markdown/code, `l`
switches a code cell between go and glr, `dd` deletes, `ctrl+s` saves, `q` quits, and `?` lists every key. `V` toggles vim
bindings and `T` picks a theme.
