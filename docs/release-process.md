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

**Standing live deviation: `/releases/latest` resolves to `v3.10.6`, and it got
there during the Phase 1 rehearsal — not because of the `v3.10.6` release.**
Three facts, each checkable against the live API:

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

Repair it **before** the next `v3` release. Until then, that run publishes its
image normally and then ends **red** in layer 3 — by design, since the image must
never be withheld over a flag. Re-point the pointer explicitly — never by
re-tagging or re-releasing:

```bash
id=$(curl -s https://api.github.com/repos/honeydipper/honeydipper/releases/tags/v4.0.0 | jq -r .id)
curl -X PATCH -H "Authorization: Bearer $TOKEN" -H "Accept: application/vnd.github+json" \
  https://api.github.com/repos/honeydipper/honeydipper/releases/$id \
  -d '{"make_latest":"true"}'
curl -s https://api.github.com/repos/honeydipper/honeydipper/releases/latest | jq -r .tag_name  # v4.0.0
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
   They are debris to be cleaned up in a later phase; nothing depends on them.
2. **`makeLatest: false` is inert on the pinned plugin** (§2, layer 2). The
   effective controls are the maintenance-branch shape (`make_latest: "false"`)
   and the CI assertion. If a future pin implements the option, the declaration
   starts working with no further change; if a future pin *rejects* unknown
   options, the failure surfaces at the pin bump, which is rehearsed before
   merge.
3. **`/releases/latest` currently points at `v3.10.6`**, as a side effect of the
   Phase 1 rehearsal's create/delete cycle (§2) — not because any release was
   created with `make_latest: "true"`. This change mutates no release state; the
   repair is the one-off PATCH in §2, and until it is applied layer 3 fails every
   `v3` release *after* publishing its image, on purpose.
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
7. **The legacy Codefresh release trigger** (`.codefresh/honeydipper-release.yml`)
   must still be disabled once the CircleCI path is accepted; a late Codefresh
   build could overwrite an image CircleCI already published.
8. **`dev` and `main` still have the old shape.** Their tag-triggered
   `publish-image-tag` job and the separate `publish_release_image` publish path
   remain until the later phases of this rollout; the `v3` line is the pilot.
   Reusing the shared `publish-image-steps` command on `dev` additionally
   requires scoping the stable-only rejection to `v3.*` tags, because `dev`
   legitimately publishes `v4.0.0-dev.N`; see the Phase 3 handoff note in
   `.circleci/config.yml`.
9. **Rehearsals must not touch the production release list.** The Phase 1
   rehearsal created and deleted releases in `honeydipper/honeydipper` itself,
   which moved the repository's "Latest" pointer (§2). Future rehearsals of this
   pipeline must run against a scratch repository or fork, and any rehearsal that
   does touch the real release list must re-check `/releases/latest` afterwards.
