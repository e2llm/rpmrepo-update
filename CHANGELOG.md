# Changelog

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
