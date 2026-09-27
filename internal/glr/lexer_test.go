package glr

import (
	"strings"
	"testing"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
)

func tokens(t *testing.T, src string) []chroma.Token {
	t.Helper()
	it, err := chroma.Coalesce(Lexer).Tokenise(nil, src)
	if err != nil {
		t.Fatal(err)
	}
	var out []chroma.Token
	for tok := it(); tok != chroma.EOF; tok = it() {
		if strings.TrimSpace(tok.Value) != "" {
			out = append(out, tok)
		}
	}
	return out
}

func TestLexer(t *testing.T) {
	src := `# orders by region
load data/orders.csv as orders
filter discount is_not_null and region == "eu" \
  and unit_price > 4.5
groupby region unit_price:sum:total
with ratio = col("a").abs() / 2
%export orders
frob x
`
	want := []struct {
		value string
		typ   chroma.TokenType
	}{
		{"# orders by region", chroma.CommentSingle},
		{"load", chroma.Keyword},
		{"data/orders.csv", chroma.Name},
		{"as", chroma.KeywordReserved},
		{"orders", chroma.Name},
		{"filter", chroma.Keyword},
		{"discount", chroma.Name},
		{"is_not_null", chroma.KeywordReserved},
		{"and", chroma.KeywordReserved},
		{"region", chroma.Name},
		{"==", chroma.Operator},
		{`"eu"`, chroma.LiteralString},
		{`\`, chroma.Punctuation},
		{"and", chroma.KeywordReserved},
		{"unit_price", chroma.Name},
		{">", chroma.Operator},
		{"4.5", chroma.LiteralNumber},
		{"groupby", chroma.Keyword},
		{"region", chroma.Name},
		{"unit_price", chroma.Name},
		{":", chroma.Punctuation},
		{"sum", chroma.NameBuiltin},
		{":total", chroma.NameVariable},
		{"with", chroma.Keyword},
		{"ratio", chroma.Name},
		{"=", chroma.Operator},
		{"col", chroma.NameFunction},
		{"(", chroma.Punctuation},
		{`"a"`, chroma.LiteralString},
		{").", chroma.Punctuation},
		{"abs", chroma.NameFunction},
		{"()", chroma.Punctuation},
		{"/", chroma.Operator},
		{"2", chroma.LiteralNumber},
		{"%export orders", chroma.CommentPreproc},
		{"frob", chroma.NameFunction},
		{"x", chroma.Name},
	}
	got := tokens(t, src)
	for i, w := range want {
		if i >= len(got) {
			t.Fatalf("missing tokens from %d (%q)", i, w.value)
		}
		g := got[i]
		if strings.TrimSpace(g.Value) != w.value || g.Type != w.typ {
			t.Fatalf("token %d: got %q %s, want %q %s", i, g.Value, g.Type, w.value, w.typ)
		}
	}
}

func TestLexerRegistered(t *testing.T) {
	for _, name := range []string{"glr", "golars"} {
		if lexers.Get(name) == nil {
			t.Errorf("lexers.Get(%q) is nil", name)
		}
	}
	if lexers.Match("x.glr") == nil {
		t.Error("*.glr isn't matched")
	}
	// Commands that share a prefix with a longer one.
	if got := tokens(t, "sum_all\n"); got[0].Value != "sum_all" || got[0].Type != chroma.Keyword {
		t.Fatalf("%+v", got)
	}
}
