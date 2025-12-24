package backend

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFSBackendWriteReadDelete(t *testing.T) {
	dir := t.TempDir()
	b := NewFSBackend(dir)

	ctx := context.Background()
	path := "test/file.txt"
	data := []byte("hello world")

	// Write
	if err := b.WriteFile(ctx, path, data); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Read
	got, err := b.ReadFile(ctx, path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != string(data) {
		t.Fatalf("got %q, want %q", got, data)
	}

	// Exists
	exists, err := b.Exists(ctx, path)
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if !exists {
		t.Fatalf("expected file to exist")
	}

	// Delete
	if err := b.DeleteFile(ctx, path); err != nil {
		t.Fatalf("DeleteFile: %v", err)
	}

	// Should not exist after delete
	exists, err = b.Exists(ctx, "test/file.txt")
	if err != nil {
		t.Fatalf("Exists after delete: %v", err)
	}
	if exists {
		t.Fatalf("expected file to not exist after delete")
	}
}

func TestFSBackendListRepodata(t *testing.T) {
	dir := t.TempDir()
	b := NewFSBackend(dir)
	ctx := context.Background()

	// Create repodata directory with files
	repodata := filepath.Join(dir, "repodata")
	if err := os.MkdirAll(repodata, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repodata, "repomd.xml"), []byte("test"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repodata, "primary.xml.gz"), []byte("test"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	files, err := b.ListRepodata(ctx)
	if err != nil {
		t.Fatalf("ListRepodata: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d: %v", len(files), files)
	}
}

func TestFSBackendListRPMs(t *testing.T) {
	dir := t.TempDir()
	b := NewFSBackend(dir)
	ctx := context.Background()

	// Create RPM files and repodata (should be skipped)
	if err := os.WriteFile(filepath.Join(dir, "foo.rpm"), []byte("rpm1"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bar.rpm"), []byte("rpm2"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	repodata := filepath.Join(dir, "repodata")
	if err := os.MkdirAll(repodata, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repodata, "repomd.xml"), []byte("test"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	rpms, err := b.ListRPMs(ctx)
	if err != nil {
		t.Fatalf("ListRPMs: %v", err)
	}
	if len(rpms) != 2 {
		t.Fatalf("expected 2 RPMs, got %d: %v", len(rpms), rpms)
	}
}

func TestFSBackendRepoRoot(t *testing.T) {
	b := NewFSBackend("/srv/repo")
	if b.RepoRoot() != "/srv/repo" {
		t.Fatalf("expected /srv/repo, got %s", b.RepoRoot())
	}
}

func TestFSBackendCanceledContext(t *testing.T) {
	dir := t.TempDir()
	b := NewFSBackend(dir)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// All operations should return context error
	_, err := b.ListRepodata(ctx)
	if err != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", err)
	}

	_, err = b.ReadFile(ctx, "test")
	if err != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", err)
	}

	err = b.WriteFile(ctx, "test", []byte("data"))
	if err != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", err)
	}

	_, err = b.Exists(ctx, "test")
	if err != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", err)
	}

	err = b.DeleteFile(ctx, "test")
	if err != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", err)
	}

	_, err = b.ListRPMs(ctx)
	if err != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestFSBackendDeleteNonExistent(t *testing.T) {
	dir := t.TempDir()
	b := NewFSBackend(dir)
	ctx := context.Background()

	// Deleting non-existent file should not error
	err := b.DeleteFile(ctx, "nonexistent.txt")
	if err != nil {
		t.Fatalf("DeleteFile of non-existent should not error: %v", err)
	}
}

func TestFSBackendExistsNonExistent(t *testing.T) {
	dir := t.TempDir()
	b := NewFSBackend(dir)
	ctx := context.Background()

	exists, err := b.Exists(ctx, "nonexistent.txt")
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if exists {
		t.Fatalf("expected file to not exist")
	}
}

// S3 helper function tests

func TestParseS3URI(t *testing.T) {
	tests := []struct {
		uri        string
		wantBucket string
		wantPrefix string
		wantErr    bool
	}{
		{"s3://bucket", "bucket", "", false},
		{"s3://bucket/", "bucket", "", false},
		{"s3://bucket/prefix", "bucket", "prefix", false},
		{"s3://bucket/prefix/path", "bucket", "prefix/path", false},
		{"s3://bucket/prefix/path/", "bucket", "prefix/path", false},
		{"http://bucket/prefix", "", "", true},
		{"s3://", "", "", true},
		{"", "", "", true},
	}

	for _, tt := range tests {
		bucket, prefix, err := parseS3URI(tt.uri)
		if (err != nil) != tt.wantErr {
			t.Errorf("parseS3URI(%q) error = %v, wantErr %v", tt.uri, err, tt.wantErr)
			continue
		}
		if bucket != tt.wantBucket {
			t.Errorf("parseS3URI(%q) bucket = %q, want %q", tt.uri, bucket, tt.wantBucket)
		}
		if prefix != tt.wantPrefix {
			t.Errorf("parseS3URI(%q) prefix = %q, want %q", tt.uri, prefix, tt.wantPrefix)
		}
	}
}

func TestKeyJoin(t *testing.T) {
	tests := []struct {
		prefix string
		path   string
		want   string
	}{
		{"", "", ""},
		{"", "path", "path"},
		{"prefix", "", "prefix"},
		{"prefix", "path", "prefix/path"},
		{"prefix/", "path", "prefix/path"},
		{"prefix", "/path", "prefix/path"},
		{"prefix/", "/path", "prefix/path"},
		{"prefix", "a/b/c", "prefix/a/b/c"},
		{"", ".", ""},
		{"prefix", ".", "prefix"},
	}

	for _, tt := range tests {
		got := keyJoin(tt.prefix, tt.path)
		if got != tt.want {
			t.Errorf("keyJoin(%q, %q) = %q, want %q", tt.prefix, tt.path, got, tt.want)
		}
	}
}
