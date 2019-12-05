# Setup a test/dev environment locally

## Setup Go environment

 * Setup a directory as your go work directory and add it to GOPATH. Assuming go 1.13.1 or up is installed, gvm is recommended to manage multiple versions of go. You may want to persist the GOPATH in your bash_profile

```bash
mkdir ~/go
export GOPATH=$GOPATH:$PWD/go
export PATH=$PATH:$GOPATH/bin
```

## Clone the code

```bash
go get github.com/honeydipper/honeydipper
```

or 

```sh
git clone https://github.com/honeydipper/honeydipper.git
```

## Build and test

 * Build

```bash
go install -v ./...
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

## Create local config REPO

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

## Start Honeydipper daemon

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
