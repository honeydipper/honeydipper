# Branch Divergence Report (Phase 1 — evidence-based, direction corrected)

**Status**: authoritative for the v4.0.0 release-engineering project, Phase 1
**Audit date**: performed against live `origin` refs at the SHAs in the inventory below
**Supersedes**: the earlier per-area audit that assumed `main` was the newer line. That assumption was wrong.

---

## 0. TL;DR — the correction

`main` is **not** newer than `v3`. All three long-lived branches share one merge-base, and
`main` **is** that merge-base. `main` is the dormant, oldest line; `v3` is the modern,
actively-maintained 3.x line; `dev` is the v4 integration line.

Concretely, per area:

| Area | Winner | One-line evidence |
|---|---|---|
| Dependency / toolchain (`go.mod`, `go.sum`, `.circleci/config.yml`) | **v3** | `v3` go 1.25.0 + CI `golang:1.27.3-alpine`; `main` go 1.24.1 + CI `golang:1.24.1-alpine`, no `publish-image` job |
| v4-era architecture docs (`HONEYDIPPER_CONTEXT.md`, `FUNCTION_MAP.md`) | **dev** (and only dev) | present on `dev` only; absent on **both** `main` and `v3` |
| `README.md` / `docs/README.md` | **dev** (v4 content); `main` and `v3` are byte-identical at the old content | `README.md` sha `8b546ea0` on `main` **and** `v3`; `98c71d3e` on `dev` |
| `docs/releasing.md` | **v3** (then dev variant) | present on `v3` (3331 B) and `dev` (2682 B); absent on `main` |
| Release config `.releaserc` | **policy-wins** (canonical config lands in Phase 2) | three different configs; none is authoritative today |
| Tracked `coverage.txt` | **drop** (dev-only artifact bug) | tracked on `dev` only, 43330 B, committed by `d3aea6b`, no branch ignores it |

**Reconciliation direction**: for toolchain/dependency/release-hardening content the flow is
`v3 → (main after promotion)`; for v4 product content the flow is `dev → v4 → main`.
Nothing is ever reconciled "from main" on the assumption that main is newer — main is the ancestor.

---

## 1. Ref inventory at audit time

```console
$ for b in main dev v3; do echo -n "$b: "; git rev-parse origin/$b; done
main: 0d4c36e5f7839bac00ab83d15454dbdabb02ddcf
dev:  264bd3fd91815e25f79c88206abc6031b7962064
v3:   bec2249b985e05555a00bbed734683f249d57e58

$ curl -s https://api.github.com/repos/honeydipper/honeydipper | jq -r '.default_branch'
dev
```

> Note: the **default branch is `dev`**, not `main`. `main` is protected but dormant.

| Branch | SHA | Module | `go` directive | CI default image | `publish-image` job | `.releaserc` branches |
|---|---|---|---|---|---|---|
| `main` | `0d4c36e5` | `.../honeydipper/v3` | 1.24.1 | `golang:1.24.1-alpine` | **absent** | `["main"]` |
| `v3` | `bec2249b` | `.../honeydipper/v3` | 1.25.0 | `golang:1.27.3-alpine` | present (`tag` + `recovery`) | `["main","v3"]` |
| `dev` | `264bd3fd` | `.../honeydipper/v4` | 1.27.3 | `golang:1.27.3-alpine` | present (`tag` + `recovery`) | `["main","v3",{dev:beta}]` |

**Deviation vs. the plan's state table**: the plan recorded `dev` at `cb50cddb` (4.0.0-dev.14 era).
`dev` has since advanced by 4 commits to `264bd3f`; the freeze branch was cut from the current tip
(see §7). The `dev` line advanced, it did not change character:

```console
$ git log --oneline cb50cddb..264bd3f
264bd3f chore(release): 4.0.0-dev.15
1f40320 chore(deps): update module github.com/moby/spdystream to v0.5.1 [security] (#835)
4253549 fix(deps): update module google.golang.org/grpc to v1.83.2 [security] (#833)
bbffa00 fix(deps): update module github.com/go-git/go-git/v5 to v5.19.2 [security] (#689)
```

---

## 2. Ancestry: `main` is the merge-base of everything

```console
$ for p in "main dev" "main v3" "v3 dev"; do set -- $p; git merge-base --is-ancestor origin/$1 origin/$2 \
    && echo "$1 IS ancestor of $2" || echo "$1 is NOT ancestor of $2"; done
main IS ancestor of dev
main IS ancestor of v3
v3 is NOT ancestor of dev

$ git merge-base --is-ancestor origin/dev origin/main || echo "dev is NOT ancestor of main"
dev is NOT ancestor of main

$ for p in "main dev" "main v3" "v3 dev"; do set -- $p; echo -n "merge-base $1...$2 : "; git merge-base origin/$1 origin/$2; done
merge-base main...dev : 0d4c36e5f7839bac00ab83d15454dbdabb02ddcf
merge-base main...v3  : 0d4c36e5f7839bac00ab83d15454dbdabb02ddcf
merge-base v3...dev   : 0d4c36e5f7839bac00ab83d15454dbdabb02ddcf
```

All three merge-bases are identical and equal to `main`'s tip ⇒ **`main` is the common ancestor**,
`v3` and `dev` are two independent descendants of it.

### Commit divergence (`--left-right --cherry-pick`, i.e. patch-equivalent commits collapsed)

```console
$ for p in "main dev" "main v3" "v3 dev"; do set -- $p; echo "--- $1...$2 ---";
    git rev-list --left-right --cherry-pick --count origin/$1...origin/$2; done
--- main...dev ---
0	224          # only-in-main: 0, only-in-dev: 224
--- main...v3 ---
0	17           # only-in-main: 0, only-in-v3: 17
--- v3...dev ---
17	224          # only-in-v3: 17, only-in-dev: 224
```

`only-in-main: 0` in every pairing — **`main` carries no unique commit** relative to either
descendant once patch-equivalent commits are collapsed. There is nothing on `main` to "keep"
that is not already reachable from `v3` or `dev`.

### The 17 commits unique to `v3`

```console
$ git log --left-right --cherry-pick --format='%m %h %ad %s' --date=short origin/main...origin/v3
> bec2249 2026-10-28 chore(release): 3.10.5
> ce63ca3 2026-10-27 docs(release): note how skipped CI directives block v3 releases (#848)
> 50196a2 2026-10-27 fix(deps): build with Go 1.27.3 to address CVE-2026-111943 (#847)
> 89f3a56 2026-10-02 chore(release): 3.10.4 [skip ci]
> d2dc272 2026-10-02 Merge pull request #828 from .../fix/DTSHOPSRE-7816-v3-image-vulnerabilities
> d79749a 2026-10-02 fix(drivers): preserve Ollama tool schemas
> 4c4f0e0 2026-10-01 fix(deps): restore Go 1.25 lint compatibility
> caf8876 2026-10-01 fix(security): remediate v3 image vulnerabilities
> 0f95002 2026-09-28 chore(release): 3.10.3 [skip ci]
> 6d1aae5 2026-09-28 Merge pull request #827 from .../fix/DTSHOPSRE-7816-v3-release-exact-tag
> dc0c7e3 2026-09-28 fix(release): build v3 images from exact tag
> 1d675d3 2026-09-28 chore(release): 3.10.2 [skip ci]
> 00c257f 2026-09-28 Merge pull request #826 from .../fix/workflow-thread-context-race
> 0e59990 2026-09-28 fix(workflow): synchronize thread context snapshots
> 7735358 2026-03-19 chore(release): 3.10.1 [skip ci]
> 014ef36 2026-03-19 ci: semantic release for v3 (#688)
> aa8651b 2026-03-19 fix: sync creating parallel childs to avoid concurrent map access (#687)
```

These are **security remediations and release-pipeline hardening**, dated after `main`'s last
touch — the empirical proof that `v3`, not `main`, is the modern maintenance line.

---

## 3. Area A — dependency / toolchain files ⇒ **V3 WINS**

```console
$ git diff --stat origin/main origin/v3 -- go.mod go.sum .circleci/config.yml
 .circleci/config.yml | 176 ++++++++++++++++++++++++++++++++++++++++-
 go.mod               | 123 ++++++++++----------
 go.sum               | 314 +++++++++++++++++++++++++++++++---------------------------
 3 files changed, 383 insertions(+), 230 deletions(-)
```

### `go.mod` / `go.sum`

```console
$ for b in main v3 dev; do printf "%-5s module=%-52s go=%s\n" "$b" \
    "$(git show origin/$b:go.mod | sed -n 's/^module //p')" "$(git show origin/$b:go.mod | sed -n 's/^go //p')"; done
main  module=github.com/honeydipper/honeydipper/v3                go=1.24.1
v3    module=github.com/honeydipper/honeydipper/v3                go=1.25.0
dev   module=github.com/honeydipper/honeydipper/v4                go=1.27.3

$ for b in main v3 dev; do printf "%-5s go.sum lines=%s\n" "$b" "$(git show origin/$b:go.sum | wc -l)"; done
main  go.sum lines=699
v3    go.sum lines=681
dev   go.sum lines=757
```

```console
$ git diff origin/main origin/v3 -- go.mod | head -24
 module github.com/honeydipper/honeydipper/v3
 
-go 1.24.1
+go 1.25.0
 
 require (
-	cloud.google.com/go v0.120.1 // indirect
-	cloud.google.com/go/pubsub v1.49.0
-	cloud.google.com/go/spanner v1.80.0
-	cloud.google.com/go/storage v1.51.0
+	cloud.google.com/go v0.123.0 // indirect
+	cloud.google.com/go/pubsub v1.50.1
+	cloud.google.com/go/spanner v1.87.0
+	cloud.google.com/go/storage v1.56.0
 	github.com/DataDog/datadog-go v4.8.3+incompatible
 ...
-	github.com/googleapis/gax-go/v2 v2.14.1
+	github.com/googleapis/gax-go/v2 v2.17.0
```

Every changed dependency line moves **forward** from `main` to `v3`. Nothing moves backward.
`v3` is a strict dependency superset-in-time of `main`.

### `.circleci/config.yml`

```console
$ for b in main v3 dev; do echo -n "$b default image: "; \
    git show origin/$b:.circleci/config.yml | grep -m1 'image: golang' | sed 's/.*image: //'; done
main default image: golang:1.24.1-alpine
v3   default image: golang:1.27.3-alpine
dev  default image: golang:1.27.3-alpine

$ # job presence
main  publish-image=0  approve-gates=(none)      recovery-filter=(absent)
v3    publish-image=3  approve-gates=approve-v3-release
dev   publish-image=3  approve-gates=approve-dev-release
```

Recovery-path filters (verbatim):

```yaml
# origin/v3 — publish-image-recovery
filters: |
  pipeline.parameters.release_tag != ""
  and (pipeline.git.branch == "v3" or pipeline.git.branch == "main")
```
```yaml
# origin/dev — publish-image-recovery
filters: |
  pipeline.parameters.release_tag != ""
  and (pipeline.git.branch == "dev" or pipeline.git.branch == "main")
```
`main` has no `publish_release_image` workflow at all — it cannot publish an image, and cannot
recover one. `v3`'s recovery filter already covers `v3 or main`, which is exactly the surface
Phase 5 needs.

### Lint toolchain

```console
main  lint image = golangci/golangci-lint:v1.64.8-alpine       (integration job image: golang:1.26.0-alpine)
v3    lint image = golang:1.26.10-alpine + go install golangci-lint@v1.64.8   (with comment: golangci-lint v1
                     cannot read go1.27 export data, keep linting on go1.26 — CVE-2026-111943 fixed)
dev   lint image = golangci/golangci-lint:v2.15.0-alpine
```

**Area A verdict: v3 wins.** Reconciliation must never overwrite `go.mod`/`go.sum`/
`.circleci/config.yml` on `v3` with `main`'s content. When `main` becomes the v4 line (Phase 3),
its content comes from `v4`/`dev`, and the v3 toolchain hardening stays on `v3`.

---

## 4. Area B — architecture docs ⇒ **DEV WINS, and they do NOT belong on v3**

```console
$ for f in HONEYDIPPER_CONTEXT.md FUNCTION_MAP.md README.md docs/README.md docs/releasing.md; do ...
  HONEYDIPPER_CONTEXT.md   main ABSENT   v3 ABSENT   dev present (34306 B, sha 68b5523a)
  FUNCTION_MAP.md          main ABSENT   v3 ABSENT   dev present (16136 B, sha f728a6a0)
  README.md                main present  v3 present  dev present
                           main sha 8b546ea0 (7069 B) == v3 sha 8b546ea0 (7069 B);  dev sha 98c71d3e (15321 B)
  docs/README.md           main sha 0d19e377 == v3 sha 0d19e377;                dev sha f28ece63
  docs/releasing.md        main ABSENT   v3 present (3331 B)                    dev present (2682 B)
```

Correction to the earlier audit: the plan predicted the architecture docs would be
"present on main/dev". **They are on `dev` only** — `main` never had them either.

```console
$ git show origin/dev:HONEYDIPPER_CONTEXT.md | head -1
# Honeydipper v4 — Comprehensive AI Knowledge Base
$ git show origin/dev:FUNCTION_MAP.md | head -1
# Function Map for Honeydipper v4
```

Both documents are explicitly **v4-scoped**: they describe `module .../v4`, go 1.27.3, the
`internal/agent/` service, `pkg/workflow/`, MCP tooling and the agent conversation contract. None
of that exists on `v3` (`internal/workflow/` instead of `pkg/workflow/`, no agent service, no MCP).

**Area B verdict: dev wins.** Explicit recommendation, recorded as a decision:

> **Do NOT backport `HONEYDIAPPER_CONTEXT.md` / `FUNCTION_MAP.md` to `v3`.**
> They are v4 architecture documents; on `v3` they would be factually wrong (wrong module path,
> wrong package layout, non-existent services) and would mislead both humans and AI agents that
> read them as ground truth. If v3 needs orientation material, write a small **v3-accurate**
> section/file (v3 module path, `internal/workflow/`, no agent service) — never a copy of the v4 docs.

`README.md` and `docs/README.md` are byte-identical between `main` and `v3` (sha `8b546ea0` /
`0d19e377`), and `dev` carries a large v4 rewrite (`+326 / -60` on `README.md`). So `README` is not
a "main-vs-v3" divergence at all — it is a "v4-vs-v3" divergence, again resolved by **dev wins
for v4 content, no backport to v3**.

`docs/releasing.md` is the one doc that exists on `v3` but not `main` — further proof of v3's
release-engineering maturity, and the natural home for v3-specific release notes.

---

## 5. Area C — `.releaserc` ⇒ **POLICY WINS (three-way divergence, none authoritative)**

```console
$ git show origin/main:.releaserc
{ "branches": ["main"], "plugins": [ commit-analyzer, release-notes-generator, changelog, git, github ] }

$ git show origin/v3:.releaserc
{ "branches": ["main", "v3"], "plugins": [ ... , ["@semantic-release/git", {message: "chore(release): ${nextRelease.version}\n\n${nextRelease.notes}"}], ... ] }

$ git show origin/dev:.releaserc
{ "branches": ["main", "v3", { "name": "dev", "prerelease": true, "channel": "beta" }], ... }
```

Consequences that matter for the plan:

1. `main`'s config knows only about `main` ⇒ running it on `v3` would publish **wrong-channel**
   versions. It is not a superset and not a fallback.
2. `v3`'s config has no `dev` entry, so it cannot be shared upward.
3. `dev`'s config is the only one that already describes all three long-lived branches — but it
   does **not** yet express "v3 releases are prerelease by design", which is a Phase 2 concern
   (the `prerelease: true` flag on the `v3` branch entry).

**Area C verdict: policy-wins.** No branch's `.releaserc` is copied to another during Phase 1.
The canonical config is authored once, in Phase 2, and is the single source of truth. Phase 1
deliberately does **not** modify `.releaserc` on any branch.

---

## 6. Area D — tracked `coverage.txt` ⇒ **DROP**

```console
$ for b in main v3 dev; do ...; done
main  absent
v3    absent
dev   TRACKED size=43330

$ git log --oneline --diff-filter=A -- coverage.txt
d3aea6b feat: stateless workflow engine
```

`.gitignore` on all three branches contains only the Go-tool comment block (`# Output of the go
coverage tool...` at line 11) and **no pattern that ignores `coverage.txt` or `c.out`**. So `dev`
committed a build artifact by accident, and nothing prevents it from being re-added.

**Area D verdict: drop.** `coverage.txt` is a generated artifact (CI runs
`REPORT_TEST_COVERAGE=1 make unit-tests` and publishes `c.out`), tracked only on `dev`. Removal is
a non-code change and is queued for Phase 2 alongside the `.gitignore` entry; Phase 1 records the
decision only, keeping this PR docs-only.

---

## 7. Tag ancestry (answers to the Phase 1 questions)

**Q: Was `v3.10.5` tagged from `v3` or from `main`?**

```console
$ git log -1 --oneline --decorate v3.10.5
bec2249 (tag: v3.10.5, origin/v3) chore(release): 3.10.5

$ git branch -a --contains $(git rev-list -n1 v3.10.5)
  remotes/origin/v3

$ git for-each-ref refs/tags/v3.10.5 --format='%(objecttype) %(objectname:short) -> %(*objectname:short)'
commit bec2249 ->            # lightweight tag pointing directly at v3's tip
```

⇒ **Tagged from `v3`.** `v3.10.5` is the annotated-free (lightweight) tag on the tip of `v3`.

**Q: Do `v3.10.2`–`v3.10.5` share `v3`'s tag-reachable line?**

```console
$ for t in v3.10.0 v3.10.1 v3.10.2 v3.10.3 v3.10.4 v3.10.5; do c=$(git rev-list -n1 $t); \
    printf "%-8s %s in-v3=%s in-main=%s in-dev=%s\n" ...; done
v3.10.0 a7e34772 in-v3=yes in-main=yes in-dev=yes     # predates the fork: on the common ancestor
v3.10.1 7735358c in-v3=yes in-main=no  in-dev=no
v3.10.2 1d675d30 in-v3=yes in-main=no  in-dev=no
v3.10.3 0f950029 in-v3=yes in-main=no  in-dev=no
v3.10.4 89f3a566 in-v3=yes in-main=no  in-dev=no
v3.10.5 bec2249b in-v3=yes in-main=no  in-dev=no
```

⇒ **Yes.** `v3.10.1`–`v3.10.5` are all reachable from `v3` and from nothing else; they sit on one
linear, `v3`-only tag-reachable line that starts right after `v3.10.0` (the last release on the
shared ancestor). This is precisely why **`v3` history may never be reset or force-pushed**: it is
the sole source for 3.x semantic-release version computation.

**Q: Is `main` an ancestor of `dev`?**

```console
$ git merge-base --is-ancestor origin/main origin/dev && echo "main IS ancestor of dev"
main IS ancestor of dev
```

⇒ **Yes.** Same for `v3`: `main` is an ancestor of `v3` too. `main` is an ancestor of *both*.

---

## 8. Toolchain reality (baseline policy, NOT a bump task)

Recorded as the standing baseline for the `v3` line, not as work to be done:

| Item | `v3` reality | Policy |
|---|---|---|
| `go.mod` `go` directive | `1.25.0` | baseline; do not "upgrade to main's 1.24.1" — that would be a downgrade |
| CI build/test image | `golang:1.27.3-alpine` | baseline (carries the CVE-2026-111943 fix) |
| Lint image | `golang:1.26.10-alpine` + `golangci-lint@v1.64.8` | deliberate: golangci-lint v1 cannot read go1.27 export data. Pinning lint to go1.26 is **intentional**, not stale |
| Integration image | `golang:1.27.3-alpine` + `redis:7.0.11-alpine` | baseline |
| Image publishing | `publish-image` (tag-triggered, exact-tag checkout, immutable, digest evidence) | present and load-bearing |

`main` by contrast has `go 1.24.1`, `golang:1.24.1-alpine`, a `/v3` module path and **no
`publish-image` job**. Any "sync main into v3" operation would regress CVEs, the Go directive, and
image publication. That is the mechanical form of the direction correction.

---

## 9. Safety net created by this phase (immutable)

| Ref | Type | Target | SHA |
|---|---|---|---|
| `archive/v3-pre-promotion` | annotated tag **and** branch | `v3` pre-promotion head | `bec2249b985e05555a00bbed734683f249d57e58` |
| `archive/main-pre-v4` | annotated tag **and** branch | `main` pre-v4 head | `0d4c36e5f7839bac00ab83d15454dbdabb02ddcf` |
| `archive/v4-freeze` | annotated tag | `v4.0.0-dev.14` commit | `f0e411dc9b054df9cdb7a2e78e121b44366bc31c` |
| `archive/v4-freeze-dev.15` | annotated tag | `v4.0.0-dev.15` commit (= current `dev` tip) | `264bd3fd91815e25f79c88206abc6031b7962064` |
| `v4` | branch (short-lived freeze) | `dev` tip at cut time | `264bd3fd91815e25f79c88206abc6031b7962064` |

Tags are the primary, never-deleted safety net; the `archive/*` **branches** are secondary and may
be recreated from their tags at any time:

```bash
git fetch origin --tags
git branch -f archive/v3-pre-promotion archive/v3-pre-promotion^{commit}
git push origin archive/v3-pre-promotion
```

Recovery of the exact pre-promotion trees:

```bash
git checkout archive/v3-pre-promotion^{commit}     # v3 as it was, forever
git checkout archive/main-pre-v4^{commit}          # main as it was, forever
```

---

## 10. Reproduction

All of the above reproduces with no working tree and no privileges:

```bash
git fetch --tags origin
for b in main v3 dev; do git rev-parse origin/$b; done
for p in "main dev" "main v3" "v3 dev"; do
  set -- $p
  git merge-base --is-ancestor origin/$1 origin/$2 && echo "$1 IS ancestor of $2" || echo "$1 is NOT ancestor of $2"
  git rev-list --left-right --cherry-pick --count origin/$1...origin/$2
  git diff --stat origin/$1 origin/$2 -- go.mod go.sum .circleci/config.yml
done
git log --left-right --cherry-pick --format='%m %h %ad %s' --date=short origin/main...origin/v3
for t in v3.10.2 v3.10.3 v3.10.4 v3.10.5; do git branch -a --contains "$(git rev-list -n1 $t)"; done
```

### Deviation log

| Plan stated | Observed | Handling |
|---|---|---|
| default branch = `main` | default branch = **`dev`** | docs PR based on `dev` (the actual default/integration branch) |
| `dev` head = `cb50cddb` | `dev` head = `264bd3f` (+4 commits, incl. `4.0.0-dev.15`) | freeze branch cut from current tip; `archive/v4-freeze` kept at `dev.14` as planned, plus `archive/v4-freeze-dev.15` at the cut point |
| arch docs "present on main/dev" | present on **`dev` only** | recorded as corrected evidence; recommendation unchanged (no backport to `v3`) |
| `v3.10.2–5` flagged `prerelease:true` | confirmed via Releases API | documented in `docs/release-process.md` bug-vs-intended matrix |
