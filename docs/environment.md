# Environment Variable Reference

> Honeydipper uses environment variables for bootstrap configuration, authentication, and runtime behavior. This document provides a comprehensive reference for all supported environment variables.

## Table of Contents

- [Bootstrap Configuration](#bootstrap-configuration)
- [Git Authentication](#git-authentication)
- [API and JWT](#api-and-jwt)
- [Driver Management](#driver-management)
- [Repository Overrides](#repository-overrides)
- [SSH Configuration](#ssh-configuration)
- [GitHub App Integration](#github-app-integration)
- [Summary Table](#summary-table)

---

## Bootstrap Configuration

These variables control how Honeydipper loads its initial configuration.

| Variable | Required | Default | Description |
|---|---|---|---|
| `REPO` | **Yes** | — | Bootstrap configuration repository URL. Can be a git URL or a local directory path. |
| `BRANCH` | No | `master` | Branch to use in the bootstrap repository. |
| `BOOTSTRAP_PATH` | No | `/` | Path within the repository from which to load the init file. |
| `BOOTSTRAP_FILE` | No | `init.yaml` | Custom init file name. Overrides the default `init.yaml`. |
| `JOB_FILE` | Job mode only | — | Path to a workflow file. Required when running Honeydipper in job mode. |

### Bootstrap Examples

**Standard bootstrap from a git repo:**
```bash
export REPO="git@github.com:myorg/honeydipper-config.git"
export BRANCH="production"
export BOOTSTRAP_PATH="/configs"
export BOOTSTRAP_FILE="init.yaml"
```

**Bootstrap from a local directory:**
```bash
export REPO="/path/to/local/config"
export BRANCH="main"
```

**Job mode:**
```bash
export REPO="git@github.com:myorg/honeydipper-config.git"
export JOB_FILE="workflows/nightly-cleanup.yaml"
```

---

## Git Authentication

These variables configure how Honeydipper authenticates with git repositories when cloning or pulling configuration.

### HTTP Authentication

| Variable | Description |
|---|---|
| `DIPPER_GIT_PASS` | Default password for git HTTP authentication. Used when no username-specific variable is set. |
| `DIPPER_GIT_PASS_{USERNAME}` | Username-specific password. The username is uppercased and hyphens are replaced with underscores. For example, for username `deploy-bot`, use `DIPPER_GIT_PASS_DEPLOY_BOT`. |

**Priority order for HTTP auth:**
1. `DIPPER_GIT_PASS_{USERNAME}` (username-specific)
2. `DIPPER_GIT_PASS` (fallback)

### SSH Authentication

| Variable | Description |
|---|---|
| `DIPPER_SSH_KEY` | Raw SSH private key content (PEM format). Takes precedence over key file. |
| `DIPPER_SSH_KEYFILE` | Path to the SSH private key file. |
| `DIPPER_SSH_PASS` | Passphrase for the SSH private key. |
| `SSH_AUTH_SOCK` | SSH agent socket path. If set, the SSH agent is used for authentication. |

**Priority order for SSH auth:**
1. `SSH_AUTH_SOCK` (SSH agent)
2. `DIPPER_SSH_KEY` (inline key)
3. `DIPPER_SSH_KEYFILE` (key file)

### Git Auth Examples

**HTTP with username-specific password:**
```bash
export DIPPER_GIT_PASS_DEPLOY_BOT="ghp_xxxxxxxxxxxx"
```

**SSH with inline key:**
```bash
export DIPPER_SSH_KEY="$(cat ~/.ssh/id_ed25519)"
export DIPPER_SSH_PASS="my-passphrase"
```

**SSH with agent:**
```bash
export SSH_AUTH_SOCK="$SSH_AUTH_SOCK"
```

---

## API and JWT

These variables configure the HTTP API server's JWT signing.

| Variable | Required | Description |
|---|---|---|
| `HD_JWT_SIGNING_KEY` | For JWT | Secret key used to sign JWT tokens. Must be a secure, random string. |
| `HD_JWT_ISSUER` | No | JWT issuer claim. Identifies the entity issuing the token. |
| `HD_UI_URL` | No | Base URL of the Honeydipper UI. Used for constructing redirect URLs after SAML/OAuth authentication. Can also be set in config as `daemon.services.api.ui_url`. |
| `HD_SECRET_DRIVER` | No | Name of the secret driver to use (e.g., `vault`). Overrides the default secret driver configuration. |

### JWT Example

```bash
export HD_JWT_SIGNING_KEY="your-256-bit-secret-key-here"
export HD_JWT_ISSUER="honeydipper-prod"
export HD_UI_URL="https://honeydipper.example.com"
```

---

## Driver Management

These variables control where Honeydipper finds and caches drivers.

| Variable | Default | Description |
|---|---|---|
| `HONEYDIPPER_DRIVERS_BUILTIN` | `$GOPATH/bin` or `/opt/honeydipper/driver/builtin` | Path to the built-in driver directory. |
| `HONEYDIPPER_DRIVERS_CACHE` | `/opt/honeydipper/drivers/cache` | Path where remote drivers are cached after download. |
| `HONEYDIPPER_REMOTE_REQUIRE_SIGNATURE` | (not set) | When set to any value, enforces Ed25519 signature verification for remote driver packages. |

### Driver Path Priority

**Built-in drivers:**
1. `$HONEYDIPPER_DRIVERS_BUILTIN`
2. `$GOPATH/bin`
3. `/opt/honeydipper/driver/builtin`

**Remote driver cache:**
1. `$HONEYDIPPER_DRIVERS_CACHE`
2. `/opt/honeydipper/drivers/cache`

### Driver Example

```bash
export HONEYDIPPER_DRIVERS_BUILTIN="/opt/honeydipper/drivers/builtin"
export HONEYDIPPER_DRIVERS_CACHE="/opt/honeydipper/drivers/cache"
export HONEYDIPPER_REMOTE_REQUIRE_SIGNATURE="true"
```

---

## Repository Overrides

These variables allow overriding repository URLs at runtime without modifying the configuration.

| Variable | Format | Description |
|---|---|---|
| `REPO_OVERRIDE` | `<remote_url>=<local_path>` | Override a single repository. |
| `REPO_OVERRIDE_<NAME>` | `<remote_url>=<local_path>` | Named override. `<NAME>` can be any identifier. |

The format is: `<remote_url>=<local_path>`. Both sides are trimmed of whitespace.

### Repository Override Examples

**Single override:**
```bash
export REPO_OVERRIDE="git@github.com:myorg/config.git=/local/path/to/config"
```

**Multiple overrides:**
```bash
export REPO_OVERRIDE_MAIN="git@github.com:myorg/main-config.git=/local/main"
export REPO_OVERRIDE_SHARED="git@github.com:myorg/shared-config.git=/local/shared"
```

---

## GitHub App Integration

These variables configure GitHub App-based authentication for accessing GitHub repositories.

| Variable | Description |
|---|---|
| `GH_APP_ID` | GitHub App ID. |
| `GH_APP_INSTALLATION_ID` | Installation ID for the GitHub App. |
| `GH_APP_KEY` | Private key for the GitHub App (PEM format). |

When these are set, Honeydipper uses the GitHub App to generate access tokens for repository operations instead of personal access tokens.

### GitHub App Example

```bash
export GH_APP_ID="123456"
export GH_APP_INSTALLATION_ID="78901234"
export GH_APP_KEY="$(cat github-app-private-key.pem)"
```

---

## Summary Table

| Variable | Category | Required | Default |
|---|---|---|---|
| `REPO` | Bootstrap | **Yes** | — |
| `BRANCH` | Bootstrap | No | `master` |
| `BOOTSTRAP_PATH` | Bootstrap | No | `/` |
| `BOOTSTRAP_FILE` | Bootstrap | No | `init.yaml` |
| `JOB_FILE` | Bootstrap | Job mode | — |
| `DIPPER_GIT_PASS` | Git Auth | No | — |
| `DIPPER_GIT_PASS_{USERNAME}` | Git Auth | No | — |
| `DIPPER_SSH_KEY` | Git Auth (SSH) | No | — |
| `DIPPER_SSH_KEYFILE` | Git Auth (SSH) | No | — |
| `DIPPER_SSH_PASS` | Git Auth (SSH) | No | — |
| `SSH_AUTH_SOCK` | Git Auth (SSH) | No | — |
| `GH_APP_ID` | GitHub App | No | — |
| `GH_APP_INSTALLATION_ID` | GitHub App | No | — |
| `GH_APP_KEY` | GitHub App | No | — |
| `HD_JWT_SIGNING_KEY` | API/JWT | For JWT | — |
| `HD_JWT_ISSUER` | API/JWT | No | — |
| `HD_UI_URL` | API/JWT | No | — |
| `HD_SECRET_DRIVER` | API | No | — |
| `HONEYDIPPER_DRIVERS_BUILTIN` | Drivers | No | `$GOPATH/bin` |
| `HONEYDIPPER_DRIVERS_CACHE` | Drivers | No | `/opt/honeydipper/drivers/cache` |
| `HONEYDIPPER_REMOTE_REQUIRE_SIGNATURE` | Drivers | No | (disabled) |
| `REPO_OVERRIDE` | Repo Override | No | — |
| `REPO_OVERRIDE_*` | Repo Override | No | — |
