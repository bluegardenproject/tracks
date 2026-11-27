package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/bluegardenproject/tracks/internal/platform"
	"github.com/bluegardenproject/tracks/internal/plugins"
	"github.com/bluegardenproject/tracks/internal/store"
	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"
)

func newPluginCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plugin",
		Short: "add, list, update and remove plugins",
		Long: "A plugin brings trains and track types, from a git repository pinned to a commit, or a local folder " +
			"while you write one. Its code runs as you once you approve it: add and update show what it asks for first.",
	}
	cmd.AddCommand(newPluginAddCmd(), newPluginListCmd(), newPluginUpdateCmd(), newPluginRemoveCmd(),
		newPluginEnableCmd("enable", true), newPluginEnableCmd("disable", false))
	return cmd
}

// withPlugins opens the database and runs f with the plugins service.
func withPlugins(ctx context.Context, f func(plugins.Service) error) error {
	paths, err := platform.Resolve()
	if err != nil {
		return err
	}
	db, err := store.Open(ctx, paths.Database)
	if err != nil {
		return err
	}
	defer db.Close()
	return f(plugins.Service{Store: db, Dir: paths.Plugins})
}

func newPluginAddCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "add <git url>[@ref] | <local folder>",
		Short: "install a plugin once you approve what it asks for",
		Long: "Clones the repository, pins the commit its ref names (the default branch without one), checks its " +
			"plugin.yaml and shows its components, permissions, hosts, MCP servers and the code it runs, then asks " +
			"for a yes. A local folder, a path starting with /, ./, ../ or ~/, is linked instead: not pinned, its code " +
			"runs as it is on disk. Without a terminal, --yes approves.",
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			src, err := plugins.ParseSource(args[0])
			if err != nil {
				return err
			}
			if src.Local != "" {
				if src.Local, err = absPath(src.Local); err != nil {
					return err
				}
			}
			out := c.OutOrStdout()
			return withPlugins(c.Context(), func(s plugins.Service) error {
				p, err := s.Add(c.Context(), src, func(p plugins.Proposal) (bool, error) {
					fmt.Fprint(out, describeProposal(p))
					return approve(c.InOrStdin(), out, yes, "Install "+p.Manifest.Name+"?")
				})
				if err != nil {
					return err
				}
				_, err = fmt.Fprintf(out, "Installed %s in %s.\n", p.Name, p.Dir)
				return err
			})
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "approve without asking, once you've read what it asks for")
	return cmd
}

func newPluginListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "list the installed plugins",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return withPlugins(c.Context(), func(s plugins.Service) error {
				all, err := s.List(c.Context())
				if err != nil {
					return err
				}
				if len(all) == 0 {
					_, err := fmt.Fprintln(c.OutOrStdout(), "No plugins. tracks plugin add <git url> installs one.")
					return err
				}
				w := tabwriter.NewWriter(c.OutOrStdout(), 0, 4, 2, ' ', 0)
				fmt.Fprintln(w, "NAME\tCOMMIT\tCOMPONENTS\tENABLED\tSOURCE")
				for _, p := range all {
					fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", p.Name, pluginVersion(p.Plugin), p.Approval.Summary(), pluginYesNo(p.Enabled), pluginSource(p.Plugin))
				}
				return w.Flush()
			})
		},
	}
}

func newPluginUpdateCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "update [name]",
		Short: "update one plugin, or all of them, asking again when the code or the permissions changed",
		Long: "Fetches the plugin's ref and shows plugin.yaml's diff and the code files that changed. A change to " +
			"the code or to what it asks for waits for your yes; a local plugin's manifest is read again.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			out := c.OutOrStdout()
			return withPlugins(c.Context(), func(s plugins.Service) error {
				names := args
				if len(names) == 0 {
					all, err := s.List(c.Context())
					if err != nil {
						return err
					}
					for _, p := range all {
						names = append(names, p.Name)
					}
				}
				var failed []string
				for _, name := range names {
					change, err := s.Update(c.Context(), name, func(ch plugins.Change) (bool, error) {
						fmt.Fprint(out, describeChange(ch))
						if !ch.Ask {
							return true, nil
						}
						return approve(c.InOrStdin(), out, yes, "Update "+name+"?")
					})
					switch {
					case err != nil:
						fmt.Fprintf(c.ErrOrStderr(), "%s: %v\n", name, err)
						failed = append(failed, name)
					case change.Current:
						fmt.Fprintf(out, "%s is up to date.\n", name)
					default:
						fmt.Fprintf(out, "Updated %s.\n", name)
					}
				}
				if len(failed) > 0 {
					return fmt.Errorf("not updated: %s", strings.Join(failed, ", "))
				}
				return nil
			})
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "approve without asking, once you've read what changed")
	return cmd
}

func newPluginRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name>",
		Short: "uninstall a plugin and delete its folder; a local plugin's own folder stays",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			return withPlugins(c.Context(), func(s plugins.Service) error {
				if err := s.Remove(c.Context(), args[0]); err != nil {
					return err
				}
				_, err := fmt.Fprintf(c.OutOrStdout(), "Removed %s.\n", args[0])
				return err
			})
		},
	}
}

func newPluginEnableCmd(verb string, enabled bool) *cobra.Command {
	short := "offer a disabled plugin to tracks again"
	if !enabled {
		short = "hide a plugin from tracks, keeping it installed"
	}
	return &cobra.Command{
		Use:   verb + " <name>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			return withPlugins(c.Context(), func(s plugins.Service) error {
				if err := s.SetEnabled(c.Context(), args[0], enabled); err != nil {
					return err
				}
				_, err := fmt.Fprintf(c.OutOrStdout(), "%sd %s.\n", strings.ToUpper(verb[:1])+verb[1:], args[0])
				return err
			})
		},
	}
}

// errNoTerminal is returned when approval needs a terminal.
var errNoTerminal = errors.New("no terminal to approve on: read the above, then run again with --yes")

// approve asks question on a terminal, unless yes already approves.
func approve(in io.Reader, out io.Writer, yes bool, question string) (bool, error) {
	if yes {
		return true, nil
	}
	if f, ok := in.(*os.File); !ok || !terminal(f) {
		return false, errNoTerminal
	}
	fmt.Fprintf(out, "%s [y/N] ", question)
	answer, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return true, nil
	}
	return false, nil
}

func terminal(f *os.File) bool { return term.IsTerminal(f.Fd()) }

// absPath resolves ~ and a relative path against the working folder.
func absPath(p string) (string, error) {
	if rest, ok := strings.CutPrefix(p, "~"); ok && (rest == "" || rest[0] == '/') {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, rest), nil
	}
	return filepath.Abs(p)
}

func describeProposal(p plugins.Proposal) string {
	var b strings.Builder
	if p.Source.Local != "" {
		fmt.Fprintf(&b, "%s, linked from %s: not pinned, its code runs as it is on disk.\n", p.Manifest.Name, p.Source.Local)
	} else {
		fmt.Fprintf(&b, "%s, from %s at %s.\n", p.Manifest.Name, refSource(p.Source.URL, p.Source.Ref), short(p.Commit))
	}
	for _, w := range p.Warnings {
		fmt.Fprintf(&b, "Warning: %s\n", w)
	}
	b.WriteString(p.Approval.Describe())
	return b.String()
}

func describeChange(c plugins.Change) string {
	var b strings.Builder
	if c.Local {
		fmt.Fprintf(&b, "%s: its plugin.yaml asks for something else now.\n", c.Name)
	} else {
		fmt.Fprintf(&b, "%s: %s → %s\n", c.Name, short(c.From), short(c.To))
	}
	for _, w := range c.Warnings {
		fmt.Fprintf(&b, "Warning: %s\n", w)
	}
	if c.ManifestDiff != "" {
		fmt.Fprintf(&b, "plugin.yaml:\n%s", c.ManifestDiff)
		if !strings.HasSuffix(c.ManifestDiff, "\n") {
			b.WriteString("\n")
		}
	}
	if c.Unknown {
		b.WriteString("The installed commit can't be compared: treat every file as changed.\n")
	}
	if len(c.Code) > 0 {
		fmt.Fprintf(&b, "Code and other files changed:\n  %s\n", strings.Join(c.Code, "\n  "))
	}
	if len(c.Widens) > 0 {
		fmt.Fprintf(&b, "It now asks for more:\n  %s\n", strings.Join(c.Widens, "\n  "))
	}
	if c.Ask {
		b.WriteString(c.Approval.Describe())
	} else {
		b.WriteString("The code and the permissions are the same.\n")
	}
	return b.String()
}

func pluginVersion(p store.Plugin) string {
	if p.Local {
		return "local"
	}
	return short(p.Pinned)
}

func pluginSource(p store.Plugin) string {
	if p.Local {
		return p.Source
	}
	return refSource(p.Source, p.Ref)
}

func refSource(url, ref string) string {
	if ref == "" {
		return url
	}
	return url + "@" + ref
}

func short(commit string) string {
	if len(commit) > 7 {
		return commit[:7]
	}
	return commit
}

func pluginYesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
