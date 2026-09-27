package notebook

import "strings"

// Lang is the language of a code cell.
type Lang string

const (
	// Go cells are compiled and run by the Go kernel.
	Go Lang = "go"
	// GLR cells are golars scripts (.glr), run by `golars kernel-host`.
	GLR Lang = "glr"
)

// ParseLang accepts a language name as users type it ("go", "glr",
// "golars").
func ParseLang(s string) (Lang, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "go", "golang":
		return Go, true
	case "glr", "golars":
		return GLR, true
	}
	return "", false
}

// Other returns the language a cell switches to.
func (l Lang) Other() Lang {
	if l == GLR {
		return Go
	}
	return GLR
}

// metaKey namespaces gopyter's cell metadata, as nbformat asks of
// extensions: {"gopyter": {"language": "glr"}}.
const metaKey = "gopyter"

// Kernelspec and language_info of golars notebooks. They match what
// `golars-kernel install` registers and what its kernel_info_reply says,
// so a glr notebook opens in JupyterLab with golars-kernel.
func golarsKernelspec() map[string]any {
	return map[string]any{"display_name": "golars (.glr)", "language": "golars", "name": "golars"}
}

func golarsLanguageInfo() map[string]any {
	return map[string]any{
		"name": "golars", "mimetype": "text/x-glr", "file_extension": ".glr",
		"pygments_lexer": "text", "codemirror_mode": map[string]any{"name": "shell"},
	}
}

func goKernelspec() map[string]any {
	return map[string]any{"display_name": "Go (gonb)", "language": "go", "name": "gonb"}
}

func goLanguageInfo() map[string]any {
	return map[string]any{"name": "go", "file_extension": ".go", "mimetype": "text/x-go"}
}

// NewLang returns an empty notebook whose cells default to l.
func NewLang(l Lang) *Notebook {
	nb := New()
	nb.SetLang(l)
	return nb
}

// Lang is the notebook's default cell language: glr for a golars
// kernelspec, else Go.
func (nb *Notebook) Lang() Lang {
	ks, _ := nb.Metadata["kernelspec"].(map[string]any)
	name, _ := ks["name"].(string)
	lang, _ := ks["language"].(string)
	if name == "golars" || strings.EqualFold(lang, "golars") {
		return GLR
	}
	return Go
}

// SetLang makes l the default cell language, with the matching
// kernelspec and language_info.
func (nb *Notebook) SetLang(l Lang) {
	if nb.Metadata == nil {
		nb.Metadata = map[string]any{}
	}
	if l == GLR {
		nb.Metadata["kernelspec"], nb.Metadata["language_info"] = golarsKernelspec(), golarsLanguageInfo()
		return
	}
	nb.Metadata["kernelspec"], nb.Metadata["language_info"] = goKernelspec(), goLanguageInfo()
}

// CellLang returns the language stored in a cell's metadata, or def.
func CellLang(meta map[string]any, def Lang) Lang {
	g, _ := meta[metaKey].(map[string]any)
	if s, ok := g["language"].(string); ok {
		if l, ok := ParseLang(s); ok {
			return l
		}
	}
	return def
}

// SetCellLang stores l in a cell's metadata, which it returns (allocated
// when nil). A cell in the notebook's default language carries no entry.
func SetCellLang(meta map[string]any, l, def Lang) map[string]any {
	g, _ := meta[metaKey].(map[string]any)
	if l == def {
		if g != nil {
			delete(g, "language")
			if len(g) == 0 {
				delete(meta, metaKey)
			}
		}
		return meta
	}
	if meta == nil {
		meta = map[string]any{}
	}
	if g == nil {
		g = map[string]any{}
		meta[metaKey] = g
	}
	g["language"] = string(l)
	return meta
}

// Magic returns the language a cell magic on the first line of src
// selects (%%glr, %%golars or %%go), if any.
func Magic(src string) (Lang, bool) {
	first, _, _ := strings.Cut(strings.TrimLeft(src, " \t\r\n"), "\n")
	first = strings.TrimSpace(first)
	name, ok := strings.CutPrefix(first, "%%")
	if !ok || strings.ContainsAny(name, " \t") {
		return "", false
	}
	return ParseLang(name)
}

// Resolve returns the language a cell runs in and the source to run: a
// language magic on the first line wins over lang, and is blanked so
// line numbers in errors still match the cell.
func Resolve(src string, lang Lang) (Lang, string) {
	l, ok := Magic(src)
	if !ok {
		return lang, src
	}
	i := strings.Index(src, "%%")
	end := strings.IndexByte(src[i:], '\n')
	if end < 0 {
		return l, src[:i]
	}
	return l, src[:i] + src[i+end:]
}
