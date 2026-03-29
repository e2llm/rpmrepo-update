package repo

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/e2llm/rpmrepo-update/pkg/backend"
	"github.com/e2llm/rpmrepo-update/pkg/metadata"
)

// findTestRPM locates a real RPM for testing (from system cache or packages).
func findTestRPM(t *testing.T) string {
	t.Helper()
	candidates := []string{
		"/var/cache/PackageKit/10/metadata/epel-10-x86_64/packages",
		"/var/cache/dnf",
	}
	for _, dir := range candidates {
		matches, _ := filepath.Glob(filepath.Join(dir, "*.rpm"))
		if len(matches) > 0 {
			return matches[0]
		}
		// Check one level deeper
		matches, _ = filepath.Glob(filepath.Join(dir, "*", "*.rpm"))
		if len(matches) > 0 {
			return matches[0]
		}
	}
	t.Skip("no RPM files found on system for integration test")
	return ""
}

// TestInitCheckEmpty tests init + check on an empty repo.
func TestInitCheckEmpty(t *testing.T) {
	dir := t.TempDir()
	b := backend.NewFSBackend(dir)
	r := New(b)

	ctx := context.Background()

	// Init
	if err := r.InitRepo(ctx, "sha256", false); err != nil {
		t.Fatalf("InitRepo: %v", err)
	}

	// repomd.xml should exist
	exists, err := b.Exists(ctx, "repodata/repomd.xml")
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if !exists {
		t.Fatal("expected repodata/repomd.xml to exist")
	}

	// Check should pass
	result := r.CheckDetailed(ctx)
	if result.Err != nil {
		t.Fatalf("Check on empty repo: %v", result.Err)
	}
}

// TestInitForceOverwrite tests that --force overwrites existing repo.
func TestInitForceOverwrite(t *testing.T) {
	dir := t.TempDir()
	b := backend.NewFSBackend(dir)
	r := New(b)
	ctx := context.Background()

	if err := r.InitRepo(ctx, "sha256", false); err != nil {
		t.Fatalf("InitRepo: %v", err)
	}

	// Init again without force should fail
	if err := r.InitRepo(ctx, "sha256", false); err == nil {
		t.Fatal("expected error on second init without force")
	}

	// Init with force should succeed
	if err := r.InitRepo(ctx, "sha256", true); err != nil {
		t.Fatalf("InitRepo with force: %v", err)
	}
}

// TestInitSHA512 tests init with SHA-512 checksum.
func TestInitSHA512(t *testing.T) {
	dir := t.TempDir()
	b := backend.NewFSBackend(dir)
	r := New(b)
	ctx := context.Background()

	if err := r.InitRepo(ctx, "sha512", false); err != nil {
		t.Fatalf("InitRepo sha512: %v", err)
	}

	result := r.CheckDetailed(ctx)
	if result.Err != nil {
		t.Fatalf("Check: %v", result.Err)
	}
}

// TestFullCycleWithRealRPM tests init -> add -> check -> remove -> check.
func TestFullCycleWithRealRPM(t *testing.T) {
	rpmPath := findTestRPM(t)

	dir := t.TempDir()
	b := backend.NewFSBackend(dir)
	r := New(b)
	r.AllowUnknown = true
	ctx := context.Background()

	// Init
	if err := r.InitRepo(ctx, "sha256", false); err != nil {
		t.Fatalf("InitRepo: %v", err)
	}

	// Add RPM
	if err := r.AddRPMs(ctx, []string{rpmPath}, false, false, false); err != nil {
		t.Fatalf("AddRPMs: %v", err)
	}

	// Check should pass with 1 package
	result := r.CheckDetailed(ctx)
	if result.Err != nil {
		t.Fatalf("Check after add: %v", result.Err)
	}

	// Verify the RPM file was written
	rpmBase := filepath.Base(rpmPath)
	exists, err := b.Exists(ctx, rpmBase)
	if err != nil {
		t.Fatalf("Exists rpm: %v", err)
	}
	if !exists {
		t.Fatalf("expected RPM %s in repo", rpmBase)
	}

	// Load packages to verify count
	_, pkgs, _, err := r.loadPackages(ctx)
	if err != nil {
		t.Fatalf("loadPackages: %v", err)
	}
	if len(pkgs) != 1 {
		t.Fatalf("expected 1 package, got %d", len(pkgs))
	}

	// Remove by filename
	if err := r.RemoveRPMs(ctx, []string{rpmBase}, false, true, false); err != nil {
		t.Fatalf("RemoveRPMs: %v", err)
	}

	// Check should pass with 0 packages
	result = r.CheckDetailed(ctx)
	if result.Err != nil {
		t.Fatalf("Check after remove: %v", result.Err)
	}

	_, pkgs, _, err = r.loadPackages(ctx)
	if err != nil {
		t.Fatalf("loadPackages after remove: %v", err)
	}
	if len(pkgs) != 0 {
		t.Fatalf("expected 0 packages, got %d", len(pkgs))
	}
}

// TestAddDryRun verifies dry-run doesn't write anything.
func TestAddDryRun(t *testing.T) {
	rpmPath := findTestRPM(t)

	dir := t.TempDir()
	b := backend.NewFSBackend(dir)
	r := New(b)
	ctx := context.Background()

	if err := r.InitRepo(ctx, "sha256", false); err != nil {
		t.Fatalf("InitRepo: %v", err)
	}

	// Read repomd before dry-run
	repomdBefore, err := b.ReadFile(ctx, "repodata/repomd.xml")
	if err != nil {
		t.Fatalf("ReadFile repomd: %v", err)
	}

	// Add with dry-run
	if err := r.AddRPMs(ctx, []string{rpmPath}, false, true, false); err != nil {
		t.Fatalf("AddRPMs dry-run: %v", err)
	}

	// repomd should be unchanged
	repomdAfter, err := b.ReadFile(ctx, "repodata/repomd.xml")
	if err != nil {
		t.Fatalf("ReadFile repomd after: %v", err)
	}
	if string(repomdBefore) != string(repomdAfter) {
		t.Fatal("repomd.xml changed during dry-run")
	}

	// RPM should not be in repo
	rpmBase := filepath.Base(rpmPath)
	exists, err := b.Exists(ctx, rpmBase)
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if exists {
		t.Fatal("RPM should not exist after dry-run")
	}
}

// TestAddDuplicateError verifies duplicate detection.
func TestAddDuplicateError(t *testing.T) {
	rpmPath := findTestRPM(t)

	dir := t.TempDir()
	b := backend.NewFSBackend(dir)
	r := New(b)
	ctx := context.Background()

	if err := r.InitRepo(ctx, "sha256", false); err != nil {
		t.Fatalf("InitRepo: %v", err)
	}
	if err := r.AddRPMs(ctx, []string{rpmPath}, false, false, false); err != nil {
		t.Fatalf("AddRPMs: %v", err)
	}

	// Adding same RPM again without replace should error
	err := r.AddRPMs(ctx, []string{rpmPath}, false, false, false)
	if err == nil {
		t.Fatal("expected duplicate error")
	}
}

// TestAddReplaceExisting verifies replace-existing flag.
func TestAddReplaceExisting(t *testing.T) {
	rpmPath := findTestRPM(t)

	dir := t.TempDir()
	b := backend.NewFSBackend(dir)
	r := New(b)
	ctx := context.Background()

	if err := r.InitRepo(ctx, "sha256", false); err != nil {
		t.Fatalf("InitRepo: %v", err)
	}
	if err := r.AddRPMs(ctx, []string{rpmPath}, false, false, false); err != nil {
		t.Fatalf("AddRPMs: %v", err)
	}

	// Replace should succeed
	if err := r.AddRPMs(ctx, []string{rpmPath}, true, false, false); err != nil {
		t.Fatalf("AddRPMs replace: %v", err)
	}

	// Still 1 package
	_, pkgs, _, err := r.loadPackages(ctx)
	if err != nil {
		t.Fatalf("loadPackages: %v", err)
	}
	if len(pkgs) != 1 {
		t.Fatalf("expected 1 package, got %d", len(pkgs))
	}
}

// TestRemoveDryRun verifies remove dry-run.
func TestRemoveDryRun(t *testing.T) {
	rpmPath := findTestRPM(t)

	dir := t.TempDir()
	b := backend.NewFSBackend(dir)
	r := New(b)
	ctx := context.Background()

	if err := r.InitRepo(ctx, "sha256", false); err != nil {
		t.Fatalf("InitRepo: %v", err)
	}
	if err := r.AddRPMs(ctx, []string{rpmPath}, false, false, false); err != nil {
		t.Fatalf("AddRPMs: %v", err)
	}

	rpmBase := filepath.Base(rpmPath)

	// Remove with dry-run
	if err := r.RemoveRPMs(ctx, []string{rpmBase}, false, true, true); err != nil {
		t.Fatalf("RemoveRPMs dry-run: %v", err)
	}

	// RPM should still exist
	exists, err := b.Exists(ctx, rpmBase)
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if !exists {
		t.Fatal("RPM should still exist after dry-run remove")
	}

	// Package should still be in metadata
	_, pkgs, _, err := r.loadPackages(ctx)
	if err != nil {
		t.Fatalf("loadPackages: %v", err)
	}
	if len(pkgs) != 1 {
		t.Fatalf("expected 1 package after dry-run, got %d", len(pkgs))
	}
}

// TestRemoveByNEVRA tests removal by NEVRA identifier.
func TestRemoveByNEVRA(t *testing.T) {
	rpmPath := findTestRPM(t)

	dir := t.TempDir()
	b := backend.NewFSBackend(dir)
	r := New(b)
	ctx := context.Background()

	if err := r.InitRepo(ctx, "sha256", false); err != nil {
		t.Fatalf("InitRepo: %v", err)
	}
	if err := r.AddRPMs(ctx, []string{rpmPath}, false, false, false); err != nil {
		t.Fatalf("AddRPMs: %v", err)
	}

	_, pkgs, _, err := r.loadPackages(ctx)
	if err != nil {
		t.Fatalf("loadPackages: %v", err)
	}
	if len(pkgs) != 1 {
		t.Fatalf("expected 1 package, got %d", len(pkgs))
	}

	nevra := pkgs[0].NEVRA()

	// Remove by NEVRA
	if err := r.RemoveRPMs(ctx, []string{nevra}, true, false, false); err != nil {
		t.Fatalf("RemoveRPMs by NEVRA: %v", err)
	}

	_, pkgs, _, err = r.loadPackages(ctx)
	if err != nil {
		t.Fatalf("loadPackages after remove: %v", err)
	}
	if len(pkgs) != 0 {
		t.Fatalf("expected 0 packages, got %d", len(pkgs))
	}
}

// TestRemoveNotFound tests error on missing package.
func TestRemoveNotFound(t *testing.T) {
	dir := t.TempDir()
	b := backend.NewFSBackend(dir)
	r := New(b)
	ctx := context.Background()

	if err := r.InitRepo(ctx, "sha256", false); err != nil {
		t.Fatalf("InitRepo: %v", err)
	}

	err := r.RemoveRPMs(ctx, []string{"nonexistent.rpm"}, false, false, false)
	if err == nil {
		t.Fatal("expected error for nonexistent package")
	}
}

// TestCheckDetectsOrphanRPM verifies check catches unreferenced RPMs.
func TestCheckDetectsOrphanRPM(t *testing.T) {
	rpmPath := findTestRPM(t)

	dir := t.TempDir()
	b := backend.NewFSBackend(dir)
	r := New(b)
	ctx := context.Background()

	if err := r.InitRepo(ctx, "sha256", false); err != nil {
		t.Fatalf("InitRepo: %v", err)
	}

	// Add a real RPM so check enters the RPM-listing path
	if err := r.AddRPMs(ctx, []string{rpmPath}, false, false, false); err != nil {
		t.Fatalf("AddRPMs: %v", err)
	}

	// Drop an orphan RPM file directly
	if err := b.WriteFile(ctx, "orphan-1.0-1.x86_64.rpm", []byte("fake")); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	result := r.CheckDetailed(ctx)
	if result.Err == nil {
		t.Fatal("expected error about orphan RPM")
	}
}

// TestCheckDetectsMissingRPM verifies check catches missing RPMs referenced in metadata.
func TestCheckDetectsMissingRPM(t *testing.T) {
	rpmPath := findTestRPM(t)

	dir := t.TempDir()
	b := backend.NewFSBackend(dir)
	r := New(b)
	ctx := context.Background()

	if err := r.InitRepo(ctx, "sha256", false); err != nil {
		t.Fatalf("InitRepo: %v", err)
	}
	if err := r.AddRPMs(ctx, []string{rpmPath}, false, false, false); err != nil {
		t.Fatalf("AddRPMs: %v", err)
	}

	// Delete the RPM file but keep metadata
	rpmBase := filepath.Base(rpmPath)
	if err := os.Remove(filepath.Join(dir, rpmBase)); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	result := r.CheckDetailed(ctx)
	if result.Err == nil {
		t.Fatal("expected error about missing RPM")
	}
}

// TestAddWithDestPrefix tests adding RPMs with a destination prefix.
func TestAddWithDestPrefix(t *testing.T) {
	rpmPath := findTestRPM(t)

	dir := t.TempDir()
	b := backend.NewFSBackend(dir)
	r := New(b)
	r.DestPrefix = "packages"
	ctx := context.Background()

	if err := r.InitRepo(ctx, "sha256", false); err != nil {
		t.Fatalf("InitRepo: %v", err)
	}
	if err := r.AddRPMs(ctx, []string{rpmPath}, false, false, false); err != nil {
		t.Fatalf("AddRPMs: %v", err)
	}

	// RPM should be under packages/
	rpmBase := filepath.Base(rpmPath)
	exists, err := b.Exists(ctx, "packages/"+rpmBase)
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if !exists {
		t.Fatalf("expected RPM at packages/%s", rpmBase)
	}
}

// TestCheckWithWarnings verifies check returns warnings for unknown metadata types.
func TestCheckWithWarnings(t *testing.T) {
	dir := t.TempDir()
	b := backend.NewFSBackend(dir)
	r := New(b)
	r.AllowUnknown = true
	ctx := context.Background()

	if err := r.InitRepo(ctx, "sha256", false); err != nil {
		t.Fatalf("InitRepo: %v", err)
	}

	// Inject an unknown metadata type into repomd.xml
	repomdBytes, err := b.ReadFile(ctx, "repodata/repomd.xml")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	md, err := metadata.ParseRepoMD(repomdBytes)
	if err != nil {
		t.Fatalf("ParseRepoMD: %v", err)
	}
	md.Data = append(md.Data, metadata.RepoData{Type: "productid"})
	newRepomd, err := metadata.MarshalRepoMD(md)
	if err != nil {
		t.Fatalf("MarshalRepoMD: %v", err)
	}
	if err := b.WriteFile(ctx, "repodata/repomd.xml", newRepomd); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	result := r.CheckDetailed(ctx)
	if len(result.Warnings) == 0 {
		t.Fatal("expected warnings for unknown metadata type")
	}
}
