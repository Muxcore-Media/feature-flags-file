# Feature Flags File

[![CI](https://git.zem.systems/muxcore/feature-flags-file/actions/workflows/ci.yml/badge.svg)](https://github.com/Muxcore-Media/feature-flags-file/actions)
[![Go Version](https://img.shields.io/badge/Go-1.26-blue)](https://go.dev/)
[![License: GPL-3.0](https://img.shields.io/badge/License-GPL--3.0-blue.svg)](LICENSE)

**YAML file-backed feature flag provider with SIGHUP reload.**

A MuxCore sidecar module that serves feature flags from a local YAML file (`IsEnabled` / `GetVariant`). Send `SIGHUP` to reload without restart. Provides the `feature.flags` capability (`FeatureFlagProvider`).

---

## How It Works

```
Module request ──→ feature-flags-file (gRPC) ──→ flags.yaml
                              ↑
                         SIGHUP reload
```

Missing flags fall back to the caller-supplied default value.

### flags.yaml format

```yaml
my-feature:
  default: true
  variant: "on"
experiment-a:
  default: false
  variant: "control"
```

---

## Configuration

| Setting | Default | Description |
|---------|---------|-------------|
| `FEATURE_FLAGS_FILE` (env) | `flags.yaml` | Path to YAML feature-flag file |
| gRPC listen | `127.0.0.1:9402` | Feature-flags gRPC address (loopback by default) |
| HTTP listen | `:9404` | Health endpoint (`GET /health`) |

gRPC uses **TLS by default**. Auto-generated dev certificates are stored alongside the flags file (or under `FEATURE_FLAGS_TLS_DIR`). Set `MUXCORE_INSECURE_DISABLE_TLS=true` for plaintext dev only. Override certs with `MUXCORE_TLS_CERT` / `MUXCORE_TLS_KEY` or `FEATURE_FLAGS_TLS_CERT` / `FEATURE_FLAGS_TLS_KEY`.

---

## Quick Start

```bash
make build

export MUXCORE_INSECURE_DISABLE_TLS=true
./feature-flags-file --muxcore-mesh-addr localhost:9090
```

---

## Capability

`feature.flags` — File-backed feature flags (`FeatureFlagProvider`)

## License

GPL-3.0
