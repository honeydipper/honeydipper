# Enable Encrypted Config in Honeydipper

Honeydipper outsources encryption/decryption tasks to drivers. In order for Honeydipper to be able to decrypt the encrypted content in the config files, the proper driver needs to be loaded and configured. By default, the `honeydipper-config-essentials` repo `gcloud` bundle comes with a gcloud KMS driver, I will use this as an example to explain how decryption works.

<!-- toc -->

- [Loading the driver](#loading-the-driver)
- [Config the driver](#config-the-driver)
- [How to encrypt your secret](#how-to-encrypt-your-secret)
- [Using LOOKUP for runtime secret fetching](#using-lookup-for-runtime-secret-fetching)
- [Secure Exec: Running programs with decrypted secrets](#secure-exec-running-programs-with-decrypted-secrets)
  - [CLI usage](#cli-usage)
  - [How it works](#how-it-works)
  - [Environment variable format](#environment-variable-format)
  - [Bootstrapping Honeydipper](#bootstrapping-honeydipper)
  - [Driver support](#driver-support)
- [Supported drivers](#supported-drivers)

<!-- tocstop -->
## Loading the driver

When you include the `honeydipper-config-essentials` repo from your bootstrap repo like below:

```yaml
---
repos:
  - repo: https://github.com/honeydipper/honeydipper-config-essentials.git
    path: /gcloud
```

The `gcloud-kms` driver is loaded *automatically* with following daemon configuration.

```yaml
---
drivers:
  ...
  daemon:
    ...
    features:
      global:
        - name: driver:gcloud-kms
          required: true
      ...
    drivers:
      ...
      gcloud-kms:
        name: gcloud-kms
        type: builtin
        handlerData:
          shortName: gcloud-kms
```

Note that the above configuration snippet is for your information purpose, you don't have to manually type them in if you include the `gcloud` bundle from the `honeydipper-config-essential` repo.

## Config the driver

The `gcloud-kms` driver assumes that there is a default google credential where the daemon is running. This is usually the case when you run Honeydipper in gcloud either in Compute Engine or in Kubernetes clusters. See GCP documentation on how to configure the Compute Engine instance or Kubernetes clusters with a service account. If you are running this from your workstation, make sure you run `gcloud auth login` to authenticate with gcloud. The service account or the credential you are using needs to have `roles/kms.CryptoKeyDecryptor` IAM role. If you are running the Honeydipper in a docker container other than gcloud, you will need to mount a service account key file into the container and set `GOOGLE_APPLICATION_CREDENTIALS` environment variable.

The `gcloud-kms` driver expects a configuration item under `drivers.gcloud-kms` named `keyname`.

For example:

```yaml
---
drivers:
  ...
  gcloud-kms:
    keyname: projects/<your project>/locations/<region>/keyRings/<keyring name>/cryptoKeys/<key name>
  ...
```

Once this is configured in your repo and loaded by the daemon, you can start to use this driver to decrypt content in the configuration files.

## How to encrypt your secret

Assuming you have `gcloud` command installed, and authenticated, and you have the `roles/kms.CryptoKeyEncryptor` role.

```bash
echo -n xxxx_your_secret_xxxx |
  gcloud --project=<...> kms encrypt --plaintext-file=- --ciphertext-file=- --keyring=<...> --key=<...> --location=<...> |
  base64
```

Fill in the blank for `project`, `keyring`, `location` and `key` with the same information you configured for the driver. The command will output the base64 encoded cipher text. You can use this in your configuration file with `eyaml` style syntax.  For example:

```yaml
---
systems:
  my_system:
    data:
      mysecret: ENC[gcloud-kms,---base64 encoded ciphertext---]
```

See the [interpolation guide](../interpolation.md) for more information on eyaml syntax.

## Using LOOKUP for runtime secret fetching

In addition to `ENC[...]` for encrypted values, Honeydipper supports `LOOKUP[...]` syntax for fetching secrets at runtime from secret stores. This is useful when you want to avoid storing encrypted values in your config files and instead fetch them dynamically.

The syntax is:

```yaml
LOOKUP[driver,path][:printf_pattern]
```

For example, using the GCP Secret Manager driver:

```yaml
systems:
  my_system:
    data:
      api_token: LOOKUP[gcloud-secret,projects/my-project/secrets/my-api-token/versions/latest]
```

Using the Vault driver:

```yaml
systems:
  my_system:
    data:
      api_token: LOOKUP[hd-driver-vault,secret/data/myapp/apikey]
```

The `LOOKUP` syntax supports an optional `?` prefix on the path to make the lookup optional (swallowing errors if the secret is not found):

```yaml
systems:
  my_system:
    data:
      optional_setting: LOOKUP[gcloud-secret,projects/my-project/secrets/optional?]
```

See the [interpolation guide](../interpolation.md) for more information on LOOKUP syntax.

## Secure Exec: Running programs with decrypted secrets

Honeydipper's `secure-exec` feature is a **CLI tool** built into each secrets driver binary. It allows you to run any program with decrypted secrets injected into its environment, without ever exposing the plaintext secrets in configuration files, command-line arguments, or process listings. It is similar in concept to Google Berglas exec.

**This is not a daemon service configuration.** The driver binary is invoked directly from the command line or a startup script with `exec` as the first positional argument (the service name). It does not run as a long-lived process.

### CLI usage

The general command format is:

```bash
driver-binary exec -- /path/to/program [args...]
```

- The first positional argument (`exec`) tells the driver to operate in "exec mode" instead of connecting to the daemon.
- The `--` separator is **required** and separates driver arguments from the target program command.
- The program path after `--` is resolved relative to `$PATH` if not absolute.

For example, using the GCP KMS driver:

```bash
gcloud-kms exec -- /usr/bin/python3 /path/to/script.py
```

Using the GCP Secret Manager driver:

```bash
gcloud-secret exec -- /path/to/application
```

Using the Vault driver:

```bash
hd-driver-vault exec -- /path/to/application
```

### How it works

When a driver binary is invoked in exec mode:

1. It scans **all environment variables** for values with special prefixes (`hd-lookup:` or `hd-decrypt:`).
2. For each matching variable, it calls the appropriate RPC (`lookup` or `decrypt`) to resolve the secret using the driver's native API.
3. The environment variable is **updated in place** with the decrypted value.
4. The target program is executed with `syscall.Exec`, which **replaces the current process** — the driver process becomes the target process. The target program sees the decrypted secrets as normal environment variables.

Because the driver replaces itself with the target program (via `exec`), there is no intermediate process that holds the decrypted secrets, and no need for cleanup.

### Environment variable format

`secure-exec` recognizes two prefixes in environment variable values:

| Prefix | Driver RPC | Value format |
|--------|-----------|--------------|
| `hd-lookup:<path>` | `lookup` | A secret path recognized by the driver (e.g. GCP Secret Manager resource name, Vault key path) |
| `hd-decrypt:<base64>` | `decrypt` | Base64-encoded ciphertext (e.g. output of `gcloud kms encrypt` piped through `base64`) |

If the driver only implements one of the two RPCs, only the corresponding prefix can be used. For example, `gcloud-kms` only implements `decrypt`, so only `hd-decrypt:` is valid with that driver. Drivers that implement both (none currently) prefer `lookup`.

### Bootstrapping Honeydipper

The primary use case for `secure-exec` is **bootstrapping** — starting Honeydipper itself with decrypted secrets that are required during startup but must not be stored in plaintext.

For example, suppose you need a GitHub token for Honeydipper to clone configuration repositories via `https://token@github.com/...`, and you want to store that token encrypted in your environment. You can write a startup script like this:

```bash
#!/bin/bash

# Set the GitHub token as a secret reference, not in plaintext.
# The token is stored in GCP Secret Manager.
export GITHUB_TOKEN="hd-lookup:projects/my-project/secrets/github-deploy-token/versions/latest"

# Use gcloud-secret to decrypt the token and exec honeydipper.
# The honeydipper process will see GITHUB_TOKEN with the real decrypted value.
exec gcloud-secret exec -- /usr/local/bin/honeydipper
```

When Honeydipper starts and processes its bootstrap repos, it clones:

```yaml
repos:
  - repo: https://github.com/myorg/honeydipper-config.git
```

Honeydipper automatically uses `$GITHUB_TOKEN` from the environment for HTTPS authentication, so the repo is cloned successfully — but the token was never stored in any file, config, or process argument.

#### Using KMS-encrypted secrets

If your secret is encrypted with GCP KMS instead of stored in Secret Manager, use `gcloud-kms` with the `hd-decrypt:` prefix:

```bash
#!/bin/bash

# The GitHub token was encrypted with KMS and base64-encoded.
export GITHUB_TOKEN="hd-decrypt:CiQAc92x...long base64 ciphertext...="

exec gcloud-kms exec -- /usr/local/bin/honeydipper
```

#### Using Vault secrets

Similarly, with HashiCorp Vault:

```bash
#!/bin/bash

export GITHUB_TOKEN="hd-lookup:secret/data/honeydipper/github#token"

exec hd-driver-vault exec -- /usr/local/bin/honeydipper
```

### Driver support

The following drivers are compiled with `secure-exec` support and can be used as CLI tools:

| Driver binary | Default service | RPC used | Notes |
|---|---|---|---|
| `gcloud-kms` | `kms` | `decrypt` | Use with `hd-decrypt:<base64>` — decrypts base64-encoded KMS ciphertext |
| `gcloud-secret` | `google-secret` | `lookup` | Use with `hd-lookup:<path>` — looks up GCP Secret Manager secrets by resource name |
| `hd-driver-vault` | `vault` | `lookup` | Use with `hd-lookup:<path>` — looks up Vault secrets by key path |

Any of these binaries can be invoked directly with `exec` as the first argument to use the `secure-exec` feature.

## Supported drivers

Honeydipper supports multiple drivers for encryption and secret lookup:

| Driver | Type | Description |
|---|---|---|
| `gcloud-kms` | Decrypt | Google Cloud KMS for decrypting ciphertext (implements `decrypt` RPC) |
| `gcloud-secret` | Lookup | Google Cloud Secret Manager for runtime secret fetching (implements `lookup` RPC) |
| `hd-driver-vault` | Lookup | HashiCorp Vault for runtime secret fetching (implements `lookup` RPC) |

Any driver that implements the `decrypt` or `lookup` RPC can be used with the `ENC` or `LOOKUP` syntax respectively. Drivers that implement these RPCs can also be used with the `secure-exec` feature to run programs with decrypted secrets in their environment.
