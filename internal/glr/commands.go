package glr

// Command is a glr command, for completion and symbol info when
// golars-lsp isn't available.
type Command struct {
	Name, Signature, Summary string
	// ArgKind hints what the first argument is: "path", "column",
	// "frame", "count" or empty.
	ArgKind string
	// Aliases are other spellings of the command.
	Aliases []string
}

// Commands lists the glr commands. It mirrors script.Commands in golars
// (script/spec.go) at the time of writing; golars-lsp, when installed,
// knows the commands of the golars version in use.
var Commands = []Command{
	{Name: "load", Signature: "load <path> [as NAME]", Summary: "Load a csv/tsv/parquet/arrow/json/ndjson file as the focused frame (or staged under NAME).", ArgKind: "path"},
	{Name: "use", Signature: "use <NAME>", Summary: "Switch focus to a clone of a named frame.", ArgKind: "frame"},
	{Name: "stash", Signature: "stash <NAME>", Summary: "Snapshot the current focus as NAME for later `.use`.", ArgKind: ""},
	{Name: "frames", Signature: "frames", Summary: "List loaded frames.", ArgKind: ""},
	{Name: "drop_frame", Signature: "drop_frame <NAME>", Summary: "Release a named frame.", ArgKind: "frame"},
	{Name: "save", Signature: "save <path>", Summary: "Collect the focused pipeline and write it to disk.", ArgKind: "path", Aliases: []string{"write"}},
	{Name: "show", Signature: "show [N]", Summary: "Collect and print the first N rows (default 10). Same as head.", ArgKind: "count"},
	{Name: "ishow", Signature: "ishow", Summary: "Open the focused pipeline in the interactive browse TUI.", ArgKind: "", Aliases: []string{"browse"}},
	{Name: "schema", Signature: "schema", Summary: "Print column names and dtypes.", ArgKind: ""},
	{Name: "describe", Signature: "describe [col...]", Summary: "Per-column summary stats (count, null_count, mean, std, min, quartiles, max).", ArgKind: "column"},
	{Name: "head", Signature: "head [N]", Summary: "Collect and print first N rows (default 10).", ArgKind: "count"},
	{Name: "tail", Signature: "tail [N]", Summary: "Collect and print last N rows (default 10).", ArgKind: "count"},
	{Name: "select", Signature: "select <col|name = expr>[, ...]", Summary: "Project columns or expressions (lazy).", ArgKind: "column"},
	{Name: "drop", Signature: "drop <col>[,<col>...]", Summary: "Drop columns (lazy). Columns may be separated by commas or spaces.", ArgKind: "column"},
	{Name: "filter", Signature: "filter <predicate>", Summary: "Filter rows by a predicate (lazy).", ArgKind: "column"},
	{Name: "sort", Signature: "sort <col> [asc|desc] [<col> [asc|desc]]...", Summary: "Sort by one or more columns (lazy). Nulls sort first, as in polars.", ArgKind: "column"},
	{Name: "limit", Signature: "limit <N>", Summary: "Keep the first N rows (lazy).", ArgKind: "count"},
	{Name: "groupby", Signature: "groupby <k1,k2,...> <col:op[:alias] | name=expr>...", Summary: "Group by KEYS and aggregate.", ArgKind: "column"},
	{Name: "group_by_dynamic", Signature: "group_by_dynamic <time_col> every <dur> [period <dur>] [offset <dur>] [by <keys>] [closed <c>] [label <l>] <agg>...", Summary: "Group rows into time windows of TIME_COL and aggregate (lazy).", ArgKind: "column", Aliases: []string{"groupby_dynamic"}},
	{Name: "join", Signature: "join <path|NAME> on <key> [inner|left|cross]", Summary: "Join focus with a file or named frame on KEY.", ArgKind: "frame"},
	{Name: "join_asof", Signature: "join_asof <path|NAME> on <key> [by <cols>] [backward|forward|nearest] [tolerance <t>]", Summary: "Join each row to the nearest earlier (or later) row of another frame by KEY.", ArgKind: "frame"},
	{Name: "explain", Signature: "explain", Summary: "Print logical plan, optimiser trace, optimised plan.", ArgKind: ""},
	{Name: "explain_tree", Signature: "explain_tree", Summary: "Explain rendered as a box-drawn tree.", ArgKind: "", Aliases: []string{"tree"}},
	{Name: "graph", Signature: "graph", Summary: "Styled plan tree with colour coding per node kind.", ArgKind: "", Aliases: []string{"show_graph"}},
	{Name: "mermaid", Signature: "mermaid", Summary: "Emit the plan as a Mermaid flowchart.", ArgKind: ""},
	{Name: "collect", Signature: "collect", Summary: "Materialise lazy pipeline back into focused source.", ArgKind: ""},
	{Name: "reset", Signature: "reset", Summary: "Discard the lazy pipeline; keep the source.", ArgKind: ""},
	{Name: "source", Signature: "source <path>", Summary: "Run another .glr script inline.", ArgKind: "path"},
	{Name: "timing", Signature: "timing", Summary: "Toggle per-statement timing output.", ArgKind: ""},
	{Name: "info", Signature: "info", Summary: "Runtime info: Go version, heap, uptime.", ArgKind: ""},
	{Name: "clear", Signature: "clear", Summary: "Clear the screen.", ArgKind: ""},
	{Name: "help", Signature: "help", Summary: "Print the command reference.", ArgKind: "", Aliases: []string{"h", "?"}},
	{Name: "exit", Signature: "exit", Summary: "Quit the REPL. In a script, stop running further statements.", ArgKind: "", Aliases: []string{"quit", "q"}},
	{Name: "reverse", Signature: "reverse", Summary: "Reverse the row order of the focus (lazy).", ArgKind: ""},
	{Name: "sample", Signature: "sample <N> [seed]", Summary: "Replace the focus with N rows sampled without replacement.", ArgKind: "count"},
	{Name: "shuffle", Signature: "shuffle [seed]", Summary: "Randomly reorder every row of the focus (seed defaults to 42).", ArgKind: ""},
	{Name: "unique", Signature: "unique", Summary: "Drop duplicate rows over all columns (lazy).", ArgKind: ""},
	{Name: "null_count", Signature: "null_count", Summary: "Per-column null count as a single-row frame.", ArgKind: "", Aliases: []string{"null-count"}},
	{Name: "glimpse", Signature: "glimpse [N]", Summary: "Compact peek at the first N rows (default 5).", ArgKind: "count"},
	{Name: "size", Signature: "size", Summary: "Estimated Arrow byte size of the current pipeline output.", ArgKind: ""},
	{Name: "cast", Signature: "cast <col> <dtype>", Summary: "Cast a column to a dtype: i8 to i64, u8 to u64, f32, f64, bool, str, date, datetime[ms], categorical, ...", ArgKind: "column"},
	{Name: "fill_null", Signature: "fill_null <value>", Summary: "Replace nulls across all compatible columns.", ArgKind: "", Aliases: []string{"fillnull"}},
	{Name: "drop_null", Signature: "drop_null [col...]", Summary: "Drop rows with nulls in any (or the listed) columns.", ArgKind: "column", Aliases: []string{"dropnull"}},
	{Name: "rename", Signature: "rename <old> as <new>", Summary: "Rename a single column.", ArgKind: "column"},
	{Name: "sum", Signature: "sum <col>", Summary: "Print the sum of the given column.", ArgKind: "column"},
	{Name: "mean", Signature: "mean <col>", Summary: "Print the mean of the given column.", ArgKind: "column", Aliases: []string{"avg"}},
	{Name: "min", Signature: "min <col>", Summary: "Print the min of the given column.", ArgKind: "column"},
	{Name: "max", Signature: "max <col>", Summary: "Print the max of the given column.", ArgKind: "column"},
	{Name: "median", Signature: "median <col>", Summary: "Print the median of the given column.", ArgKind: "column"},
	{Name: "std", Signature: "std <col>", Summary: "Print the sample standard deviation of the given column.", ArgKind: "column"},
	{Name: "with_row_index", Signature: "with_row_index <name> [offset]", Summary: "Prepend an int64 row-index column.", ArgKind: ""},
	{Name: "pwd", Signature: "pwd", Summary: "Print the REPL working directory.", ArgKind: ""},
	{Name: "ls", Signature: "ls [path]", Summary: "List files in the given directory (default: cwd).", ArgKind: "path"},
	{Name: "cd", Signature: "cd [path]", Summary: "Change the REPL working directory (default: home).", ArgKind: "path"},
	{Name: "sum_horizontal", Signature: "sum_horizontal <out> [col...]", Summary: "Append a row-wise sum column named <out> across selected (or all numeric) columns.", ArgKind: "column"},
	{Name: "mean_horizontal", Signature: "mean_horizontal <out> [col...]", Summary: "Append a row-wise mean column (nulls skipped).", ArgKind: "column"},
	{Name: "min_horizontal", Signature: "min_horizontal <out> [col...]", Summary: "Append a row-wise min column.", ArgKind: "column"},
	{Name: "max_horizontal", Signature: "max_horizontal <out> [col...]", Summary: "Append a row-wise max column.", ArgKind: "column"},
	{Name: "all_horizontal", Signature: "all_horizontal <out> [col...]", Summary: "Append a row-wise boolean AND column across selected (or all boolean) columns.", ArgKind: "column"},
	{Name: "any_horizontal", Signature: "any_horizontal <out> [col...]", Summary: "Append a row-wise boolean OR column.", ArgKind: "column"},
	{Name: "sum_all", Signature: "sum_all", Summary: "One-row frame with the sum of every numeric column.", ArgKind: ""},
	{Name: "mean_all", Signature: "mean_all", Summary: "One-row frame with the mean of every numeric column.", ArgKind: ""},
	{Name: "min_all", Signature: "min_all", Summary: "One-row frame with the min of every numeric column.", ArgKind: ""},
	{Name: "max_all", Signature: "max_all", Summary: "One-row frame with the max of every numeric column.", ArgKind: ""},
	{Name: "std_all", Signature: "std_all", Summary: "One-row frame with the sample std of every numeric column.", ArgKind: ""},
	{Name: "var_all", Signature: "var_all", Summary: "One-row frame with the sample variance of every numeric column.", ArgKind: ""},
	{Name: "median_all", Signature: "median_all", Summary: "One-row frame with the median of every numeric column.", ArgKind: ""},
	{Name: "count_all", Signature: "count_all", Summary: "One-row frame with the non-null count of every column.", ArgKind: ""},
	{Name: "null_count_all", Signature: "null_count_all", Summary: "One-row frame with the null count of every column.", ArgKind: ""},
	{Name: "with", Signature: "with <name> = <expression>", Summary: "Append a derived column via the expression language.", ArgKind: ""},
	{Name: "unnest", Signature: "unnest <col>", Summary: "Project the fields of a struct-typed column as top-level columns.", ArgKind: "column"},
	{Name: "explode", Signature: "explode <col>[,<col>...]", Summary: "Fan out each element of a list-typed column into its own row.", ArgKind: "column"},
	{Name: "to_dummies", Signature: "to_dummies [col...] [drop_first]", Summary: "One-hot encode columns into u8 indicator columns named COL_VALUE.", ArgKind: "column"},
	{Name: "upsample", Signature: "upsample <col> <every>", Summary: "Interpolate a timestamp column at a regular interval.", ArgKind: "column"},
	{Name: "scan_csv", Signature: "scan_csv <path> [as NAME]", Summary: "Register a lazy scan of a CSV file (push-down friendly).", ArgKind: "path"},
	{Name: "scan_parquet", Signature: "scan_parquet <path> [as NAME]", Summary: "Register a lazy scan of a Parquet file.", ArgKind: "path"},
	{Name: "scan_ipc", Signature: "scan_ipc <path> [as NAME]", Summary: "Register a lazy scan of an Arrow IPC file.", ArgKind: "path", Aliases: []string{"scan_arrow"}},
	{Name: "scan_ndjson", Signature: "scan_ndjson <path> [as NAME]", Summary: "Register a lazy scan of an NDJSON file.", ArgKind: "path", Aliases: []string{"scan_jsonl"}},
	{Name: "scan_json", Signature: "scan_json <path> [as NAME]", Summary: "Register a lazy scan of a JSON (array) file.", ArgKind: "path"},
	{Name: "scan_auto", Signature: "scan_auto <path> [as NAME]", Summary: "Register a lazy scan inferring the reader from the file extension.", ArgKind: "path"},
	{Name: "fill_nan", Signature: "fill_nan <value>", Summary: "Replace NaN with VALUE in every float column (frame-level).", ArgKind: ""},
	{Name: "forward_fill", Signature: "forward_fill [limit]", Summary: "Forward-fill nulls (at most LIMIT consecutive, 0 = unlimited).", ArgKind: "", Aliases: []string{"ff"}},
	{Name: "backward_fill", Signature: "backward_fill [limit]", Summary: "Backward-fill nulls. Trailing nulls stay null.", ArgKind: "", Aliases: []string{"bf"}},
	{Name: "top_k", Signature: "top_k <K> <col>", Summary: "Replace the focus with the K rows holding the largest values in COL (descending).", ArgKind: "count"},
	{Name: "bottom_k", Signature: "bottom_k <K> <col>", Summary: "Replace the focus with the K rows holding the smallest values in COL.", ArgKind: "count"},
	{Name: "transpose", Signature: "transpose [header_col] [prefix]", Summary: "Transpose the focus (numeric/bool columns only).", ArgKind: ""},
	{Name: "unpivot", Signature: "unpivot <id_cols> [val_cols]", Summary: "Reshape wide to long. ID_COLS/VAL_COLS are comma-separated lists.", ArgKind: "column", Aliases: []string{"melt"}},
	{Name: "partition_by", Signature: "partition_by <keys>", Summary: "Split the focus into one frame per distinct key combination; prints a summary.", ArgKind: "column"},
	{Name: "skew", Signature: "skew <col>", Summary: "Print the skewness of COL (polars-default, biased).", ArgKind: "column"},
	{Name: "kurtosis", Signature: "kurtosis <col>", Summary: "Print the excess kurtosis of COL.", ArgKind: "column"},
	{Name: "approx_n_unique", Signature: "approx_n_unique <col>", Summary: "HyperLogLog estimate of the number of distinct values in COL.", ArgKind: "column", Aliases: []string{"approx_nunique"}},
	{Name: "corr", Signature: "corr <col1> <col2>", Summary: "Print the Pearson correlation between two numeric columns.", ArgKind: "column"},
	{Name: "cov", Signature: "cov <col1> <col2>", Summary: "Print the sample covariance (ddof=1) between two numeric columns.", ArgKind: "column"},
	{Name: "pivot", Signature: "pivot <index_cols> <on_col> <values_col> [agg]", Summary: "Long-to-wide pivot. agg: first/sum/mean/min/max/count (default first).", ArgKind: "column"},
}

// findCommand resolves a command name or alias.
func findCommand(name string) *Command {
	for i := range Commands {
		c := &Commands[i]
		if c.Name == name {
			return c
		}
		for _, a := range c.Aliases {
			if a == name {
				return c
			}
		}
	}
	return nil
}
