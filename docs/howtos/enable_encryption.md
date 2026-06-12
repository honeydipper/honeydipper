# Enable Encrypted Config in Honeydipper

Honeydipper outsources encryption/decryption tasks to drivers. In order for Honeydipper to be able to decrypt the encrypted content in the config files, the proper driver needs to be loaded and configured. By default, the `honeydipper-config-essentials` repo `gcloud` bundle comes with a gcloud KMS driver, I will use this as an example to explain how decryption works.

<!-- toc -->

- [Loading the driver](#loading-the-driver)
- [Config the driver](#config-the-driver)
- [How to encrypt your secret](#how-to-encrypt-your-secret)
- [Using LOOKUP for runtime secret fetching](#using-lookup-for-runtime-secret-fetching)
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

## Supported drivers

Honeydipper supports multiple drivers for encryption and secret lookup:

| Driver | Type | Description |
|---|---|---|
| `gcloud-kms` | Decrypt | Google Cloud KMS for decrypting ciphertext (implements `decrypt` RPC) |
| `gcloud-secret` | Lookup | Google Cloud Secret Manager for runtime secret fetching (implements `lookup` RPC) |
| `hd-driver-vault` | Lookup | HashiCorp Vault for runtime secret fetching (implements `lookup` RPC) |

Any driver that implements the `decrypt` or `lookup` RPC can be used with the `ENC` or `LOOKUP` syntax respectively.
