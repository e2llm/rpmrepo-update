# Changelog

## v1.2.1

- FS backend: Create files with 0644 permissions instead of 0600 (#2)

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
