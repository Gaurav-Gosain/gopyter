package glr

import (
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
)

// keywords are glr's structural words, after the grammar in golars'
// editors/tree-sitter-golars/grammar.js.
var keywords = []string{
	"as", "on", "asc", "desc", "and", "or", "is_null", "is_not_null",
	"inner", "left", "cross",
}

// Lexer highlights glr, golars' line-oriented script language: a command
// at the start of each line, then its arguments (columns and paths,
// strings, numbers, operators, col:op[:alias] aggregations, method calls
// in expressions), # comments and trailing-backslash continuations.
// gopyter's %export/%import directives and %% cell magics are shown as
// preprocessor lines.
var Lexer = lexers.Register(chroma.MustNewLexer(
	&chroma.Config{
		Name:      "glr",
		Aliases:   []string{"golars"},
		Filenames: []string{"*.glr"},
		MimeTypes: []string{"text/x-glr"},
	},
	lexerRules,
))

func commandNames() []string {
	names := make([]string, 0, len(Commands)+8)
	for _, c := range Commands {
		names = append(names, c.Name)
	}
	// Aliases the grammar knows that aren't in the command table.
	return append(names, "avg", "scan_arrow", "scan_jsonl", "melt")
}

func lexerRules() chroma.Rules {
	return chroma.Rules{
		"root": {
			{Pattern: `[ \t]+`, Type: chroma.Text},
			{Pattern: `\n`, Type: chroma.Text},
			{Pattern: `#[^\n]*`, Type: chroma.CommentSingle},
			{Pattern: `%%?[A-Za-z_]*[^\n]*`, Type: chroma.CommentPreproc},
			{Pattern: `\.?` + chroma.Words(``, `\b`, commandNames()...), Type: chroma.Keyword, Mutator: chroma.Push("args")},
			{Pattern: `\.?[A-Za-z_][A-Za-z0-9_]*`, Type: chroma.NameFunction, Mutator: chroma.Push("args")},
			{Pattern: `[^\n]+`, Type: chroma.Error},
		},
		"args": {
			{Pattern: `\\[ \t]*\n`, Type: chroma.Punctuation},
			{Pattern: `\n`, Type: chroma.Text, Mutator: chroma.Pop(1)},
			{Pattern: `[ \t]+`, Type: chroma.Text},
			{Pattern: `#[^\n]*`, Type: chroma.CommentSingle},
			{Pattern: `"(\\.|[^"\\\n])*"`, Type: chroma.LiteralString},
			{Pattern: chroma.Words(``, `\b`, keywords...), Type: chroma.KeywordReserved},
			{Pattern: `(true|false|null)\b`, Type: chroma.KeywordConstant},
			{Pattern: `([A-Za-z_][A-Za-z0-9_]*)(:)([A-Za-z_][A-Za-z0-9_]*)((?::[A-Za-z_][A-Za-z0-9_]*)?)`,
				Type: chroma.ByGroups(chroma.Name, chroma.Punctuation, chroma.NameBuiltin, chroma.NameVariable)},
			{Pattern: `([A-Za-z_][A-Za-z0-9_]*)(\()`, Type: chroma.ByGroups(chroma.NameFunction, chroma.Punctuation)},
			{Pattern: `(\.)([A-Za-z_][A-Za-z0-9_]*)(\()`, Type: chroma.ByGroups(chroma.Punctuation, chroma.NameFunction, chroma.Punctuation)},
			{Pattern: `-?[0-9]+(\.[0-9]+)?([eE][-+]?[0-9]+)?\b`, Type: chroma.LiteralNumber},
			{Pattern: `[A-Za-z_][A-Za-z0-9_./:-]*`, Type: chroma.Name},
			{Pattern: `(==|!=|<=|>=|[<>=+\-*/%!&|])`, Type: chroma.Operator},
			{Pattern: `[(),.]`, Type: chroma.Punctuation},
			{Pattern: `[^\s]`, Type: chroma.Text},
		},
	}
}
