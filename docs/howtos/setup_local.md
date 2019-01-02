# Setup a test/dev environment locally

## Setup Go environment

 * Setup a directory as your go work directory and add it to GOPATH. Assuming go 1.11 or up is installed, gvm is recommended to manage multiple versions of go. You may want to persist the GOPATH in your bash_profile
```bash
mkdir gocode
export GOPATH=$GOPATH:$PWD/gocode
```
 * Clone the code
```bash
cd gocode
mkdir -p src/github.com/honeyscience/
cd src/github.com/honeyscience/
git clone git@github.com:honeyscience/honeydipper.git
```
 * Load the dependencies
```bash
brew install dep
cd honeydipper
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
 * (Optional) For pre-commit hooks
```bash
brew install pre-commit
pre-commit install --install-hooks
```
 * Start your local dipper daemon, see admin guide(coming soon) for detail
```bash
REPO=file:///path/to/your/rule/repo honeydipper
```

## Create local config REPO

Honeydipper is designed to pull config directly from a git repo. Before you can bootstrap your honeydipper daemon, you will need to make sure two things.
 1. Have a redis server running locally
 2. Make sure your are authenticated with google and having "Cloud KMS Crypto Encryptor/Decryptor" role.

Follow below steps to get your daemon bootstrapped

 * Creat your local root repo
```bash
git init mytest
cd mytest
cat <<EOF > init.yaml
includes:
  - git@github.com:honeyscience/honeydipper-test-config.git
EOF
git add init.yaml
git commit -m 'init' -a
```
 * Start your daemon with the local root repo
```bash
REPO=file:///path/to/mytest honeydipper
```

Notes: the remote repo is intended to be shared so, it might be suitable for your own test environment. You can override pretty much everything in your own yaml files. Or, you can skip loading the remote config and create your own config from scratch. To skip, just remove the `includes` section.

---
Let me know if you run into any issue
