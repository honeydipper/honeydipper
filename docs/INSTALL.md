# Instailling Honeydipper

<!-- toc -->

- [Prerequisites](#prerequisites)
- [Step 1: Prepare your bootstrap repo](#step-1-prepare-your-bootstrap-repo)
- [Step 2: Bootstrap your daemon](#step-2-bootstrap-your-daemon)
  * [Running in Honeydipper in Kubernetes](#running-in-honeydipper-in-kubernetes)
    + [Using helm charts](#using-helm-charts)
    + [Create your own manifest file](#create-your-own-manifest-file)
  * [Running as docker container](#running-as-docker-container)
  * [Building from source](#building-from-source)
- [Step 3: Hacking away](#step-3-hacking-away)

<!-- tocstop -->

## Prerequisites

 * A running redis server

## Step 1: Prepare your bootstrap repo
As described in the [architecture/design document](../README.md), Honeydipper loads configurations directly from one or many git repos. You can put the repo locally on the machine or pod where Honeydipper is running, or you can put the repos in GitHub, Bitbucket or Gitlab etc, or even mix them together. Make sure you configuration repo is private, and protected from unauthorized changes. Although, you can store all the sensitive information in encrypted form in the repo, you don't want this to become a target.

Inside your repo, you will need a `init.yaml` file. It is the main entrypoint that Honeydipper daemon seeks in each repo. See the [Configuration Guide](./configuration.md) for detailed explanation. Below is an example of the minimum required data to get the daemon bootstrapped:

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

### Running in Honeydipper in Kubernetes

This is the recommended way of using Honeydipper. Not only this is the easiest way to get Honeydipper started, it also enables Honeydipper to take advantage of the power of Kubernetes.

#### Using helm charts

To pass the information about the bootstrap config repo to Honeydipper daemon, the recommended way is to put all the information in a yaml file then use `--values` option during `helm install`. For example:

```yaml
# values.yaml
---
daemon:
  env:
    REPO: git@github.com/example/honeydipper-config.git
    DIPPER_SSH_KEY:
        secretKeyRef:
          name: example-secret
          key: id_rsa
```

Note that, we need to provide a ssh key for Honeydipper daemon to be able to fetch the private repo using ssh protocol. Make sure that the key exists in your cluster as a `secret`.

Once the values file is prepared, you can run the `helm install` command like below.

```bash
helm install --values values.yaml orchestrator stable/honeydipper
```

If you want to use an older version of the chart, (as of now, the latest one is 0.1.2), use `--version` to specify the chart version. By default, the chart uses the latest stable version of the Honeydipper daemon docker image, (latest is 0.1.6 as of now).  You can change the version by specifying `--set daemon.image.tag=x.x.x` in your `helm install` command.

---
We are still working on putting the Honeydipper helm chart into the official helm chart repo. For now, you can package the chart yourself using the source code as decribed below.

Download the source code using `git` or `go get` command. Run the commands below. Your path may vary.

```bash
cd  ~/go/src/github.com/honeydipper/honeydipper # your repo root
cd deployments/helm/
helm package honeydipper
```
You should see the chart file `honeydipper-x.y.z.tgz` in your current directory.

---

#### Create your own manifest file

You can use the below manifest file as a template to create your own. Note that, the basic information needed, besides the docker image for Honeydipper daemon, is the same, `REPO` and `DIPPER_SSH_KEY`.

```yaml
---
apiVersion: apps/v1beta2
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
          image: us.gcs.io/.../honeydipper:latest
          imagePullPolicy: Always
          env:
            - name: REPO
              value: git@github.com/example/honeydipper-config.git
            - name: DIPPER_SSH_KEY
              valueFrom:
                secretKeyRef:
                  namne: example-secret
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

### Running as docker container

```bash
docker run -it -e 'REPO=git@github.com/example/honeydipper-config.git' -e "DIPPER_SSH_KEY=$(cat ~/.ssh/id_rsa)"  honeydipper/honeydipper:latest
```

Replace the repo url with your own, and speicify the private key path for accessing the private repo remotely.

### Building from source

Assuming you have go 1.11 or up installed, you can use `go get` to download and build the binary.

```bash
go get -u github.com/honeydipper/honeydipper.git
pushd $GOPATH/src/github.com/honeydipper/honeydipper
dep ensure
go install ./...
popd
REPO=git@github.com/example/honeydipper-config.git DIPPER_SSH_KEY="$(cat ~/.ssh/id_rsa)" honeydipper
```
You don't have to specify `DIPPER_SSH_KEY` if the key is used by your ssh client by default.

Alternatively, you can follow the [developer setup guide](./howtos/setup_local.md) the download and build.

## Step 3: Hacking away

That's it &mdash; your Honeydipper daemon is bootstrapped. You can start to configure it to suit your needs. The daemon pulls your config repos every minute, and will reload when changes are detected. See the [Honeydipper Guides](./README.md) for more documents, including a way to setup GitHub push event-driven reload.

