# Releasing Honeydipper v3

`semantic-release` creates GitHub tags and releases from the `main` branch
automatically, and from the `v3` branch only after an approved CircleCI
approval job. CircleCI publishes Docker images from those immutable release
tags.

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
   breaking change). Other types, such as `chore` and `docs`, do not produce a
   release.
2. Open the `release` workflow for the new commit in CircleCI and approve the
   `approve-v3-release` job.
3. `semantic-release-v3` creates the tag, the GitHub release, and the changelog
   commit. The tag then triggers the `publish_release_image` workflow.

The semantic-release commit message deliberately omits `[skip ci]`. CircleCI
suppresses tag pipelines when the tagged commit contains that directive. As a
consequence the release commit also starts a `release` workflow on `v3`, which
waits at `approve-v3-release`. Leave it unapproved, or approve it and
semantic-release will find nothing to release.

CircleCI also skips any pipeline whose head commit message contains the
skip-CI directive. When merging a pull request into `v3`, use a merge commit or
edit the squash message so that no commit message in the body contains that
text. Otherwise no pipeline runs and no release is offered for approval. If that
happens, use Trigger Pipeline in CircleCI on the `v3` branch.

## Image publishing

The `publish_release_image` workflow accepts semantic-version tags beginning
with `v`. It checks out the exact tag commit, verifies that the corresponding
GitHub release exists, refuses to overwrite an existing Docker Hub tag, and
publishes a Linux/amd64 image without the leading `v` in its Docker tag.

For example, Git tag `v3.10.5` publishes:

```text
honeydipper/honeydipper:3.10.5
```

The workflow stores the source commit, platform, and immutable repository
digest in the `release-evidence` CircleCI artifact.

## Recovering a missing image

If a GitHub release exists but its Docker image does not, run the CircleCI
pipeline from `v3` and supply a string pipeline parameter named `release_tag`
containing the existing Git tag. The same validation and no-overwrite checks
apply. Never move or recreate the release tag to retry publication.

Publishing an image is not a deployment. Update deployment configuration in a
separate reviewed change, pin the immutable digest, and verify the live digest
and application behavior after rollout.

## Codefresh migration

Disable the legacy Codefresh release trigger, which runs
`.codefresh/honeydipper-release.yml`, once the CircleCI path is accepted. Do not
leave both publishers active for future tags because a late Codefresh build
could overwrite an image that CircleCI already published.
