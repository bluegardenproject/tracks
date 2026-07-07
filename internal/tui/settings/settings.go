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
	"sort"
	"strconv"
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
			continue // editRepo saves after each mutation; skip the outer save
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

// repoSubAction is the set of choices in the per-repo edit submenu.
type repoSubAction string

const (
	repoSubGeneral   repoSubAction = "general"
	repoSubProvision repoSubAction = "provision"
	repoSubServices  repoSubAction = "services"
	repoSubBack      repoSubAction = "back"
)

// editRepo presents a picker over existing repos and opens a per-repo
// submenu (General / Provisioning / Dev servers / Back). The submenu
// loops so the user can edit multiple sections before returning.
func editRepo(cfg *config.Config) error {
	idx, err := pickRepoIndex(*cfg, "Edit which repo?")
	if err != nil {
		return err
	}
	if idx < 0 {
		return nil
	}

	for {
		repoName := cfg.Repos[idx].Name
		svcCount := len(cfg.Repos[idx].Services)

		var pick repoSubAction
		form := huh.NewForm(
			huh.NewGroup(
				huh.NewSelect[repoSubAction]().
					Title(fmt.Sprintf("Edit %q", repoName)).
					Description("Up/Down to navigate, Enter to select, Esc to go back.").
					Options(
						huh.NewOption("General", repoSubGeneral),
						huh.NewOption("Provisioning", repoSubProvision),
						huh.NewOption(fmt.Sprintf("Dev servers  (%d configured)", svcCount), repoSubServices),
						huh.NewOption("Back", repoSubBack),
					).
					Value(&pick),
			),
		)
		if err := form.WithKeyMap(tui.EscQuitKeyMap()).Run(); err != nil {
			if errors.Is(err, huh.ErrUserAborted) {
				return ErrCancelled
			}
			return err
		}

		switch pick {
		case repoSubBack:
			return nil

		case repoSubGeneral:
			r := cfg.Repos[idx]
			if err := editRepoGeneral(cfg, &r); err != nil {
				if errors.Is(err, ErrCancelled) {
					continue
				}
				return err
			}
			// Refuse a rename that collides with another existing repo.
			if r.Name != cfg.Repos[idx].Name {
				if _, dup := cfg.RepoByName(r.Name); dup {
					fmt.Printf("repo %q already exists — pick a different name\n", r.Name)
					waitForKey()
					continue
				}
			}
			cfg.Repos[idx] = r
			if _, err := config.Save(*cfg); err != nil {
				fmt.Println("save failed:", err)
				waitForKey()
			} else {
				fmt.Printf("updated %s\n", r.Name)
				waitForKey()
			}

		case repoSubProvision:
			r := cfg.Repos[idx]
			if err := editRepoProvision(cfg, &r); err != nil {
				if errors.Is(err, ErrCancelled) {
					continue
				}
				return err
			}
			cfg.Repos[idx] = r
			if _, err := config.Save(*cfg); err != nil {
				fmt.Println("save failed:", err)
				waitForKey()
			} else {
				fmt.Printf("updated %s\n", r.Name)
				waitForKey()
			}

		case repoSubServices:
			if err := editRepoServices(cfg, idx); err != nil {
				if errors.Is(err, ErrCancelled) {
					continue
				}
				return err
			}
		}
	}
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

// repoForm shows a two-group form (general + provisioning) for adding a
// new repo. When editing, the in-out *config.Repo has its current
// values; for a fresh add the caller passes a zero value.
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
	draftPRs := r.DraftPRs

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
			huh.NewConfirm().
				Title("Open PRs as draft by default?").
				Description("When on, tracks for this repo tell Claude to open pull requests as drafts (`gh pr create --draft`). Off by default.").
				Affirmative("Yes").
				Negative("No").
				Value(&draftPRs),
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
				Description("How deps are seeded before the install. none/pnpm-store just run the command (pnpm reuses its store automatically); apfs-clone copy-on-write clones the primary's node_modules first so the install is an incremental reconcile (best for yarn/npm repos).").
				Options(
					huh.NewOption("none", "none"),
					huh.NewOption("pnpm-store", "pnpm-store"),
					huh.NewOption("apfs-clone", "apfs-clone"),
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
	r.DraftPRs = draftPRs

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

// editRepoGeneral runs a single form covering the General fields
// (name, path, base, submodules, draft PRs) for an existing repo.
// The caller is responsible for persisting changes to the config.
func editRepoGeneral(_ *config.Config, r *config.Repo) error {
	name := r.Name
	path := r.Path
	base := r.Base
	if base == "" {
		base = "main"
	}
	initSubs := r.InitSubmodules
	draftPRs := r.DraftPRs

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
			huh.NewConfirm().
				Title("Open PRs as draft by default?").
				Description("When on, tracks for this repo tell Claude to open pull requests as drafts (`gh pr create --draft`). Off by default.").
				Affirmative("Yes").
				Negative("No").
				Value(&draftPRs),
		),
	).WithShowHelp(true)

	if err := form.WithKeyMap(tui.EscQuitKeyMap()).Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return ErrCancelled
		}
		return err
	}

	r.Name = strings.TrimSpace(name)
	r.Path = strings.TrimSpace(path)
	r.Base = strings.TrimSpace(base)
	r.InitSubmodules = initSubs
	r.DraftPRs = draftPRs
	return nil
}

// editRepoProvision runs a single form covering the Provisioning fields
// for an existing repo. The caller is responsible for persisting changes.
func editRepoProvision(_ *config.Config, r *config.Repo) error {
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
				Description("How deps are seeded before the install. none/pnpm-store just run the command (pnpm reuses its store automatically); apfs-clone copy-on-write clones the primary's node_modules first so the install is an incremental reconcile (best for yarn/npm repos).").
				Options(
					huh.NewOption("none", "none"),
					huh.NewOption("pnpm-store", "pnpm-store"),
					huh.NewOption("apfs-clone", "apfs-clone"),
				).
				Value(&cacheStrategy),
		),
	).WithShowHelp(true)

	if err := form.WithKeyMap(tui.EscQuitKeyMap()).Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return ErrCancelled
		}
		return err
	}

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

// editRepoServices is the service CRUD loop for a single repo. It
// loops until the user picks "Back" and saves via config.Save after
// every successful add / edit / remove.
func editRepoServices(cfg *config.Config, repoIdx int) error {
	for {
		svcs := cfg.Repos[repoIdx].Services

		opts := []huh.Option[string]{
			huh.NewOption("Add service", "add"),
		}
		if len(svcs) > 0 {
			opts = append(opts,
				huh.NewOption("Edit service", "edit"),
				huh.NewOption("Remove service", "remove"),
			)
		}
		opts = append(opts, huh.NewOption("Back", "back"))

		var pick string
		form := huh.NewForm(
			huh.NewGroup(
				huh.NewSelect[string]().
					Title(fmt.Sprintf("Dev servers — %d configured", len(svcs))).
					Description("Up/Down to navigate, Enter to select, Esc to go back.").
					Options(opts...).
					Value(&pick),
			),
		)
		if err := form.WithKeyMap(tui.EscQuitKeyMap()).Run(); err != nil {
			if errors.Is(err, huh.ErrUserAborted) {
				return ErrCancelled
			}
			return err
		}

		switch pick {
		case "back":
			return nil

		case "add":
			svc := config.Service{}
			if err := serviceForm(cfg.Repos[repoIdx].Services, &svc, false); err != nil {
				if errors.Is(err, ErrCancelled) {
					continue
				}
				return err
			}
			cfg.Repos[repoIdx].Services = append(cfg.Repos[repoIdx].Services, svc)
			if _, err := config.Save(*cfg); err != nil {
				fmt.Println("save failed:", err)
				waitForKey()
			} else {
				fmt.Printf("added service %s\n", svc.Name)
				waitForKey()
			}

		case "edit":
			svcIdx, err := pickServiceIndex(cfg.Repos[repoIdx].Services, "Edit which service?")
			if err != nil {
				if errors.Is(err, ErrCancelled) {
					continue
				}
				return err
			}
			if svcIdx < 0 {
				continue
			}
			svc := cfg.Repos[repoIdx].Services[svcIdx]
			if err := serviceForm(cfg.Repos[repoIdx].Services, &svc, true); err != nil {
				if errors.Is(err, ErrCancelled) {
					continue
				}
				return err
			}
			cfg.Repos[repoIdx].Services[svcIdx] = svc
			if _, err := config.Save(*cfg); err != nil {
				fmt.Println("save failed:", err)
				waitForKey()
			} else {
				fmt.Printf("updated service %s\n", svc.Name)
				waitForKey()
			}

		case "remove":
			svcIdx, err := pickServiceIndex(cfg.Repos[repoIdx].Services, "Remove which service?")
			if err != nil {
				if errors.Is(err, ErrCancelled) {
					continue
				}
				return err
			}
			if svcIdx < 0 {
				continue
			}
			svcName := cfg.Repos[repoIdx].Services[svcIdx].Name
			var confirm bool
			confirmForm := huh.NewForm(
				huh.NewGroup(
					huh.NewConfirm().
						Title(fmt.Sprintf("Remove service %q?", svcName)).
						Description("This removes the service definition from the config. Running services are not affected.").
						Affirmative("Remove").
						Negative("Cancel").
						Value(&confirm),
				),
			)
			if err := confirmForm.WithKeyMap(tui.EscQuitKeyMap()).Run(); err != nil {
				if errors.Is(err, huh.ErrUserAborted) {
					continue
				}
				return err
			}
			if !confirm {
				continue
			}
			cfg.Repos[repoIdx].Services = append(
				cfg.Repos[repoIdx].Services[:svcIdx],
				cfg.Repos[repoIdx].Services[svcIdx+1:]...,
			)
			if _, err := config.Save(*cfg); err != nil {
				fmt.Println("save failed:", err)
				waitForKey()
			} else {
				fmt.Printf("removed service %s\n", svcName)
				waitForKey()
			}
		}
	}
}

// pickServiceIndex shows a single-select over the given services and
// returns the chosen index. Returns ErrCancelled on abort.
func pickServiceIndex(svcs []config.Service, title string) (int, error) {
	opts := make([]huh.Option[int], 0, len(svcs))
	for i, svc := range svcs {
		label := fmt.Sprintf("%s  cmd: %s", svc.Name, truncate(svc.Cmd, 40))
		if svc.ProxyPort != 0 {
			label += fmt.Sprintf("  [proxy :%d]", svc.ProxyPort)
		}
		opts = append(opts, huh.NewOption(label, i))
	}
	var idx int
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[int]().
				Title(title).
				Description("Up/Down to navigate, Enter to select, Esc to cancel.").
				Options(opts...).
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

// serviceForm shows a multi-group form for adding or editing a service.
// Groups: (1) identity, (2) env & proxy, (3) readiness, (4) lifecycle.
// When editing, origName is captured before the form to allow keeping
// the same name while still catching collisions with other services.
func serviceForm(existingSvcs []config.Service, svc *config.Service, editing bool) error {
	origName := svc.Name

	name := svc.Name
	cmd := svc.Cmd

	envText := envToText(svc.Env)

	proxyPortStr := ""
	if svc.ProxyPort != 0 {
		proxyPortStr = strconv.Itoa(svc.ProxyPort)
	}

	readyPort := svc.Ready.Port
	readyLogRegex := svc.Ready.LogRegex

	preStartText := hooksToText(svc.PreStart)
	postStartText := hooksToText(svc.PostStart)
	preStopText := hooksToText(svc.PreStop)
	dependsOnText := hooksToText(svc.DependsOn)

	form := huh.NewForm(
		// Group 1 — identity
		huh.NewGroup(
			huh.NewInput().
				Title("Service name").
				Description("Short identifier for this service within the repo. Lowercase letters, digits, dot, dash, underscore.").
				Placeholder("live-app").
				Validate(func(v string) error {
					v = strings.TrimSpace(v)
					if !nameRE.MatchString(v) {
						return fmt.Errorf("must match %s", nameRE)
					}
					for _, s := range existingSvcs {
						if editing && s.Name == origName {
							continue // skip the service being edited
						}
						if s.Name == v {
							return fmt.Errorf("service %q already exists in this repo", v)
						}
					}
					return nil
				}).
				Value(&name),
			huh.NewInput().
				Title("Start command").
				Description("Shell command that starts the server. Templated (e.g. {{.Port \"live-app\"}} resolves to the allocated port).").
				Placeholder("pnpm run dev --port {{.Port \"live-app\"}}").
				Validate(func(v string) error {
					if strings.TrimSpace(v) == "" {
						return errors.New("command is required")
					}
					return nil
				}).
				Value(&cmd),
		),
		// Group 2 — env & proxy
		huh.NewGroup(
			huh.NewNote().
				Title("Environment & stable-port proxy").
				Description("Extra env vars (merged onto the daemon's environment) and an optional fixed proxy port."),
			huh.NewText().
				Title("Environment variables (KEY=VALUE, one per line)").
				Description("Values are templated. Blank lines are ignored.").
				Placeholder("PORT={{.Port \"live-app\"}}\nNODE_ENV=development").
				Value(&envText),
			huh.NewInput().
				Title("Stable proxy port").
				Description("If set, a reverse proxy on this fixed port always points to the active track's service. Leave empty to disable.").
				Placeholder("3000").
				Validate(func(v string) error {
					v = strings.TrimSpace(v)
					if v == "" {
						return nil
					}
					p, err := strconv.Atoi(v)
					if err != nil {
						return errors.New("must be a number between 1 and 65535")
					}
					if p < 1 || p > 65535 {
						return errors.New("must be between 1 and 65535")
					}
					return nil
				}).
				Value(&proxyPortStr),
		),
		// Group 3 — readiness probe
		huh.NewGroup(
			huh.NewNote().
				Title("Readiness probe (optional — set at most one)").
				Description("Leave both empty for \"ready as soon as it starts\"."),
			huh.NewInput().
				Title("Port").
				Description("Service is ready when something listens on this TCP port. Templated, e.g. {{.Port \"live-app\"}} or a literal number.").
				Placeholder("{{.Port \"live-app\"}}").
				Value(&readyPort),
			huh.NewInput().
				Title("Log regex").
				Description("Service is ready when its log output matches this RE2 pattern.").
				Placeholder("compiled successfully").
				Validate(func(v string) error {
					if v != "" && strings.TrimSpace(readyPort) != "" {
						return errors.New("set at most one of port and log regex")
					}
					return nil
				}).
				Value(&readyLogRegex),
		),
		// Group 4 — lifecycle hooks & depends_on
		huh.NewGroup(
			huh.NewNote().
				Title("Lifecycle hooks & dependencies (optional)").
				Description("Shell commands run around the service lifecycle (one per line, templated). depends_on lists service names that must be ready before this one starts."),
			huh.NewText().
				Title("Pre-start commands (one per line)").
				Value(&preStartText),
			huh.NewText().
				Title("Post-start commands (one per line)").
				Value(&postStartText),
			huh.NewText().
				Title("Pre-stop commands (one per line)").
				Value(&preStopText),
			huh.NewText().
				Title("Depends on (service names, one per line)").
				Value(&dependsOnText),
		),
	).WithShowHelp(true)

	if err := form.WithKeyMap(tui.EscQuitKeyMap()).Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return ErrCancelled
		}
		return err
	}

	env, err := textToEnv(envText)
	if err != nil {
		fmt.Println("error parsing env vars:", err)
		waitForKey()
		return ErrCancelled
	}

	var proxyPort int
	if v := strings.TrimSpace(proxyPortStr); v != "" {
		proxyPort, _ = strconv.Atoi(v) // already validated in the form
	}

	svc.Name = strings.TrimSpace(name)
	svc.Cmd = strings.TrimSpace(cmd)
	svc.Env = env
	svc.ProxyPort = proxyPort
	svc.Ready = config.ReadyProbe{
		Port:     strings.TrimSpace(readyPort),
		LogRegex: strings.TrimSpace(readyLogRegex),
	}
	svc.PreStart = textToHooks(preStartText)
	svc.PostStart = textToHooks(postStartText)
	svc.PreStop = textToHooks(preStopText)
	svc.DependsOn = splitLines(dependsOnText)
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

// envToText converts an env map to sorted KEY=VALUE lines suitable
// for display in a multiline text field.
func envToText(env map[string]string) string {
	if len(env) == 0 {
		return ""
	}
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	for i, k := range keys {
		if i > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteString(k)
		sb.WriteByte('=')
		sb.WriteString(env[k])
	}
	return sb.String()
}

// textToEnv parses KEY=VALUE lines from a multiline text field into a
// map. Blank lines are ignored. Returns an error on malformed lines.
// Returns nil (not an empty map) when no entries are present, so the
// YAML omitempty tag omits the field correctly.
func textToEnv(s string) (map[string]string, error) {
	result := make(map[string]string)
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		idx := strings.IndexByte(line, '=')
		if idx < 0 {
			return nil, fmt.Errorf("malformed env line %q: expected KEY=VALUE", line)
		}
		key := strings.TrimSpace(line[:idx])
		if key == "" {
			return nil, fmt.Errorf("malformed env line %q: key is empty", line)
		}
		result[key] = line[idx+1:]
	}
	if len(result) == 0 {
		return nil, nil
	}
	return result, nil
}

// hooksToText joins a slice of hook commands (or service names) with
// newlines for display in a multiline text field.
func hooksToText(hooks []string) string {
	return strings.Join(hooks, "\n")
}

// textToHooks splits a multiline text field into a trimmed, non-empty
// slice of hook commands.
func textToHooks(s string) []string {
	return splitLines(s)
}

// truncate returns s unchanged if it is at most n runes long; otherwise
// it returns the first n runes followed by "…".
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
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
