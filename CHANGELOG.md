# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Initial project scaffold from muxcore-module-starter
- YAML file-backed `feature.flags` provider (`IsEnabled` / `GetVariant`)
- SIGHUP hot-reload of the flags file
- gRPC listen default `:9402`, HTTP `/health` on `:9403`
- `FEATURE_FLAGS_FILE` env override (default `flags.yaml`)
