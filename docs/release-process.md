# Honeydipper v3 Release Process

**Scope**: the release ceremony, channels, "Latest" policy, manual repair path and
branch rules for the **3.x maintenance line** (`v3`).
**Companion document**: [`docs/releasing.md`](./releasing.md) — image publishing
mechanics, registry credentials and the release race this process removes.

---

## 1. One approval authorizes the release **and** the image

The image is published by the release pipeline, so the two former approvals
(`approve-v3-release`, then `approve-v3-image` in a tag pipeline) collapse into
one.

| Step | CircleCI job | Branch | Gate |
|---|---|---|---|
| 1 | `approve-v3-release` (`type: approval`) | `v3` | **the** approval |
| 2 | `semantic-release-v3` | `v3` | `requires: [approve-v3-release]` |
| 3 | `publish-image-release` | `v3` | `requires: [semantic-release-v3]` |

How the tag crosses from step 2 to step 3, and why that is the only way:

- `semantic-release-v3` records `git rev-parse HEAD` **before** running
  semantic-release, keeps an **empty** `/tmp/release/release-tag.txt`, then after
  the run resolves the published tag with
  `build/ci/write-release-tag.sh … 'v3.*'` (highest tag matching `v3.*` that
  points at the new HEAD) and `persist_to_workspace`s that file.
- `publish-image-release` `attach_workspace`s it at `/tmp/release` **before**
  `checkout`, then reads it. `BASH_ENV`, job `environment` and step outputs are
  all job-local, so the workspace file is the only channel the tag can use.
- No release in the run (e.g. a re-approval with no new commits) ⇒ the file is
  empty ⇒ the publish job prints
  `no release created in this pipeline; nothing to publish`, halts green and
  publishes nothing. It never reaches the no-overwrite guard, so a no-op stays
  green instead of reporting a false "already published" failure.
- `circleci-agent step halt` skips the remaining steps but does **not** kill the
  running script, which is why every halt is immediately followed by `exit 0`.

The guard is unit-tested (`bash build/ci/write-release-tag.test.sh`) and
deliberately refuses to guess: if HEAD advanced but no `v3.*` tag points at it
(a concurrent push during the release), it exits non-zero and writes nothing.

---

## 2. Channels, the `prerelease` flag and the "Latest" policy

| Source branch | `.releaserc` entry | channel | GitHub `prerelease` | repo "Latest" |
|---|---|---|---|---|
| `main` (v4) | `"main"` | `latest` | `false` | **yes — the product line** |
| `v3` (3.x) | `{ "name": "v3", "range": "3.x", "channel": "3.x" }` | `3.x` | `false` (maintenance branch, not a prerelease channel) | **never** |

**Policy: a v3 release must never take the repository's "Latest" slot. `v4.0.0`
(and the v4 line after it) is the only "Latest" release.**

Three layers enforce it, in order of strength:

1. **The branch shape.** `@semantic-release/github@12.0.11` derives the API field
   `make_latest` from the matched branch
   (`lib/is-latest-release.js`: `type === "release" && main ? "true" : "false"`).
   `v3` matches as a *maintenance* branch, so the plugin sends
   `make_latest: "false"`. Only a `main`-style release branch gets `"true"`.
2. **The explicit `.releaserc` declaration** `["@semantic-release/github",
   { "makeLatest": false }]`. Kept deliberately as a v3-only declaration of
   intent; note that the pinned plugin does **not** implement an option of that
   name — `lib/resolve-config.js` destructures a fixed key set and silently drops
   unknown keys (verified: `verify()` produces the same error set with and
   without it). It therefore adds no behavior today and becomes load-bearing only
   if a future pin implements it. **It must never appear on `dev` or `main`**,
   where the release line is supposed to take "Latest".
3. **The CI assertion** — the step *"Assert no 3.x release holds GitHub Latest"*,
   guarded by `case "$RELEASE_TAG" in v3.*)` so the shared command stays
   branch-agnostic (the recovery job can also run from `main`). It fails when the
   `tag_name` of `/releases/latest` matches `v3.*` **at all**, not only when it
   equals the tag this run published. One rule, two states, both caught: the theft
   event (`latest == $RELEASE_TAG`) and the standing deviation below, where an
   *earlier* 3.x release holds the slot — a self-comparison would wave that
   through on every subsequent 3.x release. It runs *after* "Record immutable
   image evidence" on purpose: a `makeLatest` regression must go red **without**
   withholding an already-published image, which would strand the release with no
   image and force a recovery run.

**The pointer is live state, not a constant: read it, do not assume it.** The
policy above is the invariant; the value it currently resolves to is whatever
GitHub last computed, and GitHub rewrites it on every release create, delete and
re-flag. Read it with:

```bash
curl -s https://api.github.com/repos/honeydipper/honeydipper/releases/latest | jq -r .tag_name
```

A `v3.*` answer means the deviation below is (still) standing; a `v4.*` answer
means it is closed. Earlier revisions of this document asserted a concrete value
here, which decays the moment the pointer moves — so the assertion is gone and
only the invariant and the read command remain.

**The standing deviation: the pointer was moved to `v3.10.6` during the Phase 1
rehearsal — not by the `v3.10.6` release, and not by any layer above.** Three
facts, each checkable against the live API (these are history, so they do not
decay):

1. `v3.10.6` did **not** take "Latest" when it was published. The pinned plugin
   created that release from the `v3` maintenance branch, so it was sent with
   `make_latest: "false"` (layer 1), and its `published_at`
   (`2026-11-15T04:02:22Z`) matches the release pipeline log to the second
   (tag `04:02:21`, release `04:02:22`). It was therefore created by
   `@semantic-release/github`, **not by hand**, a second before the cancellation
   landed. Up to that point `/releases/latest` was still `v4.0.0`.
2. The pointer flipped to `v3.10.6` **later, inside the Phase 1 rehearsal
   window**, almost certainly as a side effect of creating and then deleting the
   rehearsal releases: when a release is deleted, GitHub recomputes the "Latest"
   pointer and can land on the newest non-prerelease release **regardless of the
   `make_latest` flag it was created with**. So the rehearsal mutated live
   release state at the pointer level. That is recorded here plainly instead of
   being attributed to the incident, and §5 residual 9 keeps future rehearsals
   off the production release list.
3. Nothing in the layers above put the pointer there, and nothing in them can
   undo it: layers 1 and 2 govern what is *sent* when a release is created, and
   layer 3 merely *observes* the pointer.

The repair is **deferred by the user** (§6 item 1) — it mutates live release
state, which the cleanup phase was told not to touch, so **it has not been
applied** and this document does not claim the pointer was repaired. Until it is,
a `v3` release publishes its image normally and then ends **red** in layer 3 — by
design, since the image must never be withheld over a flag. Re-point the pointer
explicitly — never by re-tagging or re-releasing:

```bash
id=$(curl -s https://api.github.com/repos/honeydipper/honeydipper/releases/tags/v4.0.0 | jq -r .id)
curl -X PATCH -H "Authorization: Bearer $TOKEN" -H "Accept: application/vnd.github+json" \
  https://api.github.com/repos/honeydipper/honeydipper/releases/$id \
  -d '{"make_latest":"true"}'
curl -s https://api.github.com/repos/honeydipper/honeydipper/releases/latest | jq -r .tag_name
# must now print a v4 tag -- verify it, do not assume v4.0.0 specifically
```

### Why `"main"` is still listed in `v3`'s `branches`

`"main"` looks vestigial — the `v3` pipeline never runs on `main`, and `main`'s
own `.releaserc` is what governs releases there. It cannot be removed:
`semantic-release@25.0.9` requires **at least one `release`-type branch**
(`lib/definitions/branches.js`, `release.branchesValidator` = `1..3`). With only
the `v3` maintenance entry, every run fails before analysing commits:

```text
ERELEASEBRANCHES The release branches are invalid in the `branches` configuration.
… Your configuration for the problematic branches is []
```

So the entry is load-bearing configuration, not decoration. The real hazards it
was blamed for — a `v3` tree leaking into `main`, or `v3` releasing on `main` —
are prevented by the branch filters and by the rule in §4, not by deleting it.

---

## 3. Manual repair runbook

`publish-image-recovery` is the only job left in the `publish_release_image`
workflow. It is un-gated on purpose: supplying `release_tag` is the operator's
authorization, and the job cannot create or mutate a release (it requires the
tag *and* the GitHub release to already exist, and refuses to overwrite a
published Docker Hub tag).

```bash
curl -X POST https://circleci.com/api/v2/project/gh/honeydipper/honeydipper/pipeline \
  -H "Circle-Token: $CIRCLECI_TOKEN" -H 'Content-Type: application/json' \
  -d '{"branch":"v3","parameters":{"release_tag":"v3.10.7"}}'
```

(The same run is available from the CircleCI UI: *Trigger Pipeline* → branch
`v3` → string parameter `release_tag`.)

Use it for exactly two states:

| Symptom | Cause | Action |
|---|---|---|
| **Release exists, image missing** | `publish-image-release` failed after `semantic-release-v3` succeeded | run the recovery above for that tag. It re-checks the tag, the release and the Docker Hub tag, then publishes and records `release-evidence`. |
| **Tag without a release** | the release was cancelled between the tag push and the `publish` step (`v3.10.6` itself won that race — see §2 — but nothing bounds where the cancellation lands) | create the GitHub release for the existing tag — keeping its `prerelease`/`make_latest` flags correct, and leaving §2's "Latest" slot alone — then run the recovery for that tag. Do **not** push a new tag and do **not** move the existing one. |

Interpreting the outcome (the recovery job runs the same
`publish-image-steps` command as the in-pipeline publisher, so it also runs the
layer-3 "Latest" assertion — a recovery run can therefore end red on that step
*after* publishing, which is the §2 repair prompt, not a publication failure):

| Job output | Meaning | Action |
|---|---|---|
| green + `release-evidence` artifact | published | verify the digest matches Docker Hub, then pin it in the deployment change |
| `Docker Hub tag already exists: …` | the guard worked; the image was already published | nothing to repair; if you expected a new image, the tag — not the image — is wrong |
| `Docker Hub tag lookup returned HTTP …` | Docker Hub hiccup or registry auth problem | retry; investigate only if persistent |
| `release tag does not exist: …` | wrong tag, or it was never pushed | use the real tag |
| `REGRESSION: a maintenance release holds GitHub Latest (…)` | layer 3: some `v3.*` release holds the repo "Latest" slot — this run's tag or an earlier one (§2) | the image **is** published; run the one-off PATCH in §2 to re-point "Latest" at `v4.0.0`, then confirm with `/releases/latest` |
| the `curl --fail …/releases/tags/…` tripwire fails | no GitHub release for that tag | create the release first (row 2 above) |
| `Release … does not have a GitHub release` / `404` | same as above | same |
| `v3 releases are stable only; refusing prerelease-shaped tag: …` | a `release_tag` containing `-` was supplied | 3.x publishes stable versions only; a prerelease image must come from the channel that produced it |

Never move or recreate a release tag to retry publication: the image is built
from the exact tagged commit, and a moved tag makes tag and image disagree
forever.

---

## 4. `main` is promotion-only

`main` is the v4 release channel, and during this rollout it is **promotion-only**:

- **No pull requests target `main`. No direct commits or pushes either.**
  Feature, fix and CI work lands on the line that owns it (`v3` for 3.x, `dev`
  for v4) and reaches `main` only by promotion.
- Promotion is a **merge commit, not a PR**: `dev` → `main`, performed once per
  promotion, so `main`'s history contains a single recorded promotion point
  instead of a PR trail.
- Until that promotion, `dev` and `main` must stay **byte-identical** for the two
  files this change-set family touches. Expected blobs:

  | File | Blob |
  |---|---|
  | `.circleci/config.yml` | `b81ebbb1f1edd21c65ba105e379ee75549b6cb4e` |
  | `.releaserc` | `2da501cf8be5a5d8aa19e3bb1e0dc5117d8f3172` |

  Drift check (run it before and after any change to either file, and again at
  promotion):

  ```bash
  git fetch -q origin dev main
  git rev-parse origin/dev:.circleci/config.yml origin/main:.circleci/config.yml
  git rev-parse origin/dev:.releaserc            origin/main:.releaserc
  # both lines must print the same blob twice, equal to the table above
  cmp -s <(git show origin/dev:.circleci/config.yml) <(git show origin/main:.circleci/config.yml)
  cmp -s <(git show origin/dev:.releaserc)           <(git show origin/main:.releaserc)
  ```

- `v3` is expected to **diverge** from `dev`/`main` here, by design: no `dev`
  branch entry, `makeLatest: false`, and the inline release image. Do not "fix"
  that divergence by copying `dev`'s files onto `v3`, and do not carry
  `makeLatest` onto `dev`/`main`.

---

## 5. Recorded residuals

Known and accepted at the time of this change:

1. **A tag pipeline is still created for every release, and it is empty.** The
   release commit's `[skip ci]` suppresses the *branch-push* pipeline, not the
   tag push. With `publish-image-tag` deleted, those pipelines contain no jobs.
   They are inert, nothing depends on them, and the class is a deferred residual
   tracked as §6 item 3.
2. **`makeLatest: false` is inert on the pinned plugin** (§2, layer 2). The
   effective controls are the maintenance-branch shape (`make_latest: "false"`)
   and the CI assertion. If a future pin implements the option, the declaration
   starts working with no further change; if a future pin *rejects* unknown
   options, the failure surfaces at the pin bump, which is rehearsed before
   merge.
3. **The `/releases/latest` pointer deviates from the policy**, as a side effect
   of the Phase 1 rehearsal's create/delete cycle (§2) — not because any release
   was created with `make_latest: "true"`. Read the current value with §2's
   command instead of assuming a tag; nothing in this repo mutates release state,
   the repair is the one-off PATCH in §2, it is **deferred by the user**
   (§6 item 1), and until it is applied layer 3 fails every `v3` release *after*
   publishing its image, on purpose.
4. **`HD_VERSION` is inert on `v3`.** The build passes
   `--build-arg HD_VERSION=$RELEASE_TAG` for parity with `main`, but `v3`'s
   `build/docker/Dockerfile` declares no `ARG HD_VERSION` and the `v3` Go tree
   has no `main._honeydipperVersion`, so the argument neither fails nor affects
   the image. Stamping the version into the 3.x image needs its own change
   (Dockerfile + `cmd/honeydipper`), and must not be smuggled into a CI-only
   change-set.
5. **Re-approving is not evidence that a release happened.** A second approval
   with no new commits ends in the green halt described in §1. Confirm the
   release by the tag and the `release-evidence` artifact, not by job colour.
6. **The `v3` release job can still be cancelled by a human**, and a cancelled
   `semantic-release-v3` can leave a pushed tag without a release. That state is
   repairable (§3) but not impossible; `[skip ci]` only removes the *automatic*
   cancellation window.
7. ~~**The legacy Codefresh release trigger** (`.codefresh/honeydipper-release.yml`)
   must still be disabled once the CircleCI path is accepted.~~ **Closed in
   Phase 5**: the tag-triggered release spec is deleted, so no tag can start a
   Codefresh image build from this repository any more. The Codefresh-side
   pipeline object is an operator action, tracked as §6 item 6 (§6.1).
8. ~~**`dev` and `main` still have the old shape.**~~ **Superseded** (kept for
   the record, because the pilot relationship is why this line's config looks the
   way it does): Phase 3 moved `dev` to the inline publisher and scoped the
   stable-only rejection to `v3.*` tags, Phase 4 promoted it to `main` and added
   `publish-image-release-main`, and Phase 5 (below) retires the legacy
   publishers. **Leave the "Phase 3 handoff" note inside `v3`'s
   `.circleci/config.yml` in place**: it remains accurate about why *this*
   command is unscoped, it is not a pending task on the other line, and that line
   no longer depends on anything this line must change. Do not port this line's
   assumptions back the other way either — `dev` legitimately resolves
   prerelease-shaped tags and `main` resolves stable ones.
9. **Rehearsals must not touch the production release list.** The Phase 1
   rehearsal created and deleted releases in `honeydipper/honeydipper` itself,
   which moved the repository's "Latest" pointer (§2). Future rehearsals of this
   pipeline must run against a scratch repository or fork, and any rehearsal that
   does touch the real release list must re-check `/releases/latest` afterwards.

---

## 6. Phase 5: cleanup, and the deferred risk register

Phase 5 is the **final** phase of this rollout, and this line is its `v3`-scoped half. It retires the
legacy publisher, re-verifies the debris sweeps at this head, and makes this document stop asserting
state that drifts. It **changes no release behaviour**: the job graph, `'v3.*'` as the tag glob, the
pinned semantic-release toolchain and `build/ci/write-release-tag.sh` (13 cases) are untouched, and no
tag or release was created, moved, re-flagged or deleted. `.circleci/config.yml` on this branch is
**not modified at all** by this phase.

The `dev`/`main` line carries its own copy of this register (`docs/release-process.md` there, §16),
because the two lines' configs have diverged by design (§4) and one PR cannot span both.

### 6.1 What the cleanup changed

| Change | Result |
|---|---|
| `.codefresh/honeydipper-release.yml` **deleted** | no in-repo definition of a tag-triggered `honeydipper/honeydipper` image build remains. A triggered Codefresh run on a `v3.*` tag is then *expected* to fail at config resolution, before its `dockerPushRelease` step — only a guarantee once the pipeline object no longer holds its own copy of the config (§6.2 item 6, `docs/releasing.md`) |
| Codefresh documented as **retired** | this line's docs no longer describe a Codefresh pipeline as a publisher of release images (§5 item 7) |
| `CIRCLE_TAG` sweep (re-verified at this head) | the publish path never read it here: at this head the name appeared **nowhere** in `v3`'s tree — not in `.circleci/config.yml`, not in `build/ci/*`, not in a doc. Phase 2 established this; Phase 5 re-verified it and found **nothing to fix**, so no edit was invented. This row is now the only place the name occurs on this line, and it exists to record that |
| deleted-job sweep (re-verified) | **no** executable reference to `publish-image-tag`, `approve-v3-image` or `approve-dev-image` here. The remaining mentions (§1, §5 item 1, `docs/releasing.md`) are prose that *records the deletion* |
| glob sweep (re-verified) | `v3` resolves with `'v3.*'` only. **No `v*-*` and no branch-scoped glob** exists in this line's config or docs outside the shared guard's own test fixtures (T8/T9 in `build/ci/write-release-tag.test.sh`, identical to `dev`'s copy, fixed at 13 cases and deliberately not touched) |
| guard blobs | `build/ci/write-release-tag.sh` = `e3d9710…`, `write-release-tag.test.sh` = `b02a4a7…` — byte-identical to `dev`/`main`, unchanged |
| GitHub "Latest" claims | §2 and §5 item 3 now state the invariant and the read command, not a tag. **Nothing here claims the pointer was repaired** |

### 6.2 Deferred residuals (risk register)

Every item is **open by decision, not by oversight**. "Deferred by user" means the operator chose to
leave release state alone during this rollout — do not "finish" one of these on your own, and do not
manufacture a commit to force a release that would resolve one incidentally.

1. **`PATCH {"make_latest":"true"}` on `v4.0.0` — DEFERRED BY USER.**
   *Owner*: repository admin / release operator. *Reason*: it mutates live release state, which this
   phase was explicitly told not to touch; read the current value with §2's command. **Consequences
   while open:** (a) on this line, layer 3 keeps failing every `v3` release *after* it publishes its
   image, which is the intended prompt rather than a publication failure; (b) on the stable channel,
   the only automated Latest check is `v3.*`-scoped, so it **skips silently** on a stable `v4.0.x`
   release from `main` — `case "$RELEASE_TAG" in v3.*)` matches nothing and the step exits green. The
   stable channel therefore has **no automated tripwire** on the pointer; the manual promotion check in
   the `dev`/`main` release process is the only control that remains.
2. **The stable-channel build/push/evidence path has never executed — DEFERRED BY USER.**
   *Owner*: release operator. Only the **halt** path has ever run on `main` (pipeline 4888: no
   releasing commit in the promotion range ⇒ `no release created in this pipeline; nothing to publish`
   ⇒ green halt). Everything after that halt — exact-tag checkout, GitHub-release existence tripwire,
   Docker Hub no-overwrite probe, `docker build`, `docker push`, digest evidence, credential logout —
   has run from the inline publisher on **no** channel. *Reason*: deferred at the user's request, to
   keep this phase free of any CI run that touches release state. That is a policy choice, **not** a
   prohibition the rehearsal would violate: the rehearsal is **non-mutating in the state the
   repository is actually in**. `v4.0.0`'s image already exists on Docker Hub, so a
   `release_tag=v4.0.0` recovery run (§3) red-ends at `Refuse to overwrite a published tag` — a
   read-only Docker Hub probe that runs *before* `Build release image` and `Publish release image` —
   and publishes nothing. It remains the recommended first de-risk before the first real stable
   release, for whoever owns this residual. (*If* the `4.0.0` tag were ever absent from Docker Hub,
   the same run would instead get past the probe and build and push — which is exactly what makes it
   a meaningful test of the path, and exactly why running it is the release operator's call, not this
   cleanup's.) *Until then*: verify the `release-evidence` artifact, never job colour.
3. **The empty-tag-pipelines class.** `[skip ci]` suppresses *branch-push* pipelines, not *tag* pushes,
   so a tag pushed onto a release commit still creates a CircleCI pipeline — and, with no tag-triggered
   job left in `publish_release_image`, that pipeline is **jobless** (§5 item 1). *Owner*: CircleCI
   project admin (a project setting, not a commit). *Reason*: removing the class changes no release
   behaviour and cannot be done from this repository.
4. **Historical `v3.10.1`–`v3.10.5` are flagged `prerelease: true`.** Confirmed from the Releases API:
   they are completed 3.x patches, so the flag is wrong for them, but it is **presentation only** and
   changes no tag, commit or image digest. *Owner*: release operator, optional. *Reason*: re-flagging
   is a release-state mutation — this phase **documents it and takes no action**, and none of those
   releases was touched.
5. **The `sort -V` tie-break favours a prerelease over its stable counterpart.** `sort -V` ranks
   `v4.0.1-dev.1` **above** `v4.0.1`, so a general `v*` glob would hand a prerelease tag to the
   publisher if a prerelease tag and a stable tag ever pointed at the same release commit. This line
   never uses that glob — `v3` resolves `'v3.*'` and rejects prerelease-shaped tags before building —
   so the hazard belongs to `dev`/`main`'s config. It is **unreachable under merge-commit-only
   promotion** (main's pre-release HEAD is always the promotion merge commit; the release commit is
   created *by* the release, so no earlier tag can already sit on it). *Owner*: release-process
   maintainers. *Reason*: hardening a state the current mechanics cannot produce needs its own guard
   test cases, and this line's guard is a fixed 13-case suite that must not change. **Revisit the
   tie-break only if promotion mechanics ever change (fast-forward promotion or manual stable
   tagging).** The same sentence sits in the `.circleci/config.yml` "Determine published tag" comment
   on `dev`/`main`, so a search finds all three.
6. **The Codefresh pipeline object and its Git-tag trigger still exist in the Codefresh UI.**
   *Owner*: Codefresh account admin. *Reason*: the pipeline and its trigger live outside this
   repository, so no commit can remove them. A tag push still starts a Codefresh run; the in-repo
   deletion removes the release spec, and such a run is **expected** to fail at config resolution
   before any `build`/`push` step.
   **That is an expectation, not a guarantee, and no safety argument may rest on it.** It holds only
   if the pipeline object references its spec by repository path. A *classic* Codefresh pipeline
   stores its config in the pipeline object's own database, so for that shape deleting the repo file
   changes nothing about what a triggered run executes — it would still build and push
   `honeydipper/honeydipper:$CF_RELEASE_TAG` from the tagged revision, racing the inline
   `publish-image-release`. And do not count a downstream failure as protection either: what would be
   relied on is a run dying *early*, not an image staying unbuilt.
   The item therefore closes in the Codefresh UI — delete the pipeline object, or at minimum disable
   its Git-tag trigger once you have confirmed which shape it is — and that must happen **before the
   next stable tag**.
