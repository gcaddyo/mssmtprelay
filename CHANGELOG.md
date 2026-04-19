# Changelog

## Unreleased

### Added
- Release metadata command: `relayctl version`.
- `Makefile` release workflow for cross-platform binaries and checksums.
- `scripts/license_audit.sh` for GPL-family detection.
- Licensing docs: `LICENSE`, `THIRD_PARTY_LICENSES.md`, `RELEASE.md`.

### Changed
- Docker build now supports release metadata args (`VERSION`, `COMMIT`, `BUILD_DATE`).
- Compose build args propagate release metadata.
- Insecure auth mode (`ALLOW_INSECURE_AUTH`) remains supported for private deployments.

