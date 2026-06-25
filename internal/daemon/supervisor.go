package daemon

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/bluegardenproject/tracks/internal/claude"
	"github.com/bluegardenproject/tracks/internal/git"
	"github.com/bluegardenproject/tracks/internal/notify"
	"github.com/bluegardenproject/tracks/internal/state"
	"github.com/bluegardenproject/tracks/internal/tmux"
)

// supervisor wraps one running Claude session for one track.
// Unlike the original child-process model, the process itself is
// owned by tmux: we tracked it down via the pane_pid returned when
// the window was opened, and we watch it with kill(pid, 0) +
// `tmux has-window` checks.
//
// Owning the process via tmux is what lets the user *interact*
// with Claude in the track window. A daemon-owned child process
// would have no TTY and no way for the user to type into it.
type supervisor struct {
	trackID      string
	windowName   string
	pid          int
	sentinelPath string
	cancel       context.CancelFunc
	done         chan struct{}

	// lastPane is the most recent capture-pane snapshot. Used to
	// detect a stalled pane (= Claude waiting for user input).
	lastPane         string
	lastPaneChangeAt time.Time

	// prWatcherStarted gates startPRWatcher to a single goroutine
	// per track, since the pane-snapshot scan can re-detect the
	// same URL on every poll.
	prWatcherStarted bool

	// lastWaitingNotifyAt rate-limits EventWaiting per track. The
	// status flicks Running↔Waiting whenever Claude's TUI ticks a
	// spinner; without throttling we'd notify the user every few
	// seconds on the same outstanding question.
	lastWaitingNotifyAt time.Time
}

// waitingNotifyMinInterval is the shortest gap between two
// EventWaiting notifications for the same track. The user will
// notice the dashboard's pink "waiting" highlight; an OS-level
// notification at this cadence is enough without spamming.
const waitingNotifyMinInterval = 2 * time.Minute

// startSupervisor opens a tmux window for the track with claude
// running inside it and starts the watcher goroutines.
func (s *Server) startSupervisor(ctx context.Context, t state.Track) (*supervisor, error) {
	sentinelPath, err := s.sentinelPathFor(t.ID)
	if err != nil {
		return nil, err
	}
	// A stale sentinel from a previous run (e.g. crash + restart)
	// would make the supervisor finalize instantly. Remove it.
	_ = os.Remove(sentinelPath)

	opts, err := claude.BuildOptions(s.cfg, t, s.socketDir, sentinelPath)
	if err != nil {
		return nil, err
	}
	tm := tmux.New()
	window := t.WindowName()
	pid, err := tm.NewWindowReturningPaneID(s.cfg.Tmux.SessionName, window, opts.ShellCommand(), opts.CWD)
	if err != nil {
		return nil, fmt.Errorf("open tmux window: %w", err)
	}

	supCtx, cancel := context.WithCancel(ctx)
	sup := &supervisor{
		trackID:          t.ID,
		windowName:       window,
		pid:              pid,
		sentinelPath:     sentinelPath,
		cancel:           cancel,
		done:             make(chan struct{}),
		lastPaneChangeAt: time.Now(),
	}

	// Persist the live state.
	t.Status = state.StatusRunning
	t.PID = pid
	if err := s.store.Put(t); err != nil {
		// We've already opened the window. Close it and bail —
		// otherwise the daemon would be orphaned from the truth.
		_ = tm.KillWindow(s.cfg.Tmux.SessionName, window)
		cancel()
		return nil, fmt.Errorf("persist running state: %w", err)
	}

	s.mu.Lock()
	if s.supervisors == nil {
		s.supervisors = make(map[string]*supervisor)
	}
	s.supervisors[t.ID] = sup
	s.mu.Unlock()

	go s.watchTrackProcess(supCtx, sup)
	return sup, nil
}

// watchTrackProcess polls the pane pid + the sentinel file to
// decide when Claude has finished, and watches the pane's
// rendered contents to detect when Claude is sitting at its
// prompt waiting for user input.
//
// Liveness rules:
//
//   - sentinel exists → Claude exited; finalize (pane stays alive
//     as a regular shell so the user can poke around the worktree).
//   - pid dead → backstop for when the wrapper itself died; finalize.
//   - otherwise → pane content drives running/waiting state.
func (s *Server) watchTrackProcess(ctx context.Context, sup *supervisor) {
	defer close(sup.done)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	tm := tmux.New()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if sup.sentinelPath != "" {
				if _, err := os.Stat(sup.sentinelPath); err == nil {
					s.finalizeTrack(sup.trackID)
					s.mu.Lock()
					delete(s.supervisors, sup.trackID)
					s.mu.Unlock()
					return
				}
			}
			if !processAlive(sup.pid) {
				s.finalizeTrack(sup.trackID)
				s.mu.Lock()
				delete(s.supervisors, sup.trackID)
				s.mu.Unlock()
				return
			}
			s.refreshRunningStatus(tm, sup)
		}
	}
}

// sentinelPathFor returns the file the shell wrapper touches when
// Claude exits. Lives under <state_dir>/sentinels/<track-id>.done
// so the daemon can find them across restarts.
func (s *Server) sentinelPathFor(trackID string) (string, error) {
	dir, err := s.cfg.ResolveStateDir()
	if err != nil {
		return "", err
	}
	sentinelDir := filepath.Join(dir, "sentinels")
	if err := os.MkdirAll(sentinelDir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(sentinelDir, trackID+".done"), nil
}

// paneIdleThreshold is how long the pane must be unchanged before
// we flip the track to Waiting. Short enough that the dashboard
// reflects "Claude wants you" within a few seconds; long enough
// that a brief thinking pause doesn't flicker.
const paneIdleThreshold = 6 * time.Second

// refreshRunningStatus captures the pane content and updates the
// stored track to Running or Waiting based on whether the snapshot
// changed since the last poll. Also persists a short
// ANSI-stripped tail of the pane on the track so the dashboard can
// surface what's happening without switching windows.
//
// Errors from capture-pane are swallowed — they shouldn't bring
// down the supervisor.
func (s *Server) refreshRunningStatus(tm *tmux.Client, sup *supervisor) {
	snapshot, err := tm.CapturePane(s.cfg.Tmux.SessionName, sup.windowName)
	if err != nil {
		return
	}
	if snapshot != sup.lastPane {
		sup.lastPane = snapshot
		sup.lastPaneChangeAt = time.Now()
	}
	t, ok := s.store.Get(sup.trackID)
	if !ok || t.Status.IsTerminal() {
		return
	}
	idle := time.Since(sup.lastPaneChangeAt) > paneIdleThreshold
	target := t.Status
	switch {
	case idle && t.Status != state.StatusWaiting:
		target = state.StatusWaiting
	case !idle && t.Status != state.StatusRunning:
		target = state.StatusRunning
	}
	snippet, awaiting := paneSnippet(snapshot)
	prURL := scanForPRURL(snapshot)
	changes := s.aggregateChanges(t)
	updatedRepos, rolledUpBranch := s.refreshBranches(t)

	prevStatus := t.Status
	prevPRURL := t.PRURL

	if target == t.Status &&
		snippet == t.LastOutput &&
		awaiting == t.AwaitingInput &&
		(prURL == "" || prURL == t.PRURL) &&
		changes == t.Changes &&
		rolledUpBranch == t.Branch &&
		reposBranchesEqual(updatedRepos, t.Repos) {
		return
	}
	t.Status = target
	t.LastOutput = snippet
	t.AwaitingInput = awaiting
	t.Changes = changes
	t.Repos = updatedRepos
	t.Branch = rolledUpBranch
	if prURL != "" && prURL != t.PRURL {
		t.PRURL = prURL
	}
	_ = s.store.Put(t)

	// Fire notifications on the transitions that matter to a user
	// who isn't looking at the dashboard right now. EventWaiting
	// gets a per-track cooldown so the Running↔Waiting flicker
	// caused by Claude's TUI spinners doesn't spam the user.
	if target == state.StatusWaiting && prevStatus != state.StatusWaiting {
		if time.Since(sup.lastWaitingNotifyAt) >= waitingNotifyMinInterval {
			s.notifyEvent(string(notify.EventWaiting), "tracks: Claude needs you",
				labelFor(t)+" is waiting for input")
			sup.lastWaitingNotifyAt = time.Now()
		}
	}
	if t.PRURL != "" && prevPRURL == "" {
		s.notifyEvent(string(notify.EventPROpened), "tracks: PR opened",
			labelFor(t)+" → "+t.PRURL)
		// Kick off the gh-poll loop for this PR.
		s.startPRWatcher(sup, t.PRURL)
	}
}

// labelFor returns a short human label for a track — slug if the
// user gave one, otherwise the branch name. Used in notification
// bodies so the user can tell which track wants them.
func labelFor(t state.Track) string {
	if t.Slug != "" {
		return t.Slug
	}
	return t.Branch
}

// notifyEvent forwards to the notifier only when the user's
// config has the event enabled. Centralised here so every call
// site stays a one-liner.
func (s *Server) notifyEvent(event, title, body string) {
	if !s.cfg.Notify.EventEnabled(event) {
		return
	}
	s.notifier.Send(title, body)
}

// prURLPattern matches the TRACKS_PR_URL=<url> marker Claude is
// asked to emit in the prompt suffix (see internal/claude/spawn.go).
//
// We deliberately require either an http(s) URL or the literal
// `none` sentinel. The instruction text in the suffix uses the
// placeholder `<url>` to teach Claude the format, and a permissive
// `\S+` capture would (incorrectly) grab that placeholder on the
// very first poll — before Claude has produced anything — and
// fire a fake "PR opened" notification.
var prURLPattern = regexp.MustCompile(`TRACKS_PR_URL=(https?://\S+|none)`)

// refreshBranches re-reads each worktree's current branch via
// `git branch --show-current` and returns an updated copy of
// t.Repos together with a roll-up branch name for the top-level
// Track.Branch field.
//
// Roll-up rule: prefer the first worktree branch that isn't the
// daemon's `tracks/<id-tail>` placeholder — that's the one Claude
// renamed for the actual commit. If every worktree is still on
// the placeholder we keep the placeholder so the dashboard's
// branch column never goes blank.
func (s *Server) refreshBranches(t state.Track) ([]state.TrackRepo, string) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out := make([]state.TrackRepo, len(t.Repos))
	rolled := t.Branch
	rolledIsPlaceholder := strings.HasPrefix(rolled, "tracks/")
	for i, tr := range t.Repos {
		out[i] = tr
		current, err := git.NewWorktreeClient(tr.Path).CurrentBranch(ctx)
		if err != nil || current == "" {
			continue
		}
		out[i].Branch = current
		// First non-placeholder we find wins the roll-up.
		if !strings.HasPrefix(current, "tracks/") && rolledIsPlaceholder {
			rolled = current
			rolledIsPlaceholder = false
		}
	}
	return out, rolled
}

// reposBranchesEqual reports whether two TrackRepo slices carry
// identical Branch fields in order. Used to short-circuit
// state.json writes when nothing actually changed.
func reposBranchesEqual(a, b []state.TrackRepo) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Branch != b[i].Branch {
			return false
		}
	}
	return true
}

// aggregateChanges sums ShortStat results across every worktree
// the track owns. Cross-repo tracks then read as a single row in
// the dashboard — same shape as the `Repos` field.
//
// Uses its own short-deadline context so a stuck git invocation
// can't wedge the supervisor's 2-second poll loop.
func (s *Server) aggregateChanges(t state.Track) state.Changes {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var agg state.Changes
	for _, tr := range t.Repos {
		repo, ok := s.cfg.RepoByName(tr.Name)
		if !ok {
			continue
		}
		stat := git.NewWorktreeClient(tr.Path).ShortStat(ctx, "origin/"+repo.Base)
		agg.Files += stat.Files
		agg.Insertions += stat.Insertions
		agg.Deletions += stat.Deletions
	}
	return agg
}

// scanForPRURL pulls the URL portion out of a TRACKS_PR_URL=<url>
// marker in the pane snapshot. Returns "" when no marker is
// present or the value is the sentinel "none".
func scanForPRURL(snapshot string) string {
	matches := prURLPattern.FindStringSubmatch(snapshot)
	if len(matches) < 2 {
		return ""
	}
	v := strings.TrimSpace(matches[1])
	if v == "" || v == "none" {
		return ""
	}
	return v
}

// paneSnippet returns a snippet of pane content suitable for the
// dashboard, plus a bool indicating whether Claude is sitting at a
// confirmation/choice prompt.
//
// Claude's TUI renders such prompts with a distinctive `☐ Confirm`
// (or other `☐ <title>`) header at the top and a numbered option
// list with `❯ \d+\.` for the cursor. When we see those markers,
// we return everything from the marker downward so the user can
// read the whole question. Otherwise we fall back to the last
// handful of non-empty lines.
func paneSnippet(snapshot string) (string, bool) {
	stripped := stripANSI(snapshot)
	lines := strings.Split(stripped, "\n")
	// `tmux capture-pane` returns each line padded to the original
	// pane's width with trailing spaces. Strip them so downstream
	// renderers don't think a 200-char blank tail is real content.
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t\r")
	}

	if start := findPromptStart(lines); start >= 0 {
		end := len(lines)
		for end > start && lines[end-1] == "" {
			end--
		}
		return collapseBlanks(lines[start:end]), true
	}

	// No prompt marker — return last 8 non-empty lines.
	const n = 8
	out := make([]string, 0, n)
	for i := len(lines) - 1; i >= 0 && len(out) < n; i-- {
		if lines[i] == "" {
			continue
		}
		out = append([]string{lines[i]}, out...)
	}
	return strings.Join(out, "\n"), false
}

// collapseBlanks turns runs of empty lines into a single empty
// line AND drops decorative-only lines (long unbroken runs of
// box-drawing characters Claude's TUI uses to separate sections).
// Both look fine in the live pane but waste space — and confuse
// the dashboard's wrap logic — in the snippet.
func collapseBlanks(lines []string) string {
	out := make([]string, 0, len(lines))
	prevBlank := false
	for _, line := range lines {
		if isDecorative(line) {
			continue
		}
		blank := line == ""
		if blank && prevBlank {
			continue
		}
		out = append(out, line)
		prevBlank = blank
	}
	for len(out) > 0 && out[0] == "" {
		out = out[1:]
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return strings.Join(out, "\n")
}

// decorativeRE matches lines composed entirely of whitespace and
// horizontal-rule characters — the separators Claude's TUI draws
// between groups in a Confirm prompt.
var decorativeRE = regexp.MustCompile(`^[\s─━═━‾_\-]+$`)

func isDecorative(line string) bool {
	if strings.TrimSpace(line) == "" {
		return false
	}
	return decorativeRE.MatchString(line)
}

// promptMarker matches the `☐ <title>` headline that Claude's TUI
// puts at the top of every confirmation / choice prompt (Confirm,
// AskUserQuestion, tool permission, etc.).
var promptMarker = regexp.MustCompile(`(?m)^\s*☐\s`)

// findPromptStart returns the line index of the most recent `☐ `
// marker, or -1 if none. Most recent wins so a fresh prompt
// supersedes any old visible-but-resolved one.
func findPromptStart(lines []string) int {
	last := -1
	for i, line := range lines {
		if promptMarker.MatchString(line) {
			last = i
		}
	}
	return last
}

// ansiSeq matches the common ANSI escape sequences (CSI, OSC, and
// terminal-mode shifts). Good enough to clean a tmux capture-pane
// snapshot for human display; not a full terminal emulator.
var ansiSeq = regexp.MustCompile("\x1b\\[[0-9;?]*[ -/]*[@-~]|\x1b\\][^\x07]*\x07|\x1b[()][AB012]")

func stripANSI(s string) string {
	return ansiSeq.ReplaceAllString(s, "")
}

// finalizeTrack writes the terminal status for trackID if it isn't
// already terminal. Also fires the done/errored notification — by
// the time we get here, the user almost certainly isn't watching
// this track's window, so a system notification is the right cue.
func (s *Server) finalizeTrack(trackID string) {
	t, ok := s.store.Get(trackID)
	if !ok || t.Status.IsTerminal() {
		return
	}
	now := time.Now().UTC()
	t.ExitedAt = &now
	// We don't have a reliable exit code from the tmux-hosted
	// process; treat any natural exit as Done. (Future: parse
	// pane_dead_status via tmux.)
	t.Status = state.StatusDone
	_ = s.store.Put(t)

	s.notifyEvent(string(notify.EventDone), "tracks: track finished",
		labelFor(t)+" is done")
}

// Stop ends a running track gracefully: SIGTERM the pane's whole
// process group so Claude (and any shutdown hook it spawns) runs to
// completion while the worktree still exists, wait for the group to
// exit, SIGKILL anything left as a backstop, then close the window.
//
// Signalling the *group* (kill(-pid)) rather than just sup.pid is the
// crux: sup.pid is the wrapper shell (`sh -c 'claude …; exec
// $SHELL'`) and Claude is its child. Killing only the shell orphans
// Claude, which then races the caller's worktree removal and fails
// its Stop hook with `ENOENT '/bin/sh'` (a deleted cwd). The caller
// removes the worktree only after this returns, so by then the group
// — Claude and its hooks included — is gone.
func (sup *supervisor) Stop(sessionName string) {
	if sup == nil {
		return
	}
	sup.terminateGroup(5 * time.Second)
	_ = tmux.New().KillWindow(sessionName, sup.windowName)
}

// Kill ends a track with prejudice: SIGKILL the whole process group
// at once (Claude dies before it can spawn any shutdown hook), wait
// briefly for it to be reaped, then close the window.
func (sup *supervisor) Kill(sessionName string) {
	if sup == nil {
		return
	}
	signalGroup(sup.pid, syscall.SIGKILL)
	waitGroupGone(sup.pid, 2*time.Second)
	_ = tmux.New().KillWindow(sessionName, sup.windowName)
}

// terminateGroup SIGTERMs the pane's process group, waits up to grace
// for every process in it to exit, then SIGKILLs whatever remains.
func (sup *supervisor) terminateGroup(grace time.Duration) {
	signalGroup(sup.pid, syscall.SIGTERM)
	if waitGroupGone(sup.pid, grace) {
		return
	}
	signalGroup(sup.pid, syscall.SIGKILL)
	waitGroupGone(sup.pid, 2*time.Second)
}

// signalGroup sends sig to the process group led by pid
// (kill(-pid, sig)). tmux launches each pane in its own session, so
// the pane_pid is the group leader and the negative-pid send reaches
// Claude and any children. Falls back to the bare pid if the group
// send fails (e.g. pid isn't a group leader).
func signalGroup(pid int, sig syscall.Signal) {
	if pid <= 0 {
		return
	}
	if err := syscall.Kill(-pid, sig); err != nil {
		_ = syscall.Kill(pid, sig)
	}
}

// waitGroupGone polls until the process group led by pid has no
// remaining members (kill(-pid, 0) → ESRCH) or timeout elapses.
// Returns true once the group is gone.
func waitGroupGone(pid int, timeout time.Duration) bool {
	if pid <= 0 {
		return true
	}
	deadline := time.Now().Add(timeout)
	for {
		if syscall.Kill(-pid, 0) == syscall.ESRCH {
			return true
		}
		if !time.Now().Before(deadline) {
			return syscall.Kill(-pid, 0) == syscall.ESRCH
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// stopAllSupervisors closes every active track window. Called from
// Server.Stop when the daemon is winding down. SIGTERMs in parallel
// so a slow shutdown doesn't hold the others up.
func (s *Server) stopAllSupervisors() {
	s.mu.Lock()
	sups := make([]*supervisor, 0, len(s.supervisors))
	for _, sup := range s.supervisors {
		sups = append(sups, sup)
	}
	s.mu.Unlock()
	var wg sync.WaitGroup
	for _, sup := range sups {
		wg.Add(1)
		go func(sp *supervisor) {
			defer wg.Done()
			sp.Stop(s.cfg.Tmux.SessionName)
		}(sup)
	}
	wg.Wait()
}
