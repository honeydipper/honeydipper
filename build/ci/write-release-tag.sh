#!/usr/bin/env bash
#
# write-release-tag.sh -- answer, exactly once and inside the job that can:
#                         "did *this* run create a release, and which tag?"
#
#   usage: write-release-tag.sh <head-before-file> <output-file> [tag-glob]
#     defaults: /tmp/release/head-before  /tmp/release/release-tag.txt  'v*'
#
# Why this exists (the release race)
# ----------------------------------
# semantic-release runs `prepare` (@semantic-release/git), which commits AND
# pushes, before `publish` (@semantic-release/github), which creates the GitHub
# release.  That push used to create a second pipeline on the same branch, and
# "auto-cancel redundant workflows" killed the still-running release pipeline
# before the release was published.  The fix moves the image publish into the
# release pipeline (plus `[skip ci]` on the release commit), which means the
# publish job has to know the tag *this* pipeline released.
#
# A downstream job cannot answer that by looking at "the highest v* tag at
# HEAD": on a re-approval with no new commits HEAD still carries the previous
# release tag, so it would try to republish an already published image and go
# red with "Refuse to overwrite a published tag" -- a false alarm on a
# perfectly good no-op.  Only the release job can compare HEAD *after*
# semantic-release with HEAD *before*, so it records the pre-run commit into
# <head-before-file> and this script does the comparison.
#
# Contract
# --------
#   HEAD == recorded  -> no release happened in this run.
#                        Writes an EMPTY <output-file>, prints `TAG=`, exit 0.
#                        The publish job reads the empty file, says "nothing to
#                        publish" and halts GREEN.
#   HEAD != recorded, and tags matching <tag-glob> point at HEAD
#                     -> writes the highest one (`sort -V`), prints
#                        `TAG=<tag>`, exit 0.
#   HEAD != recorded, and NO matching tag points at HEAD
#                     -> the invariant is broken (a concurrent push landed
#                        while we were releasing).  Loud one-line diagnostic on
#                        stderr, exit 1.  Deliberately NOT conflated with the
#                        no-release case: <output-file> is not written and no
#                        `TAG=` line is printed, so it can never be mistaken
#                        for a no-op.
#
# The last stdout line is always machine readable on success: `TAG=` or
# `TAG=<tag>`.  Everything else -- including all diagnostics -- goes to stderr.
#
# Input validation
# ----------------
# <head-before-file> must hold a FULL lowercase commit sha: 40 characters for a
# SHA-1 repository, 64 for SHA-256.  Anything else is rejected (exit 1) before
# any git query, because a short sha -- `git rev-parse --short HEAD`, 7 chars by
# default -- can never equal the full sha that HEAD resolves to.  Unchecked, it
# would make the "HEAD unchanged" comparison fail every time: a no-op
# re-approval would be misread as a release, and the *previous* release's tag
# (still sitting at an unchanged HEAD) would be written out with exit 0 -- the
# exact false release this script exists to prevent.  Fail loudly instead.
#
# Guarantees
# ----------
#   * no network access; read-only git (`git rev-parse`, `git tag --points-at`)
#   * <output-file> is the only file written
#   * idempotent: same inputs -> identical <output-file> contents
#   * never guesses: any input it cannot trust is a hard failure, and a failure
#     never writes (or truncates) <output-file> and never prints `TAG=`
#
# Portability
# -----------
#   `sort -V` is GNU coreutils and is present in the CircleCI images used here.
#   On macOS it is `gsort` (brew install coreutils) or run inside the CI image.
#   Invoke as `bash build/ci/write-release-tag.sh ...` so the executable bit
#   never matters.

set -eu

prog="write-release-tag.sh"
default_head_before="/tmp/release/head-before"
default_output="/tmp/release/release-tag.txt"
default_glob="v*"

usage_line() {
  printf 'usage: %s <head-before-file> <output-file> [tag-glob]  (defaults: %s  %s  %s)\n' \
    "$prog" "$default_head_before" "$default_output" "$default_glob"
}

die() { # die <reason> -- one line on stderr, exit 1 (runtime failure)
  # for "the world is not what we assumed" (no git repo, no HEAD, broken tag
  # invariant).  No usage text: the caller did not get an argument wrong.
  printf '%s: %s\n' "$prog" "$1" >&2
  exit 1
}

die_usage() { # die_usage <reason> -- die, plus the usage line (bad input/args)
  printf '%s: %s -- ' "$prog" "$1" >&2
  usage_line >&2
  exit 1
}

head_before_file="$default_head_before"
output_file="$default_output"
tag_glob="$default_glob"

if [ "$#" -ge 1 ]; then head_before_file="$1"; fi
if [ "$#" -ge 2 ]; then output_file="$2"; fi
if [ "$#" -ge 3 ]; then tag_glob="$3"; fi

[ -n "$head_before_file" ] || die_usage 'head-before file argument is empty'
[ -n "$output_file" ] || die_usage 'output file argument is empty'
[ -n "$tag_glob" ] || die_usage 'tag-glob argument is empty'

[ -e "$head_before_file" ] || die_usage "head-before file not found: $head_before_file"
[ -f "$head_before_file" ] || die_usage "head-before file is not a regular file: $head_before_file"
[ -r "$head_before_file" ] || die_usage "head-before file not readable: $head_before_file"

recorded="$(cat "$head_before_file")"

case "$recorded" in
  '')
    die_usage "head-before file is empty: $head_before_file"
    ;;
  *[!0-9a-f]*)
    die_usage "head-before file does not hold a lowercase commit sha: $head_before_file"
    ;;
esac

# Length matters as much as the charset: a *short* sha is pure hex, so it passes
# the case above, but it can never equal the full sha HEAD resolves to.  Left
# unchecked it would turn every no-op run into "HEAD advanced" and publish the
# previous release's tag -- see "Input validation" in the header.
case "${#recorded}" in
  40 | 64) ;;
  *)
    die_usage "head-before file holds a ${#recorded}-character sha, not a full commit sha -- record \`git rev-parse HEAD\`, never \`git rev-parse --short HEAD\`: $head_before_file"
    ;;
esac

git rev-parse --git-dir >/dev/null 2>&1 || die 'not inside a git repository'

if ! current="$(git rev-parse --verify HEAD 2>/dev/null)"; then
  die 'cannot resolve HEAD (no commit checked out?)'
fi

mkdir -p -- "$(dirname -- "$output_file")"

if [ "$current" = "$recorded" ]; then
  # semantic-release found no relevant commits, so this run published nothing.
  : > "$output_file"
  printf '%s: no release in this run, HEAD unchanged at %s\n' "$prog" "$current" >&2
  printf 'TAG=\n'
  exit 0
fi

tag="$(git tag --points-at HEAD --list "$tag_glob" | sort -V | tail -n 1)"

if [ -z "$tag" ]; then
  die "FATAL: HEAD advanced $recorded -> $current but no tag matching $tag_glob points at it -- concurrent push during release?"
fi

printf '%s\n' "$tag" > "$output_file"
printf '%s: released %s (%s -> %s)\n' "$prog" "$tag" "$recorded" "$current" >&2
printf 'TAG=%s\n' "$tag"
