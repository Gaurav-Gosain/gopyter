package glr

import (
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
)

// keywords are glr's structural words, after the grammar in golars'
// editors/tree-sitter-golars/grammar.js: statement options (as, on,
// by, every, ...) and the expression keywords (and, or, not, in,
// when/then/otherwise and the infix string tests).
var keywords = []string{
	"as", "on", "asc", "desc", "and", "or", "not", "in",
	"is_null", "is_not_null",
	"contains", "starts_with", "ends_with", "like", "not_like",
	"when", "then", "otherwise",
	"inner", "left", "cross",
	"every", "period", "offset", "by", "closed", "label",
	"backward", "forward", "nearest", "tolerance",
}

// namespaces are the expression namespaces of glr function calls, as in
// dt.year(ts) or ts.dt.year().
var namespaces = []string{"str", "dt", "list", "arr", "struct", "name", "bin", "cat"}

// Lexer highlights glr, golars' line-oriented script language: a command
// at the start of each line, then its arguments (columns and paths,
// strings, numbers, operators, col:op[:alias] aggregations, function
// and namespaced method calls, keyword arguments and lists in
// expressions), # comments and trailing-backslash continuations.
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
	names := make([]string, 0, len(Commands)*2)
	for _, c := range Commands {
		names = append(names, c.Name)
		for _, a := range c.Aliases {
			// `?` is matched by its own rule.
			if isIdent(a) {
				names = append(names, a)
			}
		}
	}
	return names
}

func isIdent(s string) bool {
	for i, r := range s {
		if r != '_' && (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (i == 0 || (r < '0' || r > '9') && r != '-') {
			return false
		}
	}
	return s != ""
}

const ident = `[A-Za-z_][A-Za-z0-9_]*`

// balanced matches the text of a call's arguments on one line, with
// one level of nested parentheses: what can sit between a call's `(`
// and one of its keyword arguments.
const balanced = `[^\n()]*(?:\([^\n()]*\)[^\n()]*)*`

func lexerRules() chroma.Rules {
	ns := `(?:` + joinAlt(namespaces) + `)`
	return chroma.Rules{
		"root": {
			{Pattern: `[ \t]+`, Type: chroma.Text},
			{Pattern: `\n`, Type: chroma.Text},
			{Pattern: `#[^\n]*`, Type: chroma.CommentSingle},
			{Pattern: `%%?[A-Za-z_]*[^\n]*`, Type: chroma.CommentPreproc},
			{Pattern: `\.?` + chroma.Words(``, `\b`, commandNames()...), Type: chroma.Keyword, Mutator: chroma.Push("args")},
			{Pattern: `\?(?=[ \t\n]|$)`, Type: chroma.Keyword, Mutator: chroma.Push("args")},
			{Pattern: `\.?[A-Za-z_][A-Za-z0-9_-]*`, Type: chroma.NameFunction, Mutator: chroma.Push("args")},
			{Pattern: `[^\n]+`, Type: chroma.Error},
		},
		"args": {
			{Pattern: `\\[ \t]*\n`, Type: chroma.Punctuation},
			{Pattern: `\n`, Type: chroma.Text, Mutator: chroma.Pop(1)},
			{Pattern: `[ \t]+`, Type: chroma.Text},
			{Pattern: `#[^\n]*`, Type: chroma.CommentSingle},
			{Pattern: `"(\\.|[^"\\\n])*"`, Type: chroma.LiteralString},
			{Pattern: `'(\\.|[^'\\\n])*'`, Type: chroma.LiteralString},
			// name=value keyword argument inside a call: cut(x, [0], labels=[...]).
			{Pattern: `(?<=\(` + balanced + `)(?<=[(,][ \t]*)(` + ident + `)([ \t]*)(=)(?!=)`,
				Type: chroma.ByGroups(chroma.NameAttribute, chroma.Text, chroma.Operator)},
			{Pattern: chroma.Words(``, `\b`, keywords...), Type: chroma.KeywordReserved},
			{Pattern: `(true|false|null)\b`, Type: chroma.KeywordConstant},
			{Pattern: `(` + ident + `)(:)(` + ident + `)((?::` + ident + `)?)`,
				Type: chroma.ByGroups(chroma.Name, chroma.Punctuation, chroma.NameBuiltin, chroma.NameVariable)},
			// dt.year(ts): a namespaced function.
			{Pattern: `(` + ns + `)(\.)(` + ident + `)(?=[ \t]*\()`,
				Type: chroma.ByGroups(chroma.NameNamespace, chroma.Punctuation, chroma.NameFunction)},
			// ts.dt.year(): a namespaced method.
			{Pattern: `(\.)(` + ns + `)(\.)(` + ident + `)`,
				Type: chroma.ByGroups(chroma.Punctuation, chroma.NameNamespace, chroma.Punctuation, chroma.NameFunction)},
			// x.round(2), price.sum: a method.
			{Pattern: `(\.)(` + ident + `)`, Type: chroma.ByGroups(chroma.Punctuation, chroma.NameFunction)},
			{Pattern: `(` + ident + `)(?=[ \t]*\()`, Type: chroma.NameFunction},
			// A column with a method or namespace after it (ts.dt.year()).
			{Pattern: ident + `(?=\.(?:` + ns + `\.|` + ident + `[ \t]*\())`, Type: chroma.Name},
			// Durations: 30m, 1h30m, 1mo, 3i.
			{Pattern: `-?(?:[0-9]+(?:ns|us|ms|mo|[smhdwyqi]))+\b`, Type: chroma.LiteralNumber},
			{Pattern: `-?[0-9]+(\.[0-9]+)?([eE][-+]?[0-9]+)?\b`, Type: chroma.LiteralNumber},
			{Pattern: `[A-Za-z_][A-Za-z0-9_./:-]*`, Type: chroma.Name},
			{Pattern: `(==|!=|<=|>=|//|\*\*|[<>=+\-*/%!&|])`, Type: chroma.Operator},
			{Pattern: `[()\[\],.]`, Type: chroma.Punctuation},
			{Pattern: `[^\s]`, Type: chroma.Text},
		},
	}
}

func joinAlt(words []string) string {
	out := ""
	for i, w := range words {
		if i > 0 {
			out += "|"
		}
		out += w
	}
	return out
}
