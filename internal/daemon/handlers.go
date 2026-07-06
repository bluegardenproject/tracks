package daemon

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/bluegardenproject/tracks/internal/config"
	"github.com/bluegardenproject/tracks/internal/git"
	"github.com/bluegardenproject/tracks/internal/notify"
	"github.com/bluegardenproject/tracks/internal/ports"
	"github.com/bluegardenproject/tracks/internal/provision"
	"github.com/bluegardenproject/tracks/internal/state"
	"github.com/bluegardenproject/tracks/internal/tmux"
)

// ok wraps a result payload in a successful Response. result may be
// nil for methods that don't return data.
func ok(result any) Response {
	if result == nil {
		return Response{Ok: true}
	}
	data, err := json.Marshal(result)
	if err != nil {
		return Response{Ok: false, Error: "marshal result: " + err.Error()}
	}
	return Response{Ok: true, Result: data}
}

// fail wraps a Response with Ok=false and the given message.
func fail(msg string) Response { return Response{Ok: false, Error: msg} }

func (s *Server) handlePing() Response {
	r := PingResult{Version: s.version, PID: os.Getpid(), ExePath: s.exePath}
	if !s.exeMod.IsZero() {
		r.ExeModUnixNano = s.exeMod.UnixNano()
	}
	return ok(r)
}

func (s *Server) handleLs() Response {
	return ok(LsResult{Tracks: s.store.All()})
}

func (s *Server) handleGet(raw json.RawMessage) Response {
	var p GetParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return fail("bad params: " + err.Error())
	}
	t, found := s.store.Get(p.ID)
	return ok(GetResult{Track: t, Found: found})
}

// placeholderBranch returns the temporary branch name created in
// the worktree at track start. Format: tracks/<last-6-of-id>. Short
// because it's only meant to live briefly — the CLAUDE.md
// instructions tell Claude to rename it to its proper
// <type>/<slug> before the first commit.
func placeholderBranch(trackID string) string {
	tail := trackID
	if len(tail) > 6 {
		tail = tail[len(tail)-6:]
	}
	return "tracks/" + tail
}

// reviewCheckout describes how to materialize a review worktree: the
// refspec to fetch from origin and a human-readable label to show in
// the dashboard. The worktree is always added detached at FETCH_HEAD
// right after the fetch.
type reviewCheckout struct {
	fetchRef string // arg to `git fetch origin <fetchRef>`
	label    string // display label for the track's branch column
}

// prURLNumber pulls the PR number out of a GitHub pull-request URL,
// e.g. https://github.com/owner/repo/pull/123 (with optional trailing
// /files, #discussion, query string, etc.).
var prURLNumber = regexp.MustCompile(`github\.com/[^/]+/[^/]+/pull/(\d+)`)

// parseReviewRef turns the user's review target into a reviewCheckout.
// A GitHub PR URL resolves to that PR's head ref (works for forks too,
// since `pull/<n>/head` lives on the base repo's origin); anything
// else is treated as a branch name fetched from origin.
func parseReviewRef(ref string) (reviewCheckout, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return reviewCheckout{}, fmt.Errorf("empty review target")
	}
	if m := prURLNumber.FindStringSubmatch(ref); m != nil {
		return reviewCheckout{
			fetchRef: fmt.Sprintf("pull/%s/head", m[1]),
			label:    "pr/" + m[1],
		}, nil
	}
	if strings.Contains(ref, "://") || strings.Contains(ref, "github.com") {
		return reviewCheckout{}, fmt.Errorf("not a recognizable GitHub PR URL or branch name: %q", ref)
	}
	return reviewCheckout{fetchRef: ref, label: ref}, nil
}

func (s *Server) handleNew(ctx context.Context, raw json.RawMessage, emit Emit) Response {
	var p NewParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return fail("bad params: " + err.Error())
	}

	// Resolve and validate each requested repo against config.
	repos := make([]repoSpec, 0, len(p.Repos))
	for _, name := range p.Repos {
		r, ok := s.config().RepoByName(name)
		if !ok {
			return fail(fmt.Sprintf("unknown repo %q (configure it in ~/.config/tracks/config.yaml)", name))
		}
		path, err := r.ResolveRepoPath()
		if err != nil {
			return fail(fmt.Sprintf("resolve repo %s: %v", name, err))
		}
		repos = append(repos, repoSpec{Name: r.Name, Path: path, Base: r.Base, InitSubmodules: r.InitSubmodules, Provision: r.Provision})
	}

	// A review target turns this into a detached-HEAD checkout of an
	// existing PR/branch rather than a fresh branch off base. It only
	// makes sense against a single repo: a PR number or branch name is
	// repo-specific, and fetching e.g. `pull/123/head` against the
	// wrong repo would silently pull an unrelated PR.
	var checkout *reviewCheckout
	if ref := strings.TrimSpace(p.ReviewRef); ref != "" {
		if len(repos) != 1 {
			return fail("a review target (PR URL or branch) supports exactly one repo; pick a single repo")
		}
		c, err := parseReviewRef(ref)
		if err != nil {
			return fail(err.Error())
		}
		checkout = &c
	}

	// Determine the track kind. Empty defaults to work; unknown values
	// are rejected rather than stored verbatim; a review ref always
	// means a review track regardless of what the client sent.
	kind := state.Kind(strings.TrimSpace(p.Kind))
	switch kind {
	case state.KindWork, state.KindReview, state.KindAsk, state.KindPlan:
		// recognized
	case "":
		kind = state.KindWork
	default:
		return fail(fmt.Sprintf("unknown track kind %q", p.Kind))
	}
	if checkout != nil {
		kind = state.KindReview
	}

	// Work and review tracks need a worktree, so they require at least
	// one repo. Ask/plan are worktree-less and may run with none — a
	// general question needn't be tied to any repo.
	if !kind.Worktreeless() && len(repos) == 0 {
		return fail("at least one repo required")
	}

	trackID, err := generateTrackID()
	if err != nil {
		return fail("generate id: " + err.Error())
	}
	sessionID, err := generateSessionID()
	if err != nil {
		return fail("generate session id: " + err.Error())
	}
	branch := placeholderBranch(trackID)

	stateDir, err := s.config().ResolveStateDir()
	if err != nil {
		return fail("resolve state dir: " + err.Error())
	}
	worktreeRoot := filepath.Join(stateDir, "worktrees", trackID)
	logPath := filepath.Join(stateDir, "logs", trackID+".jsonl")

	emit(fmt.Sprintf("track id %s", trackID))

	// Build the track record up front so any failure during provisioning
	// can be persisted as an errored track — carrying the prompt and the
	// reason — instead of vanishing as a transient CLI error. A git fetch
	// that dies mid-network-drop, a port clash, or a spawn error then
	// shows up in the dashboard, where the preserved prompt makes it easy
	// to retry and the message makes it easy to debug.
	t := state.Track{
		ID:         trackID,
		Branch:     branch,
		Slug:       strings.TrimSpace(p.Slug),
		Kind:       kind,
		Status:     state.StatusPending,
		LogPath:    logPath,
		TaskPrompt: p.TaskPrompt,
		SessionID:  sessionID,
		CreatedAt:  time.Now().UTC(),
	}
	// failCreate persists the in-progress track as errored (with the
	// reason) and returns the wire error, so the failure is visible and
	// retryable from the dashboard rather than only in the CLI response.
	failCreate := func(msg string) Response {
		t.Status = state.StatusErrored
		t.ErrorMsg = msg
		now := time.Now().UTC()
		t.ExitedAt = &now
		_ = s.store.Put(t)
		return fail(msg)
	}

	var (
		trackRepos     []state.TrackRepo
		rollback       = func() {}
		resolvedBranch = branch
	)
	if kind.Worktreeless() {
		// Read-only ask/plan track: no worktree, no branch. Point Claude
		// at the primary checkouts directly.
		resolvedBranch = ""
		for _, r := range repos {
			trackRepos = append(trackRepos, state.TrackRepo{Name: r.Name, Path: r.Path})
		}
	} else {
		// Review worktrees are detached at the target ref — no branch is
		// created, so there's no collision to resolve. We still store a
		// readable label so the dashboard's branch column isn't blank.
		if checkout == nil {
			resolvedBranch, err = s.resolveBranchCollision(ctx, repos, branch)
			if err != nil {
				return failCreate(err.Error())
			}
		} else {
			resolvedBranch = checkout.label
		}
		trackRepos, rollback, err = s.createWorktrees(ctx, worktreeRoot, repos, resolvedBranch, checkout, emit)
		if err != nil {
			rollback()
			return failCreate(err.Error())
		}
	}

	// Reserve a private port block for any dev servers the track's repos
	// declare. Worktreeless ask/plan tracks don't run services, so they
	// skip allocation. This is pure arithmetic — nothing is bound here.
	var allocatedPorts map[string]int
	if !kind.Worktreeless() {
		allocatedPorts, err = s.allocatePorts(trackID, repos)
		if err != nil {
			rollback()
			return failCreate("allocate ports: " + err.Error())
		}
	}

	t.Branch = resolvedBranch
	t.Repos = trackRepos
	t.Ports = allocatedPorts
	if err := s.store.Put(t); err != nil {
		rollback()
		return fail("persist state: " + err.Error())
	}

	emit("spawning claude...")
	if _, err := s.startSupervisor(ctx, t); err != nil {
		return failCreate("spawn claude: " + err.Error())
	}
	emit("claude running")
	if kind.Worktreeless() {
		emit(fmt.Sprintf("read-only %s track — run `tracks promote %s` (or menu → Promote) when ready to implement", kind, trackID))
	}

	detail := labelFor(t)
	if t.Branch != "" {
		detail += " on " + t.Branch
	}
	s.notifyEvent(string(notify.EventTrackCreated), "tracks: new track started", detail)

	return ok(NewResult{TrackID: trackID, Branch: resolvedBranch, WindowName: t.WindowName()})
}

// repoSpec is the resolved, ready-to-checkout form of one config.Repo
// the user picked.
type repoSpec struct {
	Name           string
	Path           string
	Base           string
	InitSubmodules bool
	Provision      *config.Provision
}

// resolveBranchCollision picks a branch name guaranteed not to exist
// in any participating primary repo. It tries the original name,
// then -2, -3, … up to -50 before giving up.
func (s *Server) resolveBranchCollision(ctx context.Context, repos []repoSpec, want string) (string, error) {
	for n := 1; n <= 50; n++ {
		candidate := want
		if n > 1 {
			candidate = fmt.Sprintf("%s-%d", want, n)
		}
		clash := false
		for _, r := range repos {
			c := git.NewPrimaryRepoClient(r.Path)
			exists, err := c.BranchExists(ctx, candidate)
			if err != nil {
				return "", fmt.Errorf("check branch %s in %s: %w", candidate, r.Name, err)
			}
			if exists {
				clash = true
				break
			}
		}
		if !clash {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("branch %q (and -2..-50 suffixes) all exist", want)
}

// createWorktrees provisions a worktree per repo. Returns the
// per-repo TrackRepo entries on success, or a rollback closure that
// the caller can invoke to clean up partial state on failure.
//
// emit is called before each slow step (fetch, worktree add,
// submodule init) so callers can stream progress to a user.
// When checkout is non-nil, the worktree is detached at the target
// PR/branch instead of branched fresh off base (used by review tracks).
func (s *Server) createWorktrees(ctx context.Context, root string, repos []repoSpec, branch string, checkout *reviewCheckout, emit Emit) ([]state.TrackRepo, func(), error) {
	created := make([]state.TrackRepo, 0, len(repos))
	rollback := func() {
		for _, tr := range created {
			c := git.NewPrimaryRepoClient(s.primaryPathFor(tr.Name))
			_ = c.RemoveWorktree(ctx, tr.Path)
		}
	}
	for _, r := range repos {
		dest := filepath.Join(root, r.Name)
		primary := git.NewPrimaryRepoClient(r.Path)
		// Always fetch base too: review tracks diff the target against
		// origin/<base>, so the base ref must be present locally.
		emit(fmt.Sprintf("fetching origin/%s in %s...", r.Base, r.Name))
		if err := primary.FetchWithRetry(ctx, "origin", r.Base); err != nil {
			return nil, rollback, fmt.Errorf("fetch %s/%s: %w", r.Name, r.Base, err)
		}
		if checkout != nil {
			emit(fmt.Sprintf("fetching %s in %s...", checkout.fetchRef, r.Name))
			if err := primary.FetchWithRetry(ctx, "origin", checkout.fetchRef); err != nil {
				return nil, rollback, fmt.Errorf("fetch %s in %s: %w", checkout.fetchRef, r.Name, err)
			}
			emit(fmt.Sprintf("checking out %s in %s for review...", checkout.label, r.Name))
			if err := primary.AddWorktreeDetached(ctx, dest, "FETCH_HEAD"); err != nil {
				return nil, rollback, fmt.Errorf("checkout %s for %s: %w", checkout.label, r.Name, err)
			}
		} else {
			emit(fmt.Sprintf("creating worktree for %s on %s...", r.Name, branch))
			if err := primary.AddWorktreeWithRetry(ctx, dest, branch, "origin/"+r.Base); err != nil {
				return nil, rollback, fmt.Errorf("create worktree for %s: %w", r.Name, err)
			}
		}
		created = append(created, state.TrackRepo{Name: r.Name, Path: dest})
		if r.InitSubmodules {
			emit(fmt.Sprintf("initializing submodules in %s (may take a while)...", r.Name))
			wt := git.NewWorktreeClient(dest)
			if err := wt.InitSubmodules(ctx); err != nil {
				return nil, rollback, fmt.Errorf("init submodules in %s: %w", r.Name, err)
			}
		}
		if r.Provision != nil {
			emit(fmt.Sprintf("provisioning %s (copying env + installing deps)...", r.Name))
			if err := provision.Run(ctx, provisionOptions(r.Path, dest, r.Provision), emit); err != nil {
				return nil, rollback, fmt.Errorf("provision %s: %w", r.Name, err)
			}
		}
	}
	return created, rollback, nil
}

// provisionOptions builds provision.Options from a repo's primary path,
// its new worktree path, and its config block.
// allocatePorts reserves a port for every service declared by the track's
// repos, avoiding ports already handed to other live tracks. Returns nil
// when no repo declares a service.
func (s *Server) allocatePorts(trackID string, repos []repoSpec) (map[string]int, error) {
	// Ports are keyed by service name, so names must be unique across the
	// whole track — config validation only enforces uniqueness within a
	// single repo. Two repos declaring the same service name would
	// otherwise share (and waste) a port silently, so reject it loudly.
	declaredBy := map[string]string{}
	var names []string
	for _, r := range repos {
		cr, ok := s.config().RepoByName(r.Name)
		if !ok {
			continue
		}
		for _, svc := range cr.Services {
			if prev, dup := declaredBy[svc.Name]; dup {
				return nil, fmt.Errorf("service name %q is declared by both %q and %q; names must be unique across a track", svc.Name, prev, r.Name)
			}
			declaredBy[svc.Name] = r.Name
			names = append(names, svc.Name)
		}
	}
	if len(names) == 0 {
		return nil, nil
	}
	taken := map[int]bool{}
	for _, t := range s.store.All() {
		for _, p := range t.Ports {
			taken[p] = true
		}
	}
	return ports.Allocate(trackID, names, taken)
}

func provisionOptions(primaryPath, worktreePath string, p *config.Provision) provision.Options {
	return provision.Options{
		PrimaryPath:   primaryPath,
		WorktreePath:  worktreePath,
		CopyIgnored:   p.CopyIgnored,
		CopyMode:      p.CopyMode,
		DepsCmd:       p.DepsCmd,
		CacheStrategy: p.CacheStrategy,
	}
}

// handleDone, handleKill, handleAddRepo, prompts: stubs in step 5.
// They will be filled in once the Claude spawn pipeline lands in
// step 7. Returning a clear "not implemented" error means CLI
// development can proceed against the live daemon without surprise
// crashes.

func (s *Server) handleDone(ctx context.Context, raw json.RawMessage, emit Emit) Response {
	return s.endTrack(ctx, raw, false, emit)
}

func (s *Server) handleKill(ctx context.Context, raw json.RawMessage, emit Emit) Response {
	return s.endTrack(ctx, raw, true, emit)
}

// endTrack is the shared body of done/kill. force=false sends
// SIGTERM and waits up to 5s; force=true SIGKILLs immediately.
//
// emit streams human-readable progress lines back to the caller
// so the popup can show a live console rather than freezing on a
// blank screen while we wait for git to remove worktrees.
func (s *Server) endTrack(ctx context.Context, raw json.RawMessage, force bool, emit Emit) Response {
	var p DoneParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return fail("bad params: " + err.Error())
	}
	t, found := s.store.Get(p.ID)
	if !found {
		return fail("track not found: " + p.ID)
	}
	// If a supervisor is alive, stop it first so the process exits
	// before we yank its worktrees.
	s.mu.Lock()
	sup, ok2 := s.supervisors[t.ID]
	s.mu.Unlock()
	if ok2 {
		if force {
			emit("SIGKILL claude...")
			sup.Kill(s.config().Tmux.SessionName)
		} else {
			emit("SIGTERM claude (5s grace)...")
			sup.Stop(s.config().Tmux.SessionName)
		}
		// Drop the supervisor and release its PR watcher. For a running
		// track the watch goroutine would also do this on the next poll,
		// but a track in review (StatusPR) has no live watch goroutine —
		// only the PR watcher — so we must release it here.
		s.mu.Lock()
		if s.supervisors[t.ID] == sup {
			delete(s.supervisors, t.ID)
		}
		s.mu.Unlock()
		sup.finish()
	}
	// Re-read state — the wait-goroutine may have already written
	// Done/Errored.
	t, _ = s.store.Get(p.ID)

	// Tear down any dev servers before the worktree is removed so they
	// release files and ports first. When a supervisor was alive the
	// Stop/Kill above already stopped them via the in-memory handles;
	// this is the authoritative, state-driven backstop (it also covers a
	// track that finished on its own, leaving services with no live
	// supervisor handle). Idempotent: already-dead groups just ESRCH.
	if len(t.Services) > 0 {
		emit("stopping dev servers...")
		t.Services = stopPersistedServices(t.Services, force)
		_ = s.store.Put(t)
	}

	// Close the track's tmux window. When a supervisor was alive the
	// Stop/Kill above already did this, but a track that finished on
	// its own keeps its pane alive as a shell with no supervisor left
	// to tear it down. Done before worktree removal so the window
	// still closes even if that later fails. Idempotent — KillWindow
	// is a no-op when the window is already gone.
	_ = tmux.New().KillWindow(s.config().Tmux.SessionName, t.WindowName())

	// Remove worktrees, keep branches. Skip any whose checkout is
	// already gone so ending a track is idempotent — a track that
	// finished on its own, or was ended once already, may have no
	// worktree left, and that must not turn into an error.
	//
	// Worktree-less (ask/plan) tracks hold the PRIMARY checkout paths in
	// Repos, not tracks-owned worktrees — never try to remove those.
	for _, tr := range t.Repos {
		if t.Kind.Worktreeless() {
			break
		}
		if _, statErr := os.Stat(tr.Path); os.IsNotExist(statErr) {
			continue
		}
		emit(fmt.Sprintf("removing worktree for %s...", tr.Name))
		c := git.NewPrimaryRepoClient(s.primaryPathFor(tr.Name))
		if err := c.RemoveWorktree(ctx, tr.Path); err != nil {
			return fail(fmt.Sprintf("remove worktree %s: %v", tr.Path, err))
		}
	}
	// Clean up the supervisor's sentinel so a future track with
	// the same id (unlikely but possible after Forget+New) doesn't
	// pick up a stale "claude already exited" signal.
	if path, err := s.sentinelPathFor(t.ID); err == nil {
		_ = os.Remove(path)
	}
	if !t.Status.IsTerminal() {
		t.Status = state.StatusDone
		now := time.Now().UTC()
		t.ExitedAt = &now
	}
	if err := s.store.Put(t); err != nil {
		return fail("persist state: " + err.Error())
	}
	emit("done")
	return ok(nil)
}

func (s *Server) handleAddRepo(ctx context.Context, raw json.RawMessage, emit Emit) Response {
	var p AddRepoParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return fail("bad params: " + err.Error())
	}
	t, found := s.store.Get(p.TrackID)
	if !found {
		return fail("track not found: " + p.TrackID)
	}
	if t.Kind.Worktreeless() {
		return fail("track is read-only (ask/plan); promote it to a worktree first")
	}
	r, ok2 := s.config().RepoByName(p.RepoName)
	if !ok2 {
		return fail("unknown repo: " + p.RepoName)
	}
	// Refuse if this repo is already in the track.
	for _, tr := range t.Repos {
		if tr.Name == p.RepoName {
			return fail(fmt.Sprintf("repo %q already in track", p.RepoName))
		}
	}
	primaryPath, err := r.ResolveRepoPath()
	if err != nil {
		return fail(err.Error())
	}
	stateDir, err := s.config().ResolveStateDir()
	if err != nil {
		return fail(err.Error())
	}
	dest := filepath.Join(stateDir, "worktrees", t.ID, r.Name)
	primary := git.NewPrimaryRepoClient(primaryPath)
	emit(fmt.Sprintf("fetching origin/%s in %s...", r.Base, r.Name))
	if err := primary.Fetch(ctx, "origin", r.Base); err != nil {
		return fail(err.Error())
	}
	emit(fmt.Sprintf("creating worktree for %s on %s...", r.Name, t.Branch))
	if err := primary.AddWorktreeWithRetry(ctx, dest, t.Branch, "origin/"+r.Base); err != nil {
		return fail(err.Error())
	}
	if r.InitSubmodules {
		emit(fmt.Sprintf("initializing submodules in %s...", r.Name))
		wt := git.NewWorktreeClient(dest)
		if err := wt.InitSubmodules(ctx); err != nil {
			return fail(err.Error())
		}
	}
	if r.Provision != nil {
		emit(fmt.Sprintf("provisioning %s...", r.Name))
		if err := provision.Run(ctx, provisionOptions(primaryPath, dest, r.Provision), emit); err != nil {
			// Roll back the worktree so a failed provision doesn't leave
			// a half-set-up repo attached to the track.
			_ = primary.RemoveWorktree(ctx, dest)
			return fail(fmt.Sprintf("provision %s: %v", r.Name, err))
		}
	}
	t.Repos = append(t.Repos, state.TrackRepo{Name: r.Name, Path: dest})
	if err := s.store.Put(t); err != nil {
		return fail(err.Error())
	}
	return ok(AddRepoResult{WorktreePath: dest})
}

// handlePromote turns a worktree-less ask/plan track into a work track:
// it creates a branch + worktree off base for each repo, tears down the
// read-only session, and re-spawns Claude in the worktree with edit
// permissions. A running plan-mode session can't be switched to
// edit-in-place, so promotion is a re-spawn rather than an in-place flip.
func (s *Server) handlePromote(ctx context.Context, raw json.RawMessage, emit Emit) Response {
	var p PromoteParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return fail("bad params: " + err.Error())
	}
	t, found := s.store.Get(p.ID)
	if !found {
		return fail("track not found: " + p.ID)
	}
	if !t.Kind.Worktreeless() {
		return fail("only ask/plan tracks can be promoted; this is already a working track")
	}
	if len(t.Repos) == 0 {
		return fail("track has no repos to promote")
	}

	// Rebuild repoSpecs from the track's repos via config.
	repos := make([]repoSpec, 0, len(t.Repos))
	for _, tr := range t.Repos {
		r, ok := s.config().RepoByName(tr.Name)
		if !ok {
			return fail("unknown repo: " + tr.Name)
		}
		path, err := r.ResolveRepoPath()
		if err != nil {
			return fail(err.Error())
		}
		repos = append(repos, repoSpec{Name: r.Name, Path: path, Base: r.Base, InitSubmodules: r.InitSubmodules, Provision: r.Provision})
	}

	stateDir, err := s.config().ResolveStateDir()
	if err != nil {
		return fail("resolve state dir: " + err.Error())
	}
	worktreeRoot := filepath.Join(stateDir, "worktrees", t.ID)
	resolvedBranch, err := s.resolveBranchCollision(ctx, repos, placeholderBranch(t.ID))
	if err != nil {
		return fail(err.Error())
	}

	// Create the real worktrees BEFORE tearing down the read-only
	// session, so a failure here leaves the existing track untouched.
	trackRepos, rollback, err := s.createWorktrees(ctx, worktreeRoot, repos, resolvedBranch, nil, emit)
	if err != nil {
		rollback()
		return fail(err.Error())
	}

	// Stop the read-only session and close its window before re-spawning.
	// Capture the window name BEFORE promotePrompt rewrites TaskPrompt:
	// the re-spawn must reuse the same window, which holds as long as
	// WindowName() stays stable across the prompt change (it prefers
	// Slug, and promotePrompt keeps the original text first).
	oldWindow := t.WindowName()
	s.mu.Lock()
	sup, alive := s.supervisors[t.ID]
	s.mu.Unlock()
	if alive {
		emit("stopping read-only session...")
		sup.Stop(s.config().Tmux.SessionName)
	}
	_ = tmux.New().KillWindow(s.config().Tmux.SessionName, oldWindow)

	// Re-read (Stop's watcher may have written a terminal status), then
	// flip to a work track and re-spawn with edit permissions.
	t, _ = s.store.Get(p.ID)
	t.Kind = state.KindWork
	t.Repos = trackRepos
	t.Branch = resolvedBranch
	t.Status = state.StatusPending
	t.ExitedAt = nil
	t.ExitCode = nil
	t.ErrorMsg = ""
	t.TaskPrompt = promotePrompt(t.TaskPrompt, resolvedBranch)
	if err := s.store.Put(t); err != nil {
		rollback()
		return fail("persist state: " + err.Error())
	}

	emit("spawning claude in worktree...")
	if _, err := s.startSupervisor(ctx, t); err != nil {
		t.Status = state.StatusErrored
		t.ErrorMsg = "spawn claude: " + err.Error()
		now := time.Now().UTC()
		t.ExitedAt = &now
		_ = s.store.Put(t)
		return fail("spawn claude: " + err.Error())
	}
	emit("claude running")
	s.notifyEvent(string(notify.EventTrackCreated), "tracks: track promoted",
		fmt.Sprintf("%s on %s", labelFor(t), resolvedBranch))
	return ok(PromoteResult{Branch: resolvedBranch, WindowName: t.WindowName()})
}

// promotePrompt seeds the re-spawned work session with the original
// task plus a note that the investigation/plan phase is over and a
// worktree is ready. The original text stays first so the dashboard's
// derived window label remains recognizable.
func promotePrompt(original, branch string) string {
	return strings.TrimRight(original, " \t\n\r") +
		"\n\n---\nThe read-only investigation/plan phase is complete. A worktree " +
		"has been created on branch `" + branch + "` — implement the change here."
}

func (s *Server) handleForget(raw json.RawMessage) Response {
	var p ForgetParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return fail("bad params: " + err.Error())
	}
	if p.ID == "" {
		return fail("id required")
	}
	t, found := s.store.Get(p.ID)
	if !found {
		return fail("track not found: " + p.ID)
	}
	// Refuse to forget a still-running track. Doing so would
	// orphan the supervisor goroutine and leave Claude with no
	// state entry to report into.
	if !t.Status.IsTerminal() {
		return fail(fmt.Sprintf("track %s is %s; run `tracks done %s` first",
			p.ID, t.Status, p.ID))
	}
	if _, err := s.store.Delete(p.ID); err != nil {
		return fail(err.Error())
	}
	return ok(nil)
}

func (s *Server) handlePruneCompleted() Response {
	removed := 0
	for _, t := range s.store.All() {
		if !t.Status.IsTerminal() {
			continue
		}
		if _, err := s.store.Delete(t.ID); err == nil {
			removed++
		}
	}
	return ok(PruneCompletedResult{Removed: removed})
}

func (s *Server) handlePendingPrompts() Response {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]PendingPrompt, 0, len(s.pendingPrompts))
	for _, p := range s.pendingPrompts {
		out = append(out, p.prompt)
	}
	return ok(PendingPromptsResult{Prompts: out})
}

func (s *Server) handleAnswerPrompt(raw json.RawMessage) Response {
	var p AnswerPromptParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return fail(err.Error())
	}
	s.mu.Lock()
	pc, found := s.pendingPrompts[p.ID]
	if found {
		delete(s.pendingPrompts, p.ID)
	}
	s.mu.Unlock()
	if !found {
		return fail("prompt not found: " + p.ID)
	}
	pc.reply <- p.Allow
	close(pc.reply)
	return ok(nil)
}

// RegisterPrompt blocks until a CLI/dashboard caller answers. The
// custom permission-prompt-tool (step 7) calls this from inside
// Claude's flow.
func (s *Server) RegisterPrompt(trackID, tool, detail string) bool {
	id, err := randomID(8)
	if err != nil {
		return false
	}
	reply := make(chan bool, 1)
	s.mu.Lock()
	s.pendingPrompts[id] = promptCh{
		prompt: PendingPrompt{ID: id, TrackID: trackID, Tool: tool, Detail: detail},
		reply:  reply,
	}
	s.mu.Unlock()
	return <-reply
}

// primaryPathFor looks up a configured repo's primary checkout path
// by name. Returns "" for unknown repos.
func (s *Server) primaryPathFor(name string) string {
	r, ok := s.config().RepoByName(name)
	if !ok {
		return ""
	}
	p, _ := r.ResolveRepoPath()
	return p
}

// generateTrackID produces an ID of the form YYYYMMDD-HHMMSS-<6 hex>.
// Sortable, unique enough for ~thousands of tracks, and human-readable.
func generateTrackID() (string, error) {
	suffix, err := randomID(3) // 3 bytes → 6 hex chars
	if err != nil {
		return "", err
	}
	return time.Now().UTC().Format("20060102-150405") + "-" + suffix, nil
}

// randomID returns n random bytes hex-encoded.
func randomID(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// generateSessionID returns a random RFC-4122 v4 UUID string, passed
// to `claude --session-id` so the daemon can locate the track's
// transcript for token-usage accounting.
func generateSessionID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
