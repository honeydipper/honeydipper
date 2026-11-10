# Honeydipper Release Process

**Scope**: the branch model, release channels, gating ceremonies, recovery paths, backport rules,
and automation caveats that govern releases after the v4.0.0 promotion.
**Companion documents**: [`docs/branch-divergence-report.md`](./branch-divergence-report.md) (Phase 1
evidence), [`docs/releasing.md`](./releasing.md) (image publishing mechanics and registry credentials).

---

## 1. Branch model

| Branch | Role | Lifetime | Protection | Notes |
|---|---|---|---|---|
| `main` | **v4 release line** after promotion (Phase 3). Historically the dormant v3-era line. | permanent | protected: PR + 1 review + code-owner review + status checks + push restrictions | the v4 **release channel** (`latest`); `semantic-release` runs here with **no** approval gate. **Not** the repo default branch today — that is `dev` (§10.1, §13.5) |
| `v3` | **3.x maintenance line** — security and patch releases only | permanent (through Phase 5) | protected since Phase 1: PR + 1 review + code-owner review + required checks + no force-push + push restrictions | **never reset, never force-pushed**; history is the sole input to 3.x version computation |
| `dev` | **integration / beta line** — all feature work lands here first | permanent | protected: PR + 1 review + code-owner review + strict status checks + push restrictions | **the repo default branch today** — Renovate reads its config from here (§10.1, §13.5); releases publish as `beta` prereleases |
| `v4` | **short-lived freeze line** — the exact tree being promoted into `main` | temporary, deleted after promotion | none (freeze ref, not a collaboration branch) | created from `dev` tip; see §7 |
| `archive/*` | **immutable safety net** (tags and their mirror branches) | permanent | never deleted, never moved | see §8 |

Downstream of decisions in Phase 1: **`main` is the ancestor of `v3` and `dev`**, so no
reconciliation may assume "main is newer". See the divergence report for per-area winners.

### Direction of content flow

```
feature work ──► dev ──► (v4 freeze) ──► main            # v4 product content
security fix ──► dev ──► cherry-pick -x ──► v3            # 3.x maintenance
toolchain / CI hardening ──► v3 (authoritative for 3.x)   # never copied back from main
```

---

## 2. Release channels

| Source branch | `.releaserc` entry | npm dist-tag / channel | GitHub prerelease flag | Example |
|---|---|---|---|---|
| `main` | `"main"` | `latest` (default) | **false** — becomes the repo "Latest" release | `v4.0.0` |
| `v3` | `{ name: "v3", range: "3.x", channel: "3.x" }` | `3.x` | **true — BY DESIGN** (derived from the non-default channel, **not** from a `prerelease` key) | `v3.11.0` |
| `dev` | `{ name: "dev", prerelease: true, channel: "beta" }` | `beta` | true | `v4.0.0-dev.16` |

Canonical configuration (authored once, in Phase 2 — Phase 1 does **not** touch `.releaserc`):

```jsonc
{
  "branches": [
    "main",
    { "name": "v3", "range": "3.x", "channel": "3.x" },
    { "name": "dev", "prerelease": true, "channel": "beta" }
  ]
}
```

### Why `v3` uses the *maintenance-branch* shape and **not** `prerelease: true`

`v3` is a **maintenance branch**, so its entry is `{ name: "v3", range: "3.x", channel: "3.x" }`:

- **`range: "3.x"`** is what makes it a maintenance branch. It pins version computation to the
  3.x tag line, so the next release is a stable `3.x.y` patch.
- **`channel: "3.x"`** publishes to the `3.x` dist-tag instead of `latest`. Because this is **not
  the default channel**, the `@semantic-release/github` plugin marks the resulting GitHub release
  `prerelease: true` — which is precisely the intended behavior (§3, "future `v3.11.x+`" row): it
  keeps 3.x releases out of the repo "Latest" slot so that `latest` always resolves to the newest
  v4 release.
- **`prerelease: true` is deliberately NOT set on `v3`.** In semantic-release, `prerelease: true`
  declares a *prerelease channel*: it emits prerelease-suffixed versions (e.g. `3.10.6-v3.1`)
  rather than stable `3.x.y` patches. Putting it on a maintenance branch would quietly destroy 3.x
  versioning and produce versions no 3.x consumer expects. **The GitHub `prerelease` flag is a
  consequence of the non-default channel, not a configuration key.**

The 3.x artifacts remain fully available by tag, by the `3.x` channel, and as immutable images.

---

## 3. Prerelease flag: historical bug vs. intended behavior

The `prerelease` flag has two distinct histories. Do not conflate them.

| Release(s) | Flag observed | Verdict | Action |
|---|---|---|---|
| `v3.10.0`, `v3.9.x`, `v3.8.0` (≤ 2026-02) | `prerelease: false` | correct at the time — `v3` was not yet a managed channel entry | none |
| **`v3.10.1`–`v3.10.5`** (2026-03-19 → 2026-10-28) | `prerelease: true` | **BUG.** They are full 3.x patch releases, yet they were published flagged `prerelease: true` and so never occupied the "Latest" slot. The *verdict* is certain from the Releases API; the *mechanism* is not (see below) | patch **later, in Phase 2**, by re-flagging these existing releases (`prerelease: false` is correct for them: they are completed 3.x patches) and by giving `v3` the maintenance-branch entry (`range` + `channel`, §2) |
| **future `v3.11.x+`** releases (post-Phase 2) | `prerelease: true` | **INTENDED.** `v3` is a parallel maintenance channel; its releases are deliberately not "Latest" | none — this is the design |

**Root cause: not yet established.** What is *observed* (Releases API, no inference): `v3`'s
`.releaserc` lists `"v3"` as a bare string branch — an entry carrying neither `range` nor `channel` —
and the releases published from it are flagged `prerelease: true` with the "Latest" pointer
unassigned. **The exact mechanism is to be established by the Phase 2 root-cause investigation.**
Hypotheses (release-author identity, absent `range`/`channel` pair, plugin default behavior) are
recorded in the Phase 2 investigation ticket as leads only; this document does not assert one.
Phase 2 fixes the config; the already-published releases are corrected in place (their tags and
images are untouched — only the GitHub release flag changes).

**Invariant**: changing a release's `prerelease` flag never changes its tag, its commit, or its
image digest. The flag is presentation and "Latest"-selection only.

---

## 4. The v3 two-approval ceremony ("dev-plus-extra-gate")

`v3` is `dev` plus one extra safety property: **nothing publishes to a 3.x consumer without a human
saying so twice, once for the version and once for the image.**

| Step | CircleCI job | Branch | Gate |
|---|---|---|---|
| 1 | `approve-v3-release` (`type: approval`) | `v3` | 0 — exists today |
| 2 | `semantic-release-v3` | `v3` | `requires: [approve-v3-release]` — exists today |
| 3 | `approve-v3-image` (`type: approval`) | tag pipeline | **Phase 2** — does not exist yet |
| 4 | `publish-image-tag` | tag pipeline | Phase 2 adds `requires: [approve-v3-image]` on the `v3` line only |

Ordering and why it is safe:

1. **Version approval precedes mutation.** `approve-v3-release` runs before `semantic-release-v3`,
   so no tag exists until a human approves. semantic-release then creates the tag **and** the GitHub
   release in one step, and pushes a `chore(release)` commit.
2. **Image approval follows an immutable reference.** `approve-v3-image` gates the image job, which
   is keyed off the **tag** — by the time it can be approved, the tag commit is already fixed and
   verified (`git checkout --detach`, plus a GitHub-release existence check, plus a Docker Hub
   no-overwrite probe). Approving therefore means "publish this already-frozen artifact", never
   "choose what to freeze".
3. **The two approvals are independent.** A rejected image gate leaves a valid tag and release with
   no image — recoverable later via the `release_tag` pipeline parameter (§5). A rejected version
   gate leaves nothing at all. Neither gate can corrupt the other.
4. **`main` (v4) is single-gate by design.** v4 is the primary line, its releases are the product
   itself, and its image is built from a tag that is itself the release of record; adding a second
   human gate there would buy latency, not safety. The extra gate exists on `v3` precisely because
   it is a *parallel* channel that must never be mistaken for the product line.

---

## 5. Recovery path (deliberately un-gated)

```yaml
# publish_release_image → publish-image-recovery
filters: |
  pipeline.parameters.release_tag != ""
  and (pipeline.git.branch == "v3" or pipeline.git.branch == "main")     # v3 line
  # dev line:  and (pipeline.git.branch == "dev" or pipeline.git.branch == "main")
```

Run the pipeline on the branch with pipeline parameter `release_tag=vX.Y.Z`. No approval job gates
this path. That is **intentional**:

- **It cannot create a new version.** The job refuses any `release_tag` that is not an existing Git
  tag, checks out that exact commit, asserts `HEAD == tag commit`, and requires the matching GitHub
  release to exist. The only possible outcome is "publish the image for an artifact that is already
  released".
- **It cannot overwrite.** A second guard probes Docker Hub and fails on HTTP 200, so a published
  image is never replaced.
- **It is the break-glass path.** Its whole purpose is the state "release exists, image does not" —
  i.e. a failed `publish-image` after a tag was cut. Gating it on the same approval that just failed
  would turn a one-click repair into an escalation during an incident.
- **Blast radius is bounded by the tag set**, which is bounded by §6 invariants.

So the asymmetry is: **mutating paths are gated; repair paths are not, because they cannot mutate.**
The recovery filter deliberately includes `main` alongside the line branch — after Phase 3, when
`main` becomes v4, the same recovery job must still be able to repair a 3.x image, and an operator
may legitimately run the pipeline from either checkout.

---

## 6. Backport rules (v4 → v3)

1. **Label, then land.** A fix destined for 3.x gets the **`needs-v3-backport`** label on its PR to
   `dev`/`main`. The label is the contract; a missing label means "v4 only".
2. **Cherry-pick, never hand-merge.** Backports are `git cherry-pick -x <sha>` onto `v3`, preserving
   the `(cherry picked from commit ...)` provenance line. `-x` is mandatory: it is the only durable
   record that the commit exists twice.
3. **CHANGELOG is regenerated, never hand-merged.** `CHANGELOG.md` on `v3` is owned by
   `@semantic-release/changelog`. A backport PR must not carry an edited `CHANGELOG.md`; the
   `chore(release)` commit regenerates it. Conflicts in `CHANGELOG.md` are resolved by **taking the
   branch's own copy** and re-running the release, never by merging hunks.
4. **Rebase, don't merge, on conflict.** If a cherry-pick conflicts, rebase the backport branch onto
   `v3` and resolve in the source commit. Never merge `dev` or `main` into `v3` to "make it fit" —
   that injects v4 module paths and v4-only packages into the 3.x tree.
5. **Every backport goes through a PR.** `v3` is protected (§7); direct pushes are not the mechanism,
   even for maintainers. The PR must pass `golangci-lint`, `license_finder`, `test`, `integration`
   and `check_pr_title`, and take one code-owner approval.
6. **Never backport v4 architecture docs.** `HONEYDIPPER_CONTEXT.md` and `FUNCTION_MAP.md` describe
   the `/v4` module and v4-only packages; on `v3` they would be false. See the divergence report.
7. **Never backport `go.mod` module path, `.releaserc` branches, or v4 driver packages.** Toolchain
   scope on `v3` is defined by §9.

---

## 7. Tag and image invariants

1. **No manual tags.** Every release tag is created by `semantic-release` on its configured branch.
   Hand-cut tags bypass version computation and desynchronize `CHANGELOG.md` from the tag line.
   (The only hand-made tags in this repo are the `archive/*` safety net, which are *not* releases.)
2. **No force-push anywhere.** Not on `main`, `dev`, `v3`, or a release tag. Force-pushing `v3`
   would rewrite the history semantic-release computes 3.x versions from and orphan the
   `v3.10.1`–`v3.10.5` tag-reachable line. Backups exist (§8) but are a recovery path, not a license.
3. **No tag moves or deletions.** A published tag identifies an immutable tree; the image is built
   from that exact tree. Moving the tag would make tag and image disagree forever.
4. **Images are immutable.** `publish-image` never overwrites: it probes Docker Hub and fails on an
   existing tag, records the `repo@sha256:...` digest plus platform into the `release-evidence`
   artifact, and refuses to publish when the digest is not a repository digest. Deployments pin
   digests, not floating tags.
5. **Release commit messages omit `[skip ci]`** on purpose — CircleCI suppresses tag pipelines whose
   commit carries that directive (this exact trap blocked v3 releases once; see commit `ce63ca3`).
6. **`publish_release_image` tag filter must stay explicit** — CircleCI ignores tag pushes unless at
   least one job opts in.

### The "Latest" safety-net step

Because `v3` releases publish to a non-default channel (`3.x`) and `dev` releases are
`prerelease: true`, GitHub's "Latest" pointer is **not** advanced by either of them. Every
promotion therefore ends with an explicit check:

1. Open the newest non-prerelease release on `main` and confirm it is marked **Latest**.
2. If it is not (e.g. a `v3` release was published mis-flagged as non-prerelease), set
   `make_latest` on the correct v4 release — **never** by re-tagging or re-releasing.
3. Verify `https://github.com/honeydipper/honeydipper/releases/latest` resolves to the intended v4
   version, and that it has an image whose digest matches `release-evidence`.
4. Record the verification in the promotion PR/issue.

This is the guard against the §3 bug class: a mis-flagged maintenance release must never be able to
displace the product line's "Latest".

---

## 8. Safety net (Phase 1, immutable)

| Ref | Type | SHA | Purpose |
|---|---|---|---|
| `archive/v3-pre-promotion` | annotated tag (+ mirror branch) | `bec2249b` | `v3` exactly as it was before promotion |
| `archive/main-pre-v4` | annotated tag (+ mirror branch) | `0d4c36e5` | `main` exactly as it was before v4 promotion |
| `archive/v4-freeze` | annotated tag | `f0e411dc` | `v4.0.0-dev.14` freeze point |
| `archive/v4-freeze-dev.15` | annotated tag | `264bd3fd` | `v4.0.0-dev.15`, the commit the `v4` branch was cut from |
| `v4` | branch | `264bd3fd` | short-lived freeze line, cut from `dev` |

**Never delete an `archive/*` tag.** The mirror branches may be recreated from their tags at any time:

```bash
git fetch origin --tags
git branch -f archive/v3-pre-promotion archive/v3-pre-promotion^{commit} && git push origin archive/v3-pre-promotion
git branch -f archive/main-pre-v4    archive/main-pre-v4^{commit}    && git push origin archive/main-pre-v4
```

The `v4` freeze branch is deleted after promotion; the `archive/v4-freeze*` tags keep that tree
recoverable.

---

## 9. Toolchain policy

Baseline for the `v3` line as it stands (recorded as **policy, not a pending upgrade**):

| Item | Value | Why |
|---|---|---|
| `go` directive | `1.25.0` | `main` is at 1.24.1 — moving `v3` "toward main" would be a **downgrade** |
| build/test image | `golang:1.27.3-alpine` | carries the CVE-2026-111943 fix |
| lint image | `golang:1.26.10-alpine` + `golangci-lint@v1.64.8` | golangci-lint v1 cannot read go1.27 export data; the pin is **deliberate** |
| integration image | `golang:1.27.3-alpine`, `redis:7.0.11-alpine` | matches build image |
| image publishing | `publish-image` (exact-tag checkout, no-overwrite, digest evidence) | load-bearing for §7 |

Rules:

- Toolchain moves **forward only**, and only with a CVE/security or compatibility justification.
- Lint is allowed to run on an older Go than the build/test toolchain; do not "fix" that by bumping
  the lint image past golangci-lint v1's export-data support.
- `v3` and `main`(v4) may legitimately run different Go versions during the 3.x/4.x overlap. That is
  expected, not drift to be reconciled.
- Renovate may propose Go/toolchain PRs on any branch (§10); the `ci/circleci: golangci-lint` check on
  `v3` is the gate that catches "bumped past the lint pin".

---

## 10. Renovate caveats

`renovate.json` exists on `main`, `v3` and `dev` and is currently **byte-identical**:

```json
{ "extends": ["config:recommended"],
  "packageRules": [{ "matchDepTypes": ["require"], "semanticCommitType": "fix" }],
  "prConcurrentLimit": 10,
  "schedule": ["before 3:00am on the 3 day of the month"] }
```

Operational caveats that Phase 2 must design around:

1. **Config is read from the default branch only.** The default branch is **`dev`**. `renovate.json`
   on `v3` and `main` is **not** consulted for scheduling/rules — it is only version-controlled
   documentation of intent for those branches. Any policy that must hold on `v3` has to be expressed
   in the **default-branch** config. Whether the default branch moves to `main` after promotion is
   an open Phase 3 decision — see §13.5.
2. **`baseBranches` is required for multi-line scanning.** To have Renovate open PRs against `v3`
   and `main`, the default-branch config must list them explicitly, e.g.
   `"baseBranches": ["dev", "main", "v3"]`. Without it, only the default branch is updated and the
   maintenance lines silently rot.
3. **`prConcurrentLimit` is global to the config, not per base branch.** `10` means 10 open Renovate
   PRs *across all base branches*; one noisy branch starves the others. Use per-branch
   `packageRules` with `matchBaseBranches` and `prConcurrentLimit`/`prHourlyLimit` overrides so each
   line gets its own budget.
4. **Staging must stay byte-identical across lines.** The three copies must not drift; a divergence
   produces different effective policy per line once (1) changes. Treat the `dev` copy as canonical
   and copy it verbatim (Phase 2).
5. **`semanticCommitType: "fix"` on `require` deps** means every dependency bump proposes a **patch
   release**. On `v3` that is intentional (security patches must be releasable), but it also means a
   Renovate PR is a release trigger once the approval gate is passed — sequence them with §4.
6. **Security PRs and the `[security]` suffix** (e.g. `#835`, `#833`, `#837`) indicate
   vulnerability-driven updates. They must not be batched into a routine monthly window; they travel
   the same PR path but should be merged and released promptly on both `dev` and `v3`.
7. **Renovate never touches release state.** It opens PRs; it does not tag, publish, or edit
   `.releaserc`. Version computation stays exclusively with `semantic-release` on the configured
   branches.

---

## 11. Branch protection on `v3` (applied in Phase 1)

Applied via the REST API and verified by read-back:

| Setting | Value |
|---|---|
| required status checks (strict) | `ci/circleci: golangci-lint`, `ci/circleci: license_finder`, `ci/circleci: test`, `ci/circleci: integration`, `check_pr_title` |
| required PR reviews | 1 approval, dismiss stale reviews, require code-owner review (`* @honeydipper/Honeydipper`) |
| force pushes | **disallowed** |
| branch deletion | disallowed |
| push restrictions | team `honeydipper`, users `tgreenhalgh`, `Charles546` |
| `enforce_admins` | `false` — **admin role is the release bot's bypass** |

Notes:

- **Checks are v3-real.** `test_coverage` and `Analyze (go)` are **not** required: the former only
  runs on `dev`/`main`, the latter's CodeQL workflow only triggers on `dev`/`main`. Requiring them
  would block every `v3` PR forever.
- **Release-bot bypass (verified, not assumed).** The `chore(release)` commits (`bec2249`, `89f3a56`,
  `0f95002`, `1d675d3`, and the `dev` ones) are authored by `semantic-release-bot` and pushed by the
  CircleCI `honeydipper` context token, whose owner is **`tgreenhalgh`** (confirmed as release
  author via the Releases API). That account holds the repo **`admin`** role, so with
  `enforce_admins: false` it bypasses the PR requirement to push `chore(release)` commits, while
  every non-admin contributor is subject to the full ruleset.
- **Residual weakness (documented, accepted for Phase 1).** `enforce_admins: false` also lets an
  admin force-push. That is why the `archive/*` tags exist, and why force-push is treated as
  forbidden by policy (§7) rather than only by API. Phase 2 may replace this with a repository
  **ruleset** carrying an explicit `bypass_actors` entry (`actor_type: "User"`, `actor_id: 5298300`)
  so that only the release identity bypasses and `non_fast_forward` binds everyone else.
- Reproduce the applied state:

```bash
curl -s -H "Authorization: Bearer $TOKEN" \
  https://api.github.com/repos/honeydipper/honeydipper/branches/v3/protection | jq .
```

---

## 12. Pre-release/post-release checklists

**Before tagging (any line)**

1. Target branch is the intended one (`main` = v4 product, `v3` = 3.x patch, `dev` = beta).
2. `.releaserc` on that branch knows the branch (no silent "unknown branch" fallback).
3. Required checks green; PR squashed/merged as the project prefers.
4. For `v3`: `approve-v3-release` approval granted by the release operator.

**After tagging**

1. GitHub release exists with the expected `prerelease` flag (§3).
2. `publish-image` succeeded; `release-evidence` shows `repo@sha256:...`.
3. `v3`/`dev`: confirm "Latest" still points at the newest non-prerelease v4 release (§7).
4. If the image is missing: run the **un-gated** recovery path with `release_tag=<tag>` (§5) — never
   move the tag.

---

## 13. Phase 2 follow-ups (tracked, explicitly non-blocking)

Nothing in this section gates Phase 1 acceptance or the merge of the Phase 1 documentation PR. These
items are recorded here **so they cannot be lost between phases**, and so Phase 2 inherits them as
requirements rather than rediscovering them.

### 13.1 Checks must *run* on `v3` before they can be *required*

`test_coverage` and `govulncheck` currently execute only on `dev`/`main` (§11). Making them required
checks on `v3` **before** widening their workflow/branch filters would reintroduce exactly the
deadlock Phase 1 avoided: a required check that never runs on `v3` leaves every `v3` PR permanently
pending. The order of operations is therefore fixed:

1. Change the filters so `test_coverage` and `govulncheck` actually execute on `v3`.
2. Confirm they report and pass on a real `v3` PR.
3. **Only then** add them to `required_status_checks`.

**Verify the exact CircleCI check-run names.** The required contexts in §11 are asserted from the API
as `ci/circleci: <job>` strings. Before adding any new context, read back what CircleCI actually
reports for that job (`GET /repos/{owner}/{repo}/commits/{sha}/status`, or the PR's `get_check_runs`).
A renamed or misspelled job produces a required check that never matches — which blocks merges just
as effectively as one that never runs.

### 13.2 Rehearse the release-bot bypass before the first real `v3` security release

The bypass in §11 is *attested* from earlier releases (`v3.10.2`–`v3.10.5`, `v4.0.0-dev.14/15`, all
authored by `tgreenhalgh`) plus that account's repo `admin` role. That is strong evidence, but it is
inference from releases published **before** the new protection existed — not a demonstration under
it. Before the first real post-Phase 2 `v3` security release, run a deliberate rehearsal (an approved
release-style push by the release identity onto a throwaway ref, or a full dry run of the `v3`
pipeline), so the attestation becomes a **demonstrated fact** with an observed run to point at.

### 13.3 Ruleset migration to bind `non_fast_forward` for everyone (deferred from Phase 1)

Phase 1 used classic branch protection with `enforce_admins: false` (§11), which unavoidably lets
*any* admin force-push — not only the release identity. Phase 2 should replace it with a repository
**ruleset** that:

- enables `non_fast_forward` and deletion protection for **everyone**, and
- grants a single explicit bypass — `{ "actor_type": "User", "actor_id": 5298300 }` (`tgreenhalgh`)
  — so only the release identity can bypass.

Re-verify that actor id at the time of the change. If the CircleCI release token rotates to a
different account, the `bypass_actors` entry must be updated **in the same change**, or every
`chore(release)` push will be rejected (`check_pr_title` / PR-required rules apply to it).

### 13.4 Tag protection for `archive/*` and release tags

Branch protection (§11) does **not** cover tags, and neither does `allow_force_pushes: false` on a
branch. Today the safety net (§8) is protected by policy alone. Phase 2 should add a **tag ruleset**
matching `archive/*` and the release tag pattern (`v*.*.*`) that blocks **deletion** and
**re-pointing** (non-fast-forward on tags). Until that exists, §7 rules 1–3 are enforced by
convention, and the annotated `archive/*` tags are the only durable recovery artifact — which is why
they are annotated tags *and* mirrored to branches.

### 13.5 The default branch after promotion — resolve the ambiguity in Phase 3

The repository default branch **today is `dev`** (confirmed by the GitHub API; the original plan
assumed `main`). This has a concrete operational consequence: **Renovate reads its config only from
the default branch**, so the canonical `renovate.json` must live on `dev` and must carry
`"baseBranches": ["dev", "main", "v3"]` to reach the other two lines (§10.1–§10.2). A canonical
config placed on `main` would be inert.

Phase 3 must therefore answer explicitly: **is the repo default branch switched to `main` at or after
promotion?**

- **If yes** — the Renovate config *read location* moves to `main`. `main`'s copy becomes
  authoritative and `dev`'s becomes version-controlled documentation. Update both in the same change
  and keep all three copies byte-identical (§10.4), or the maintenance lines silently lose policy.
- **If no** — `dev` stays the default branch, and this document must not conflate **"default
  branch"** (a GitHub repo setting controlling where Renovate/Dependabot read config and where new
  PRs base by default) with **"release channel"** (a `semantic-release` concept driven by
  `.releaserc`). `main` is the v4 *release channel*; it is **not** the default branch unless Phase 3
  changes it.

Either way, the decision and its Renovate consequence must be recorded in the Phase 3 promotion PR.
