# Releasing Honeydipper

`semantic-release` creates GitHub tags and releases from the `main` and `dev`
branches. CircleCI publishes Docker images from those immutable release tags.

## Registry credentials

The legacy Codefresh registry integration does not expose a reusable credential
or credential-name convention. The CircleCI release path introduces these
inputs in the restricted `honeydipper` context:

- `DOCKERHUB_ACCOUNT`: the Docker Hub organization name for an organization
  access token, or the service-account username for a personal access token;
- `DOCKERHUB_TOKEN`: an organization-owned, repository-scoped token that can push
  `honeydipper/honeydipper`.

Prefer a Docker Hub organization access token with image-push permission for
only `honeydipper/honeydipper`. If the organization cannot issue one, use a
dedicated service account rather than a personal maintainer account.

Do not store registry credentials in Git, pipeline parameters, build output, or
release artifacts.

## Automatic releases

The `publish_release_image` workflow accepts semantic-version tags beginning
with `v`. It checks out the exact tag commit, verifies that the corresponding
GitHub release exists, refuses to overwrite an existing Docker Hub tag, and
publishes a Linux/amd64 image without the leading `v` in its Docker tag.

The semantic-release commit message deliberately omits `[skip ci]`. CircleCI
suppresses tag pipelines when the tagged commit contains that directive. The
tag publisher uses an explicit tag filter because CircleCI ignores tag pushes
unless at least one workflow job opts into them.

For example, Git tag `v4.0.0-dev.1` publishes:

```text
honeydipper/honeydipper:4.0.0-dev.1
```

The workflow stores the source commit, platform, and immutable repository
digest in the `release-evidence` CircleCI artifact.

## Recovering a missing image

If a GitHub release exists but its Docker image does not, run the CircleCI
pipeline from `dev` and supply a string pipeline parameter named `release_tag`
containing the existing Git tag. The same validation and no-overwrite checks
apply. Never move or recreate the release tag to retry publication.

Publishing an image is not a deployment. Update deployment configuration in a
separate reviewed change, pin the immutable digest, and verify the live digest
and application behavior after rollout.

## Codefresh migration

Disable the legacy Codefresh release trigger after the CircleCI path is
accepted. Do not leave both publishers active for future tags because a late
Codefresh build could overwrite an image that CircleCI already published.
