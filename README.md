# Feature Flags File

[![CI](https://git.zem.systems/muxcore/feature-flags-file/actions/workflows/ci.yml/badge.svg)](https://git.zem.systems/muxcore/feature-flags-file/actions)
[![Go Version](https://img.shields.io/badge/Go-1.26-blue)](https://go.dev/)
[![License: GPL-3.0](https://img.shields.io/badge/License-GPL--3.0-blue.svg)](LICENSE)

**YAML file-backed feature flag provider with SIGHUP reload.**

A MuxCore sidecar module that serves feature flags from a local YAML file (`IsEnabled` / `GetVariant`). Send `SIGHUP` to reload without restart. Provides the `feature.flags` capability (`FeatureFlagProvider`) and live `settings` for `flags_file` / `flags_yaml`.

---

## How It Works

```
Module request ──→ feature-flags-file (gRPC) ──→ flags.yaml
                              ↑
                         SIGHUP reload
```

Missing flags fall back to the caller-supplied default value. Percent rollouts and `enabled_for` lists use the caller module ID from gRPC metadata (`x-caller-id`).

### flags.yaml format

See [`flags.yaml`](flags.yaml) for a full example.

```yaml
my-feature:
  default: true
  variant: "on"
  percent: 25
  enabled_for:
    - admin-ui
experiment-a:
  default: false
  variant: "control"
```

| Field | Description |
|-------|-------------|
| `default` | Base enabled state when no targeting rule matches |
| `variant` | Multivariate value returned by `GetVariant` |
| `percent` | Optional 0–100 rollout bucket per caller identity |
| `enabled_for` | Optional list of module IDs always enabled |

---

## Configuration

| Setting | Default | Description |
|---------|---------|-------------|
| `FEATURE_FLAGS_FILE` (env) | `flags.yaml` | Path to YAML feature-flag file |
| `FEATURE_FLAGS_GRPC_ADDR` (env) | `:9402` | Feature-flags gRPC listen address |
| `FEATURE_FLAGS_HTTP_ADDR` (env) | `:9404` | Health endpoint listen address |
| `flags_file` (settings) | `flags.yaml` | Live path update via admin-ui |
| `flags_yaml` (settings) | — | Inline YAML edit; writes to the flags file |
| `MVP_ENABLE_FEATURE_FLAGS` | `0` | Enable in `_mvp/run-host.sh` soak stack |

`Info().HTTPAddr` advertises the HTTP health listener (`:9404`) for mesh registration and admin-ui.

---

## Quick Start

```bash
make build

export MUXCORE_INSECURE_DISABLE_TLS=true
export FEATURE_FLAGS_FILE=flags.yaml
./feature-flags-file --muxcore-mesh-addr localhost:9090
```

MVP soak:

```bash
# in _mvp/.env
MVP_ENABLE_FEATURE_FLAGS=1
```

---

## Capability

`feature.flags` — File-backed feature flags (`FeatureFlagProvider`)

## License

GPL-3.0
