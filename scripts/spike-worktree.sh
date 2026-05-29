#!/usr/bin/env bash
# Throwaway spike: validates the core `tracks` worktree mechanism does NOT
# move the primary checkout's HEAD. Delete this script once v1 ships.
#
# Asserts:
#   1. `git fetch` + `git worktree add -b <new-branch> <path> origin/<base>`
#      leaves the primary checkout's branch and HEAD unchanged.
#   2. The new worktree is on the requested branch.
#   3. `git worktree remove` cleans up the worktree but preserves the branch
#      locally (so the user can `git checkout` it later from the primary).

set -euo pipefail

REPO_NAME="${REPO_NAME:-ledger-live}"
PRIMARY="$HOME/ledger/$REPO_NAME"
BASE_BRANCH="${BASE_BRANCH:-develop}"
SPIKE_BRANCH="feat/spike-tracks-$(date +%s)"
WORKTREE_ROOT="$HOME/.local/state/tracks/worktrees/spike"
WORKTREE="$WORKTREE_ROOT/$REPO_NAME"

cleanup() {
  if [ -d "$WORKTREE" ]; then
    echo "[cleanup] removing worktree $WORKTREE"
    git -C "$PRIMARY" worktree remove --force "$WORKTREE" 2>/dev/null || rm -rf "$WORKTREE"
  fi
  rmdir "$WORKTREE_ROOT" 2>/dev/null || true
}

trap cleanup EXIT

echo "=== Spike: tracks worktree isolation ==="
echo "Repo:    $PRIMARY"
echo "Branch:  $SPIKE_BRANCH (will be kept after worktree removal)"
echo "Path:    $WORKTREE"
echo

if [ ! -d "$PRIMARY/.git" ]; then
  echo "ERROR: $PRIMARY is not a git repo. Set REPO_NAME=<repo> if needed."
  exit 2
fi

# --- Capture primary state BEFORE ---
BEFORE_BRANCH=$(git -C "$PRIMARY" branch --show-current)
BEFORE_HEAD=$(git -C "$PRIMARY" rev-parse HEAD)
echo "[before] primary branch:   $BEFORE_BRANCH"
echo "[before] primary HEAD:     $BEFORE_HEAD"
echo

# --- Fetch base ---
echo "[step 1] git fetch origin $BASE_BRANCH (--no-tags)"
git -C "$PRIMARY" fetch origin "$BASE_BRANCH" --no-tags

# --- Create worktree on new branch ---
echo "[step 2] git worktree add -b $SPIKE_BRANCH $WORKTREE origin/$BASE_BRANCH"
mkdir -p "$WORKTREE_ROOT"
git -C "$PRIMARY" worktree add -b "$SPIKE_BRANCH" "$WORKTREE" "origin/$BASE_BRANCH"

# --- Capture primary state AFTER worktree add ---
AFTER_BRANCH=$(git -C "$PRIMARY" branch --show-current)
AFTER_HEAD=$(git -C "$PRIMARY" rev-parse HEAD)
WORKTREE_BRANCH=$(git -C "$WORKTREE" branch --show-current)
echo
echo "[after-add] primary branch:   $AFTER_BRANCH"
echo "[after-add] primary HEAD:     $AFTER_HEAD"
echo "[after-add] worktree branch:  $WORKTREE_BRANCH"

# --- Verify invariants (1) and (2) ---
FAIL=0
if [ "$BEFORE_BRANCH" != "$AFTER_BRANCH" ]; then
  echo "FAIL: primary branch moved ($BEFORE_BRANCH -> $AFTER_BRANCH)"
  FAIL=1
fi
if [ "$BEFORE_HEAD" != "$AFTER_HEAD" ]; then
  echo "FAIL: primary HEAD moved ($BEFORE_HEAD -> $AFTER_HEAD)"
  FAIL=1
fi
if [ "$WORKTREE_BRANCH" != "$SPIKE_BRANCH" ]; then
  echo "FAIL: worktree is on '$WORKTREE_BRANCH', expected '$SPIKE_BRANCH'"
  FAIL=1
fi

# --- Remove worktree, confirm branch survives ---
echo
echo "[step 3] git worktree remove $WORKTREE"
git -C "$PRIMARY" worktree remove --force "$WORKTREE"

if git -C "$PRIMARY" show-ref --quiet "refs/heads/$SPIKE_BRANCH"; then
  echo "[after-remove] branch '$SPIKE_BRANCH' still exists locally — good."
else
  echo "FAIL: branch '$SPIKE_BRANCH' was deleted along with the worktree"
  FAIL=1
fi

# --- Capture primary state AFTER worktree remove ---
FINAL_BRANCH=$(git -C "$PRIMARY" branch --show-current)
FINAL_HEAD=$(git -C "$PRIMARY" rev-parse HEAD)
echo "[after-remove] primary branch: $FINAL_BRANCH"
echo "[after-remove] primary HEAD:   $FINAL_HEAD"

if [ "$BEFORE_BRANCH" != "$FINAL_BRANCH" ] || [ "$BEFORE_HEAD" != "$FINAL_HEAD" ]; then
  echo "FAIL: primary checkout moved after worktree remove"
  FAIL=1
fi

# --- Clean up the spike branch so the user's repo isn't polluted ---
echo
echo "[step 4] cleanup spike branch '$SPIKE_BRANCH'"
git -C "$PRIMARY" branch -D "$SPIKE_BRANCH" >/dev/null

echo
echo "Worktree list:"
git -C "$PRIMARY" worktree list

echo
if [ "$FAIL" -eq 0 ]; then
  echo "PASS: spike validated. Cursor's primary checkout is invariant."
else
  echo "FAIL: spike invariants violated. See messages above."
fi

exit $FAIL
