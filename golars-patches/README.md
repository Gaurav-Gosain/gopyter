# golars patches

Changes to [golars](https://github.com/Gaurav-Gosain/golars) that gopyter's
golars support uses. Patches 001 and 002 were made against golars `ffe73a0` with the
reorganized `cmd/golars` (dispatch, `cmds_*.go`) that was in the working tree
on 2026-09-27; 003 and 004 against `5dd7b6a` (they are commits on
golars' `feat/notebook-frames` branch). They apply in order:

```sh
cd /path/to/golars
git apply /path/to/gopyter/golars-patches/*.patch
```

| Patch | What it adds | Why gopyter needs it |
|---|---|---|
| `001-table-mimebundle.patch` | `internal/reprtable` (a display-ready table, its HTML and JSON), `DataFrame.MimeBundle`/`MimeBundleWith`/`HTML`/`HTMLWith`, `Series.MimeBundle`/`HTML`, `dataframe.TableMIME`; `jupyter/render.HTMLWith` now uses it | Go cells show a golars value by duck typing on `MimeBundle()`. The dataframe package can't import `jupyter/render` (cycle), so the table and its HTML live in a leaf package both use. |
| `002-kernel-host-structured-tables.patch` | an optional `"structured": true` request field for `golars kernel-host`; replies then add `outputs` (stdout text and tables in order) and `table` (the auto-displayed frame) | glr cells draw real tables, with dtypes, nulls and shapes, instead of parsing ASCII. Without the flag the reply is unchanged, so golars-kernel and older gopyter builds keep working. |
| `003-gob-dataframe-series.patch` | `GobEncode`/`GobDecode` for `*DataFrame` and `*Series` as an Arrow IPC stream (`internal/arrowgob`), and a `LazyFrame.GobEncode` that explains why a plan can't be saved | Go cells keep `:=` variables with gob. Top-level frames use Arrow IPC files directly, but a Series or frames inside structs and maps go through gob. |
| `004-kernel-host-frame-ops.patch` | `op=frames` (frames with generation numbers), `op=export` and `op=import` for `golars kernel-host` | Frames are shared between Go and glr cells by name without running commands in the session. A host without the ops turns sharing off with a note. |

Without these patches gopyter still works with golars: glr cells show
golars' text and HTML output, and Go cells show DataFrames as text.
