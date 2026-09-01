# Compatibility

## Core Version

| Module Version | Core Version | Status |
|----------------|-------------|--------|
| v0.1.3         | v0.5.0+     | Current |

## Contracts

| Contract | Capability | Status |
|----------|-----------|--------|
| FeatureFlagProvider | `feature.flags` | Current |

Unknown flags return the caller’s `default_value`. Empty `variant` falls back the same way. SIGHUP reloads `FEATURE_FLAGS_FILE` in place. Percent rollouts (`percent: 0–100`) and `enabled_for` targeting use the gRPC caller identity (`x-caller-id` metadata).

Default ports: gRPC `:9402`, HTTP health `:9404`.

## Breaking Changes

This is a pre-1.0 module. Interfaces may change without notice.
