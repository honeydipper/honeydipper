# Setup a test/dev environment locally

## Using docker-compose

As of 2.4.0, we added support for running and developing using [docker compose](https://docs.docker.com/compose/). It should simplify the process of setting up and running the system and improve the developer experience.

```bash
git clone https://github.com/honeydipper/honeydipper.git
cd dev/macos # or linux
cat > .env <<EOF
REPO=<...>
BRANCH=<...>
EOF
docker-compose up
```

The container will try to use your `SSH_AUTH_SOCK` to clone remote ssh repo if needed. Or you can use `DIPPER_SSH_KEY` environment variable to pass a ssh private key directly into the container. To use a repo on local file system, use `REPO_DIR` instead of `REPO`. You can also specify `DEBUG='*'` or `DEBUG='daemon'` in the `.env` file to increase the log verbosity.

## Using local Go environment

### Setup Go environment

 * Setup a directory as your go work directory and add it to GOPATH. Assuming go 1.13.1 or up is installed, gvm is recommended to manage multiple versions of go. You may want to persist the GOPATH in your bash_profile

```bash
mkdir ~/go
export GOPATH=$GOPATH:$PWD/go
export PATH=$PATH:$GOPATH/bin
```

### Clone the code

```bash
go get github.com/honeydipper/honeydipper
```

or 

```sh
git clone https://github.com/honeydipper/honeydipper.git
```

### Build and test

 * Build

```bash
go install -v ./...
```

 * Run tests

```bash
make test
```

To run only the unit tests

```bash
make unit-tests
```

To run only the integration tests

```bash
make integration-tests
```

 * Clean up mockgen generated files

```bash
make clean
```

 * For pre-commit hooks

```bash
curl -sfL https://install.goreleaser.com/github.com/golangci/golangci-lint.sh | sh -s -- -b $(go env GOPATH)/bin v1.15.0
brew install pre-commit
pre-commit install --install-hooks
```

### Create local config REPO

Run below command to create your local config repo.

```bash
git init mytest
cd mytest
cat <<EOF > init.yaml
repos:
  - repo: https://github.com/honeydipper/honeydipper-config-essentials.git

drivers:
  redisqueue:
    connection:
      Addr: 127.0.0.1:6379
  redispubsub:
    connection:
      Addr: 127.0.0.1:6379 

rules:
  - when:
      driver: webhook
      if_match:
        url: /health
    do: {}
EOF
git add init.yaml
git commit -m 'init' -a
```

### Start Honeydipper daemon

Before you start your Honeydipper daemon, you need:

 1. Have a redis server running locally
 2. If you want to use encrypted configuration, make sure your are authenticated with google and having "Cloud KMS Crypto Encryptor/Decryptor" role. See [encryption guide](./enable_encryption.md) for detail

```bash
REPO=/path/to/mytest LOCALREDIS=1 honeydipper
```

When you use `LOCALREDIS=1` environment vairable, Honeydipper daemon will ignore the connection settings from your repo and use localhost instead.

You can also set envrionment `DEBUG="*"` to enable verbose debug logging for all parts of daemon  and drivers.

Once the daemon is running, you can access the healthcheck url like below

```
curl -D- http://127.0.0.1:8080/health
```

You should see a `200` response code. There is no payload in the response.

See [configuration guide](../configuration.md) for detail on how to configure your system.

Since 2.4.0, there is an easier way to start the daemon using `Makefile`. Simply put all the needed environment variable in a `.env` file at the top level directory, then run `make run`.
