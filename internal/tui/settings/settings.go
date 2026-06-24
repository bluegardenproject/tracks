// Package settings provides the interactive repo manager that
// `tracks menu → Settings` opens.
//
// The user can add, edit, and remove repos without ever writing
// YAML by hand. Each mutation is persisted atomically through
// config.Save, so a syntax error in the on-disk file can never
// brick the tool.
//
// Other settings (tmux session name, branch types, claude binary,
// …) are not exposed here because they rarely need touching after
// initial setup. Users who want to edit those can still open the
// file at `~/.config/tracks/config.yaml` directly.
package settings

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/bluegardenproject/tracks/internal/config"
	"github.com/bluegardenproject/tracks/internal/tui"
	"github.com/charmbracelet/huh"
)

// ErrCancelled signals the user pressed Esc / Ctrl-C. Callers should
// treat this as a clean exit, not a failure.
var ErrCancelled = errors.New("cancelled")

// Run launches the settings menu loop. Each iteration shows the
// repo list and asks for one action; the loop exits when the user
// picks "Back" or aborts.
//
// startingCfg is the config the caller already has loaded. We work
// against an in-memory copy and Save after every mutation. If the
// on-disk file failed to parse, the caller should pass an empty /
// defaulted Config — we'll back the broken file up on the first
// save so the user doesn't silently lose their old data.
func Run(startingCfg config.Config, loadError error) error {
	cfg := startingCfg
	if loadError != nil {
		fmt.Println("warning: config file did not parse:", loadError)
		fmt.Println("the next save will back it up to config.yaml.bak.<timestamp> and overwrite.")
		fmt.Println()
		if err := backupBrokenConfig(); err != nil {
			fmt.Println("warning: could not back up broken config:", err)
		}
	}
	for {
		a, err := pickAction(cfg)
		if err != nil {
			if errors.Is(err, ErrCancelled) {
				return nil
			}
			return err
		}
		switch a {
		case actionBack:
			return nil
		case actionAdd:
			if err := addRepo(&cfg); err != nil && !errors.Is(err, ErrCancelled) {
				return err
			}
		case actionEdit:
			if err := editRepo(&cfg); err != nil && !errors.Is(err, ErrCancelled) {
				return err
			}
		case actionRemove:
			if err := removeRepo(&cfg); err != nil && !errors.Is(err, ErrCancelled) {
				return err
			}
		}
		// Save after every iteration. Validate() runs inside Save,
		// so the disk is never written with invalid data.
		if _, err := config.Save(cfg); err != nil {
			fmt.Println("save failed:", err)
			waitForKey()
		}
	}
}

type action string

const (
	actionAdd    action = "add"
	actionEdit   action = "edit"
	actionRemove action = "remove"
	actionBack   action = "back"
)

// pickAction shows the top-level settings menu. The label includes
// the current repo count so the user immediately knows what they
// have.
func pickAction(cfg config.Config) (action, error) {
	var pick action
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[action]().
				Title(fmt.Sprintf("Settings — %d repos configured", len(cfg.Repos))).
				Description("Up/Down to navigate, Enter to select, Esc to go back.").
				Options(
					huh.NewOption("Add repo", actionAdd),
					huh.NewOption("Edit repo", actionEdit),
					huh.NewOption("Remove repo", actionRemove),
					huh.NewOption("Back", actionBack),
				).
				Value(&pick),
		),
	)
	if err := form.WithKeyMap(tui.EscQuitKeyMap()).Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return "", ErrCancelled
		}
		return "", err
	}
	return pick, nil
}

// nameRE is the allowed character set for repo names — same family
// as branch slugs. We validate up front so the user gets immediate
// feedback in the form instead of a confusing daemon error later.
var nameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

// addRepo prompts for a new repo's fields and appends it.
func addRepo(cfg *config.Config) error {
	r := config.Repo{}
	if err := repoForm(cfg, &r, false); err != nil {
		return err
	}
	if _, dup := cfg.RepoByName(r.Name); dup {
		fmt.Printf("repo %q already exists — pick a different name\n", r.Name)
		waitForKey()
		return nil
	}
	cfg.Repos = append(cfg.Repos, r)
	fmt.Printf("added %s\n", r.Name)
	waitForKey()
	return nil
}

// editRepo presents a picker over existing repos and walks the form
// pre-filled with current values.
func editRepo(cfg *config.Config) error {
	idx, err := pickRepoIndex(*cfg, "Edit which repo?")
	if err != nil {
		return err
	}
	if idx < 0 {
		return nil
	}
	r := cfg.Repos[idx]
	if err := repoForm(cfg, &r, true); err != nil {
		return err
	}
	// Refuse a rename that collides with another existing repo.
	if r.Name != cfg.Repos[idx].Name {
		if _, dup := cfg.RepoByName(r.Name); dup {
			fmt.Printf("repo %q already exists — pick a different name\n", r.Name)
			waitForKey()
			return nil
		}
	}
	cfg.Repos[idx] = r
	fmt.Printf("updated %s\n", r.Name)
	waitForKey()
	return nil
}

// removeRepo deletes the picked repo after a confirm prompt.
func removeRepo(cfg *config.Config) error {
	idx, err := pickRepoIndex(*cfg, "Remove which repo?")
	if err != nil {
		return err
	}
	if idx < 0 {
		return nil
	}
	r := cfg.Repos[idx]
	var confirm bool
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(fmt.Sprintf("Remove %q from the config?", r.Name)).
				Description("This does not touch the actual checkout. Only `tracks` forgets about it.").
				Affirmative("Remove").
				Negative("Cancel").
				Value(&confirm),
		),
	)
	if err := form.WithKeyMap(tui.EscQuitKeyMap()).Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return ErrCancelled
		}
		return err
	}
	if !confirm {
		return nil
	}
	cfg.Repos = append(cfg.Repos[:idx], cfg.Repos[idx+1:]...)
	fmt.Printf("removed %s\n", r.Name)
	waitForKey()
	return nil
}

// pickRepoIndex shows a single-select over cfg.Repos and returns the
// chosen index. Returns -1 when there are no repos (and prints a
// helpful message). Returns ErrCancelled on abort.
func pickRepoIndex(cfg config.Config, title string) (int, error) {
	if len(cfg.Repos) == 0 {
		fmt.Println("no repos configured yet — pick \"Add repo\" first")
		waitForKey()
		return -1, nil
	}
	options := make([]huh.Option[int], 0, len(cfg.Repos))
	for i, r := range cfg.Repos {
		options = append(options, huh.NewOption(fmt.Sprintf("%s  %s  (base: %s)", r.Name, r.Path, r.Base), i))
	}
	var idx int
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[int]().
				Title(title).
				Description("Up/Down to navigate, Enter to select, Esc to cancel.").
				Options(options...).
				Value(&idx),
		),
	)
	if err := form.WithKeyMap(tui.EscQuitKeyMap()).Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return 0, ErrCancelled
		}
		return 0, err
	}
	return idx, nil
}

// repoForm shows a 4-field form for adding/editing a repo. When
// editing, the in-out *config.Repo has its current values; for a
// fresh add the caller passes a zero value.
//
// Path input is paste-friendly: the user can paste a full
// "/Users/.../some-repo" or "~/code/some-repo" and the form
// accepts it as long as it expands to an existing directory.
// Base branch is auto-detected via `git -C <path> symbolic-ref
// refs/remotes/origin/HEAD` after the path is entered
// (best-effort — empty when detection fails so the user can type
// a default).
func repoForm(cfg *config.Config, r *config.Repo, editing bool) error {
	name := r.Name
	path := r.Path
	base := r.Base
	if !editing && base == "" {
		base = "main"
	}
	initSubs := r.InitSubmodules

	// Provisioning fields. Empty when the repo has no provision block.
	var (
		depsCmd       string
		copyIgnored   string
		copyMode      = "symlink"
		cacheStrategy = "none"
	)
	if r.Provision != nil {
		depsCmd = r.Provision.DepsCmd
		copyIgnored = strings.Join(r.Provision.CopyIgnored, "\n")
		if r.Provision.CopyMode != "" {
			copyMode = r.Provision.CopyMode
		}
		if r.Provision.CacheStrategy != "" {
			cacheStrategy = r.Provision.CacheStrategy
		}
	}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Repo name").
				Description("Short identifier shown in the picker. Lowercase letters, digits, dot, dash, underscore.").
				Placeholder("my-repo").
				Validate(func(v string) error {
					v = strings.TrimSpace(v)
					if !nameRE.MatchString(v) {
						return fmt.Errorf("must match %s", nameRE)
					}
					return nil
				}).
				Value(&name),
			huh.NewInput().
				Title("Absolute path").
				Description("Paste the full path to the primary checkout. `~/` is expanded.").
				Placeholder("~/code/my-repo").
				Validate(validatePath).
				Value(&path),
			huh.NewInput().
				Title("Base branch").
				Description("Branch new worktrees should fork from (often `develop` or `main`). Leave the suggested default if unsure.").
				Suggestions([]string{"main", "develop", "master"}).
				Validate(func(v string) error {
					if strings.TrimSpace(v) == "" {
						return errors.New("base branch is required")
					}
					return nil
				}).
				Value(&base),
			huh.NewConfirm().
				Title("Init submodules in worktrees?").
				Description("Off by default. Turn on only if this repo uses submodules (rare; adds minutes to each worktree creation).").
				Affirmative("Yes").
				Negative("No").
				Value(&initSubs),
		),
		huh.NewGroup(
			huh.NewNote().
				Title("Provisioning (optional)").
				Description("Make a fresh worktree runnable: copy gitignored files (like .env) from the primary, then install deps. Leave the command and file list empty to disable."),
			huh.NewInput().
				Title("Dependency install command").
				Description("Run in the new worktree after checkout. Empty skips it.").
				Placeholder("pnpm install --frozen-lockfile").
				Value(&depsCmd),
			huh.NewText().
				Title("Gitignored files to copy (one per line)").
				Description("Paths or globs relative to the primary checkout, e.g. .env or apps/*/.env.local.").
				Placeholder(".env\n.env.local").
				Value(&copyIgnored),
			huh.NewSelect[string]().
				Title("Copy mode").
				Description("How those files are reproduced in the worktree.").
				Options(
					huh.NewOption("symlink (link back to primary)", "symlink"),
					huh.NewOption("copy (independent copy)", "copy"),
				).
				Value(&copyMode),
			huh.NewSelect[string]().
				Title("Cache strategy").
				Description("How deps are cached. Both options just run the command for now (pnpm reuses its store automatically).").
				Options(
					huh.NewOption("none", "none"),
					huh.NewOption("pnpm-store", "pnpm-store"),
				).
				Value(&cacheStrategy),
		),
	).WithShowHelp(true)

	// Best-effort: as soon as the user finishes editing the path,
	// fill in the detected base. We don't have a per-field "on-blur"
	// hook in huh, so we accept the values after Run and refine
	// afterwards if the user left base as its default.
	if err := form.WithKeyMap(tui.EscQuitKeyMap()).Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return ErrCancelled
		}
		return err
	}

	path = strings.TrimSpace(path)
	name = strings.TrimSpace(name)
	base = strings.TrimSpace(base)

	if !editing && (base == "main" || base == "") {
		if detected := detectBaseBranch(path); detected != "" {
			base = detected
		}
	}

	r.Name = name
	r.Path = path
	r.Base = base
	r.InitSubmodules = initSubs

	// Build the provision block only if something was configured.
	depsCmd = strings.TrimSpace(depsCmd)
	copyList := splitLines(copyIgnored)
	if depsCmd == "" && len(copyList) == 0 {
		r.Provision = nil
	} else {
		r.Provision = &config.Provision{
			DepsCmd:       depsCmd,
			CacheStrategy: cacheStrategy,
			CopyIgnored:   copyList,
			CopyMode:      copyMode,
		}
	}
	return nil
}

// splitLines turns a multiline text field into a trimmed, non-empty
// slice of entries.
func splitLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			out = append(out, line)
		}
	}
	return out
}

// validatePath accepts an absolute path or a "~/..." path. We
// expand "~/" and report whether the resulting directory exists.
// A non-existent path returns a soft warning (not an error) so
// the user can configure a checkout they haven't cloned yet.
func validatePath(v string) error {
	v = strings.TrimSpace(v)
	if v == "" {
		return errors.New("path is required")
	}
	if !strings.HasPrefix(v, "/") && !strings.HasPrefix(v, "~/") && v != "~" {
		return errors.New("must be absolute (e.g. /Users/me/repo) or start with ~/")
	}
	// Don't fail on missing dir — the user might still be cloning.
	return nil
}

// detectBaseBranch tries `git -C <path> symbolic-ref
// refs/remotes/origin/HEAD`. Returns the short branch name on
// success, "" on any failure. Best-effort only.
func detectBaseBranch(rawPath string) string {
	expanded := expandHome(rawPath)
	cmd := exec.Command("git", "-C", expanded, "symbolic-ref", "refs/remotes/origin/HEAD")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	// Output is "refs/remotes/origin/<branch>".
	s := strings.TrimSpace(string(out))
	const prefix = "refs/remotes/origin/"
	if !strings.HasPrefix(s, prefix) {
		return ""
	}
	return strings.TrimPrefix(s, prefix)
}

// expandHome is a local copy of the same helper in internal/config.
// We don't import the helper there because it's unexported; copying
// 6 lines keeps the package boundary clean.
func expandHome(p string) string {
	if !strings.HasPrefix(p, "~") {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	return filepath.Join(home, strings.TrimPrefix(p, "~"))
}

// backupBrokenConfig copies the existing config file to
// config.yaml.bak.<timestamp> so a fresh save doesn't quietly lose
// the user's previous content. Best-effort — missing file is fine
// (nothing to back up).
func backupBrokenConfig() error {
	p, err := config.Path()
	if err != nil {
		return err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	stamp := time.Now().UTC().Format("20060102-150405")
	bak := p + ".bak." + stamp
	if err := os.WriteFile(bak, data, 0o644); err != nil {
		return err
	}
	fmt.Printf("backed up to %s\n", bak)
	return nil
}

// waitForKey blocks until the user presses Enter, so feedback
// messages don't vanish before the popup re-renders the menu.
func waitForKey() {
	fmt.Print("\npress enter to continue…")
	var b [1]byte
	_, _ = os.Stdin.Read(b[:])
}
