# Changelog

## v1.3.1

- Security (Go stdlib): rebuilt on Go 1.25.11 — fixes `crypto/x509` quadratic hostname verification (CVE-2026-27145) and `net/textproto` error injection (CVE-2026-42507)
- Security (deps): bump `golang.org/x/crypto` v0.49.0 -> v0.52.0
- No functional changes

## v1.3.0

- Sign repomd.xml on `add` and `remove` when `--sign-repodata` is set (contribution by Jared Hamlin, #4)
- Fix CVE-2025-22869: bump golang.org/x/crypto to v0.49.0
- S3: migrate from deprecated `feature/s3/manager` to `feature/s3/transfermanager`
- Build: Go 1.25

## v1.2.1

- FS backend: Create files with 0644 permissions instead of 0600 (fixes #2)

## v1.2.0

- Build: Add Windows binary (amd64)
- S3: Add `--s3-region` flag to override AWS region
- S3: Add `--s3-disable-etag` flag for R2 and other S3-compatible storage without full ETag support

Note: On Windows, `--sign-rpms` is not available (requires rpmsign). `--sign-repodata` requires GPG4Win.

## v1.1.0

- S3: Use Upload Manager for multipart upload support (fixes large RPM uploads to MinIO)
- S3: Clean up temp files after successful copy
- Build: Static binaries (CGO_ENABLED=0) for Alpine/musl compatibility

## v1.0.0

- Initial release
- Backends: filesystem (`fs`) and S3 (`s3`) with MinIO support
- Commands: `init`, `add`, `remove`, `check`
- S3 atomic updates via ETag-based conflict detection
- GPG signing support for repodata and RPMs
- SHA-256 and SHA-512 checksum support
- Multi-arch binaries: linux/amd64, linux/arm64, darwin/amd64, darwin/arm64
