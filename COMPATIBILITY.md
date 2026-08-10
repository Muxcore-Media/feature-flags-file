# Compatibility

## Core Version

| Module Version | Core Version | Status |
|----------------|-------------|--------|
| v0.1.0         | v0.4.0+     | Current |

## Contracts

| Contract | Capability | Status |
|----------|-----------|--------|
| FeatureFlagsService | `feature.flags` | Current |

Unknown flags return the caller’s `default_value`. Empty `variant` falls back the same way. SIGHUP reloads `FEATURE_FLAGS_FILE` in place.

Default ports: gRPC `:9402`, HTTP health `:9404`.

## Breaking Changes

This is a pre-1.0 module. Interfaces may change without notice.
