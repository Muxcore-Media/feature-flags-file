# Changelog

## [0.1.3] — 2026-08-10

### Fixed

- Self-hosted CI (`runs-on: self-hosted`; `go test` without `-race` for laptop runners)

### Changed

- `muxcore.json` declares `FeatureFlagProvider` contract and `minCoreVersion` **0.5.0**

## [0.1.2] — 2026-08-10

### Added

- Advertise `settings` capability so admin-ui discovers SettingsProvider without ListAll probing.

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0/).

## [Unreleased]

## [0.1.4] - 2026-10-05

### Changed
- CI runs on GitHub-hosted runners from the umbrella template; retired-origin workflows removed.
- Dependencies resolve from published GitHub tags (no filesystem `replace`); requires core v0.6.0.

## [0.1.1] — 2026-08-10

### Added

- `RegisterSettings` / `SettingsUpdater` for live `flags_file` (`FEATURE_FLAGS_FILE`) with immediate YAML reload
- Pin `core` / contracts / `sdk/go/module` to **v0.5.2**

## [0.1.0] — 2026-08-09

### Added

- YAML file-backed `feature.flags` provider (`IsEnabled` / `GetVariant`) with request default fallbacks
- SIGHUP hot-reload of the flags file
- gRPC listen default `:9402`, HTTP `/health` on `:9404` (avoids auth-local `:9403`)
- `FEATURE_FLAGS_FILE` env override (default `flags.yaml`)
- Unit tests: YAML load, defaults, env override, reload, SIGHUP
