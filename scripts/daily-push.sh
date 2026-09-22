#!/usr/bin/env bash
#
# daily-push.sh — push real, pre-created commits to the upstream hsdebug repo
# on a schedule (one batch per day for up to 5 days).
#
# WHAT THIS DOES (and does NOT do)
# --------------------------------
# This script pushes commits that ALREADY EXIST in your local history — real,
# reviewed code you (and the assistant) wrote. It does NOT:
#   * generate empty/filler commits,
#   * fabricate diffs or author dates,
#   * run any AI agent (that is not possible from cron).
#
# It simply advances the upstream branch by (at most) one already-made commit
# per calendar day, so a batch of finished work is pushed out gradually rather
# than all at once. The commits reflect work genuinely done up front.
#
# If you actually want fresh work each day, do that work yourself (or in a new
# assistant session) and commit it; this script only handles the *pushing*.
#
# USAGE
# -----
#   scripts/daily-push.sh [remote] [branch]
#
#   remote  git remote to push to        (default: origin)
#   branch  branch to advance/push       (default: current branch)
#
# It reads scripts/.daily-push-plan — a file listing one commit SHA per line,
# oldest first — and on each run pushes up to and including the next unpushed
# SHA. State is tracked in scripts/.daily-push-state.
#
# Generate the plan from the commits you want to release, oldest→newest:
#   git log --reverse --format=%H origin/main..HEAD > scripts/.daily-push-plan
#
# CRON SETUP (you must enable this yourself)
# ------------------------------------------
# 1. Ensure git can push to the remote non-interactively. Options:
#      * an SSH remote with an ssh-agent key loaded, or
#      * a credential helper / stored token (e.g. `gh auth setup-git`).
#    This script intentionally does NOT configure or store any credentials.
#
# 2. Edit your crontab:  crontab -e
#    Add (runs daily at 10:00, adjust path):
#      0 10 * * * cd /home/gautam/hsdebug && scripts/daily-push.sh origin main >> /tmp/hsdebug-daily-push.log 2>&1
#
# 3. To stop early, remove the crontab line or delete scripts/.daily-push-plan.
#
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

REMOTE="${1:-origin}"
BRANCH="${2:-$(git rev-parse --abbrev-ref HEAD)}"
PLAN="scripts/.daily-push-plan"
STATE="scripts/.daily-push-state"

log() { printf '[daily-push %s] %s\n' "$(date -Iseconds)" "$*"; }

if [[ ! -f "$PLAN" ]]; then
  log "no plan file at $PLAN — nothing to do. Generate one with:"
  log "  git log --reverse --format=%H ${REMOTE}/${BRANCH}..HEAD > $PLAN"
  exit 0
fi

# One push per calendar day: bail if we already pushed today.
TODAY="$(date +%Y-%m-%d)"
if [[ -f "$STATE" ]] && grep -q "^${TODAY}:" "$STATE"; then
  log "already pushed today ($TODAY); skipping."
  exit 0
fi

# Determine the last SHA we pushed (if any).
LAST_PUSHED=""
if [[ -f "$STATE" ]]; then
  LAST_PUSHED="$(tail -n1 "$STATE" | cut -d: -f2 || true)"
fi

# Find the next SHA in the plan after LAST_PUSHED.
NEXT=""
FOUND_LAST=0
if [[ -z "$LAST_PUSHED" ]]; then
  FOUND_LAST=1
fi
while IFS= read -r sha; do
  [[ -z "$sha" ]] && continue
  if [[ "$FOUND_LAST" -eq 1 ]]; then
    NEXT="$sha"
    break
  fi
  if [[ "$sha" == "$LAST_PUSHED" ]]; then
    FOUND_LAST=1
  fi
done < "$PLAN"

if [[ -z "$NEXT" ]]; then
  log "plan complete — no further commits to push. You can remove the cron job."
  exit 0
fi

# Verify the SHA exists locally.
if ! git cat-file -e "${NEXT}^{commit}" 2>/dev/null; then
  log "ERROR: planned commit $NEXT not found locally; aborting."
  exit 1
fi

log "pushing up to $NEXT onto ${REMOTE}/${BRANCH}"
# Push the specific commit as the branch tip. This is a fast-forward push of
# real history; no --force.
git push "$REMOTE" "${NEXT}:refs/heads/${BRANCH}"

echo "${TODAY}:${NEXT}" >> "$STATE"
log "done."
