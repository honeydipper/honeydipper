# Installing Honeydipper v4

<!-- toc -->

- [Prerequisites](#prerequisites)
- [Step 1: Prepare your bootstrap repo](#step-1-prepare-your-bootstrap-repo)
- [Step 2: Bootstrap your daemon](#step-2-bootstrap-your-daemon)
  * [Running in Kubernetes](#running-in-kubernetes)
    + [Using helm charts](#using-helm-charts)
    + [Create your own manifest file](#create-your-own-manifest-file)
  * [Running as a docker container](#running-as-a-docker-container)
  * [Building from source](#building-from-source)
- [Step 3: Hacking away](#step-3-hacking-away)
- [Environment Variables Reference](#environment-variables-reference)

<!-- tocstop -->

> **⚠ v4 Branch Notice**
>
> This document covers the **v4** release of Honeydipper, which is under active development.
> The source code is on the `v4` branch (module path: `github.com/honeydipper/honeydipper/v4`).
> Docker images use the `v4` tag. Building from source requires checking out the `v4` branch.
> See the [Release Notes](../../CHANGELOG.md) for the latest changes.

## Prerequisites

* A running Redis server (required for event bus, caching, and session storage)
  * The `redisqueue` driver is used as the default event bus
  * The `redispubsub` driver (aliased as `api-broadcast` in daemon config) handles internal broadcasting
* Git (required for cloning config repositories)

## Step 1: Prepare your bootstrap repo

As described in the [architecture/design document](../README.md), Honeydipper loads configurations directly from one or many git repos. You can put the repo locally on the machine or pod where Honeydipper is running, or you can put the repos in GitHub, Bitbucket or Gitlab etc, or even mix them together. Make sure your configuration repo is private, and protected from unauthorized changes. Although, you can store all the sensitive information in encrypted form in the repo, you don't want this to become a target.

Inside your repo, you will need a `init.yaml` file. It is the main entrypoint that Honeydipper daemon seeks in each repo. See the [Configuration Guide](./configuration.md) for detailed explanation. Below is an example of the minimum required content to get the daemon bootstrapped:

```yaml
# init.yaml
---
repos:
  - repo: https://github.com/honeydipper/honeydipper-config-essentials.git

drivers:
  redisqueue:
    connection:
      Addr: <redis server IP>:<port>
      # uncomment below line if your redis server requires authentication
      # Password: xxxxxxxx
  redispubsub:
    connection:
      Addr: <redis server IP>:<port>
      # uncomment below line if your redis server requires authentication
      # Password: xxxxxxxx
```

## Step 2: Bootstrap your daemon

### Running in Kubernetes

This is the recommended way of running Honeydipper. Not only is this the easiest way to get Honeydipper started, it also enables Honeydipper to take advantage of the power of Kubernetes.

#### Using helm charts

To pass the information about the bootstrap config repo to Honeydipper daemon, the recommended way is to put all the information in a values yaml file rather than use `--set` options during `helm install`. For example:

```yaml
# values.yaml
---
daemon:
  env:
    - name: REPO
      value: git@github.com/example/honeydipper-config.git
    - name: DIPPER_SSH_KEY
      valueFrom:
        secretKeyRef:
          name: example-secret
          key: id_rsa
```

Note that, we need to provide an ssh key for Honeydipper daemon to be able to fetch the private repo using ssh protocol. Make sure that the key exists in your cluster as a `secret`.

Once the values file is prepared, you can run the `helm install` command like below.

```bash
helm install --values values.yaml orchestrator incubator/honeydipper
```

If you want to use a specific version of the chart, use `--version` to specify the chart version. The latest chart version is `0.1.10`. By default, the chart uses the latest stable version of the Honeydipper daemon docker image (tagged `v4`). You can override the image version by specifying `--set daemon.image.tag=<version>` in your `helm install` command.

---

Currently, the chart is available from the [honeydipper-charts repo](https://github.com/honeydipper/honeydipper-charts) and from [Artifact Hub](https://artifacthub.io/packages/helm/honeydipper/honeydipper). You may also choose to customize and build the chart by yourself following the steps below.

```bash
git clone git@github.com:honeydipper/honeydipper-charts.git
cd honeydipper-charts
helm package honeydipper
```

You should see the chart file `honeydipper-x.y.z.tgz` in your current directory.

---

#### Create your own manifest file

You can use the below manifest file as a template to create your own. Note that the basic information needed, besides the docker image for Honeydipper daemon, is the same: `REPO` and `DIPPER_SSH_KEY`.

```yaml
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: honeydipper-daemon
  labels:
    app: honeydipper-daemon
spec:
  template:
    metadata:
      name: honeydipper-daemon
    spec:
      containers:
        - name: honeydipper-daemon
          image: honeydipper/honeydipper:v4
          imagePullPolicy: Always
          env:
            - name: REPO
              value: git@github.com/example/honeydipper-config.git
            - name: DIPPER_SSH_KEY
              valueFrom:
                secretKeyRef:
                  name: example-secret
                  key: id_rsa
```

For the webhook driver, you will need to create a service.

```yaml
apiVersion: v1
kind: Service
metadata:
  name: honeydipper-webhook
spec:
  type: LoadBalancer
  ports:
  - name: webhook
    targetPort: 8080
    port: 8080
  selector:
    app: honeydipper-daemon
```

### Running as a docker container

```bash
docker run -it -e 'REPO=git@github.com/example/honeydipper-config.git' -e "DIPPER_SSH_KEY=$(cat ~/.ssh/id_rsa)" honeydipper/honeydipper:v4
```

Replace the repo URL with your own, and specify the private key for accessing the private repo remotely. You may replace the value of `DIPPER_SSH_KEY` with a deploy key for your config repo.

### Building from source

#### Prerequisites:

* **Go >= 1.25** — required (module path is `github.com/honeydipper/honeydipper/v4`)
* Git
* POSIX compliant shell

> **Note:** Honeydipper v4 uses [Go modules](https://go.dev/blog/using-go-modules). The `GO111MODULE` environment variable does not need to be set; module mode is automatic in Go 1.13+.

#### Instructions

```bash
git clone https://github.com/honeydipper/honeydipper.git
cd honeydipper
git checkout v4
go install -v ./...
REPO=git@github.com/example/honeydipper-config.git DIPPER_SSH_KEY="$(cat ~/.ssh/id_rsa)" honeydipper
```

You don't have to specify `DIPPER_SSH_KEY` if the key is used by your ssh client by default.

Alternatively, you can follow the [developer setup guide](./howtos/setup_local.md) to download, build, and run with a local development environment.

## Step 3: Hacking away

That's it &mdash; your Honeydipper daemon is bootstrapped. You can start to configure it to suit your needs. The daemon pulls your config repos every 60 seconds, and will reload when changes are detected. See the [Honeydipper Guides](./README.md) for more documents, including a way to set up GitHub push event-driven reload.

## Environment Variables Reference

Honeydipper uses several environment variables for configuration. Below is a summary of the most important ones:

| Environment Variable | Description |
|---|---|
| `REPO` | **(Required)** Bootstrap Git repository URL |
| `BRANCH` | Bootstrap branch (default: repository default branch) |
| `BOOTSTRAP_PATH` | Path within the repo to the bootstrap file |
| `BOOTSTRAP_FILE` | Custom init file name (default: `init.yaml`) |
| `DIPPER_SSH_KEY` | SSH private key for Git authentication |
| `DIPPER_SSH_KEYFILE` | Path to SSH private key file |
| `SSH_AUTH_SOCK` | SSH agent socket path (alternative to `DIPPER_SSH_KEY`) |
| `HD_JWT_SIGNING_KEY` | JWT signing key for the HTTP API |
| `HONEYDIPPER_DRIVERS_BUILTIN` | Search path for built-in driver binaries |
| `HONEYDIPPER_DRIVERS_CACHE` | Cache directory for remote driver downloads |
| `LOCALREDIS` | Set to `1` to use local Redis for development/testing |

Additional environment variables are supported for GitHub App authentication, HTTP basic auth for Git, repo path overrides, and more. For the complete list of environment variables, see the [Honeydipper Context](../../HONEYDIPPER_CONTEXT.md) section on Environment Variables.
