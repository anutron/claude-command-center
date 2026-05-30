#!/usr/bin/env bash
# Context helper for the /bookmark skill.
# Exposes individual lookups as subcommands so the skill's `!`...`` blocks
# stay free of $(...) / backticks (which trigger Claude Code's static-analysis
# permission prompts).

set -euo pipefail

cmd="${1:-}"

case "$cmd" in
  repo)
    top="$(git rev-parse --show-toplevel 2>/dev/null || true)"
    if [ -n "$top" ]; then
      basename "$top"
    else
      basename "$PWD"
    fi
    ;;

  branch)
    git branch --show-current 2>/dev/null || echo "not a git repo"
    ;;

  sessions-dir)
    # Claude stores session JSONLs under ~/.claude/projects/<encoded-path>/
    # where the encoded path is the absolute project path with '/' -> '-'.
    encoded="$(echo "$PWD" | sed 's|/|-|g')"
    echo "$HOME/.claude/projects/$encoded"
    ;;

  is-worktree)
    common="$(git rev-parse --git-common-dir 2>/dev/null || true)"
    gitdir="$(git rev-parse --git-dir 2>/dev/null || true)"
    if [ -n "$common" ] && [ -n "$gitdir" ] && [ "$common" != "$gitdir" ]; then
      echo "yes"
    else
      echo "no"
    fi
    ;;

  main-repo)
    git worktree list 2>/dev/null | head -1 | awk '{print $1}'
    ;;

  *)
    echo "usage: bookmark-context.sh {repo|branch|sessions-dir|is-worktree|main-repo}" >&2
    exit 2
    ;;
esac
