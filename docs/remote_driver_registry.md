# Remote Driver And Registry Guide

<!-- toc -->

- [Overview](#overview)
- [What Is Implemented](#what-is-implemented)
- [Quick Start](#quick-start)
- [Daemon Registry Configuration](#daemon-registry-configuration)
- [Remote Driver Configuration](#remote-driver-configuration)
  * [Registry-Based Driver (Recommended)](#registry-based-driver-recommended)
  * [Direct URL Driver (Bypass Path)](#direct-url-driver-bypass-path)
  * [Local Driver Source](#local-driver-source)
- [Source Policy Controls](#source-policy-controls)
- [Signature Verification](#signature-verification)
- [Registry Manifest Format](#registry-manifest-format)
- [Cache Directory Structure](#cache-directory-structure)
- [Package Installation](#package-installation)
- [Operational Notes](#operational-notes)
- [Troubleshooting](#troubleshooting)
- [TODO (Future Plan)](#todo-future-plan)

<!-- tocstop -->

## Overview

Honeydipper supports loading `remote` drivers by acquiring binaries at runtime, verifying integrity, and executing from a local cache.

This guide explains how to configure:

1. Shared registries under `drivers.daemon`.
2. Remote drivers that resolve from a registry by name.
3. Policy controls for `registry`, `direct`, and `local` sources.
4. Signature verification for stronger trust.
5. Automatic package installation for remote driver dependencies.

## What Is Implemented

Current behavior:

1. Cache-first acquisition for remote artifacts.
2. SHA-256 verification for all acquired artifacts.
3. Optional signature verification using `ed25519` (`publicKey` + `signature`).
4. Registry manifest resolution by `version` or `channel`.
5. Daemon-level named registries under `drivers.daemon.registries`.
6. Reserved builtin registry name: `builtin`.
7. Source policy enforcement in config resolution:
   - `registry` allowed by default.
   - `direct` denied by default.
   - `local` denied by default.
8. Automatic package installation for required system packages.
9. Directory-based mutex with timeout for cache access synchronization.

## Quick Start

1. Define one or more registries under `drivers.daemon.registries`.
2. Add a remote driver under `drivers.daemon.drivers.<driverName>`.
3. Use `handlerData.registry` to reference a named registry.
4. Map the driver to features in `drivers.daemon.featureMap`.
5. Ensure source policy allows the chosen source type.

## Daemon Registry Configuration

Example:

```yaml
---
drivers:
  daemon:
    registries:
      github-public:
        baseURL: https://example.com/driver-registry
        requireSignature: true
        publicKey: <base64-ed25519-public-key>
```

Notes:

1. `baseURL` is accepted and normalized to `registryURL` during resolution.
2. `registryURL` can also be used directly in registry definitions.
3. The reserved registry name `builtin` cannot be defined in config.

## Remote Driver Configuration

### Registry-Based Driver (Recommended)

```yaml
---
drivers:
  daemon:
    drivers:
      custom-webhook:
        name: custom-webhook
        type: remote
        handlerData:
          registry: github-public
          channel: stable
          # optional: version: 1.2.3
```

Resolution order:

1. If `version` is set, that version is used.
2. Otherwise `channel` is used (default: `stable`).
3. Otherwise manifest `latest` is used.

### Direct URL Driver (Bypass Path)

Direct URL is supported but denied by default by source policy.

```yaml
---
drivers:
  daemon:
    remoteDriverPolicy:
      direct:
        enabled: true
    drivers:
      custom-direct:
        name: custom-direct
        type: remote
        handlerData:
          url: https://example.com/bin/custom-direct-linux-amd64
          sha256: <sha256-hex>
          fileName: custom-direct
          requireSignature: true
          publicKey: <base64-ed25519-public-key>
          signature: <base64-ed25519-signature>
```

### Local Driver Source

Local source is also denied by default.

```yaml
---
drivers:
  daemon:
    remoteDriverPolicy:
      local:
        enabled: true
    drivers:
      custom-local:
        name: custom-local
        type: remote
        handlerData:
          localPath: /opt/honeydipper/drivers/local/custom-local
```

## Source Policy Controls

Configure under `drivers.daemon.remoteDriverPolicy`:

```yaml
---
drivers:
  daemon:
    remoteDriverPolicy:
      registry:
        enabled: true
      direct:
        enabled: false
      local:
        enabled: false
```

Defaults when policy is not set:

1. `registry.enabled = true`
2. `direct.enabled = false`
3. `local.enabled = false`

## Signature Verification

Signature verification fields can come from:

1. Driver `handlerData` directly.
2. Registry artifact entry.
3. Registry version-level key.
4. Registry top-level key.

Fields:

1. `requireSignature` (bool)
2. `publicKey` (base64 ed25519 public key)
3. `signature` (base64 signature over artifact digest bytes)

Global strict mode can be enabled with environment variable:

```bash
HONEYDIPPER_REMOTE_REQUIRE_SIGNATURE=true
```

## Registry Manifest Format

Each driver is resolved from `<registryURL>/<driverName>.json`.

Example:

```json
{
  "driver": "custom-webhook",
  "latest": "1.2.3",
  "publicKey": "<base64-ed25519-public-key>",
  "channels": {
    "stable": "1.2.3",
    "canary": "1.3.0-rc1"
  },
  "versions": {
    "1.2.3": {
      "publicKey": "<optional-version-level-key>",
      "artifacts": [
        {
          "os": "linux",
          "arch": "amd64",
          "url": "https://example.com/bin/custom-webhook-linux-amd64",
          "sha256": "<sha256-hex>",
          "fileName": "custom-webhook",
          "publicKey": "<optional-artifact-level-key>",
          "signature": "<base64-ed25519-signature>"
        }
      ]
    }
  }
}
```

## Cache Directory Structure

Remote drivers are stored in a cache directory structure organized by their SHA-256 digest:

```
<remotePath>/sha256/<sha256-driver-digest>/
  └── <fileName>
```

Where:
- `<remotePath>` is the root cache directory (defaults to `/opt/honeydipper/drivers/cache`)
- `<sha256-driver-digest>` is the hex-encoded SHA-256 hash of the driver binary
- `<fileName>` is the name of the driver executable

The remote path can be configured using the `HONEYDIPPER_DRIVERS_CACHE` environment variable.

Example directory structure:
```
/opt/honeydipper/drivers/cache/sha256/
  └── a1b2c3d4e5f6.../
      └── custom-webhook-linux-amd64
```

### Directory-Based Mutex

To prevent concurrent downloads of the same driver, Honeydipper uses a directory-based mutex mechanism. A lock is acquired by creating a directory named after the driver's SHA-256 digest. If the directory already exists, the process waits for the lock to be released (with a default timeout of 30 seconds).

If the timeout is exceeded, the acquisition fails with a timeout error.

## Package Installation

Honeydipper can automatically install required system packages before launching a remote driver. This is useful when a remote driver has dependencies on system libraries or tools.

To enable package installation, add a `requiredPackages` section to the driver's `handlerData`:

```yaml
---
drivers:
  daemon:
    drivers:
      custom-driver:
        name: custom-driver
        type: remote
        handlerData:
          registry: github-public
          requiredPackages:
            apk:
              - libssl1.1
              - curl
            apt:
              - libssl1.1
              - curl
            dnf:
              - openssl
              - curl
            brew:
              - openssl
```

Package managers are auto-detected based on the host OS:
- `apk`: Alpine Linux
- `apt`: Debian/Ubuntu
- `dnf`: Fedora/RHEL
- `brew`: macOS

If the detected package manager is not listed in `requiredPackages`, the driver will fail to start with an error.

## Operational Notes

1. Cache root defaults to `/opt/honeydipper/drivers/cache`.
2. Set `HONEYDIPPER_DRIVERS_CACHE` to change cache location.
3. Artifacts are keyed by digest and re-verified on reuse.
4. Daemon resolves registry config and source policy before runtime load.
5. Directory-based mutex timeout defaults to 30 seconds.

## Troubleshooting

Common failures:

1. `remote driver source is not allowed by policy: direct`
   - Enable `drivers.daemon.remoteDriverPolicy.direct.enabled`.
2. `builtin remote registry cannot be overridden`
   - Remove `drivers.daemon.registries.builtin` from config.
3. `failed resolving remote driver version from registry`
   - Check `version`, `channel`, and manifest `versions` content.
4. Signature errors
   - Validate base64 values and signer key/signature match.
5. `timeout waiting for cache lock`
   - Check if another process is downloading the same driver. Wait or manually remove the lock directory if the process crashed.
6. `requiredPackages does not define packages for detected package manager`
   - Add the detected package manager (`apk`, `apt`, `dnf`, or `brew`) to the `requiredPackages` section.

## TODO (Future Plan)

Deferred enhancements:

1. Rollout controls
   - Enforce pinned-version mode for stricter environments.
   - Channel allowlists per environment.
2. Known-good fallback
   - Auto-fallback to previous verified artifact on acquisition/startup failure.
3. Extended telemetry
   - Dedicated counters for policy deny categories and fallback actions.
4. Progressive rollout support
   - Canary policy and staged adoption controls.
5. Supply-chain hardening
   - Optional provenance/attestation checks.
   - Optional SBOM policy enforcement.
