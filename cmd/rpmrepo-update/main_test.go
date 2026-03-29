package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

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
		matches, _ = filepath.Glob(filepath.Join(dir, "*", "*.rpm"))
		if len(matches) > 0 {
			return matches[0]
		}
	}
	t.Skip("no RPM files found on system for integration test")
	return ""
}

func TestRunVersion(t *testing.T) {
	err := run(context.Background(), []string{"--version"})
	if err != nil {
		t.Fatalf("run --version: %v", err)
	}
}

func TestRunHelp(t *testing.T) {
	err := run(context.Background(), []string{"--help"})
	if err != nil {
		t.Fatalf("run --help: %v", err)
	}
}

func TestRunMissingCommand(t *testing.T) {
	err := run(context.Background(), []string{})
	if err == nil {
		t.Fatal("expected error for missing command")
	}
}

func TestRunUnknownCommand(t *testing.T) {
	err := run(context.Background(), []string{"bogus"})
	if err == nil {
		t.Fatal("expected error for unknown command")
	}
}

func TestRunInitMissingRepoRoot(t *testing.T) {
	err := run(context.Background(), []string{"init"})
	if err == nil {
		t.Fatal("expected error for missing --repo-root")
	}
}

func TestRunInit(t *testing.T) {
	dir := t.TempDir()
	err := run(context.Background(), []string{"--repo-root", dir, "init"})
	if err != nil {
		t.Fatalf("run init: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "repodata", "repomd.xml")); err != nil {
		t.Fatalf("repomd.xml not created: %v", err)
	}
}

func TestRunCheck(t *testing.T) {
	dir := t.TempDir()
	if err := run(context.Background(), []string{"--repo-root", dir, "init"}); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := run(context.Background(), []string{"--repo-root", dir, "check"}); err != nil {
		t.Fatalf("check: %v", err)
	}
}

func TestRunCheckJSON(t *testing.T) {
	dir := t.TempDir()
	if err := run(context.Background(), []string{"--repo-root", dir, "init"}); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := run(context.Background(), []string{"--repo-root", dir, "--output", "json", "check"}); err != nil {
		t.Fatalf("check json: %v", err)
	}
}

func TestRunAddRemoveCycle(t *testing.T) {
	rpmPath := findTestRPM(t)
	dir := t.TempDir()

	if err := run(context.Background(), []string{"--repo-root", dir, "init"}); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := run(context.Background(), []string{"--repo-root", dir, "add", rpmPath}); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := run(context.Background(), []string{"--repo-root", dir, "check"}); err != nil {
		t.Fatalf("check after add: %v", err)
	}

	rpmBase := filepath.Base(rpmPath)
	if err := run(context.Background(), []string{"--repo-root", dir, "remove", "--delete-files", rpmBase}); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if err := run(context.Background(), []string{"--repo-root", dir, "check"}); err != nil {
		t.Fatalf("check after remove: %v", err)
	}
}

func TestRunAddDryRun(t *testing.T) {
	rpmPath := findTestRPM(t)
	dir := t.TempDir()

	if err := run(context.Background(), []string{"--repo-root", dir, "init"}); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := run(context.Background(), []string{"--repo-root", dir, "add", "--dry-run", rpmPath}); err != nil {
		t.Fatalf("add dry-run: %v", err)
	}

	// RPM should not be in repo
	rpmBase := filepath.Base(rpmPath)
	if _, err := os.Stat(filepath.Join(dir, rpmBase)); !os.IsNotExist(err) {
		t.Fatal("RPM should not exist after dry-run")
	}
}

func TestRunRemoveDryRun(t *testing.T) {
	rpmPath := findTestRPM(t)
	dir := t.TempDir()

	if err := run(context.Background(), []string{"--repo-root", dir, "init"}); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := run(context.Background(), []string{"--repo-root", dir, "add", rpmPath}); err != nil {
		t.Fatalf("add: %v", err)
	}

	rpmBase := filepath.Base(rpmPath)
	if err := run(context.Background(), []string{"--repo-root", dir, "remove", "--dry-run", rpmBase}); err != nil {
		t.Fatalf("remove dry-run: %v", err)
	}

	// RPM should still exist
	if _, err := os.Stat(filepath.Join(dir, rpmBase)); err != nil {
		t.Fatal("RPM should still exist after dry-run remove")
	}
}

func TestRunAddMissingRepoRoot(t *testing.T) {
	err := run(context.Background(), []string{"add", "foo.rpm"})
	if err == nil {
		t.Fatal("expected error for missing --repo-root")
	}
}

func TestRunRemoveMissingRepoRoot(t *testing.T) {
	err := run(context.Background(), []string{"remove", "foo.rpm"})
	if err == nil {
		t.Fatal("expected error for missing --repo-root")
	}
}

func TestRunCheckMissingRepoRoot(t *testing.T) {
	err := run(context.Background(), []string{"check"})
	if err == nil {
		t.Fatal("expected error for missing --repo-root")
	}
}

func TestRunInvalidBackend(t *testing.T) {
	dir := t.TempDir()
	err := run(context.Background(), []string{"--backend", "nosql", "--repo-root", dir, "init"})
	if err == nil {
		t.Fatal("expected error for invalid backend")
	}
}

func TestRunInvalidLogLevel(t *testing.T) {
	dir := t.TempDir()
	err := run(context.Background(), []string{"--repo-root", dir, "--log-level", "trace", "init"})
	if err == nil {
		t.Fatal("expected error for invalid log level")
	}
}

func TestRunInvalidOnDuplicate(t *testing.T) {
	rpmPath := findTestRPM(t)
	dir := t.TempDir()

	if err := run(context.Background(), []string{"--repo-root", dir, "init"}); err != nil {
		t.Fatalf("init: %v", err)
	}
	err := run(context.Background(), []string{"--repo-root", dir, "add", "--on-duplicate", "skip", rpmPath})
	if err == nil {
		t.Fatal("expected error for invalid --on-duplicate")
	}
}
