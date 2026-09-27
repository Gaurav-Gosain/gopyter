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

func TestLexerExpressions(t *testing.T) {
	src := `with y = dt.year(ts) // 2 ** 3 % 7
with n = name.str.to_uppercase()
select a, b = cut(x, [0, 10], labels=['lo', "hi"], left_closed=true)
filter k not in [1, 2] and v == null
with p = when a > 1 then 'big' otherwise x.round(2)
join_asof quotes on ts by sym backward tolerance 2m
groupby_dynamic ts every 1h n=amount.count()
to_dummies dept
`
	want := []struct {
		value string
		typ   chroma.TokenType
	}{
		{"with", chroma.Keyword}, {"y", chroma.Name}, {"=", chroma.Operator},
		{"dt", chroma.NameNamespace}, {".", chroma.Punctuation}, {"year", chroma.NameFunction},
		{"(", chroma.Punctuation}, {"ts", chroma.Name}, {")", chroma.Punctuation},
		{"//", chroma.Operator}, {"2", chroma.LiteralNumber}, {"**", chroma.Operator},
		{"3", chroma.LiteralNumber}, {"%", chroma.Operator}, {"7", chroma.LiteralNumber},

		{"with", chroma.Keyword}, {"n", chroma.Name}, {"=", chroma.Operator},
		{"name", chroma.Name}, {".", chroma.Punctuation}, {"str", chroma.NameNamespace},
		{".", chroma.Punctuation}, {"to_uppercase", chroma.NameFunction}, {"()", chroma.Punctuation},

		{"select", chroma.Keyword}, {"a", chroma.Name}, {",", chroma.Punctuation},
		{"b", chroma.Name}, {"=", chroma.Operator}, {"cut", chroma.NameFunction},
		{"(", chroma.Punctuation}, {"x", chroma.Name}, {",", chroma.Punctuation},
		{"[", chroma.Punctuation}, {"0", chroma.LiteralNumber}, {",", chroma.Punctuation},
		{"10", chroma.LiteralNumber}, {"],", chroma.Punctuation}, {"labels", chroma.NameAttribute},
		{"=", chroma.Operator}, {"[", chroma.Punctuation}, {"'lo'", chroma.LiteralString},
		{",", chroma.Punctuation}, {`"hi"`, chroma.LiteralString}, {"],", chroma.Punctuation},
		{"left_closed", chroma.NameAttribute}, {"=", chroma.Operator}, {"true", chroma.KeywordConstant},
		{")", chroma.Punctuation},

		{"filter", chroma.Keyword}, {"k", chroma.Name}, {"not", chroma.KeywordReserved},
		{"in", chroma.KeywordReserved}, {"[", chroma.Punctuation}, {"1", chroma.LiteralNumber},
		{",", chroma.Punctuation}, {"2", chroma.LiteralNumber}, {"]", chroma.Punctuation},
		{"and", chroma.KeywordReserved}, {"v", chroma.Name}, {"==", chroma.Operator},
		{"null", chroma.KeywordConstant},

		{"with", chroma.Keyword}, {"p", chroma.Name}, {"=", chroma.Operator},
		{"when", chroma.KeywordReserved}, {"a", chroma.Name}, {">", chroma.Operator},
		{"1", chroma.LiteralNumber}, {"then", chroma.KeywordReserved}, {"'big'", chroma.LiteralString},
		{"otherwise", chroma.KeywordReserved}, {"x", chroma.Name}, {".", chroma.Punctuation},
		{"round", chroma.NameFunction}, {"(", chroma.Punctuation}, {"2", chroma.LiteralNumber},
		{")", chroma.Punctuation},

		{"join_asof", chroma.Keyword}, {"quotes", chroma.Name}, {"on", chroma.KeywordReserved},
		{"ts", chroma.Name}, {"by", chroma.KeywordReserved}, {"sym", chroma.Name},
		{"backward", chroma.KeywordReserved}, {"tolerance", chroma.KeywordReserved}, {"2m", chroma.LiteralNumber},

		{"groupby_dynamic", chroma.Keyword}, {"ts", chroma.Name}, {"every", chroma.KeywordReserved},
		{"1h", chroma.LiteralNumber}, {"n", chroma.Name}, {"=", chroma.Operator},
		{"amount", chroma.Name}, {".", chroma.Punctuation}, {"count", chroma.NameFunction},
		{"()", chroma.Punctuation},

		{"to_dummies", chroma.Keyword}, {"dept", chroma.Name},
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

// Every command and alias starts a statement as a keyword.
func TestLexerCommands(t *testing.T) {
	for _, c := range Commands {
		for _, name := range append([]string{c.Name}, c.Aliases...) {
			got := tokens(t, name+" x\n")
			if len(got) == 0 || got[0].Value != name || got[0].Type != chroma.Keyword {
				t.Errorf("%q: %+v", name, got)
			}
		}
	}
}
