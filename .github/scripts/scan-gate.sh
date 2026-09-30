#!/usr/bin/env bash
# scan-gate.sh — turn an rg/grep-shaped "if <scan>; then <fail>; fi" gate into one
# that also fails when the scan tool itself errors, instead of silently passing.
#
# The 0/1/2 exit-code contract shared by rg and grep -r:
#   0 = a match was found            (this is the *violation* case for these gates —
#                                      the pattern we're scanning FOR must not exist)
#   1 = the scan ran cleanly, no match (this is the *clean tree* case — pass)
#   2 = the scan tool itself errored (unreadable file, missing/bad path, bad glob, ...)
# A missing binary makes the shell itself return 127 (command not found), which falls
# into the same "not 0, not 1" bucket as a real tool error.
#
# `if <scan>; then <fail>; fi` treats every non-zero exit as "no match", so on a
# tool error (2, 127, ...) the gate passes silently — the exact bug this file fixes.
#
# It gets worse: when any scanned path errors, rg and `grep -r` both exit 2 even if
# they *also* printed a real match before hitting the unreadable path. A tool error
# can therefore swallow a genuine violation, not just a clean run.
#
# Usage (source this file, then call the function — each `run:` step in a GitHub
# Actions workflow is its own bash process, so a function can't be defined once per
# job; source it from $GITHUB_WORKSPACE at the top of every step that needs it):
#
#   source "$GITHUB_WORKSPACE/.github/scripts/scan-gate.sh"
#   scan_gate "<message printed with ::error:: on a violation>" rg -n ... || exit 1
#
# A message with more than one ::error:: line uses a literal "\n" (interpreted via
# `printf '%b'`, not an embedded newline) to stay a single YAML block-scalar line.
#
# Do not rely on `set -e` to catch a bad exit code inside this function — it is
# called from an `if`/`||` context, where `set -e` is suspended for the command
# whose result is being tested, so the exit code must be captured explicitly.
scan_gate() {
  local message="$1"
  shift
  local cmd_name="$1"
  local rc=0
  "$@" || rc=$?
  case "$rc" in
    0)
      while IFS= read -r line; do
        echo "::error::${line}" >&2
      done < <(printf '%b\n' "$message")
      return 1
      ;;
    1)
      return 0
      ;;
    *)
      echo "::error::scan tool failed (exit $rc): $cmd_name — treating as a gate failure, not a clean tree" >&2
      return 1
      ;;
  esac
}
