# Setup a test/dev environment locally

## Setup Go environment

 * Setup a directory as your go work directory and add it to GOPATH. Assuming go 1.11 or up is installed, gvm is recommended to manage multiple versions of go. You may want to persist the GOPATH in your bash_profile

```bash
mkdir ~/go
export GOPATH=$GOPATH:$PWD/go
export PATH=$PATH:$GOPATH/bin
```

 * Clone the code

```bash
go get github.com/honeydipper/honeydipper
```

 * Load the dependencies

```bash
brew install dep
cd ~/go/src/github.com/honeydipper/honeydipper
dep ensure
```

## Build and run

 * Build

```bash
go install ./...
```

 * Run test

```bash
go test -v ./...
```

 * (Optional) For colored test results

```bash
go get -u github.com/rakyll/gotest
gotest -v ./...
```

 * For pre-commit hooks

```bash
curl -sfL https://install.goreleaser.com/github.com/golangci/golangci-lint.sh | sh -s -- -b $(go env GOPATH)/bin v1.15.0
brew install pre-commit
pre-commit install --install-hooks
```

 * Start your local dipper daemon

```bash
REPO=/path/to/your/local/config/repo honeydipper
```

## Create local config REPO

Honeydipper is designed to pull config directly from a git repo. Before you can bootstrap your honeydipper daemon, you will need to make sure two things.

 1. Have a redis server running locally
 2. If you want to use encrypted configuration, make sure your are authenticated with google and having "Cloud KMS Crypto Encryptor/Decryptor" role. See [encryption guide](./enable_encryption.md) for detail

Follow below steps to create your local repo.

 * Creat your local root repo

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
      conditions:
        url: /health
    do:
      content: noop
EOF
git add init.yaml
git commit -m 'init' -a
```

 * Start your daemon with the local root repo

```bash
REPO=/path/to/mytest honeydipper
```

 * Access the healthcheck url

```
curl -D- http://127.0.0.1:8080/health
```

You should see a `200` response code. There is no payload in the response.

See [configuration guide](../configuration.md) for detail on how to configure your system.
