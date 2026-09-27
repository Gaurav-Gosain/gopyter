package glr

import (
	"strings"

	"github.com/Gaurav-Gosain/gopyter/internal/notebook"
)

// ImportScript turns a .glr script into a glr notebook: one code cell per
// block of lines separated by blank lines, and the comment lines that
// open a block as a markdown cell before it.
func ImportScript(src string) *notebook.Notebook {
	nb := notebook.NewLang(notebook.GLR)
	nb.Cells = nil
	add := func(kind notebook.CellType, lines []string) {
		text := strings.TrimSpace(strings.Join(lines, "\n"))
		if text == "" {
			return
		}
		nb.Cells = append(nb.Cells, &notebook.Cell{ID: notebook.NewID(), Type: kind, Source: text})
	}
	flush := func(block []string) {
		i := 0
		var prose []string
		for ; i < len(block); i++ {
			t := strings.TrimSpace(block[i])
			if !strings.HasPrefix(t, "#") {
				break
			}
			t = strings.TrimSpace(strings.TrimPrefix(t, "#"))
			if t == "^?" {
				continue // golars' example markers for "show output here"
			}
			prose = append(prose, t)
		}
		add(notebook.Markdown, prose)
		add(notebook.Code, block[i:])
	}
	var block []string
	for l := range strings.SplitSeq(strings.ReplaceAll(src, "\r\n", "\n"), "\n") {
		if strings.TrimSpace(l) == "" {
			flush(block)
			block = nil
			continue
		}
		block = append(block, l)
	}
	flush(block)
	if len(nb.Cells) == 0 {
		nb.Cells = notebook.New().Cells
	}
	return nb
}
