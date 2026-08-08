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

	// A document target makes this a doc-review track: no worktree, no
	// git ref — Claude reads a local file (or directory of files) and
	// reviews it, with any attached repos serving as read-only ground
	// truth for the claims it makes.
	var docPath string
	if rawPath := strings.TrimSpace(p.DocPath); rawPath != "" {
		resolved, err := ResolveDocPath(rawPath)
		if err != nil {
			return fail(err.Error())
		}
		docPath = resolved
	}

	// Determine the track kind. Empty defaults to work; unknown values
	// are rejected rather than stored verbatim; a review ref always
	// means a review track regardless of what the client sent, and a
	// document path always means a doc review.
	kind := state.Kind(strings.TrimSpace(p.Kind))
	switch kind {
	case state.KindWork, state.KindReview, state.KindAsk, state.KindPlan, state.KindDoc:
		// recognized
	case "":
		kind = state.KindWork
	default:
		return fail(fmt.Sprintf("unknown track kind %q", p.Kind))
	}
	if checkout != nil {
		kind = state.KindReview
	}
	if docPath != "" {
		if checkout != nil {
			return fail("a track reviews either a PR/branch or a document, not both")
		}
		kind = state.KindDoc
	}
	if kind == state.KindDoc && docPath == "" {
		return fail("a doc review needs a document path (a local file or directory)")
	}

	// Review shape. Candor and the section switches only mean something
	// to a review, so they're cleared on the other kinds rather than
	// stored as dead weight a later promotion could inherit.
	if p.Candor != 0 && (p.Candor < state.MinCandor || p.Candor > state.MaxCandor) {
		return fail(fmt.Sprintf("candor must be between %d and %d (got %d)", state.MinCandor, state.MaxCandor, p.Candor))
	}
	candor := p.Candor
	if kind != state.KindReview && kind != state.KindDoc {
		candor = 0
	}
	skipClaimCheck := p.DocSkipClaimCheck && kind == state.KindDoc
	skipOpinion := p.DocSkipOpinion && kind == state.KindDoc

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

	// A doc-review track has no branch, so with an empty slug its
	// dashboard row and tmux tab would carry nothing identifying. The
	// document's name is the obvious label. Derived here rather than
	// stored on the draft, so relaunching re-derives it from whatever
	// path the user ends up with.
	slug := strings.TrimSpace(p.Slug)
	if slug == "" && docPath != "" {
		base := filepath.Base(docPath)
		slug = strings.TrimSuffix(base, filepath.Ext(base))
	}

	// Build the track record up front so any failure during provisioning
	// can be persisted as an errored track — carrying the prompt and the
	// reason — instead of vanishing as a transient CLI error. A git fetch
	// that dies mid-network-drop, a port clash, or a spawn error then
	// shows up in the dashboard, where the preserved prompt makes it easy
	// to retry and the message makes it easy to debug.
	t := state.Track{
		ID:                trackID,
		Branch:            branch,
		Slug:              slug,
		Kind:              kind,
		Status:            state.StatusPending,
		DocPath:           docPath,
		Candor:            candor,
		DocSkipClaimCheck: skipClaimCheck,
		DocSkipOpinion:    skipOpinion,
		LogPath:           logPath,
		TaskPrompt:        p.TaskPrompt,
		SessionID:         sessionID,
		CreatedAt:         time.Now().UTC(),
	}
	// draft captures exactly what the user entered so a failed creation
	// can be saved and relaunched without re-typing anything. Stored on
	// the errored track and carried through to a StatusDraft if the user
	// saves it. Kind is the resolved kind (review refs force review).
	draft := &state.DraftSpec{
		Repos:      p.Repos,
		TaskPrompt: p.TaskPrompt,
		Slug:       strings.TrimSpace(p.Slug),
		ReviewRef:  strings.TrimSpace(p.ReviewRef),
		// Resolved, not raw: a relative path would otherwise re-resolve
		// against the daemon's cwd on a later relaunch.
		DocPath:           docPath,
		Kind:              string(kind),
		Candor:            candor,
		DocSkipClaimCheck: skipClaimCheck,
		DocSkipOpinion:    skipOpinion,
	}
	// failCreate persists the in-progress track as errored (with the
	// reason and the draft spec) and returns the wire error, so the
	// failure is visible and retryable from the dashboard rather than
	// only in the CLI response. The track ID rides along in the result
	// even on failure so an interactive caller can offer to save the
	// attempt as a draft or dismiss it (see MethodSaveDraft / Forget).
	failCreate := func(msg string) Response {
		if hint := authHintPrefix(msg); hint != "" {
			msg = hint + msg
		}
		t.Status = state.StatusErrored
		t.ErrorMsg = msg
		t.Draft = draft
		now := time.Now().UTC()
		t.ExitedAt = &now
		s.persist(t, "failed creation")
		resultRaw, _ := json.Marshal(NewResult{TrackID: trackID})
		return Response{Ok: false, Error: msg, Result: resultRaw}
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
		trackRepos, rollback, err = s.createWorktrees(ctx, worktreeRoot, repos, resolvedBranch, checkout, emit, true)
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
	switch {
	case kind == state.KindDoc:
		emit("reviewing " + docPath)
	case kind.Worktreeless():
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
// skipDeps, when true, skips running DepsCmd so that dependency
// installation is deferred to the first `tracks up` call.
func (s *Server) createWorktrees(ctx context.Context, root string, repos []repoSpec, branch string, checkout *reviewCheckout, emit Emit, skipDeps bool) ([]state.TrackRepo, func(), error) {
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
			// Only defer deps when the caller asked AND the repo has at
			// least one service — repos with no services never trigger
			// `tracks up`, so they must install deps eagerly here.
			deferDeps := skipDeps
			if deferDeps {
				cfgRepo, ok := s.config().RepoByName(r.Name)
				if !ok || len(cfgRepo.Services) == 0 {
					deferDeps = false
				}
			}
			msg := "provisioning %s (copying env files; deps deferred to first `tracks up`)..."
			if !deferDeps {
				msg = "provisioning %s (copying env + installing deps)..."
			}
			emit(fmt.Sprintf(msg, r.Name))
			if err := provision.Run(ctx, provisionOptions(r.Path, dest, r.Provision, deferDeps), emit); err != nil {
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

func provisionOptions(primaryPath, worktreePath string, p *config.Provision, skipDeps bool) provision.Options {
	return provision.Options{
		PrimaryPath:   primaryPath,
		WorktreePath:  worktreePath,
		CopyIgnored:   p.CopyIgnored,
		CopyMode:      p.CopyMode,
		DepsCmd:       p.DepsCmd,
		CacheStrategy: p.CacheStrategy,
		SkipDepsCmd:   skipDeps,
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
	// A draft has no Claude process, worktree, or tmux window to end.
	// Ending it would flip it to Done (terminal) via the tail below and
	// make it eligible for prune — silently destroying the saved
	// parameters this feature exists to preserve. Dismiss it with `x`
	// (forget) or launch it with `L` instead.
	if t.Status == state.StatusDraft {
		return fail(fmt.Sprintf("track %s is a draft — dismiss it (x) or launch it (L); a draft can't be ended", p.ID))
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
		// but a track in review (StatusPROpen) has no live watch goroutine —
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
	// release files and ports first. Dev servers run in their own panes
	// (separate process groups), so sup.Stop/Kill above did NOT touch them
	// — this state-driven kill by persisted PGID is their teardown, and it
	// also covers a track that finished on its own with no live supervisor.
	// Idempotent: already-dead groups just ESRCH.
	if len(t.Services) > 0 {
		emit("stopping dev servers...")
		t.Services = stopPersistedServices(t.Services, force)
		s.persist(t, "stopped dev servers")
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
	// Completed rather than IsTerminal: ending an *interrupted* track is
	// the user saying they're finished with it, so it must settle on an end
	// state and stop being offered for reopen (its worktree is gone now
	// anyway).
	if !t.Status.Completed() {
		if t.Status == state.StatusInterrupted {
			// The "tracks was shut down…" note described a state the track
			// is no longer in; leaving it behind would misreport a closed track.
			t.ErrorMsg = ""
		}
		t.Status = terminalStatusFor(t)
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
		return fail("track is read-only (ask/plan/doc); for ask/plan, promote it to a worktree first")
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
		// Defer deps only when services exist to trigger `tracks up`.
		deferDeps := len(r.Services) > 0
		msg := "provisioning %s (copying env files; deps deferred to first `tracks up`)..."
		if !deferDeps {
			msg = "provisioning %s (copying env + installing deps)..."
		}
		emit(fmt.Sprintf(msg, r.Name))
		if err := provision.Run(ctx, provisionOptions(primaryPath, dest, r.Provision, deferDeps), emit); err != nil {
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
	// Doc reviews are worktree-less but not promotable: the target is a
	// document, not a diff, so "start editing the code" has no meaning
	// here — the follow-up to a doc review is a new work track. Allowing
	// it would also strand DocPath on a work-kind record.
	if t.Kind == state.KindDoc {
		return fail("a doc-review track can't be promoted; start a work track for the changes it found")
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
	trackRepos, rollback, err := s.createWorktrees(ctx, worktreeRoot, repos, resolvedBranch, nil, emit, true)
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
		s.persist(t, "promote spawn failure")
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

// handleResume re-opens a finished track's Claude session. It:
//  1. Verifies the track is terminal and has a SessionID.
//  2. Re-creates any worktrees that were removed by Done, on the same branch.
//  3. Resets the track to a pending state.
//  4. Spawns claude --resume <sessionID> in a new tmux window.
func (s *Server) handleResume(ctx context.Context, raw json.RawMessage, emit Emit) Response {
	var p ResumeParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return fail("bad params: " + err.Error())
	}
	t, found := s.store.Get(p.ID)
	if !found {
		return fail("track not found: " + p.ID)
	}
	if !t.Status.IsTerminal() {
		return fail(fmt.Sprintf("track %s is %s; only finished tracks can be resumed", p.ID, t.StatusLabel()))
	}
	if t.SessionID == "" {
		return fail("track has no session ID; cannot resume")
	}

	window, err := s.resumeTrackSession(ctx, t, emit)
	if err != nil {
		return fail(err.Error())
	}
	return ok(ResumeResult{WindowName: window})
}

// resumeTrackSession re-creates any worktrees the track lost, resets its
// status, and spawns `claude --resume <session-id>` in a fresh tmux
// window. Returns the window name.
//
// Shared by MethodResume (one track the user picked) and MethodReopen
// (every track interrupted by the last shutdown). Callers own the
// eligibility checks; this does the work.
//
// The track is claimed atomically (terminal → Pending) before any work
// starts, so two concurrent resumes — two `tracks reopen` runs, or the
// startup prompt answered in two terminals — can't both spawn
// `claude --resume` on the same session. On failure the claim is
// released back to the status the track came in with, keeping an
// interrupted track interrupted (and therefore retryable, and out of
// reach of prune/gc) rather than downgrading it to errored.
func (s *Server) resumeTrackSession(ctx context.Context, t state.Track, emit Emit) (string, error) {
	// Refuse once the daemon is winding down: a window spawned now would
	// have its watcher cancelled immediately, leaving `running` behind for
	// the next start to clean up.
	if s.shuttingDown.Load() {
		return "", fmt.Errorf("tracks is shutting down")
	}

	// mutate runs exactly once under the store's lock in both Store
	// implementations, so a flag set inside it is an exact "we took the
	// claim" signal — the returned Track alone can't distinguish our own
	// Pending from one another caller just wrote.
	var prev state.Track
	var claimedByUs bool
	claimed, found, err := s.store.Update(t.ID, func(cur *state.Track) bool {
		if !cur.Status.IsTerminal() {
			return false // already claimed, or live again
		}
		prev = *cur
		cur.Status = state.StatusPending
		cur.ExitedAt = nil
		cur.ExitCode = nil
		cur.ErrorMsg = ""
		claimedByUs = true
		return true
	})
	if !found {
		return "", fmt.Errorf("track not found: %s", t.ID)
	}
	if !claimedByUs {
		if err != nil {
			return "", fmt.Errorf("persist state: %w", err)
		}
		return "", fmt.Errorf("track %s is %s — already being resumed", t.ID, claimed.Status)
	}
	t = claimed

	// release hands the claim back, restoring the exit bookkeeping the
	// claim cleared — a resume that fails must not make a finished track
	// look like it just exited (which would inflate its reported runtime)
	// or lose its exit code.
	release := func(status state.Status, msg string) {
		s.update(t.ID, "resume claim release", func(cur *state.Track) bool {
			cur.Status = status
			cur.ErrorMsg = msg
			cur.ExitCode = prev.ExitCode
			cur.ExitedAt = prev.ExitedAt
			if status.IsTerminal() && cur.ExitedAt == nil {
				now := time.Now().UTC()
				cur.ExitedAt = &now
			}
			return true
		})
	}

	// FileStore.Update mutates its in-memory map before flushing and
	// reports the flush error, so a failed write leaves the claim applied.
	// Hand it back rather than wedging the track in Pending with no
	// supervisor behind it.
	if err != nil {
		release(prev.Status, "resume: persist state: "+err.Error())
		return "", fmt.Errorf("persist state: %w", err)
	}

	// Re-create any worktrees that were removed when the track was closed.
	// Worktree-less (ask/plan) tracks have no worktrees to restore.
	if !t.Kind.Worktreeless() {
		for _, tr := range t.Repos {
			if _, err := os.Stat(tr.Path); err == nil {
				continue // worktree still on disk
			}
			restoreErr := func(err error) error {
				release(prev.Status, err.Error())
				return err
			}
			r, ok := s.config().RepoByName(tr.Name)
			if !ok {
				return "", restoreErr(fmt.Errorf("unknown repo: %s", tr.Name))
			}
			primaryPath, err := r.ResolveRepoPath()
			if err != nil {
				return "", restoreErr(err)
			}
			branch := tr.Branch
			if branch == "" {
				branch = t.Branch
			}
			if branch == "" {
				return "", restoreErr(fmt.Errorf("cannot determine branch for repo %s", tr.Name))
			}
			emit(fmt.Sprintf("re-creating worktree for %s on %s...", tr.Name, branch))
			primary := git.NewPrimaryRepoClient(primaryPath)
			// Prune stale administrative entries first so a previously
			// force-removed worktree dir doesn't block re-creation.
			_ = primary.PruneWorktrees(ctx)
			if err := primary.CheckoutWorktree(ctx, tr.Path, branch); err != nil {
				return "", restoreErr(fmt.Errorf("re-create worktree %s: %w", tr.Name, err))
			}
		}
	}

	// Tear down any dev servers still recorded on the track before their
	// panes go with the window below. A pane kill is cosmetic — the
	// authoritative teardown is the process-group kill by persisted PGID
	// (see stopAllSupervisors) — so skipping this would leave Services
	// claiming `ready` for processes nobody owns, and a later `tracks up`
	// would decline to start them ("already running"). Same order
	// endTrack uses. Usually a no-op for an interrupted track, whose
	// services were already stopped at shutdown; it's a resumed *done*
	// track that can still have live ones.
	if t, ok := s.store.Get(t.ID); ok && len(t.Services) > 0 {
		emit("stopping dev servers left from the previous run...")
		s.teardownTrackServices(t.ID, true)
	}

	// Close any window left over from the track's previous life. A track
	// interrupted by an unclean death (tmux survived, only the pane's
	// process died) still has its window, and tmux happily creates a
	// second one with the same name — after which selecting or killing
	// that window by name is ambiguous. Idempotent when there's none.
	_ = tmux.New().KillWindow(s.config().Tmux.SessionName, t.WindowName())

	emit("spawning claude (resume)...")
	if _, err := s.startSupervisorResume(ctx, t); err != nil {
		// An interrupted track stays interrupted: the user hasn't finished
		// with it, and Errored is Completed() — which would put its
		// worktree in reach of prune-completed and `tracks gc`.
		failStatus := state.StatusErrored
		if prev.Status == state.StatusInterrupted {
			failStatus = state.StatusInterrupted
		}
		release(failStatus, "spawn claude: "+err.Error())
		return "", fmt.Errorf("spawn claude: %w", err)
	}
	emit("claude running")
	// Worktree-less kinds (doc/ask/plan) have no branch, so the "on
	// <branch>" tail is omitted rather than rendered blank — same shape
	// handleNew uses for its own notification.
	detail := labelFor(t)
	if t.Branch != "" {
		detail += " on " + t.Branch
	}
	s.notifyEvent(string(notify.EventTrackCreated), "tracks: track resumed", detail)
	return t.WindowName(), nil
}

// handleReopen brings back the tracks that were interrupted when tracks
// last shut down — the counterpart to markInterruptedOnShutdown. With no
// IDs it reopens every interrupted track, oldest first; with IDs it
// reopens exactly those.
//
// Failures are per-track: one track whose branch has since been deleted
// must not stop the others from coming back, so each is reported in the
// result rather than failing the whole call.
func (s *Server) handleReopen(ctx context.Context, raw json.RawMessage, emit Emit) Response {
	var p ReopenParams
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &p); err != nil {
			return fail("bad params: " + err.Error())
		}
	}

	targets, err := s.reopenTargets(p.IDs)
	if err != nil {
		return fail(err.Error())
	}

	var res ReopenResult
	for _, t := range targets {
		if !t.Resumable() {
			res.Failed = append(res.Failed, ReopenFailure{
				ID:    t.ID,
				Error: "no session ID — this track predates session tracking and can't be reopened",
			})
			continue
		}
		emit(fmt.Sprintf("reopening %s (%s)...", t.ID, labelFor(t)))
		window, err := s.resumeTrackSession(ctx, t, emit)
		if err != nil {
			res.Failed = append(res.Failed, ReopenFailure{ID: t.ID, Error: err.Error()})
			continue
		}
		res.Reopened = append(res.Reopened, ReopenedTrack{ID: t.ID, WindowName: window})
	}
	return ok(res)
}

// reopenTargets resolves the tracks handleReopen should act on. An empty
// ids slice selects every interrupted track, oldest first (Store.All is
// CreatedAt-ascending) so reopened windows land in the order the tracks
// were created. Explicit ids are validated: a track that isn't
// interrupted is a caller mistake worth an error, not a silent skip
// (`tracks resume` is the way to re-open a finished one).
func (s *Server) reopenTargets(ids []string) ([]state.Track, error) {
	if len(ids) > 0 {
		out := make([]state.Track, 0, len(ids))
		for _, id := range ids {
			t, found := s.store.Get(id)
			if !found {
				return nil, fmt.Errorf("track not found: %s", id)
			}
			if t.Status != state.StatusInterrupted {
				return nil, fmt.Errorf("track %s is %s, not interrupted; use `tracks resume %s` instead",
					id, t.Status, id)
			}
			out = append(out, t)
		}
		return out, nil
	}

	var out []state.Track
	for _, t := range s.store.All() {
		if t.Status == state.StatusInterrupted {
			out = append(out, t)
		}
	}
	return out, nil
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
	// state entry to report into. A draft has no process, so it can
	// always be dismissed.
	if !t.Status.IsTerminal() && t.Status != state.StatusDraft {
		return fail(fmt.Sprintf("track %s is %s; run `tracks done %s` first",
			p.ID, t.StatusLabel(), p.ID))
	}
	if _, err := s.store.Delete(p.ID); err != nil {
		return fail(err.Error())
	}
	return ok(nil)
}

// handleSaveDraft turns a failed-creation (or otherwise finished) track
// that still carries its Draft parameters into a saved draft, so the
// user keeps what they entered instead of dismissing the attempt. The
// draft can be launched later (see handleLaunch). ErrorMsg is preserved
// so the dashboard can show why it became a draft.
func (s *Server) handleSaveDraft(raw json.RawMessage) Response {
	var p SaveDraftParams
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
	if t.Draft == nil {
		return fail(fmt.Sprintf("track %s has no saved parameters to draft", p.ID))
	}
	if !t.Status.IsTerminal() && t.Status != state.StatusDraft {
		return fail(fmt.Sprintf("track %s is %s; only a finished or failed track can be saved as a draft", p.ID, t.StatusLabel()))
	}
	t.Status = state.StatusDraft
	t.ExitedAt = nil
	t.ExitCode = nil
	if err := s.store.Put(t); err != nil {
		return fail("persist state: " + err.Error())
	}
	return ok(nil)
}

// handleLaunch (re)creates a track from its saved Draft parameters. It
// reuses handleNew's full creation path (fresh worktree, fresh ID), then
// drops the original draft on success. If creation fails again, the
// throwaway errored record handleNew persisted is removed and the
// original draft is kept — with its reason refreshed — so nothing is
// lost and it stays launchable.
func (s *Server) handleLaunch(ctx context.Context, raw json.RawMessage, emit Emit) Response {
	var p LaunchParams
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
	if t.Draft == nil {
		return fail(fmt.Sprintf("track %s has no saved parameters to launch", p.ID))
	}
	if !t.Status.IsTerminal() && t.Status != state.StatusDraft {
		return fail(fmt.Sprintf("track %s is %s; only a draft or finished/failed track can be launched", p.ID, t.StatusLabel()))
	}

	params := NewParams{
		Repos:             t.Draft.Repos,
		TaskPrompt:        t.Draft.TaskPrompt,
		Slug:              t.Draft.Slug,
		ReviewRef:         t.Draft.ReviewRef,
		DocPath:           t.Draft.DocPath,
		Kind:              t.Draft.Kind,
		Candor:            t.Draft.Candor,
		DocSkipClaimCheck: t.Draft.DocSkipClaimCheck,
		DocSkipOpinion:    t.Draft.DocSkipOpinion,
	}
	rawNew, err := json.Marshal(params)
	if err != nil {
		return fail("marshal params: " + err.Error())
	}

	resp := s.handleNew(ctx, rawNew, emit)
	if resp.Ok {
		// Draft consumed by a successful launch.
		s.forget(p.ID, "draft consumed by launch")
		return resp
	}

	// Relaunch failed. handleNew persisted a fresh errored+draft record
	// under a new ID; drop it so we don't leave a duplicate, and keep the
	// original draft, refreshing its reason to the latest failure.
	if len(resp.Result) > 0 {
		var nr NewResult
		if json.Unmarshal(resp.Result, &nr) == nil && nr.TrackID != "" && nr.TrackID != p.ID {
			s.forget(nr.TrackID, "duplicate record from a failed relaunch")
		}
	}
	if cur, ok := s.store.Get(p.ID); ok {
		cur.ErrorMsg = resp.Error
		s.persist(cur, "refreshed draft failure reason")
	}
	return resp
}

// authHintPrefix returns a short, actionable prefix when msg looks like a
// GitHub authentication / authorization failure — an expired token,
// revoked SSH key, or a repo the user can't access — so a failed creation
// tells the user to re-authenticate rather than surfacing a raw git
// error. Returns "" for anything that isn't clearly an auth problem.
func authHintPrefix(msg string) string {
	m := strings.ToLower(msg)
	// Unambiguously auth/remote-flavored: fire on their own.
	for _, needle := range []string{
		"authentication failed",
		"invalid username or password",
		"terminal prompts disabled",
		"could not read username",
		"support for password authentication was removed",
		"remote: permission to",
		"please make sure you have the correct access rights",
	} {
		if strings.Contains(m, needle) {
			return authHintText
		}
	}
	// Generic phrases ("permission denied", "403") also show up in local
	// filesystem errors, so only treat them as a GitHub auth problem when
	// the message is clearly about a git remote.
	remote := strings.Contains(m, "github.com") || strings.Contains(m, "git@") ||
		strings.Contains(m, "https://") || strings.Contains(m, "remote:") ||
		strings.Contains(m, "fetch ") || strings.Contains(m, "origin")
	if remote {
		for _, needle := range []string{"permission denied", "access denied", "403 forbidden", "invalid credentials"} {
			if strings.Contains(m, needle) {
				return authHintText
			}
		}
	}
	return ""
}

const authHintText = "GitHub access denied — your token or SSH key may be expired or lack access to this repo. Re-authenticate, then launch this draft again.\n\n"

func (s *Server) handlePruneCompleted() Response {
	removed := 0
	for _, t := range s.store.All() {
		// Completed only: an interrupted track is terminal but the user
		// still means to reopen it, so a "clear completed" sweep must not
		// take it.
		if !t.Status.Completed() {
			continue
		}
		if s.forget(t.ID, "prune completed") {
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
