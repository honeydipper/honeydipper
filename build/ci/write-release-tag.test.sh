#!/usr/bin/env bash
#
# Self-contained tests for build/ci/write-release-tag.sh.
#
#   run: bash build/ci/write-release-tag.test.sh
#
# No test framework, no network, no fixtures on disk: every case builds its own
# throwaway git repository under a single `mktemp -d`, thrown away on exit.
# Prints PASS/FAIL per case and exits non-zero if any case failed.

set -u

here="$(cd -- "$(dirname -- "$0")" && pwd)"
guard="$here/write-release-tag.sh"

tmp_root="$(mktemp -d)"
trap 'rm -rf "$tmp_root"' EXIT

if [ ! -f "$guard" ]; then
  printf 'cannot find %s\n' "$guard" >&2
  exit 1
fi

# ---------------------------------------------------------------------------
# harness
# ---------------------------------------------------------------------------

passed=0
failed=0
failures=""

run_case() { # run_case <test-function>
  local name
  local out

  name="$1"
  if out="$("$name" 2>&1)"; then
    printf 'PASS %s\n' "$name"
    passed=$((passed + 1))
  else
    printf 'FAIL %s\n' "$name"
    printf '%s\n' "$out" | sed 's/^/    /'
    failed=$((failed + 1))
    failures="$failures $name"
  fi
}

assert() { # assert <what> <actual> <expected>
  if [ "$2" = "$3" ]; then
    return 0
  fi
  printf '  %s\n    expected: <%s>\n    actual:   <%s>\n' "$1" "$3" "$2"
  return 1
}

expect() { # expect <what> <actual> <expected> -- aborts the current case
  assert "$@" || exit 1
}

# ---------------------------------------------------------------------------
# fixtures
# ---------------------------------------------------------------------------

repo_init() { # repo_init <dir>
  mkdir -p "$1"
  git -C "$1" init --quiet
  git -C "$1" config user.email "release-tag-test@example.com"
  git -C "$1" config user.name "release tag test"
  git -C "$1" config commit.gpgsign false
  git -C "$1" config tag.gpgsign false
}

make_commit() { # make_commit <repo> <message> -- echoes the new commit sha
  printf '%s\n' "$2" >>"$1/log.txt"
  git -C "$1" add --all
  git -C "$1" commit --quiet --message "$2"
  git -C "$1" rev-parse HEAD
}

record() { # record <repo> <head-before-file> [rev]
  local repo
  local file
  local rev

  repo="$1"
  file="$2"
  rev="${3:-}"
  if [ -z "$rev" ]; then
    rev="$(git -C "$repo" rev-parse HEAD)"
  fi
  printf '%s\n' "$rev" >"$file"
}

run_guard() { # run_guard <repo> [args...] -- fills RUN_OUT RUN_ERR RUN_RC
  local repo
  local errfile

  repo="$1"
  shift
  errfile="$tmp_root/.stderr"

  RUN_OUT=""
  RUN_ERR=""
  RUN_RC=0
  : >"$errfile"
  RUN_OUT="$(cd -- "$repo" && bash "$guard" "$@" 2>"$errfile")" || RUN_RC=$?
  RUN_ERR="$(cat "$errfile")"
}

last_line() { # last_line <text>
  printf '%s\n' "$1" | tail -n 1
}

line_count() { # line_count <text> -- non-empty lines
  printf '%s\n' "$1" | grep -c . || true
}

# ---------------------------------------------------------------------------
# cases
# ---------------------------------------------------------------------------

# T1 -- the re-approval no-op: nothing happened in this run, even though HEAD
# still carries the previous release tag.  Must produce an EMPTY output file.
t1_no_release_writes_empty_file() {
  local repo hb out a

  repo="$tmp_root/t1"
  hb="$tmp_root/t1.head-before"
  out="$tmp_root/t1/release-tag.txt"
  repo_init "$repo"
  a="$(make_commit "$repo" 'feat: one')"
  git -C "$repo" tag v3.10.6 "$a"
  record "$repo" "$hb" "$a"

  run_guard "$repo" "$hb" "$out"

  expect 'T1 exit code' "$RUN_RC" 0
  expect 'T1 last stdout line' "$(last_line "$RUN_OUT")" 'TAG='
  expect 'T1 output file created' "$(test -f "$out" && echo yes || echo no)" 'yes'
  expect 'T1 output file empty' "$(test -s "$out" && echo non-empty || echo empty)" 'empty'
  expect 'T1 diagnostic explains unchanged HEAD' \
    "$(printf '%s' "$RUN_ERR" | grep -c 'HEAD unchanged')" '1'
}

# T2 -- a release did happen: the tag at the advanced HEAD is written out.
t2_picks_tag_of_advanced_head() {
  local repo hb out a b

  repo="$tmp_root/t2"
  hb="$tmp_root/t2.head-before"
  out="$tmp_root/t2.tag"
  repo_init "$repo"
  a="$(make_commit "$repo" 'feat: one')"
  git -C "$repo" tag v3.10.6 "$a"
  b="$(make_commit "$repo" 'fix: two')"
  git -C "$repo" tag v3.10.7 "$b"
  record "$repo" "$hb" "$a"

  run_guard "$repo" "$hb" "$out"

  expect 'T2 exit code' "$RUN_RC" 0
  expect 'T2 last stdout line' "$(last_line "$RUN_OUT")" 'TAG=v3.10.7'
  expect 'T2 output file content' "$(cat "$out")" 'v3.10.7'
}

# T3 -- HEAD advanced but no tag points at it: the invariant is broken.  Exit 1
# with one loud line, and never write (or truncate) the output file.
t3_head_advanced_without_tag_fails() {
  local repo hb out rmout a

  repo="$tmp_root/t3"
  hb="$tmp_root/t3.head-before"
  out="$tmp_root/t3.tag"
  rmout="$tmp_root/t3.absent.tag"
  repo_init "$repo"
  a="$(make_commit "$repo" 'feat: one')"
  git -C "$repo" tag v3.10.6 "$a"
  make_commit "$repo" 'fix: untagged' >/dev/null
  record "$repo" "$hb" "$a"
  printf 'sentinel\n' >"$out"

  run_guard "$repo" "$hb" "$out"

  expect 'T3 exit code' "$RUN_RC" 1
  expect 'T3 existing output file untouched' "$(cat "$out")" 'sentinel'
  expect 'T3 no TAG= line on stdout' "$(printf '%s' "$RUN_OUT" | grep -c '^TAG=')" '0'
  expect 'T3 single diagnostic line' "$(line_count "$RUN_ERR")" '1'
  expect 'T3 diagnostic is loud and specific' \
    "$(printf '%s' "$RUN_ERR" | grep -c 'no tag matching v\* points at it')" '1'
  expect 'T3 diagnostic names the race' \
    "$(printf '%s' "$RUN_ERR" | grep -c 'concurrent push during release')" '1'

  # a missing output file must not be created by the failing run either
  run_guard "$repo" "$hb" "$rmout"
  expect 'T3 failing run exit code' "$RUN_RC" 1
  expect 'T3 output file not created' "$(test -e "$rmout" && echo present || echo absent)" 'absent'
}

# T4 -- the glob decides which tag shapes count; anything else at HEAD is
# ignored (a `scratch-1` tag must never be picked up).
t4_glob_filters_tag_shapes() {
  local repo hb out a

  repo="$tmp_root/t4"
  hb="$tmp_root/t4.head-before"
  out="$tmp_root/t4.tag"
  repo_init "$repo"
  a="$(make_commit "$repo" 'feat: base')"
  record "$repo" "$hb" "$a"
  make_commit "$repo" 'feat: release' >/dev/null
  git -C "$repo" tag v4.1.0
  git -C "$repo" tag v4.1.0-dev.3
  git -C "$repo" tag scratch-1

  run_guard "$repo" "$hb" "$out" 'v*-*'

  expect 'T4 exit code' "$RUN_RC" 0
  expect 'T4 last stdout line' "$(last_line "$RUN_OUT")" 'TAG=v4.1.0-dev.3'
  expect 'T4 output file content' "$(cat "$out")" 'v4.1.0-dev.3'
}

# T5 -- usage errors: exit 1, exactly one line on stderr carrying the usage
# string, and nothing written.
t5_usage_errors() {
  local repo hb out missing empty nogit

  repo="$tmp_root/t5"
  hb="$tmp_root/t5.head-before"
  out="$tmp_root/t5.tag"
  missing="$tmp_root/t5.missing"
  empty="$tmp_root/t5.empty"
  nogit="$tmp_root/t5.nogit"
  repo_init "$repo"
  record "$repo" "$hb"
  : >"$empty"
  mkdir -p "$nogit"

  # explicit empty arguments
  run_guard "$repo" "" "$out"
  expect 'T5 empty head-before arg exit code' "$RUN_RC" 1
  expect 'T5 empty head-before arg is one line' "$(line_count "$RUN_ERR")" '1'
  expect 'T5 empty head-before arg carries usage' \
    "$(printf '%s' "$RUN_ERR" | grep -c 'usage: write-release-tag.sh')" '1'
  run_guard "$repo" "$hb" ""
  expect 'T5 empty output arg exit code' "$RUN_RC" 1
  expect 'T5 empty output arg carries usage' \
    "$(printf '%s' "$RUN_ERR" | grep -c 'usage: write-release-tag.sh')" '1'

  # missing head-before file
  run_guard "$repo" "$missing" "$out"
  expect 'T5 missing head-before exit code' "$RUN_RC" 1
  expect 'T5 missing head-before is one line' "$(line_count "$RUN_ERR")" '1'
  expect 'T5 missing head-before diagnostic' \
    "$(printf '%s' "$RUN_ERR" | grep -c 'head-before file not found')" '1'
  expect 'T5 missing head-before carries usage' \
    "$(printf '%s' "$RUN_ERR" | grep -c 'usage: write-release-tag.sh')" '1'

  # empty head-before file
  run_guard "$repo" "$empty" "$out"
  expect 'T5 empty head-before exit code' "$RUN_RC" 1
  expect 'T5 empty head-before diagnostic' \
    "$(printf '%s' "$RUN_ERR" | grep -c 'head-before file is empty')" '1'

  # not a git repository
  printf '%s\n' '0123456789abcdef0123456789abcdef01234567' >"$empty"
  run_guard "$nogit" "$empty" "$out"
  expect 'T5 not-a-repo exit code' "$RUN_RC" 1
  expect 'T5 not-a-repo diagnostic' \
    "$(printf '%s' "$RUN_ERR" | grep -c 'not inside a git repository')" '1'

  expect 'T5 nothing written' "$(test -e "$out" && echo present || echo absent)" 'absent'
}

# T6 -- idempotent: the same inputs produce identical output, twice.
t6_idempotent() {
  local repo hb out a b first_out first_err first_content

  repo="$tmp_root/t6"
  hb="$tmp_root/t6.head-before"
  out="$tmp_root/t6.tag"
  repo_init "$repo"
  a="$(make_commit "$repo" 'feat: one')"
  git -C "$repo" tag v3.10.6 "$a"
  b="$(make_commit "$repo" 'fix: two')"
  git -C "$repo" tag v3.10.7 "$b"
  record "$repo" "$hb" "$a"

  run_guard "$repo" "$hb" "$out"
  expect 'T6 first run exit code' "$RUN_RC" 0
  first_out="$RUN_OUT"
  first_err="$RUN_ERR"
  first_content="$(cat "$out")"

  run_guard "$repo" "$hb" "$out"
  expect 'T6 second run exit code' "$RUN_RC" 0
  expect 'T6 stdout identical' "$RUN_OUT" "$first_out"
  expect 'T6 stderr identical' "$RUN_ERR" "$first_err"
  expect 'T6 output file identical' "$(cat "$out")" "$first_content"
  expect 'T6 output file content' "$(cat "$out")" 'v3.10.7'
}

# T7 -- the default glob is 'v*': with no third argument, rehearsal-*/other
# prefixed tags at HEAD are ignored.
t7_default_glob_is_v_star() {
  local repo hb out a b

  repo="$tmp_root/t7"
  hb="$tmp_root/t7.head-before"
  out="$tmp_root/t7.tag"
  repo_init "$repo"
  a="$(make_commit "$repo" 'feat: one')"
  git -C "$repo" tag v3.10.6 "$a"
  b="$(make_commit "$repo" 'fix: two')"
  git -C "$repo" tag v3.10.7 "$b"
  # decoys a `*` glob would pick (both sort after `v`), so this case fails if
  # the default glob ever stops being 'v*'
  git -C "$repo" tag rehearsal-9.9.9 "$b"
  git -C "$repo" tag wip-9.9.9 "$b"
  record "$repo" "$hb" "$a"

  run_guard "$repo" "$hb" "$out"

  expect 'T7 exit code' "$RUN_RC" 0
  expect 'T7 last stdout line' "$(last_line "$RUN_OUT")" 'TAG=v3.10.7'
  expect 'T7 output file content' "$(cat "$out")" 'v3.10.7'
}

# T8 -- the v3 maintenance glob ('v3.*') cannot be stolen by a v4 tag.
t8_v3_glob_ignores_v4_tags() {
  local repo hb out a

  repo="$tmp_root/t8"
  hb="$tmp_root/t8.head-before"
  out="$tmp_root/t8.tag"
  repo_init "$repo"
  a="$(make_commit "$repo" 'feat: base')"
  record "$repo" "$hb" "$a"
  make_commit "$repo" 'fix: release' >/dev/null
  git -C "$repo" tag v3.10.7
  git -C "$repo" tag v4.1.0-beta.1

  run_guard "$repo" "$hb" "$out" 'v3.*'

  expect 'T8 exit code' "$RUN_RC" 0
  expect 'T8 last stdout line' "$(last_line "$RUN_OUT")" 'TAG=v3.10.7'
  expect 'T8 output file content' "$(cat "$out")" 'v3.10.7'
}

# T9 -- the dev glob ('v*-*', prerelease shape only) can never select a
# promotion-carried release tag that also sits on HEAD.
t9_dev_glob_ignores_promoted_release_tag() {
  local repo hb out a

  repo="$tmp_root/t9"
  hb="$tmp_root/t9.head-before"
  out="$tmp_root/t9.tag"
  repo_init "$repo"
  a="$(make_commit "$repo" 'feat: base')"
  record "$repo" "$hb" "$a"
  make_commit "$repo" 'fix: release' >/dev/null
  git -C "$repo" tag v4.0.0
  git -C "$repo" tag v4.1.0-beta.1

  run_guard "$repo" "$hb" "$out" 'v*-*'

  expect 'T9 exit code' "$RUN_RC" 0
  expect 'T9 last stdout line' "$(last_line "$RUN_OUT")" 'TAG=v4.1.0-beta.1'
  expect 'T9 output file content' "$(cat "$out")" 'v4.1.0-beta.1'
}

# T10 -- the documented defaults are wired: omitting <output-file> writes
# /tmp/release/release-tag.txt, omitting everything uses
# /tmp/release/head-before as well.
t10_documented_defaults_are_wired() {
  local repo hb out a saved have_saved

  repo="$tmp_root/t10"
  hb="$tmp_root/t10.head-before"
  out="/tmp/release/release-tag.txt"
  saved="$tmp_root/t10.saved-release-tag.txt"
  repo_init "$repo"
  a="$(make_commit "$repo" 'feat: one')"
  git -C "$repo" tag v3.10.6 "$a"
  record "$repo" "$hb" "$a"
  make_commit "$repo" 'fix: two' >/dev/null
  git -C "$repo" tag v3.10.7

  # only <head-before-file> given -> default output path
  have_saved=no
  if [ -f "$out" ]; then
    cp "$out" "$saved"
    have_saved=yes
  fi

  run_guard "$repo" "$hb"
  expect 'T10 exit code' "$RUN_RC" 0
  expect 'T10 default output path used' "$(cat "$out" 2>/dev/null || echo '<missing>')" 'v3.10.7'

  # restore whatever the developer/environment had there
  if [ "$have_saved" = yes ]; then
    cp "$saved" "$out"
  else
    rm -f "$out"
  fi

  # no arguments at all -> documented default head-before path.  In a clean
  # environment that file does not exist, so it must be a one-line usage error.
  if [ -e /tmp/release/head-before ]; then
    run_guard "$repo"
    expect 'T10 defaults exit code' "$RUN_RC" 0
    expect 'T10 defaults last stdout line' "$(last_line "$RUN_OUT")" 'TAG='
    rm -f "$out"
  else
    run_guard "$repo"
    expect 'T10 no-args exit code' "$RUN_RC" 1
    expect 'T10 no-args is one line' "$(line_count "$RUN_ERR")" '1'
    expect 'T10 no-args names the default file' \
      "$(printf '%s' "$RUN_ERR" | grep -c 'head-before file not found: /tmp/release/head-before')" '1'
    expect 'T10 no-args carries usage' \
      "$(printf '%s' "$RUN_ERR" | grep -c 'usage: write-release-tag.sh')" '1'
  fi
}

# T11 -- several tags match at HEAD: the highest by *version* order wins
# (lexicographic order would pick v3.10.9 over v3.10.10).
t11_highest_version_wins() {
  local repo hb out a

  repo="$tmp_root/t11"
  hb="$tmp_root/t11.head-before"
  out="$tmp_root/t11.tag"
  repo_init "$repo"
  a="$(make_commit "$repo" 'feat: base')"
  record "$repo" "$hb" "$a"
  make_commit "$repo" 'fix: release' >/dev/null
  git -C "$repo" tag v3.10.9
  git -C "$repo" tag v3.10.10

  run_guard "$repo" "$hb" "$out"

  expect 'T11 exit code' "$RUN_RC" 0
  expect 'T11 last stdout line' "$(last_line "$RUN_OUT")" 'TAG=v3.10.10'
  expect 'T11 output file content' "$(cat "$out")" 'v3.10.10'
}

# ---------------------------------------------------------------------------
# main
# ---------------------------------------------------------------------------

run_case t1_no_release_writes_empty_file
run_case t2_picks_tag_of_advanced_head
run_case t3_head_advanced_without_tag_fails
run_case t4_glob_filters_tag_shapes
run_case t5_usage_errors
run_case t6_idempotent
run_case t7_default_glob_is_v_star
run_case t8_v3_glob_ignores_v4_tags
run_case t9_dev_glob_ignores_promoted_release_tag
run_case t10_documented_defaults_are_wired
run_case t11_highest_version_wins

printf '\n%d passed, %d failed\n' "$passed" "$failed"
if [ "$failed" -ne 0 ]; then
  printf 'failed cases:%s\n' "$failures"
  exit 1
fi
