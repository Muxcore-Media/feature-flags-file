# Feature Flags File

[![CI](https://github.com/Muxcore-Media/feature-flags-file/actions/workflows/ci.yml/badge.svg)](https://github.com/Muxcore-Media/feature-flags-file/actions)
[![Go Version](https://img.shields.io/badge/Go-1.26-blue)](https://go.dev/)
[![License: GPL-3.0](https://img.shields.io/badge/License-GPL--3.0-blue.svg)](LICENSE)

**YAML file-backed feature flag provider with SIGHUP reload.**

A MuxCore sidecar module that serves feature flags from a local YAML file (`IsEnabled` / `GetVariant`). Send `SIGHUP` to reload without restart. Provides the `feature.flags` capability.

---

## How It Works

```
Module request ──→ feature-flags-file (gRPC) ──→ flags.yaml
                              ↑
                         SIGHUP reload
```

Missing flags fall back to the caller-supplied default value.

---

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `FEATURE_FLAGS_FILE` | `flags.yaml` | Path to YAML feature-flag file |
| gRPC listen | `:9402` | Feature-flags gRPC address |
| HTTP listen | `:9403` | Health/HTTP listen address |

---

## Quick Start

```bash
make build

export MUXCORE_INSECURE_DISABLE_TLS=true
./feature-flags-file --muxcore-mesh-addr localhost:9090
```

---

## Capability

`feature.flags` — File-backed feature flags

## License

GPL-3.0
