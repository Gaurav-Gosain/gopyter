package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"strings"
	"syscall"

	"charm.land/fang/v2"
	"charm.land/lipgloss/v2"
	"github.com/Gaurav-Gosain/gopyter/internal/complete"
	"github.com/Gaurav-Gosain/gopyter/internal/config"
	"github.com/Gaurav-Gosain/gopyter/internal/glr"
	"github.com/Gaurav-Gosain/gopyter/internal/kernel"
	"github.com/Gaurav-Gosain/gopyter/internal/notebook"
	"github.com/Gaurav-Gosain/gopyter/internal/runner"
	"github.com/Gaurav-Gosain/gopyter/internal/ui"
	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"
)

// version and commit can be set at build time with -ldflags "-X
// main.version=... -X main.commit=...". When empty, fang reports the module
// version from the build info (e.g. for go install ...@vX.Y.Z).
var (
	version = ""
	commit  = ""
)

// appVersion is the version reported by %version: the -ldflags one, else
// the module version from the build info, like fang does.
func appVersion() string {
	if version != "" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" {
		return bi.Main.Version
	}
	return "dev"
}

// loadNotebook opens a notebook, or starts one in lang when path is
// empty or doesn't exist yet.
func loadNotebook(path string, lang notebook.Lang) (*notebook.Notebook, error) {
	if path == "" {
		return notebook.NewLang(lang), nil
	}
	nb, err := notebook.Load(path)
	if errors.Is(err, fs.ErrNotExist) {
		return notebook.NewLang(lang), nil
	}
	return nb, err
}

// parseLang reads the --lang flag.
func parseLang(s string) (notebook.Lang, error) {
	l, ok := notebook.ParseLang(s)
	if !ok {
		return "", fmt.Errorf("unknown language %q (use go or glr)", s)
	}
	return l, nil
}

// importScript offers to turn a .glr script into a notebook next to it.
// It returns the notebook and the path to save it at.
func importScript(cmd *cobra.Command, path string) (*notebook.Notebook, string, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	target := strings.TrimSuffix(path, filepath.Ext(path)) + ".ipynb"
	if _, err := os.Stat(target); err == nil {
		return nil, "", fmt.Errorf("%s is a glr script, and %s already exists: open that instead", path, target)
	}
	if term.IsTerminal(os.Stdin.Fd()) {
		cmd.PrintErrf("%s is a glr script. Import it as the notebook %s? [Y/n] ", path, target)
		answer, _ := bufio.NewReader(os.Stdin).ReadString('\n') // EOF means the default
		if a := strings.ToLower(strings.TrimSpace(answer)); a != "" && a != "y" && a != "yes" {
			return nil, "", errors.New("not imported")
		}
	}
	return glr.ImportScript(string(src)), target, nil
}

// session holds what runs cells: the Go kernel, and the glr kernel with
// its completer.
type session struct {
	k    *kernel.Kernel
	glr  *glr.Kernel
	comp *glr.Completer
}

func newSession(workdir string) (*session, error) {
	k, err := kernel.New(workdir)
	if err != nil {
		return nil, err
	}
	s := &session{k: k, glr: glr.NewKernel(k)}
	s.comp = glr.NewCompleter(func() string { return s.glr.Dir })
	return s, nil
}

// close stops golars and removes the kernel workspace, reporting (but not
// failing on) errors.
func (s *session) close() {
	if err := s.comp.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "gopyter: stopping golars-lsp: %v\n", err)
	}
	if err := s.glr.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "gopyter: stopping golars: %v\n", err)
	}
	closeKernel(s.k)
}

// notebookDir is the directory of a notebook file, where glr cells
// resolve relative paths (as Jupyter kernels run in the notebook's
// directory).
func notebookDir(path string) string {
	dir := "."
	if path != "" {
		dir = filepath.Dir(path)
	}
	if abs, err := filepath.Abs(dir); err == nil {
		return abs
	}
	return dir
}

func rootCmd() *cobra.Command {
	var (
		workdir    string
		theme      string
		syntax     string
		noComplete bool
		vim        bool
		model      string
		langFlag   string
	)
	cmd := &cobra.Command{
		Use:   "gopyter [notebook.ipynb]",
		Short: "A slick terminal notebook for Go",
		Long: "gopyter is a Jupyter-style notebook for Go that runs entirely in your terminal.\n\n" +
			"Declarations (func, type, var, const, import) persist across cells, statements run\n" +
			"inside main(), and a trailing expression is displayed as the cell's result.\n" +
			"Notebooks are stored as .ipynb files compatible with the GoNB Jupyter kernel\n" +
			"(https://github.com/janpfeifer/gonb), which inspired gopyter's execution model,\n" +
			"cell commands and widgets.",
		Example: "  # start a new notebook\n  gopyter\n\n" +
			"  # open (or create) a notebook\n  gopyter analysis.ipynb\n\n" +
			"  # execute a notebook headlessly and store the outputs\n  gopyter run analysis.ipynb --save",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := ""
			if len(args) == 1 {
				path = args[0]
			}
			lang, err := parseLang(langFlag)
			if err != nil {
				return err
			}
			var nb *notebook.Notebook
			imported := strings.EqualFold(filepath.Ext(path), ".glr")
			if imported {
				nb, path, err = importScript(cmd, path)
			} else {
				nb, err = loadNotebook(path, lang)
			}
			if err != nil {
				return err
			}
			sess, err := newSession(workdir)
			if err != nil {
				return err
			}
			defer sess.close()
			k := sess.k
			sess.glr.Dir = notebookDir(path)
			settings := loadSettings(cmd)
			name, err := resolveTheme(cmd, theme, settings)
			if err != nil {
				return err
			}
			aiModel, aiOn, err := resolveModel(cmd, model, settings)
			if err != nil {
				return err
			}
			// An explicit --vim / --vim=false wins for this session only.
			if !cmd.Flags().Changed("vim") {
				vim = settings.Vim
			}
			opts := ui.Options{
				Path: path, Notebook: nb, Kernel: k, Unsaved: imported,
				Vim: vim, SaveVim: saveVim,
				Theme: name, SyntaxTheme: syntax, SaveTheme: saveTheme,
				// Probe before the TUI takes over the terminal, so the
				// query can't race the program's input reader.
				LightBackground: !lipgloss.HasDarkBackground(os.Stdin, os.Stdout),
			}
			opts.GLR = sess.glr
			if !noComplete {
				engine := complete.New(k)
				defer func() { _ = engine.Close() }()
				opts.Completer = engine
				opts.GLRCompleter = sess.comp
			}
			opts.AI = newAI(aiModel, aiOn)
			return ui.Run(cmd.Context(), opts)
		},
	}
	cmd.PersistentFlags().StringVar(&workdir, "workdir", "", "persistent kernel workspace (Go module) directory; a temporary one is used by default")
	cmd.Flags().StringVar(&theme, "theme", "", "color theme for this session (see 'gopyter themes'); the saved theme is used by default")
	cmd.Flags().StringVar(&syntax, "syntax-theme", "", "chroma syntax highlighting style, overriding the theme's")
	cmd.Flags().BoolVar(&noComplete, "no-complete", false, "disable code completion (gopls)")
	cmd.Flags().BoolVar(&vim, "vim", false, "use vim key bindings in edit mode for this session (toggle and save with V); the saved setting is used by default")
	cmd.Flags().StringVar(&langFlag, "lang", "go", "language of a new notebook's cells: go, or glr for a golars notebook (saved with golars-kernel's kernelspec)")
	cmd.Flags().StringVar(&model, "model", "", "AI model for this session, as provider/model or a provider name, or off (see 'gopyter model'); the saved one is used by default")

	cmd.AddCommand(runCmd(&workdir), themesCmd(), modelCmd())
	return cmd
}

func runCmd(workdir *string) *cobra.Command {
	var (
		save     bool
		failFast bool
	)
	cmd := &cobra.Command{
		Use:   "run <notebook.ipynb | script.glr>",
		Short: "Execute every code cell of a notebook and print the outputs",
		Long: "Execute every code cell of a notebook and print the outputs.\n\n" +
			"Go cells run in the current directory; glr cells run in the notebook's directory,\n" +
			"like a Jupyter kernel. A .glr script runs as a notebook of its blocks.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var nb *notebook.Notebook
			var err error
			isScript := strings.EqualFold(filepath.Ext(args[0]), ".glr")
			if isScript {
				var src []byte
				if src, err = os.ReadFile(args[0]); err == nil {
					nb = glr.ImportScript(string(src))
				}
			} else {
				nb, err = notebook.Load(args[0])
			}
			if err != nil {
				return err
			}
			if isScript && save {
				return errors.New("--save needs a notebook; open the script with gopyter to import it")
			}
			sess, err := newSession(*workdir)
			if err != nil {
				return err
			}
			defer sess.close()
			k := sess.k
			if dir, err := os.Getwd(); err == nil {
				k.RunDir = dir
			}
			sess.glr.Dir = notebookDir(args[0])
			if w, _, err := term.GetSize(os.Stdout.Fd()); err == nil && w > 20 {
				runner.TableWidth = w
			}
			failed := runner.Run(cmd.Context(), k, sess.glr, nb, cmd.OutOrStdout(), cmd.InOrStdin(), failFast)
			if save {
				if err := nb.Save(args[0]); err != nil {
					return err
				}
			}
			if failed > 0 {
				return fmt.Errorf("%d cell%s failed", failed, strings.Repeat("s", min(failed-1, 1)))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&save, "save", false, "write the outputs back into the notebook")
	cmd.Flags().BoolVar(&failFast, "fail-fast", false, "stop at the first failing cell")
	return cmd
}

// loadSettings reads the saved settings. Errors are only reported, so a
// broken config never prevents gopyter from starting.
func loadSettings(cmd *cobra.Command) config.Settings {
	s, err := config.Load()
	if err != nil {
		cmd.PrintErrf("gopyter: reading settings: %v\n", err)
		return config.Settings{}
	}
	return s
}

// resolveTheme picks the UI theme: the --theme flag, else the saved one,
// else the default. A bad flag is an error; a bad saved value only a warning,
// so a stale config never prevents gopyter from starting.
func resolveTheme(cmd *cobra.Command, flag string, s config.Settings) (string, error) {
	if flag != "" {
		if !ui.ValidTheme(flag) {
			return "", fmt.Errorf("unknown theme %q (available: %s)", flag, strings.Join(ui.ThemeNames(), ", "))
		}
		return flag, nil
	}
	if s.Theme == "" {
		return ui.DefaultTheme, nil
	}
	if !ui.ValidTheme(s.Theme) {
		cmd.PrintErrf("gopyter: unknown saved theme %q, using %s\n", s.Theme, ui.DefaultTheme)
		return ui.DefaultTheme, nil
	}
	return s.Theme, nil
}

// saveTheme persists the theme picked in the UI.
func saveTheme(name string) error {
	return config.Update(func(s *config.Settings) { s.Theme = name })
}

// saveVim persists the vim bindings setting toggled in the UI.
func saveVim(on bool) error {
	return config.Update(func(s *config.Settings) { s.Vim = on })
}

func themesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "themes",
		Short: "List the available color themes",
		Long: "List the available color themes. The active one is marked with *.\n\n" +
			"Pick a theme inside gopyter with T (or the ◐ theme button); the choice is\n" +
			"saved to the user config directory. --theme overrides it for one session.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			current, err := resolveTheme(cmd, "", loadSettings(cmd))
			if err != nil {
				return err
			}
			var b strings.Builder
			for _, n := range ui.ThemeNames() {
				mark := "  "
				if n == current {
					mark = "* "
				}
				b.WriteString(mark)
				b.WriteString(n)
				b.WriteByte('\n')
			}
			if p, err := config.Path(); err == nil {
				b.WriteString("\nsettings: ")
				b.WriteString(p)
				b.WriteByte('\n')
			}
			_, err = io.WriteString(cmd.OutOrStdout(), b.String())
			return err
		},
	}
}

// closeKernel removes the kernel workspace, reporting (but not failing on)
// cleanup errors.
func closeKernel(k *kernel.Kernel) {
	if err := k.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "gopyter: cleaning up kernel workspace: %v\n", err)
	}
}

func main() {
	kernel.Version = appVersion()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := fang.Execute(ctx, rootCmd(), fang.WithVersion(version), fang.WithCommit(commit)); err != nil {
		os.Exit(1)
	}
}
