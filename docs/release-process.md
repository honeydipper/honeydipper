# Honeydipper Release Process

**Scope**: the branch model, release channels, the approval ceremony, the "Latest" policy, the manual
repair path, and automation caveats that govern releases after the v4.0.0 promotion.
**Companion documents**: [`docs/branch-divergence-report.md`](./branch-divergence-report.md) (Phase 1
evidence), [`docs/releasing.md`](./releasing.md) (image publishing mechanics, registry credentials and
the release race this process removes).

---

## 1. Branches and channels

### 1.1 Decision: `dev` is the permanent default branch

> **The `dev` branch remains the repository default branch permanently.** It is never switched to
> `main`, at promotion or afterwards.
>
> Consequently two concepts are kept permanently separate:
>
> - **Default branch (`dev`)** — the integration branch; the branch Renovate reads its configuration
>   from, the branch the dependency dashboard lives on, and the branch that new pull requests target
>   by default.
> - **Release channel (`main`)** — the v4 stable/"latest" release line, populated only by promotion
>   from `dev`.
>
> Dependency automation watches **`dev` and `v3` only**. `main` is deliberately not watched: `dev` is
> upstream of `main`, so dependency updates enter through `dev` and reach the release line when `dev`
> is promoted.

**Hard verification gate** — run this at every promotion. If it does not print `dev`, stop and
escalate; do not proceed with the promotion:

```console
$ gh api repos/honeydipper/honeydipper --jq .default_branch
dev
```

The decision above is only real while this command keeps printing `dev`. A silent change to
`default_branch` would relocate where Renovate reads its config (§10) and where new pull requests
base by default — including promotion PRs (§1.2).

| Branch | Role | Lifetime | Protection | Notes |
|---|---|---|---|---|
| `main` | **v4 release line** after promotion (Phase 4). Historically the dormant v3-era line. | permanent | protected: PR + 1 review + code-owner review + status checks + push restrictions | the v4 **release channel** (`latest`); `semantic-release` runs here with **no** approval gate. **Permanently not** the repo default branch — that is `dev` (§1.1). Not watched by Renovate (§1.3). **Promotion-only** during this rollout (§1.4) |
| `v3` | **3.x maintenance line** — security and patch releases only | permanent (through Phase 5) | protected since Phase 1: PR + 1 review + code-owner review + required checks + no force-push + push restrictions | **never reset, never force-pushed**; history is the sole input to 3.x version computation. Watched by Renovate, **patch-only** (§1.3) |
| `dev` | **integration / beta line** — all feature work lands here first | permanent | protected: PR + 1 review + code-owner review + strict status checks + push restrictions | **the repository default branch, permanently** (§1.1) — Renovate reads its config from here (§10); releases publish as `beta` prereleases **and publish their image from the release pipeline itself** (§4) |
| `v4` | **short-lived freeze line** — the exact tree being promoted into `main` | temporary, deleted after promotion | none (freeze ref, not a collaboration branch) | created from `dev` tip; see §7 |
| `archive/*` | **immutable safety net** (tags and their mirror branches) | permanent | never deleted, never moved | see §8 |

Downstream of decisions in Phase 1: **`main` is the ancestor of `v3` and `dev`**, so no
reconciliation may assume "main is newer". See the divergence report for per-area winners.

### 1.2 Promotion PRs must target `main` explicitly

GitHub bases a new pull request on the repository **default branch**, which is `dev` (§1.1). A
promotion PR opened without an explicit base therefore targets the *integration* branch — it looks
like a promotion but merges v4 content straight back into `dev` and promotes nothing.

**Promotion PRs must set `base: main` explicitly:**

```bash
gh pr create --base main --head <promotion-branch> \
  --title "chore(release): promote v4.0.0" --body "<promotion checklist>"
```

Confirm the base before asking for review:

```console
$ gh pr view <n> --json baseRefName --jq .baseRefName
main
```

If it prints `dev`, open the PR again against `main` — do not simply flip the base after review has
started, because the diff reviewers already approved was computed against the wrong target.

### 1.3 Dependency automation scope

Renovate is scoped to **`baseBranchPatterns: ["dev", "v3"]`** (formerly `baseBranches`; renamed
to match the current Renovate schema and avoid a deprecation-driven config migration).

**`dev` (default branch, integration).** All dependency, Docker-base-image and Go-toolchain updates
are raised here. `dev` is the upstream of the release line: `main` receives v4 code exclusively by
promotion from `dev`. Watching `main` as well would produce duplicate PRs for the same update on two
branches, with the `main` copy either conflicting at promotion or being silently overwritten by the
`dev` copy.

**`v3` (maintenance).** Watched because v3 never receives promotions — it only receives cherry-picks —
so it has no upstream to inherit updates from. Every update here must be a `fix:` (patch), not a
`feat:`.

**`main` (release channel).** Not watched by design. It is a projection of `dev`.

**Known trade-off and compensating control.** Because `main` is unwatched, a release line can lag
behind `dev` between promotions. A dependency fix required on the release line before the next
promotion is handled by the documented **release-line hotfix path**: cherry-pick the `fix(deps): …`
commit from `dev` onto `main` as a normal PR, which `semantic-release` turns into a `4.x.y` patch
release. Target: any security fix affecting a shipped 4.x image is either promoted or hotfixed to
`main` within **7 days** of being merged to `dev`; a promotion is triggered immediately if the fix is
critical.

Note: Dependabot/GitHub advisory alerts are computed from the **default branch's** dependency graph
— that is `dev`, not `main`. A vulnerable dependency in `main` therefore surfaces only while `main`'s
manifest still matches `dev`'s; precisely in the lag window this section is about, `main` drops out
of the advisory graph and the alert can go quiet. The intended release-line tripwire is a
`govulncheck` CI job on `main`; it now runs for `dev` pipelines (added after Phase 1's gap report) but
**not on `v3`**, where each branch's own config governs its own pipelines (§13.1). Until it runs on
the release line itself, the operative control is the **7-day hotfix SLA** above: an advisory
affecting a shipped 4.x image is cherry-picked to `main` (or promoted) within 7 days, and the hotfix
path is the remedy.

### Direction of content flow

```
feature work ──► dev ──► (v4 freeze) ──► main            # v4 product content
security fix ──► dev ──► cherry-pick -x ──► v3            # 3.x maintenance
toolchain / CI hardening ──► v3 (authoritative for 3.x)   # never copied back from main
```

### 1.4 `main` is promotion-only

`main` is the v4 **release channel**, and during this rollout it is
**promotion-only**:

- **No pull requests target `main`. No direct commits or pushes either.** Feature, fix and CI work
  lands on the line that owns it (`v3` for 3.x, `dev` for v4) and reaches `main` only by promotion.
- Promotion is a **merge commit, not a PR**: `dev` → `main`, performed once per promotion, so
  `main`'s history contains a single recorded promotion point instead of a PR trail.
- **`dev` and `main` are expected to diverge while Phase 3 lands.** Before the Phase 3 change-set they
  were byte-identical for the two files it touches; that change-set moves `dev` to the inline release
  publisher and **does not touch `main`** (it is promotion-only, so nothing may be edited there
  directly). The promotion in Phase 4 restores identity.

  | File | Before Phase 3 (`dev` == `main`) | After Phase 3 (`dev` only) |
  |---|---|---|
  | `.circleci/config.yml` | `b81ebbb1f1edd21c65ba105e379ee75549b6cb4e` | `68f85fedb6cee7e5c0f1076ab6575eef167b0f77` |
  | `.releaserc` | `2da501cf8be5a5d8aa19e3bb1e0dc5117d8f3172` | `48b3d777021ce4330572969b16762f400e9491f7` |

  Drift check — run it before and after any change to either file, and again at promotion:

  ```bash
  git fetch -q origin dev main
  git rev-parse origin/dev:.circleci/config.yml origin/main:.circleci/config.yml
  git rev-parse origin/dev:.releaserc            origin/main:.releaserc
  cmp -s <(git show origin/dev:.circleci/config.yml) <(git show origin/main:.circleci/config.yml)
  cmp -s <(git show origin/dev:.releaserc)           <(git show origin/main:.releaserc)
  ```

  Before the promotion the `cmp` lines must **differ** (that is the recorded drift) and `main`'s two
  blobs must still be the "before" column. After the promotion both `git rev-parse` lines must print
  the same blob twice, equal to the "after" column. A `main` blob that differs from the "before"
  column *before* the promotion means someone edited `main` directly — stop and investigate.

- `v3` is expected to **diverge** from `dev`/`main` here, by design: no `dev` branch entry,
  `makeLatest: false`, the `v3.*` tag glob and the unscoped stable-only rejection. Do not "fix" that
  divergence by copying `dev`'s files onto `v3`, and **do not carry `makeLatest` onto `dev`/`main`**
  (it is v3-only; §2 layer 2).

---

## 2. Release channels

| Source branch | `.releaserc` entry | npm dist-tag / channel | GitHub prerelease flag | repo "Latest" | Example |
|---|---|---|---|---|---|
| `main` | `"main"` | `latest` (default) | **false** — becomes the repo "Latest" release | **yes — the product line** | `v4.0.0` |
| `v3` | `{ name: "v3", range: "3.x", channel: "3.x" }` | `3.x` | **true — BY DESIGN** (derived from the non-default channel, **not** from a `prerelease` key) | **never** | `v3.11.0` |
| `dev` | `{ name: "dev", prerelease: true, channel: "beta" }` | `beta` | **true** | **never** | `v4.0.1-dev.1` |

Canonical configuration on `dev` (the file governs pipelines on `dev` only; each branch's own copy
governs its own pipelines):

```jsonc
{
  "branches": [
    "main",
    { "name": "v3", "range": "3.x", "channel": "3.x" },
    { "name": "dev", "prerelease": true, "channel": "beta" }
  ],
  "plugins": [
    "@semantic-release/commit-analyzer",
    "@semantic-release/release-notes-generator",
    "@semantic-release/changelog",
    ["@semantic-release/git", {
      "message": "chore(release): ${nextRelease.version} [skip ci]\n\n${nextRelease.notes}",
      "assets": ["CHANGELOG.md"]
    }],
    "@semantic-release/github"
  ]
}
```

**`dev` publishes prereleases, so `dev` never takes "Latest".** `prerelease: true` is what makes that
true, and it is also why `[skip ci]` belongs in the release commit subject (§4) and why the stable-only
rejection in the publish command is scoped to `v3.*` (§5).

**`makeLatest` does not appear on `dev`.** It is a `v3`-only declaration of intent (§2, layer 2 in the
v3 document): the pinned `@semantic-release/github@12.0.11` silently drops unknown options, and on
`dev` it would be wrong anyway — the v4 line is supposed to hold "Latest".

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
  declares a *prerelease channel*: it emits prerelease-suffixed versions (e.g. `3.10.6-v3.1`) rather
  than stable `3.x.y` patches. Putting it on a maintenance branch would quietly destroy 3.x
  versioning and produce versions no 3.x consumer expects. **The GitHub `prerelease` flag is a
  consequence of the non-default channel, not a configuration key.**

The 3.x artifacts remain fully available by tag, by the `3.x` channel, and as immutable images.

### Why `"main"` stays in `dev`'s `branches` list

`"main"` looks vestigial on `dev` — `dev`'s pipeline never runs on `main`, and `main`'s own
`.releaserc` governs releases there. It **cannot be removed**: `semantic-release@25.0.9` requires at
least one `release`-type branch. `dev` is a *prerelease* entry and `v3` is a *maintenance* entry, so
`"main"` is the only release-type entry left; deleting it leaves the validator with an empty release
branch list and **every run fails before analysing commits**:

```text
ERELEASEBRANCHES The release branches are invalid in the `branches` configuration.
… Your configuration for the problematic branches is []
```

Verified locally with the pinned toolchain: the same `.releaserc` succeeds once `main` exists on the
remote and fails with the message above when it does not. The entry is load-bearing configuration,
not decoration. The real hazards it was blamed for — a `v3` tree leaking into `main`, or `dev`
releasing on `main` — are prevented by the branch filters and by §1.4, not by deleting it.

---

## 3. Prerelease flag: historical bug vs. intended behavior

The `prerelease` flag has two distinct histories. Do not conflate them.

| Release(s) | Flag observed | Verdict | Action |
|---|---|---|---|
| `v3.10.0`, `v3.9.x`, `v3.8.0` (≤ 2026-02) | `prerelease: false` | correct at the time — `v3` was not yet a managed channel entry | none |
| **`v3.10.1`–`v3.10.5`** (2026-03-19 → 2026-10-28) | `prerelease: true` | **BUG.** They are full 3.x patch releases, yet they were published flagged `prerelease: true` and so never occupied the "Latest" slot. The *verdict* is certain from the Releases API; the *mechanism* is not (see below) | patched in Phase 2 for future releases by giving `v3` the maintenance-branch entry (`range` + `channel`, §2). Re-flagging these historical releases (`prerelease: false` is correct for them: they are completed 3.x patches) remains optional, presentation-only cleanup |
| **future `v3.11.x+`** releases (post-Phase 2) | `prerelease: true` | **INTENDED.** `v3` is a parallel maintenance channel; its releases are deliberately not "Latest" | none — this is the design |

**Root cause: recorded as unresolved, and it no longer governs future releases.** What is *observed*
(Releases API, no inference): `v3`'s former `.releaserc` listed `"v3"` as a bare string branch — an
entry carrying neither `range` nor `channel` — and the releases published from it were flagged
`prerelease: true` with the "Latest" pointer unassigned. Phase 2 did not need the mechanism in order
to fix the cause's neighbourhood: `v3` now carries the maintenance-branch entry (`range` + `channel`,
§2), which is what governs every future 3.x release, and the flag now follows the non-default channel
by design. The historical releases keep their flags — the flag is presentation only, and nobody has
re-flagged them in place.

**Invariant**: changing a release's `prerelease` flag never changes its tag, its commit, or its
image digest. The flag is presentation and "Latest"-selection only.

---

## 4. The approval ceremony: one approval for the release **and** the image

`dev` used to gate the version behind `approve-dev-release` and then publish the image from a
separate tag-triggered pipeline that had to survive on its own; `v3` additionally required a second
approval (`approve-v3-image`) for that pipeline. The image is now published by the release pipeline,
so there is exactly **one** approval:

| Step | CircleCI job | Branch | Gate |
|---|---|---|---|
| 1 | `approve-dev-release` (`type: approval`) | `dev` | **the** approval |
| 2 | `semantic-release-dev` | `dev` | `requires: [approve-dev-release]` |
| 3 | `publish-image-release` | `dev` | `requires: [semantic-release-dev]` |

The same shape is live on `v3` (its own `approve-v3-release`, `semantic-release-v3`,
`publish-image-release`); the old `approve-v3-image` gate was deleted with the tag-triggered publish
job. `main` has no approval gate at all, by design: it is the product line and is released by
promotion (§1.4).

How the tag crosses from step 2 to step 3, and why that is the only way:

- `semantic-release-dev` records `git rev-parse HEAD` **before** running semantic-release, keeps an
  **empty** `/tmp/release/release-tag.txt`, then after the run resolves the published tag with
  `build/ci/write-release-tag.sh … 'v*-*'` (highest tag matching `v*-*` that points at the new HEAD)
  and `persist_to_workspace`s that file. The glob is prerelease-shaped **on purpose**: `dev` publishes
  prereleases, and the stable `v4.0.0` tag already sits on `dev`'s own history, so a stable tag must
  never be selected for republication from the beta channel. The `main` invocation of the same job
  (and `main` after promotion) uses the general `v*` glob, because its releases *are* stable.
- `publish-image-release` `attach_workspace`s it at `/tmp/release` **before** `checkout`, then reads
  it. `BASH_ENV`, job `environment` and step outputs are all job-local, so the workspace file is the
  only channel the tag can use.
- No release in the run (e.g. a re-approval with no new commits) ⇒ the file is empty ⇒ the publish
  job prints `no release created in this pipeline; nothing to publish`, halts green and publishes
  nothing. It never reaches the no-overwrite guard, so a no-op stays green instead of reporting a
  false "already published" failure.
- `circleci-agent step halt` skips the remaining steps but does **not** kill the running script, which
  is why every halt is immediately followed by `exit 0` in the same `run:` block.
- The recovery job (`publish-image`) resolves the tag from the `release_tag` parameter only and
  therefore attaches **no** workspace: a parameter-triggered pipeline has no upstream workspace, and
  attaching one would fail the job.

The guard is unit-tested (`bash build/ci/write-release-tag.test.sh`, 13 cases) and deliberately
refuses to guess: if HEAD advanced but no matching tag points at it (a concurrent push during the
release), it exits non-zero and writes nothing.

**Ordering and why it is safe.** The approval precedes every mutation, so no tag exists until a human
approves. semantic-release then creates the tag **and** the GitHub release in one step and pushes the
`chore(release)` commit, and the image job publishes from the already-frozen tag commit (detached
checkout, GitHub-release existence check, Docker Hub no-overwrite probe). Approving means "release
this, then publish its image" — never "choose what to freeze".

---

## 5. Manual repair runbook

`publish-image-recovery` is the only job left in the `publish_release_image` workflow. It is un-gated
on purpose: supplying `release_tag` is the operator's authorization, and the job cannot create or
mutate a release (it requires the tag *and* the GitHub release to already exist, and refuses to
overwrite a published Docker Hub tag).

```yaml
# publish_release_image → publish-image-recovery (on dev)
filters: |
  pipeline.parameters.release_tag != ""
  and (pipeline.git.branch == "dev" or pipeline.git.branch == "main")
```

```bash
curl -X POST https://circleci.com/api/v2/project/gh/honeydipper/honeydipper/pipeline \
  -H "Circle-Token: $CIRCLECI_TOKEN" -H 'Content-Type: application/json' \
  -d '{"branch":"dev","parameters":{"release_tag":"v4.0.1-dev.1"}}'
```

(The same run is available from the CircleCI UI: *Trigger Pipeline* → branch `dev` → string parameter
`release_tag`.) The filter keeps `main` alongside `dev` because an operator may legitimately run the
pipeline from the release-line checkout; it does not import `v3`'s branch list, because `v3` supplies
its own parameter path from its own config.

Use it for exactly two states:

| Symptom | Cause | Action |
|---|---|---|
| **Release exists, image missing** | `publish-image-release` failed after `semantic-release-dev` succeeded | run the recovery above for that tag. It re-checks the tag, the release and the Docker Hub tag, then publishes and records `release-evidence`. |
| **Tag without a release** | the release pipeline was cancelled or failed between the tag push and `publish` (§4 explains why that window exists at all) | create the GitHub release for the existing tag — keeping its `prerelease` flag correct — then run the recovery for that tag. Do **not** push a new tag and do **not** move the existing one. |

Interpreting the outcome (the recovery job runs the same `publish-image-steps` command as the
in-pipeline publisher):

| Job output | Meaning | Action |
|---|---|---|
| green + `release-evidence` artifact | published | verify the digest matches Docker Hub, then pin it in the deployment change |
| `no release created in this pipeline; nothing to publish` | no tag was resolved (empty/absent workspace file **and** no `release_tag`) | supply `release_tag`, or approve the release pipeline instead of the publish job |
| `Docker Hub tag already exists: …` | the guard worked; the image was already published | nothing to repair; if you expected a new image, the tag — not the image — is wrong |
| `Docker Hub tag lookup returned HTTP …` | Docker Hub hiccup or registry auth problem | retry; investigate only if persistent |
| `release tag does not exist: …` | wrong tag, or it was never pushed | use the real tag |
| `the curl --fail …/releases/tags/…` tripwire fails | no GitHub release for that tag | create the release first (row 2 above) |
| `v3 releases are stable only; refusing prerelease-shaped tag: …` | a **`v3.*`** `release_tag` containing `-` was supplied | 3.x publishes stable versions only; `dev` prereleases (`v4.0.1-dev.1`) are accepted normally |
| `REGRESSION: a maintenance release holds GitHub Latest (…)` | the v3-scoped "Latest" assertion fired for a `v3.*` tag | the image **is** published; repair the pointer with the one-off PATCH described in the v3 release process |

Never move or recreate a release tag to retry publication: the image is built from the exact tagged
commit, and a moved tag makes tag and image disagree forever.

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
5. **Release commit subjects carry `[skip ci]`** (`chore(release): <version> [skip ci]`). That is what
   removes the redundant branch pipeline that used to cancel the release (§4). It does **not**
   suppress *tag* pipelines — a tag on such a commit still produces a tag pipeline — but
   `publish_release_image` now holds only the parameter-gated recovery job, so those pipelines contain
   no jobs. `chore(release)` still parses as non-releasing, so the release commit cannot retrigger a
   release.
6. **Nothing depends on a tag pipeline any more.** `publish-image-tag` and its tag filters are
   deleted; the tag-triggered path existed only because the image used to be published by a separate
   pipeline. Do not reintroduce a tag-triggered publish job: it would fire for every release *in
   addition to* the inline publish, behind its own approval, and race the inline one.
7. **`HD_VERSION` is live on `dev`.** The build passes `--build-arg HD_VERSION=<tag>`, and
   `build/docker/Dockerfile` declares `ARG HD_VERSION` and stamps `main._honeydipperVersion`, so the
   published `v4.0.x-dev.N` image reports its own version. Do not drop the build-arg when editing the
   publish command.

### The "Latest" safety-net step

`dev` releases are prereleases (`prerelease: true`, `beta` channel), and `v3` releases go to the
non-default `3.x` channel, so **neither can take "Latest"**. `main` — and only `main` — owns the
repository's "Latest" slot; it holds `v4.0.0` today.

`publish-image-steps` carries one assertion, "Assert no 3.x release holds GitHub Latest". It is
scoped with `case "$RELEASE_TAG" in v3.*)` because the command is shared with the recovery job, which
can run from either checkout. Consequences on `dev`:

- For any `v4.0.x-dev.N` (or stable `v4.0.x`) tag the step **skips** — it prints
  `not a 3.x release (…); skipping the Latest assertion` and exits green. The assertion is a no-op
  here, not a check that dev passed.
- For a `v3.*` tag (a recovery run driven from `dev`) it still fails on **any** `v3.*` latest tag, not
  only the tag being published, and it runs *after* "Record immutable image evidence" so a flag
  regression can never withhold an already-published image.

Every promotion therefore still ends with an explicit check:

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
| build/test image | `golang:1.27.4-alpine` | carries the CVE-2026-111943 and GO-2026-6859 fixes |
| lint image | `golang:1.26.10-alpine` + `golangci-lint@v1.64.8` | golangci-lint v1 cannot read go1.27 export data; the pin is **deliberate** |
| integration image | `golang:1.27.4-alpine`, `redis:7.0.11-alpine` | matches build image |
| image publishing | `publish-image-steps` (exact-tag checkout, no-overwrite, digest evidence), run inline by `publish-image-release` and by the recovery job | load-bearing for §7 |

Rules:

- Toolchain moves **forward only**, and only with a CVE/security or compatibility justification.
- `GOTOOLCHAIN=local` is enforced wherever Go runs for us: in every CI job that builds or tests,
  and in the Docker build stages (`build/docker/Dockerfile`, `Dockerfile-dev`). A build must
  never silently download a toolchain that differs from the pinned image.
- Lint is allowed to run on an older Go than the build/test toolchain; do not "fix" that by bumping
  the lint image past golangci-lint v1's export-data support.
- `v3` and `main`(v4) may legitimately run different Go versions during the 3.x/4.x overlap. That is
  expected, not drift to be reconciled.
- Renovate may propose Go/toolchain PRs on `dev` and `v3` only (§1.3, §10); the `ci/circleci: golangci-lint` check on
  `v3` is the gate that catches "bumped past the lint pin".

---

## 10. Dependency automation (Renovate)

### 10.1 Authoritative configuration

`renovate.json` is read **only from `dev`** (the repository default branch, §1.1). The copy below is
canonical and deliberately **comment-free**, so a byte-for-byte `cmp` across branches is trivial. All
explanation lives in §10.2, never in the JSON.

```json
{
  "$schema": "https://docs.renovatebot.com/renovate-schema.json",
  "extends": ["config:recommended"],
  "baseBranchPatterns": ["dev", "v3"],
  "dependencyDashboard": true,
  "dependencyDashboardApproval": false,
  "osvVulnerabilityAlerts": true,
  "vulnerabilityAlerts": {
    "enabled": true,
    "labels": ["security"],
    "prCreation": "immediate",
    "automerge": false
  },
  "packageRules": [
    { "matchDepTypes": ["require"], "semanticCommitType": "fix" },
    { "matchBaseBranches": ["v3"], "semanticCommitType": "fix" },
    { "matchBaseBranches": ["v3"], "matchDatasources": ["docker", "golang-version"], "semanticCommitType": "fix" }
  ],
  "prHourlyLimit": 0,
  "prConcurrentLimit": 10,
  "schedule": ["before 3:00am on the 3 day of the month"]
}
```

> **Status.** Phase 1 changes `renovate.json` on **no** branch. This is the Phase 2 target
> configuration, recorded here so it is reviewed once and then applied verbatim.

### 10.2 What each setting does

| Setting | Why |
|---|---|
| `baseBranchPatterns: ["dev", "v3"]` | Current schema's option name (formerly `baseBranches`; renamed to avoid a deprecation-driven config migration). `main` removed. `dev` is both the default branch and an explicit entry; `main` is excluded cleanly — because it is **not** the default branch, Renovate cannot implicitly include it. |
| `osvVulnerabilityAlerts: true` | OSV detection (experimental; **direct** dependencies only). Extra advisory coverage. |
| `vulnerabilityAlerts.enabled` + `prCreation: "immediate"` | GitHub-advisory detection including **transitive** dependencies. Immediacy is built in — Renovate ignores `schedule` for vulnerability alerts. The `security` label must already exist in the repo; if it does not, Renovate silently skips applying it (the PR is still opened, just unlabelled). |
| `dependencyDashboardApproval: false` | No checkbox gate before PRs are opened. The dashboard lives on `dev`, with `dev` and `v3` sections. |
| `matchBaseBranches: ["v3"]` rules | `v3`-only. On `v3` these force `fix` typing for every bump and for docker/golang-version datasources. On `dev`, **non-`require` datasources keep their natural typing** (`feat` allowed → a 4.x minor at the next promotion); `require`-type bumps do **not**, because packageRule 0 (`matchDepTypes: ["require"] → fix`) is repo-wide and applies to `dev` too. |
| `prHourlyLimit: 0`, `prConcurrentLimit: 10` | One **shared repo-wide budget**: `prConcurrentLimit` (and `prHourlyLimit`) applies to the whole repo, so `dev` and `v3` draw from the same 10-concurrent pool — not two 10-slot buckets. `prHourlyLimit: 0` disables the hourly cap. |
| `schedule` (monthly) | Routine updates on `dev` and `v3`. Security PRs bypass it by built-in behavior. |
| `govulncheck` on `main` + `v3` CI (**not yet present**) | Intended compensating control for the OSV **direct-dependencies-only** gap and the release-line advisory blind spot (§1.3). Phase 1 verified that no `govulncheck` job exists in CI yet; tracked in §13.1. |

### 10.3 Only `dev`'s copy is operative

1. **Config is read from the default branch only, and that branch is `dev`.** `renovate.json` on
   `main` and `v3` is **INERT**: Renovate ignores configuration on non-default branches **by
   default** — it reads a non-default branch's config only when `useBaseBranchConfig` is explicitly
   set, and that key is deliberately absent here. Those copies have **no** effect on behavior — they
   are kept for merge hygiene only (§10.4). The byte-identical `cmp` gate below would surface any
   edit to the authoritative file, including one that added `useBaseBranchConfig`.
2. **Watched branches are `dev` and `v3`, and nothing else** (§1.3). `main` is not watched by design.
3. **To change dependency automation, open a PR against `dev`.** A `renovate.json` change merged to
   `v3` or `main` changes nothing at all. This is the practical rule; the rest of this section is the
   explanation behind it.
4. **Enforce byte-identity mechanically**, not by review — `cmp` in CI or a pre-commit hook:

```bash
# dev's copy is the reference
git fetch -q origin dev v3 main
cmp -s <(git show origin/dev:renovate.json) <(git show origin/v3:renovate.json)
cmp -s <(git show origin/dev:renovate.json) <(git show origin/main:renovate.json)
```

### 10.4 Why the inert copies are still kept byte-identical

The `main` and `v3` copies cannot affect behavior (§10.3), yet they are kept byte-identical to
`dev`'s copy **for merge hygiene only**: a diverged copy manufactures spurious merge conflicts during
promotion and cherry-picks, and resolving such a conflict badly is how the file gets lost.

The "inert" property is a **default**, not a hard guarantee: Renovate reads config from the default
branch only unless `useBaseBranchConfig` is explicitly enabled. That key is absent from the
byte-compared canonical file, so enabling it would require editing `dev`'s copy — which the §10.3
`cmp` gate makes visible on every branch.

- Phase 3 promotion **cannot break `v3` automation** — at worst it overwrites `main`'s **inert** copy.
- Phase 3 promotion **can still lose the file** if the merge is resolved badly — which is why the
  byte-identical gate stays.

### 10.5 Operational caveats

1. **`semanticCommitType: "fix"` on `require` deps** means every dependency bump proposes a **patch
   release**. On `v3` that is intentional (security patches must be releasable), but it also means a
   Renovate PR is a release trigger once the approval gate is passed — sequence them with §4.
2. **Security PRs carry a `[security]` suffix** (e.g. `#835`, `#833`, `#837`) and are
   vulnerability-driven. They must not be batched into a routine monthly window; they travel the same
   PR path but are merged and released promptly on both `dev` and `v3`.
3. **`prConcurrentLimit` is a single repo-wide budget shared across all base branches (`dev` and
   `v3`), not per base branch** — a noisy `dev` batch can consume the budget and delay `v3` PRs.
   Renovate extracts and looks up dependencies per base branch but concatenates all base branches
   into one list and calls `updateRepo` once, so the limit is enforced once for the whole repo. Do
   not reintroduce a per-branch budget model.
4. **Renovate never touches release state.** It opens PRs; it does not tag, publish, or edit
   `.releaserc`. Version computation stays exclusively with `semantic-release` on the configured
   branches.
5. **Release-line latency is a known, accepted trade-off** with a documented compensating control —
   the hotfix path and 7-day target in §1.3.

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

**Before releasing on `dev`**

1. Target branch is the intended one (`main` = v4 product, `v3` = 3.x patch, `dev` = beta).
2. `.releaserc` on that branch knows the branch (no silent "unknown branch" fallback).
3. Required checks green; PR squashed/merged as the project prefers.
4. The merge commit / squash message contains **no** skip-CI directive (otherwise no pipeline runs —
   §4, `docs/releasing.md`).
5. `approve-dev-release` approval granted by the release operator — **once**, for release *and* image.
6. For a **promotion** PR: the base is `main`, set explicitly with `--base main` (§1.2) — GitHub
   would otherwise default it to `dev`, the integration branch.

**After releasing on `dev`**

1. GitHub release exists as a **prerelease** with the `v4.0.x-dev.N` tag (§2).
2. `publish-image-release` succeeded in the **same** pipeline; `release-evidence` shows
   `repo@sha256:...` matching Docker Hub.
3. No second `publish_release_image` job ran for that release, and no `publish-image-tag` exists any
   more (§7 rule 6).
4. Confirm "Latest" still points at `v4.0.0` (§7) — `dev` releases never move it.
5. If the image is missing: run the **un-gated** recovery path with `release_tag=<tag>` (§5) — never
   move the tag.
6. Confirm the image reports the right version: `docker run --rm honeydipper/honeydipper:<tag> --version`.

---

## 13. Phase 2 follow-ups (tracked, explicitly non-blocking)

Nothing in this section gates Phase 1 acceptance or the merge of the Phase 1 documentation PR. These
items are recorded here **so they cannot be lost between phases**, and so Phase 2 inherits them as
requirements rather than rediscovering them.

### 13.1 Checks must *run* on `v3` before they can be *required*

`test_coverage` executes only on `dev`/`main` (§11), and **no `govulncheck` job runs on `v3`** (Phase 1
verified that nothing in `v3`'s CI references it; a `govulncheck` job has since been added on `dev`
and runs for `dev` pipelines only — each branch's own config governs its own pipelines). Making either a required check on `v3` **before** it actually
runs would reintroduce exactly the deadlock Phase 1 avoided: a required check that never runs on
`v3` leaves every `v3` PR permanently pending. The order of operations is therefore fixed:

1. Add a `govulncheck` job (running on `main` and `v3`) and change the `test_coverage` filter so both
   actually execute on `v3`.
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

### 13.5 The default branch after promotion — **RESOLVED** (closed, not a follow-up)

**Decision: `dev` remains the repository default branch permanently.** It is never switched to
`main`, at promotion or afterwards; default branch and release channel are kept permanently separate
(§1.1). Dependency automation therefore watches **`dev` and `v3` only**, and the canonical
`renovate.json` uses `baseBranchPatterns: ["dev", "v3"]` with `main` deliberately excluded (§1.3, §10).

Consequences for Phase 3 — none of which require a further decision:

- The Renovate config **read location does not move** at promotion. It stays `dev`.
- Promotion does **not** re-point `default_branch`. `main` stays a release channel, never a default
  branch.
- Because new PRs base on the default branch, promotion PRs must pass `--base main` explicitly
  (§1.2). That is the one operational consequence of this decision.
- The §1.1 verification gate is re-run at promotion, so the setting cannot drift silently.

This item is **closed**. It is retained only so the reasoning is not re-litigated in Phase 3.

---

## 14. Recorded residuals (Phase 3, `dev`)

Known and accepted when the inline publisher landed on `dev`:

1. **A tag pipeline is still created for every release, and it is empty.** The release commit's
   `[skip ci]` suppresses the *branch-push* pipeline, not the tag push. With `publish-image-tag`
   deleted, those pipelines contain no jobs. They are debris to be cleaned up in the final phase;
   nothing depends on them, and they no longer sit behind an approval gate (§7 rule 6).
2. **`dev` and `main` diverge until the promotion.** `main` is promotion-only, so this change-set
   touches `dev` only; `main`'s blobs stay at the "before" column of the §1.4 table until the
   promotion merges `dev` into `main`. Do not "sync" either file by hand in between.
3. **`dev` publishes prereleases, so it never becomes GitHub "Latest".** The v3-scoped assertion in
   `publish-image-steps` is a no-op for every `dev` release — it skips, which is the design, not a
   gap: `dev` releases must not hold the slot (`v4.0.0` does, see §7).
4. **The tag glob is branch-scoped, not a literal `v*-*`.** `dev` resolves prerelease-shaped tags
   only (`v*-*`); the same job definition serves `semantic-release-main`, and after the promotion this
   very file *is* `main`'s config, where releases are stable (`v4.0.1`). Hard-coding `v*-*` for both
   would fail every post-promotion stable release at "Determine published tag" — after its tag and
   GitHub release already existed. The branch scope keeps `dev` safe (a stable tag can never be
   selected) without breaking `main`.
5. **Re-approving is not evidence that a release happened.** A second approval with no new commits
   ends in the green halt described in §4. Confirm the release by the tag and the `release-evidence`
   artifact, not by job colour.
6. **A human can still cancel the release.** A cancelled `semantic-release-dev` can leave a pushed tag
   without a release. That state is repairable (§5) but not impossible; `[skip ci]` only removes the
   *automatic* cancellation window.
7. **The legacy Codefresh release trigger** (`.codefresh/honeydipper-release.yml`) must still be
   disabled once the CircleCI path is accepted; a late Codefresh build could overwrite an image
   CircleCI already published.
8. **`CIRCLE_TAG` is no longer read** by the publish path: the tag arrives from the `release_tag`
   parameter or the workspace file only. Remove the leftover references, if any, in the final cleanup
   phase.
9. **`makeLatest` must never appear on `dev` or `main`.** It is a `v3`-only declaration of intent and,
   on the pinned plugin, inert (`v3` documents it as layer 2); adding it here would be a policy
   statement that the v4 line must not hold "Latest".
10. **`HD_VERSION` is live on `dev`** (unlike `v3`): `build/docker/Dockerfile` declares
    `ARG HD_VERSION` and stamps `main._honeydipperVersion`, so the `v4.0.x-dev.N` image reports its
    own version. The build-arg is meaningful here, not decorative.
11. **Rehearsals must not touch the production release list.** Use a scratch repository or fork; any
    rehearsal that does touch the real release list must re-check `/releases/latest` afterwards
    (creating and deleting releases can move the pointer).
