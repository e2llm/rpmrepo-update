**Title**
Incremental RPM Repository Metadata Updater

---

## 1. Problem Statement

Current situation:

* YUM/DNF repositories are maintained with `createrepo` / `createrepo_c`.
* These tools operate on a local directory view: all `*.rpm` files plus existing `repodata/`.
* CI pipelines typically build one RPM per job and store artifacts in a remote backend (e.g. S3).
* To add a single new RPM to a repo stored on S3, the naïve pattern is:

  * Sync entire repo from S3 to CI worker
  * Run `createrepo_c --update`
  * Sync entire repo back to S3

This does not scale: bandwidth and time grow with repo size, even for small incremental changes.

**Need:**

A tool that can incrementally update YUM/DNF repository metadata (add/remove packages) by manipulating metadata files directly, without needing a full local copy of all RPMs.

---

## 2. Goals and Non-Goals

### 2.1 Goals

1. Add one or more new RPMs to an existing YUM/DNF repo by updating metadata incrementally.
2. Remove RPMs from a repo (metadata, and optionally the RPM files themselves).
3. Minimize data transfer when the repo is stored remotely (e.g., S3).
4. Keep the repo in a consistent state from DNF/YUM’s perspective at all times.
5. Provide:

   * A CLI (`rpmrepo-update`, name TBD).
   * A library API for embedding in other tools.

### 2.2 Non-goals

* Not re-implementing `createrepo_c` wholesale.
* Not managing every metadata extension from day one (delta, modules, etc. are handled in a limited way).
* Not building a full “repo manager” (no ACLs, UI, scheduling, etc.).

---

## 3. Assumptions and Constraints

* Standard YUM/DNF repo layout:

  ```text
  /repo/
    *.rpm
    repodata/
      repomd.xml
      primary.xml.gz or primary.sqlite.bz2
      filelists.xml.gz or filelists.sqlite.bz2
      other.xml.gz or other.sqlite.bz2
      (optional: prestodelta, modules.yaml, other extras)
  ```

* Clients must always see a **consistent snapshot**:

  * Metadata files referenced by `repomd.xml` must match the checksums and sizes listed there.
  * Updates must be atomic from the client’s POV: they either see the old state or the new state, not a partial mix.

* Tool is allowed to:

  * Download and re-upload metadata files in `repodata/`.
  * Download new RPM(s) being added (to read headers).
  * Avoid downloading existing RPMs just to update metadata.

* Target environment:

  * Linux.
  * Language: Python or Go (implementation decision; spec is language-agnostic).
  * Access to:

    * Local filesystem.
    * S3 (initial remote backend).
  * RPM header parsing must be done via library bindings, not fragile text parsing.

---

## 4. High-Level Design

### 4.1 Components

1. **Backend abstraction**

   Abstracts over where the repo lives:

   ```python
   class RepoBackend:
       def list_repodata(self) -> list[str]          # paths under repodata/
       def read_file(self, path: str) -> bytes
       def write_file(self, path: str, data: bytes) -> None
       def exists(self, path: str) -> bool
       def list_rpms(self) -> list[str]             # paths of *.rpm under repo root
   ```

   Initial implementations:

   * `FSBackend` – local filesystem (`/srv/yum/repo`).
   * `S3Backend` – S3 bucket/prefix (`s3://bucket/prefix`).

2. **Metadata handler**

   Responsibilities:

   * Read, parse, and write:

     * `repodata/repomd.xml`
     * `primary.xml.gz`
     * `filelists.xml.gz`
     * `other.xml.gz`
   * v1: **XML-only** for core types (primary/filelists/other). sqlite is not supported in v1.
   * Preserve checksum algorithms and non-core metadata entries.

3. **RPM inspector**

   * Given an RPM file (local path or bytes), extract:

     * NEVRA (name, epoch, version, release, arch)
     * Summary, description
     * Provides, requires, conflicts, obsoletes
     * File list
     * Changelog entries (for `other.xml`)

   * Must use proper RPM libraries:

     * Python: `rpm` module (`python3-rpm`).
     * Go: library such as `github.com/cavaliergopher/rpm`.

   * No parsing of `rpm -qp` text output in v1.

4. **Update engine**

   * Core operations:

     * `add_rpms(rpm_paths: list[str], replace_existing: bool = False)`
     * `remove_rpms(identifiers: list[str], delete_files: bool = False)`

   * Uses metadata handler + backend to:

     * Load current metadata.
     * Apply changes in-memory.
     * Serialize, compress, and write updated metadata.
     * Update `repomd.xml` entries and checksums.

5. **CLI front-end**

   * Binary name: `rpmrepo-update` (placeholder).
   * Subcommands:

     * `init`
     * `add`
     * `remove`
     * `check`

6. **Library API**

   * Thin wrapper around backend + update engine:

     ```python
     backend = S3Backend(bucket="my-yum-repo", prefix="centos/9/x86_64")
     repo = Repo(backend)

     repo.init_repo(checksum="sha256")
     repo.add_rpms(["build/foo-1.2.3-1.x86_64.rpm"], replace_existing=True)
     repo.remove_rpms(["foo-1.2.3-1.x86_64.rpm"], delete_files=False)
     ```

---

## 5. Detailed Behavior

### 5.1 Repo Layout

* Repo root is a directory or S3 prefix that contains:

  * RPM files: `*.rpm`
  * Metadata directory: `repodata/` with `repomd.xml` and metadata files.

* Tool operates on **one repo root** at a time.

### 5.2 Metadata Types and Ownership

**Core types (owned and regenerated by the tool):**

* `primary`
* `filelists`
* `other`

On each update:

* These are fully regenerated (from their in-memory representation).
* Old versions of these files may remain on disk/S3, but only the ones referenced by `repomd.xml` are considered active. For S3, new metadata should be uploaded under temp keys/prefix and `repomd.xml` updated last (with ETag guard) to maintain atomicity.

**Delta metadata (`prestodelta`):**

* If `<data type="prestodelta">` is present in `repomd.xml`:

  * v1 behavior: **remove** `prestodelta` entries from `repomd.xml` during any update.
  * Tool does not generate or maintain `prestodelta` files.
  * Existing `prestodelta` files may remain on disk/S3 but are no longer referenced.

* Documented behavior: users who need deltas must regenerate them externally (e.g. `createrepo_c --deltas`) and re-integrate via other tooling.

**Modular metadata (`modules` / modulemd):**

* If `<data type="modules">` entries exist in `repomd.xml`:

  * v1 behavior: **preserve them unchanged**.

    * Entries are copied verbatim from the old `repomd.xml` to the new.
    * No checksum recalculation; values are reused as-is.
    * The underlying `modules.yaml` (or similar) file is not touched.

* Documented behavior: tool does not manage module streams. If packages referenced by modules are added/removed, modular metadata might become semantically stale; it’s the caller’s responsibility to maintain `modules.yaml`.

**Unknown/other `<data>` types:**

* Any `<data>` type other than `primary`, `filelists`, `other`, `prestodelta`, and `modules` is treated as **unknown**.
* v1 behavior:

  * Copy these entries verbatim from old `repomd.xml` to new `repomd.xml`.
  * Do not attempt to verify or recompute checksums for their payloads.
  * At runtime, emit a warning per type, e.g.:

    ```text
    warn: preserving unknown metadata type 'productid' from repomd.xml; checksum not verified
    ```

* Modules are preserved as-is (warning that they may become semantically stale), prestodelta entries are removed on write.

---

### 5.3 Initial Metadata Discovery

On any operation that requires existing metadata:

1. From backend, read `repodata/repomd.xml`.
2. Parse it to identify the current active metadata files for:

   * `primary`
   * `filelists`
   * `other`
3. Download and decompress the current `primary.xml.gz`, `filelists.xml.gz`, `other.xml.gz`.

Failure cases:

* `repomd.xml` missing:

  * For `add`, `remove`, `check`: error: repo not initialized.
* Core types missing:

  * Error: unsupported/incomplete repo state.

### 5.4 Checksum Algorithms

* For each `<data>` type managed by the tool (`primary`, `filelists`, `other`):

  * Detect the checksum algorithm currently used (`sha256`, `sha512`, etc.) from `repomd.xml`.
  * Preserve the same algorithm when generating new metadata.
  * Write updated `<checksum>` and `<open-checksum>` values with the same algorithm.

* The tool does not introduce new algorithms; it always reuses whatever the repo already uses, or the default chosen at `init` time.

---

### 5.5 `init` Command

Purpose: create a new empty repo.

Behavior:

* Create `repodata/` if missing.
* Create minimal `primary.xml.gz`, `filelists.xml.gz`, `other.xml.gz` representing an empty package set.
* Compute checksums and sizes for these files.
* Create `repomd.xml` referencing those three files.

Options:

* `--checksum sha256|sha512` – checksum algorithm for core types (default: `sha256`).
* Fails if `repomd.xml` already exists unless `--force` is provided.

---

### 5.6 Adding RPMs

Inputs:

* List of local RPM paths (or backend-resolvable paths) to add.
* Options:

  * `--replace-existing` – allow replacing an existing package with same NEVRA.

Steps:

1. Load current metadata (`repomd.xml` + core XML files).

2. For each RPM:

   * Fetch RPM file (full file for v1).
   * Use RPM inspector to extract header data:

     * NEVRA
     * Provides/requires/etc.
     * File lists
     * Changelog
   * Update in-memory metadata:

     * `primary`: add or replace `<package>` entry.
     * `filelists`: add or replace entry for its file list.
     * `other`: add or replace entry for changelog.
   * If `replace_existing` is false and a package with same NEVRA exists:

     * Error or skip (configurable, but default: error).

3. Serialize updated metadata:

   * Generate new XML for `primary`, `filelists`, `other`.
   * Compress to `*.xml.gz`.
   * Compute checksums and sizes using the existing algorithms.

4. Update `repomd.xml`:

   * For `primary/filelists/other`, update:

     * `location href`
     * `checksum` and `open-checksum`
     * `size` and `open-size`
     * `timestamp`
   * Remove any `prestodelta` entries.
   * Pass-through `modules` and unknown `<data>` types as described above.

5. Write changes (atomicity rules depend on backend; see S3 section).

Result: updated repo metadata including new packages.

---

### 5.7 Removing RPMs

Inputs:

* RPM identifiers:

  * File names (e.g. `foo-1.2.3-1.x86_64.rpm`).
  * Optionally NEVRA-based identifiers (`--by-nevra`).
* Options:

  * `--delete-files` – also delete RPMs from backend.

Steps:

1. Load current metadata.
2. Resolve identifiers to metadata entries:

   * Match by filename and/or NEVRA.
3. Remove corresponding entries from:

   * `primary`
   * `filelists`
   * `other`
4. Optionally remove the RPM files themselves (`delete_files`).
5. Regenerate and write metadata as in the add-flow.

---

### 5.8 `check` Command

Purpose: validate repo consistency.

Checks:

* All packages declared in `primary` have corresponding RPM files in backend.
* Optionally warn if RPM files exist that are not referenced in metadata.
* Detect obvious mismatches (e.g., duplicate NEVRA entries).

Output:

* Non-zero exit code on serious inconsistency.
* Human-readable summary of issues.

---

## 6. Backend Specifics

### 6.1 Filesystem Backend (FSBackend)

* `repo-root` is a local path, e.g. `/srv/yum/repo`.
* Reads/writes are direct file operations.
* For atomicity:

  * Write metadata to temp files (e.g. in `repodata/.tmp-<uuid>`).
  * `rename()` into final names.
  * Update `repomd.xml` last.

### 6.2 S3 Backend (S3Backend)

* `repo-root` is `s3://bucket/prefix`.
* `list_repodata` uses `ListObjectsV2` on `prefix/repodata/`.
* `read_file` / `write_file` use `GetObject` / `PutObject`.

#### 6.2.1 Concurrency model (check-then-write)

To reduce risk of concurrent updates:

1. On read:

   * `HEAD` or `GET` `repodata/repomd.xml`.
   * Store `ETag` as `etag_old`.

2. Do all metadata work locally.

3. Before write:

   * `HEAD` `repodata/repomd.xml` again.
   * If current ETag != `etag_old`:

     * Abort with a conflict error:

       * `conflict: repomd.xml changed since read; retry`

4. If ETag unchanged:

   * Upload new metadata files (`primary.xml.gz`, `filelists.xml.gz`, `other.xml.gz`) under their final keys.
   * Upload new `repomd.xml` last.

Notes:

* This is a **best-effort** concurrency guard. There is a small race window between step 3 and 4. For most CI setups with one “publisher” job, this is sufficient.
* For high-contention repos, recommend external locking:

  * DynamoDB-based lock table (acquire lock before step 1, release after step 4).
  * Or serialize updates at the CI/orchestrator level.

---

## 7. CLI Specification

Binary name: `rpmrepo-update` (placeholder).

### 7.1 Global Flags

* `--backend {fs,s3}`
* `--repo-root PATH_OR_URI`

  * fs: `/srv/yum/repo`
  * s3: `s3://bucket/prefix`
* `--log-level {error,info,debug}`

### 7.2 Commands

#### `init`

```bash
rpmrepo-update \
  --backend s3 \
  --repo-root s3://my-yum-repo/centos/9/x86_64 \
  init --checksum sha256
```

* Initializes an empty repo.
* Options:

  * `--checksum sha256|sha512` (default `sha256`).
  * `--force` – overwrite existing `repomd.xml`.

#### `add`

```bash
rpmrepo-update \
  --backend fs \
  --repo-root /srv/yum/centos/9/x86_64 \
  add build/foo-1.2.3-1.x86_64.rpm \
  --replace-existing
```

Options:

* `--replace-existing` – replace existing packages with same NEVRA.
* `--dry-run` – show planned changes, no writes.

#### `remove`

```bash
rpmrepo-update \
  --backend s3 \
  --repo-root s3://my-yum-repo/centos/9/x86_64 \
  remove foo-1.2.3-1.x86_64.rpm \
  --delete-files
```

Options:

* `--delete-files` – remove matching RPMs from backend.
* `--by-nevra NAME-EPOCH:VERSION-RELEASE.ARCH` – remove by NEVRA instead of filename.

#### `check`

```bash
rpmrepo-update \
  --backend fs \
  --repo-root /srv/yum/centos/9/x86_64 \
  check
```

* Validates repo and reports inconsistencies.

---

## 8. Library API (Sketch)

Python-style example:

```python
from rpmrepo import Repo, FSBackend, S3Backend

backend = S3Backend(bucket="my-yum-repo", prefix="centos/9/x86_64")
repo = Repo(backend)

repo.init_repo(checksum="sha256")
repo.add_rpms(["build/foo-1.2.3-1.x86_64.rpm"], replace_existing=True)
repo.remove_rpms(["foo-1.2.3-1.x86_64.rpm"], delete_files=False)
repo.check()
```

Core methods:

* `Repo.init_repo(checksum: str = "sha256")`
* `Repo.add_rpms(rpm_paths: list[str], replace_existing: bool = False)`
* `Repo.remove_rpms(identifiers: list[str], delete_files: bool = False)`
* `Repo.check()`

---

## 9. Error Handling and Logging

* Missing `repomd.xml` for non-`init` commands → non-zero exit, clear message.
* Unsupported format (sqlite-only metadata) → error: `unsupported: sqlite-only metadata in v1`.
* RPM parse failure:

  * Default: abort entire operation, report offending file.
  * Optional `--skip-broken` in future versions.
* S3 ETag mismatch → non-zero exit with conflict message.
* Unknown `<data>` types:

  * Preserved verbatim.
  * Warn once per type:

    ```text
    warn: preserving unknown metadata type '<type>' from repomd.xml; checksum not verified
    ```

---

## 10. Testing Requirements

* **Round-trip stability:**

  * Parse and re-emit `primary/filelists/other` for a repo without changes.
  * `dnf repoquery` / `dnf list` behavior remains identical.

* **Add flow:**

  * Add new RPM(s) to a repo.
  * Verify via `dnf repoquery` and `dnf install` that:

    * New packages are visible.
    * Existing packages still work.

* **Remove flow:**

  * Remove packages and confirm:

    * They no longer appear in `dnf repoquery`.
    * Remaining packages are unaffected.

* **Extra metadata:**

  * Repos with `prestodelta`: ensure entries are removed from `repomd.xml`.
  * Repos with `modules` and custom `<data>`:

    * Ensure entries are preserved and warnings emitted only for unknown types.

* **S3 backend:**

  * Validate minimal data transfer (metadata + new RPMs only).
  * Exercise conflict path by simulating concurrent modifications of `repomd.xml`.

---

## 11. Recommended Implementation Order

1. RPM inspector (library-based header parsing).
2. Metadata handler for `primary/filelists/other` (XML-only), with local fixture tests.
3. Filesystem backend + `init` + `add` command (local-only, no S3).
4. S3 backend, including ETag check-then-write logic.
5. `remove` command.
6. `check` command.
7. Extras: better logging, unknown `<data>` warnings, etc.

This document is the target behavior for v1. Further enhancements (sqlite support, range-based header reads, DynamoDB locks, delta/module management) can be layered on without changing the core contract.

---

## 12. Next Phases / Implementation Plan

1. **S3 atomic writes**
   * Upload new core metadata under temp keys/prefix.
   * Conditional `repomd.xml` write using ETag/If-Match; clean temp keys afterward.
   * Surface conflicts with clear retry guidance.

2. **Conflict handling & ergonomics**
   * Detect duplicate NEVRA in metadata before write; configurable replace behavior.
   * `--dry-run` for add/remove to show planned changes only.
   * Optional RPM destination path control (preserve existing layout vs. repo root).

3. **Check/logging improvements**
   * Severity levels (warn vs. error) and structured output (JSON) for CI.
   * Explicit module/unknown metadata warnings in CLI.

4. **Testing & fixtures**
   * Metadata round-trip stability tests; add/remove flow tests on FS.
   * Mocked S3 tests covering ETag conflicts and minimal transfer.
   * RPM fixtures for NEVRA/dep/file/changelog parsing.

5. **Packaging & ergonomics**
   * Version flag, build/install scripts, optional config/env defaults for backend/repo root.
   * Detect sqlite-only repos and emit helpful errors; hooks for future sqlite/delta/module integration.
