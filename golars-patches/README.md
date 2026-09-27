# golars patches

Changes to [golars](https://github.com/Gaurav-Gosain/golars) that gopyter's
golars support uses. They are written against golars `ffe73a0` and apply in
order:

```sh
cd /path/to/golars
git apply /path/to/gopyter/golars-patches/*.patch
```

| Patch | What it adds | Why gopyter needs it |
|---|---|---|
| `001-table-mimebundle.patch` | `internal/reprtable` (a display-ready table, its HTML and JSON), `DataFrame.MimeBundle`/`MimeBundleWith`/`HTML`/`HTMLWith`, `Series.MimeBundle`/`HTML`, `dataframe.TableMIME`; `jupyter/render.HTMLWith` now uses it | Go cells show a golars value by duck typing on `MimeBundle()`. The dataframe package can't import `jupyter/render` (cycle), so the table and its HTML live in a leaf package both use. |
| `002-kernel-host-structured-tables.patch` | an optional `"structured": true` request field for `golars kernel-host`; replies then add `outputs` (stdout text and tables in order) and `table` (the auto-displayed frame) | glr cells draw real tables, with dtypes, nulls and shapes, instead of parsing ASCII. Without the flag the reply is unchanged, so golars-kernel and older gopyter builds keep working. |

Without these patches gopyter still works with golars: glr cells show
golars' text and HTML output, and Go cells show DataFrames as text.
