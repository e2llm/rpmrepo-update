# rpmrepo-update

A fast, incremental RPM repository metadata updater written in Go. Add or remove packages from YUM/DNF repositories without downloading the entire repo.

## Features

- **Incremental updates** - Add or remove RPMs without syncing entire repositories
- **Multiple backends** - Supports local filesystem and S3 (including S3-compatible storage like MinIO)
- **Atomic operations** - Maintains repository consistency with ETag-based conflict detection for S3
- **GPG signing** - Sign repository metadata and RPM packages
- **Checksum support** - SHA-256 and SHA-512 checksums
- **Dry-run mode** - Preview changes before applying them

## Installation

```bash
go install github.com/e2llm/rpmrepo-update/cmd/rpmrepo-update@latest
```

Or build from source:

```bash
git clone https://github.com/e2llm/rpmrepo-update.git
cd rpmrepo-update
go build ./cmd/rpmrepo-update
```

## Quick Start

### Initialize a new repository

```bash
rpmrepo-update --backend fs --repo-root /path/to/repo init --checksum sha256
```

### Add RPM packages

```bash
rpmrepo-update --backend fs --repo-root /path/to/repo add package.rpm
```

### Remove packages

```bash
rpmrepo-update --backend fs --repo-root /path/to/repo remove package.rpm --delete-files
```

### Validate repository

```bash
rpmrepo-update --backend fs --repo-root /path/to/repo check
```

## S3 Backend

Works with AWS S3 and S3-compatible storage (MinIO, etc.):

```bash
export AWS_ACCESS_KEY_ID=your-key
export AWS_SECRET_ACCESS_KEY=your-secret

# Initialize repo on S3
rpmrepo-update \
  --backend s3 \
  --repo-root s3://bucket/prefix \
  init --checksum sha256

# Add package to S3-hosted repo
rpmrepo-update \
  --backend s3 \
  --repo-root s3://bucket/prefix \
  add package.rpm

# Use custom S3 endpoint (MinIO)
rpmrepo-update \
  --backend s3 \
  --s3-endpoint https://minio.example.com \
  --repo-root s3://bucket/prefix \
  add package.rpm
```

## GPG Signing

```bash
# Sign repository metadata
rpmrepo-update \
  --backend fs --repo-root /path/to/repo \
  --sign-repodata --gpg-key KEYID \
  add package.rpm

# Re-sign RPMs before adding
rpmrepo-update \
  --backend fs --repo-root /path/to/repo \
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
