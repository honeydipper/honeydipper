# Releasing Honeydipper

`semantic-release` creates GitHub tags and releases from the `main` branch
automatically, and from the `dev` branch only after an approved CircleCI
approval job. On `dev` the Docker image is published by the same pipeline that
creates the release, so **one** approval covers both.

For the branch model, the release channels, the approval ceremony, the "Latest"
policy, the manual repair runbook and the `main`-is-promotion-only rule, see
[Release Process](./release-process.md).

## Registry credentials

The CircleCI release path reads these inputs from the restricted `honeydipper`
context:

- `DOCKERHUB_ACCOUNT`: the Docker Hub organization name for an organization
  access token, or the service-account username for a personal access token;
- `DOCKERHUB_TOKEN`: an organization-owned, repository-scoped token that can push
  `honeydipper/honeydipper`.

The legacy Codefresh registry integration does not expose a reusable credential
or credential-name convention, so the CircleCI path introduces the two above.
Prefer a Docker Hub organization access token with image-push permission for
only `honeydipper/honeydipper`. If the organization cannot issue one, use a
dedicated service account rather than a personal maintainer account.

Do not store registry credentials in Git, pipeline parameters, build output, or
release artifacts.

## Cutting a dev release

1. Merge the change into `dev` with a releasable commit type (`fix`, `feat`, or a
   breaking change). Other types, such as `chore`, `docs` and `ci`, do not
   produce a release.
2. Open the `release` workflow for the new commit in CircleCI and approve
   `approve-dev-release` — **once**. That single approval authorizes both the
   release and its image.
3. `semantic-release-dev` creates the tag, the GitHub release and the changelog
   commit, then records the published tag in a workspace file
   (`/tmp/release/release-tag.txt`).
4. `publish-image-release` requires `semantic-release-dev`, reads that file, and
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

## Cutting a stable release (promotion)

`main` has **no approval gate**: the promotion itself is the trigger. Push the `--no-ff` merge commit
(`chore(release): promote dev to main`, no skip-CI token anywhere in its message — see
[Release Process §1.4](./release-process.md#14-main-is-promotion-only)) and the `release` workflow runs

1. `semantic-release-main` — creates the tag, the GitHub release and the changelog commit;
2. `publish-image-release-main` — `requires: [semantic-release-main]`, publishes the image inline from
   the workspace file the release job wrote, so a promotion never leaves a GitHub release with no
   image.

Both steps are the same jobs (and the same command) `dev` uses; only the branch filter and the
`requires:` target differ. A promotion whose carried commits contain no releasing type (only
`ci`/`chore`/`docs`) releases nothing: `semantic-release-main` reports no relevant changes and the
publish job halts green with `no release created in this pipeline; nothing to publish`. Confirm that
with a dry run of the merged tree instead of adding a commit to force a release.

## Why the release pipeline publishes its own image

`semantic-release` runs `prepare` (`@semantic-release/git`, which commits and
pushes the release commit **and** its tag) before `publish`
(`@semantic-release/github`, which creates the GitHub release). That push used
to start a second pipeline on `dev`; with CircleCI's "auto-cancel redundant
workflows" enabled, the newer pipeline cancelled the still-running release
pipeline. Where the cancellation lands is a race nothing controls: landing
before `publish` loses the release, landing after `prepare` leaves the run dead
before it can hand anything onward. Re-running cannot repair either outcome: the
tag is already on the remote, so semantic-release reports "no relevant changes".

Two changes close that window:

1. The release commit message carries `[skip ci]` in its **subject**. That
   suppresses the branch-push pipeline for the release commit — the pipeline that
   used to cancel the release.
2. The image is published by `publish-image-release` inside the release pipeline
   itself, so no second pipeline has to survive or be approved. The former
   tag-triggered `publish-image-tag` job is gone, and `approve-dev-release` is now
   the only gate.

`[skip ci]` suppresses branch-push pipelines. It does **not** suppress tag
pipelines: a tag that points at a commit carrying that directive still produces a
`publish_release_image` pipeline. After this change that workflow has no
tag-triggered job left — it only holds the parameter-gated
`publish-image-recovery` — so such pipelines contain no jobs. They are harmless
debris, cleaned up separately, and they are exactly why the release no longer
depends on a tag pipeline for anything.

CircleCI also skips any pipeline whose head commit message contains the
skip-CI directive. When merging a pull request into `dev`, use a merge commit or
edit the squash message so that no commit message in the body contains that
text. Otherwise no pipeline runs and no release is offered for approval. If that
happens, use Trigger Pipeline in CircleCI on the `dev` branch.

## Image publishing

`publish-image-release` and the manual `publish-image` (recovery) job share one
CircleCI command (`publish-image-steps`). It resolves the release tag, checks
out the exact tag commit, verifies that the matching GitHub release exists,
refuses to overwrite an existing Docker Hub tag, builds a Linux/amd64 image,
pushes it, and records the source commit, platform and immutable repository
digest in the `release-evidence` artifact.

For example, once `v4.0.0` is the released stable version, the next `dev`
prerelease is `v4.0.1-dev.1` and publishes:

```text
honeydipper/honeydipper:4.0.1-dev.1
```

The image tag drops the leading `v`. `dev` publishes prereleases by design, so a
dash in the tag is normal here; the "stable only" rejection in the shared command
is scoped to `v3.*` tags, the channel it was written for, and never fires for
`dev`'s own releases.

The build deliberately passes `--build-arg HD_VERSION=<tag>`, and `dev`'s
`build/docker/Dockerfile` does consume it: it declares `ARG HD_VERSION` and
stamps `main._honeydipperVersion` through `-ldflags`. The image for
`v4.0.1-dev.1` therefore reports that version, so `HD_VERSION` is live on `dev`
(and `main` after promotion), unlike the 3.x line.

Because `dev` releases are prereleases, they never take the repository's
"Latest" slot; the "Assert no 3.x release holds GitHub Latest" step in the
shared command is tag-scoped (`v3.*`) and is a no-op for every `dev` release.

## Recovering a missing image

If a GitHub release exists but its Docker image does not, run the CircleCI
pipeline from `dev` and supply a string pipeline parameter named `release_tag`
containing the existing Git tag. That path is deliberately un-gated — supplying
the parameter is the authorization — and it is described in
[the repair runbook](./release-process.md#5-manual-repair-runbook).

The same validation and no-overwrite checks apply, so a recovery run for an
already-published tag ends red at `Refuse to overwrite a published tag`. That is
the guard working, not a publication failure. Never move or recreate the release
tag to retry publication.

Publishing an image is not a deployment. Update deployment configuration in a
separate reviewed change, pin the immutable digest, and verify the live digest
and application behavior after rollout.

## Codefresh migration

Disable the legacy Codefresh release trigger, which runs
`.codefresh/honeydipper-release.yml`, once the CircleCI path is accepted. Do not
leave both publishers active for future tags because a late Codefresh build
could overwrite an image that CircleCI already published.

The trigger is retired in the cleanup phase of this rollout; until then, do not
tag a release while both publishers are enabled.
