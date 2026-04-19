# Release Guide

This document describes how to produce distributable release artifacts.

## 1. Pre-release checks

```bash
make test
./scripts/license_audit.sh
```

## 2. Build cross-platform binaries

```bash
make release VERSION=v1.0.0
```

Generated artifacts are placed in `dist/`:

- `relayctl-linux-amd64.tar.gz`
- `relayctl-linux-arm64.tar.gz`
- `relayctl-darwin-amd64.tar.gz`
- `relayctl-darwin-arm64.tar.gz`
- `*.sha256`

## 3. Build Docker image with metadata

```bash
VERSION=v1.0.0 \
COMMIT=$(git rev-parse --short HEAD) \
BUILD_DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ) \
docker compose build app
```

The runtime image includes OCI labels for version/revision/date.

## 4. Verify binary version

```bash
./relayctl version
```

or in container:

```bash
docker compose run --rm app version
```

## 5. Publish checklist

- [ ] `LICENSE` included
- [ ] `THIRD_PARTY_LICENSES.md` reviewed
- [ ] `README.md` updated for user-facing changes
- [ ] release notes/changelog prepared

