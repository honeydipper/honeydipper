# Releasing Honeydipper v3

`semantic-release` creates GitHub tags and releases from the `main` branch
automatically, and from the `v3` branch only after an approved CircleCI
approval job. On `v3` the Docker image is published by the same pipeline that
creates the release, so **one** approval covers both.

For channels, the "Latest" policy, the manual repair runbook and the
`main`-is-promotion-only rule, see [Release Process](./release-process.md).

## Registry credentials

The CircleCI release path reads these inputs from the restricted `honeydipper`
context, the same ones used by the `dev` release pipeline:

- `DOCKERHUB_ACCOUNT`: the Docker Hub organization name for an organization
  access token, or the service-account username for a personal access token;
- `DOCKERHUB_TOKEN`: an organization-owned, repository-scoped token that can push
  `honeydipper/honeydipper`.

Do not store registry credentials in Git, pipeline parameters, build output, or
release artifacts.

## Cutting a v3 release

1. Merge the change into `v3` with a releasable commit type (`fix`, `feat`, or a
   breaking change). Other types, such as `chore`, `docs` and `ci`, do not
   produce a release.
2. Open the `release` workflow for the new commit in CircleCI and approve
   `approve-v3-release` — **once**. That single approval authorizes both the
   release and its image.
3. `semantic-release-v3` creates the tag, the GitHub release and the changelog
   commit, then records the published tag in a workspace file
   (`/tmp/release/release-tag.txt`).
4. `publish-image-release` requires `semantic-release-v3`, reads that file, and
   publishes the image from the pipeline that is already running. Nothing else
   needs approving.

If `semantic-release` finds no relevant commits — for example after re-approving
with no new commits — the recorded tag file is empty and the publish job halts
green with:

```text
no release created in this pipeline; nothing to publish
```

That is a no-op, not a failure, and it never reaches the
`Refuse to overwrite a published tag` guard.

## Why the release pipeline publishes its own image

`semantic-release` runs `prepare` (`@semantic-release/git`, which commits and
pushes the release commit **and** its tag) before `publish`
(`@semantic-release/github`, which creates the GitHub release). That push used
to start a second pipeline on `v3`; with CircleCI's "auto-cancel redundant
workflows" enabled, the newer pipeline cancelled the still-running release
pipeline. Where the cancellation lands is a race nothing controls: for `v3.10.6`
it landed *after* `publish`, one second after the tag push (tag `04:02:21`,
release `04:02:22`), so that release exists — and, being created from the
maintenance branch, it did not take "Latest". A slightly earlier landing loses
the release instead, and a later one leaves the run dead before it can hand
anything onward. Re-running cannot repair either outcome: the tag is already on
the remote, so semantic-release reports "no relevant changes".

Two changes close that window:

1. The release commit message carries `[skip ci]` in its **subject**. That
   suppresses the branch-push pipeline for the release commit — the pipeline
   that used to cancel the release.
2. The image is published by `publish-image-release` inside the release pipeline
   itself, so no second pipeline has to survive or be approved. The former
   tag-triggered `approve-v3-image` / `publish-image-tag` pair is gone; the two
   approvals are now one.

`[skip ci]` suppresses branch-push pipelines. It does **not** suppress tag
pipelines: a tag that points at a commit carrying that directive still produces
a `publish_release_image` pipeline. After this change that workflow has no
tag-triggered job left — it only holds the parameter-gated
`publish-image-recovery` — so such pipelines contain no jobs. They are harmless
debris, cleaned up separately, and they are exactly why the release no longer
depends on a tag pipeline for anything.

CircleCI also skips any pipeline whose head commit message contains the
skip-CI directive. When merging a pull request into `v3`, use a merge commit or
edit the squash message so that no commit message in the body contains that
text. Otherwise no pipeline runs and no release is offered for approval. If that
happens, use Trigger Pipeline in CircleCI on the `v3` branch.

## Image publishing

`publish-image-release` and the manual `publish-image` (recovery) job share one
CircleCI command (`publish-image-steps`). It resolves the release tag, checks
out the exact tag commit, verifies that the matching GitHub release exists,
refuses to overwrite an existing Docker Hub tag, builds a Linux/amd64 image,
pushes it, and records the source commit, platform and immutable repository
digest in the `release-evidence` artifact.

For example, Git tag `v3.10.5` publishes:

```text
honeydipper/honeydipper:3.10.5
```

The image tag drops the leading `v`. The 3.x line is stable only, so a tag with
a `-` in it (a prerelease) is rejected before anything is built.

The build deliberately passes `--build-arg HD_VERSION=<tag>`, matching `main`.
The `v3` Dockerfile does not consume that argument yet (it declares no
`ARG HD_VERSION` and stamps no version into the binary), so today the argument
is inert here — see the residuals in
[the release process](./release-process.md#5-recorded-residuals).

After the image is published and evidenced, one more step asserts that **no
`v3.*` release holds the repository's "Latest" release** — this run's tag and any
earlier 3.x release alike, so a standing deviation cannot slip through. It runs
last on purpose: a flag regression must go red without withholding an image that
is already published. While the deviation recorded in
[the release process](./release-process.md#2-channels-the-prerelease-flag-and-the-latest-policy)
stands, `v3` releases end red at this step *after* their image is published; that
red is the prompt to run the one-off repair, not a publishing failure.

## Recovering a missing image

If a GitHub release exists but its Docker image does not, run the CircleCI
pipeline from `v3` and supply a string pipeline parameter named `release_tag`
containing the existing Git tag. That path is deliberately un-gated — supplying
the parameter is the authorization — and it is described in
[the repair runbook](./release-process.md#3-manual-repair-runbook).

Never move or recreate the release tag to retry publication.

Publishing an image is not a deployment. Update deployment configuration in a
separate reviewed change, pin the immutable digest, and verify the live digest
and application behavior after rollout.

## Codefresh migration

Disable the legacy Codefresh release trigger, which runs
`.codefresh/honeydipper-release.yml`, once the CircleCI path is accepted. Do not
leave both publishers active for future tags because a late Codefresh build
could overwrite an image that CircleCI already published.
