# rpmrepo-update

[![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?logo=go)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

A fast, incremental RPM repository metadata updater written in Go. Add or remove packages from YUM/DNF repositories without downloading the entire repo.

## Why?

Traditional `createrepo` requires downloading the entire repository metadata to add a single package. For large repositories on S3/MinIO, this means:
- Slow CI pipelines (minutes instead of seconds)
- High bandwidth costs
- Race conditions with parallel builds

**rpmrepo-update** solves this with incremental updates and atomic S3 operations.

## Quick Example

```bash
# Add new RPM to S3-hosted repo — no full sync needed
rpmrepo-update --backend s3 --repo-root s3://my-bucket/repo add ./my-package-1.0.0.rpm
```

## Features

- **Incremental updates** - Add or remove RPMs without syncing entire repositories
- **Multiple backends** - Supports local filesystem and S3 (including S3-compatible storage like MinIO)
- **Atomic operations** - Maintains repository consistency with ETag-based conflict detection for S3
- **GPG signing** - Sign repository metadata and RPM packages
- **Checksum support** - SHA-256 and SHA-512 checksums
- **Dry-run mode** - Preview changes before applying them

## Comparison with createrepo

| Feature | createrepo_c | rpmrepo-update |
|---------|--------------|----------------|
| Downloads entire repo | Yes | No |
| Incremental updates | No | Yes |
| Native S3 support | No | Yes |
| Atomic operations (ETag) | No | Yes |
| Parallel CI safe | No | Yes |
| Time for 1 package | O(repo size) | O(1) |

## Installation

### Pre-built Binaries (Recommended)

Download the latest release:

```bash
# Linux (amd64)
curl -L https://github.com/e2llm/rpmrepo-update/releases/latest/download/rpmrepo-update-linux-amd64 -o rpmrepo-update
chmod +x rpmrepo-update
sudo mv rpmrepo-update /usr/local/bin/

# Linux (arm64)
curl -L https://github.com/e2llm/rpmrepo-update/releases/latest/download/rpmrepo-update-linux-arm64 -o rpmrepo-update
chmod +x rpmrepo-update
sudo mv rpmrepo-update /usr/local/bin/

# macOS (Apple Silicon)
curl -L https://github.com/e2llm/rpmrepo-update/releases/latest/download/rpmrepo-update-darwin-arm64 -o rpmrepo-update
chmod +x rpmrepo-update
sudo mv rpmrepo-update /usr/local/bin/
```

Or download a specific version from [Releases](https://github.com/e2llm/rpmrepo-update/releases).

### From Source

Requires Go 1.21+:

```bash
go install github.com/e2llm/rpmrepo-update/cmd/rpmrepo-update@latest
```

## Quick Start

### Initialize a New Repository

```bash
# Local filesystem
rpmrepo-update --backend fs --repo-root /var/www/yum/myrepo init --checksum sha256

# S3/MinIO
rpmrepo-update --backend s3 --repo-root s3://packages/myrepo init --checksum sha256
```

### Add RPM Packages

```bash
rpmrepo-update --backend s3 --repo-root s3://packages/myrepo add ./myapp-1.0.0-1.el9.x86_64.rpm
```

### Remove Packages

```bash
rpmrepo-update --backend s3 --repo-root s3://packages/myrepo remove myapp-1.0.0-1.el9.x86_64.rpm --delete-files
```

### Validate Repository

```bash
rpmrepo-update --backend s3 --repo-root s3://packages/myrepo check
```

## Real-World CI/CD Example

A typical setup: build RPMs in CI, publish to S3-hosted YUM repo, clients install via `dnf`.

```
┌─────────────────┐     ┌──────────────────┐     ┌─────────────────┐
│   CI Pipeline   │────▶│  rpmrepo-update  │────▶│  S3 / MinIO     │
│   (build RPM)   │     │  (update repo)   │     │  (yum repo)     │
└─────────────────┘     └──────────────────┘     └────────┬────────┘
                                                          │
                                                          ▼
                                                 ┌─────────────────┐
                                                 │   YUM Clients   │
                                                 │  (dnf install)  │
                                                 └─────────────────┘
```

### Repository Structure

```
s3://packages/myapp/el9/x86_64/
├── myapp-2.0.0-1.el9.x86_64.rpm
├── myapp-1.9.0-1.el9.x86_64.rpm
└── repodata/
    ├── repomd.xml
    ├── <checksum>-primary.xml.gz
    ├── <checksum>-filelists.xml.gz
    └── <checksum>-other.xml.gz
```

### GitLab CI Example

```yaml
stages:
  - build
  - publish

build-rpm:
  stage: build
  image: rockylinux/rockylinux:9
  script:
    - dnf install -y rpm-build rpmdevtools
    - rpmbuild -bb myapp.spec
  artifacts:
    paths:
      - ~/rpmbuild/RPMS/**/*.rpm

publish-yum:
  stage: publish
  image: golang:1.23-alpine
  needs:
    - build-rpm
  variables:
    REPO_ROOT: "s3://packages/myapp/el9/x86_64"
  script:
    - go install github.com/e2llm/rpmrepo-update/cmd/rpmrepo-update@latest
    - |
      for rpm in ~/rpmbuild/RPMS/**/*.rpm; do
        rpmrepo-update --backend s3 \
          --repo-root "$REPO_ROOT" \
          add "$rpm" --replace-existing
      done
  rules:
    - if: '$CI_COMMIT_TAG =~ /^v[0-9]+/'
```

### GitHub Actions Example

```yaml
publish-rpm:
  runs-on: ubuntu-latest
  steps:
    - uses: actions/checkout@v4

    - name: Download RPM artifact
      uses: actions/download-artifact@v4
      with:
        name: rpm-package

    - name: Set up Go
      uses: actions/setup-go@v5
      with:
        go-version: '1.23'

    - name: Publish to YUM repo
      env:
        AWS_ACCESS_KEY_ID: ${{ secrets.S3_ACCESS_KEY }}
        AWS_SECRET_ACCESS_KEY: ${{ secrets.S3_SECRET_KEY }}
      run: |
        go install github.com/e2llm/rpmrepo-update/cmd/rpmrepo-update@latest
        rpmrepo-update --backend s3 \
          --s3-endpoint https://s3.example.com \
          --repo-root s3://packages/myapp/el9/x86_64 \
          add ./*.rpm --replace-existing
```

### Client Configuration

Create `/etc/yum.repos.d/myapp.repo`:

```ini
[myapp]
name=My Application
baseurl=https://s3.example.com/packages/myapp/el9/$basearch
enabled=1
gpgcheck=0
```

Then install:

```bash
dnf install myapp
```

## S3 Backend

Works with AWS S3 and S3-compatible storage (MinIO, etc.):

```bash
export AWS_ACCESS_KEY_ID=your-key
export AWS_SECRET_ACCESS_KEY=your-secret

# AWS S3
rpmrepo-update --backend s3 \
  --repo-root s3://my-bucket/yum/el9/x86_64 \
  add package.rpm

# MinIO or other S3-compatible
rpmrepo-update --backend s3 \
  --s3-endpoint https://minio.example.com \
  --repo-root s3://packages/el9/x86_64 \
  add package.rpm
```

## Atomicity & Conflict Handling

### What is atomic:
- Single package add/remove — metadata update is atomic via S3 ETag
- `repomd.xml` is written last, after all other files

### Conflict detection:
- Uses S3 ETag (If-Match) for optimistic locking
- Parallel updates to same repo will fail-fast with conflict error
- Safe to retry — no partial state

### Recommended CI pattern for high concurrency:

```bash
rpmrepo-update --backend s3 --repo-root s3://bucket/repo add pkg.rpm || {
  echo "Conflict detected, retrying..."
  sleep $((RANDOM % 5))
  rpmrepo-update --backend s3 --repo-root s3://bucket/repo add pkg.rpm
}
```

## GPG Signing

```bash
# Sign repository metadata
rpmrepo-update \
  --backend s3 --repo-root s3://packages/repo \
  --sign-repodata --gpg-key KEYID \
  add package.rpm

# Re-sign RPMs before adding
rpmrepo-update \
  --backend s3 --repo-root s3://packages/repo \
  --sign-rpms --gpg-key KEYID \
  add package.rpm
```

## Command Reference

### Global Flags

| Flag | Description |
|------|-------------|
| `--backend` | Backend type: `fs` (filesystem) or `s3` |
| `--repo-root` | Repository root path or S3 URI |
| `--s3-endpoint` | Custom S3 endpoint URL (for MinIO, etc.) |
| `--log-level` | Log level: `error`, `info`, `debug` |
| `--output` | Output format: `text`, `json` |
| `--sign-repodata` | Sign repomd.xml with GPG |
| `--sign-rpms` | Re-sign RPMs before adding |
| `--gpg-key` | GPG key ID for signing |

### Commands

#### `init`
Create an empty repository.
```bash
rpmrepo-update init [--checksum sha256|sha512] [--force]
```

#### `add`
Add RPM packages to the repository.
```bash
rpmrepo-update add <rpm-files...> [--replace-existing] [--dry-run] [--dest-prefix path]
```

#### `remove`
Remove packages from the repository.
```bash
rpmrepo-update remove <identifiers...> [--by-nevra] [--delete-files] [--dry-run]
```

#### `check`
Validate repository integrity.
```bash
rpmrepo-update check [--output json]
```

## Requirements

- Go 1.21 or later
- For GPG signing: `gpg` command available in PATH
- For RPM re-signing: `rpmsign` command available in PATH

## License

MIT License - see [LICENSE](LICENSE) for details.
