package metadata

import (
	"bytes"
	"compress/gzip"
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// memBackend is a simple in-memory backend for metadata tests.
type memBackend struct {
	files map[string][]byte
}

func (m *memBackend) ListRepodata(ctx context.Context) ([]string, error) {
	var out []string
	for k := range m.files {
		if strings.HasPrefix(k, "repodata/") {
			out = append(out, k)
		}
	}
	return out, nil
}
func (m *memBackend) ReadFile(ctx context.Context, path string) ([]byte, error) {
	if d, ok := m.files[path]; ok {
		return d, nil
	}
	return nil, os.ErrNotExist
}
func (m *memBackend) WriteFile(ctx context.Context, path string, data []byte) error {
	m.files[path] = data
	return nil
}
func (m *memBackend) DeleteFile(ctx context.Context, path string) error {
	delete(m.files, path)
	return nil
}
func (m *memBackend) Exists(ctx context.Context, path string) (bool, error) {
	_, ok := m.files[path]
	return ok, nil
}
func (m *memBackend) ListRPMs(ctx context.Context) ([]string, error) { return nil, nil }
func (m *memBackend) RepoRoot() string                               { return "mem" }

func TestLoadRepoMD(t *testing.T) {
	// Build a valid repo using BuildEmptyCoreFiles
	now := time.Unix(1000, 0)
	coreFiles, repomd, err := BuildEmptyCoreFiles("sha256", now)
	if err != nil {
		t.Fatalf("BuildEmptyCoreFiles: %v", err)
	}
	repomdBytes, err := MarshalRepoMD(repomd)
	if err != nil {
		t.Fatalf("MarshalRepoMD: %v", err)
	}

	mb := &memBackend{files: make(map[string][]byte)}
	mb.files["repodata/repomd.xml"] = repomdBytes
	for _, cf := range coreFiles {
		mb.files[cf.Path] = cf.Compressed
	}

	ctx := context.Background()
	md, err := LoadRepoMD(ctx, mb)
	if err != nil {
		t.Fatalf("LoadRepoMD: %v", err)
	}
	if len(md.Data) == 0 {
		t.Fatal("expected data entries in repomd")
	}
}

func TestLoadRepoMDNotFound(t *testing.T) {
	mb := &memBackend{files: make(map[string][]byte)}
	ctx := context.Background()
	_, err := LoadRepoMD(ctx, mb)
	if err == nil {
		t.Fatal("expected error for missing repomd.xml")
	}
}

func TestParseRepoMDInvalid(t *testing.T) {
	_, err := ParseRepoMD([]byte("not xml"))
	if err == nil {
		t.Fatal("expected error for invalid XML")
	}
}

func TestGetCoreData(t *testing.T) {
	md := RepoMD{
		Data: []RepoData{
			{Type: "primary"},
			{Type: "filelists"},
			{Type: "other"},
			{Type: "modules"},
		},
	}
	primary, filelists, other := GetCoreData(md)
	if primary == nil || filelists == nil || other == nil {
		t.Fatal("expected all three core data types")
	}
	if primary.Type != "primary" {
		t.Errorf("expected primary, got %s", primary.Type)
	}
}

func TestGetCoreDataMissing(t *testing.T) {
	md := RepoMD{
		Data: []RepoData{
			{Type: "primary"},
		},
	}
	primary, filelists, other := GetCoreData(md)
	if primary == nil {
		t.Fatal("expected primary")
	}
	if filelists != nil || other != nil {
		t.Fatal("expected filelists and other to be nil")
	}
}

func TestReadAndVerifyCore(t *testing.T) {
	now := time.Unix(1000, 0)
	coreFiles, repomd, err := BuildEmptyCoreFiles("sha256", now)
	if err != nil {
		t.Fatalf("BuildEmptyCoreFiles: %v", err)
	}

	mb := &memBackend{files: make(map[string][]byte)}
	for _, cf := range coreFiles {
		mb.files[cf.Path] = cf.Compressed
	}

	ctx := context.Background()
	primary, _, _ := GetCoreData(repomd)
	if primary == nil {
		t.Fatal("expected primary in repomd")
	}

	cf, err := ReadAndVerifyCore(ctx, mb, *primary)
	if err != nil {
		t.Fatalf("ReadAndVerifyCore: %v", err)
	}
	if cf.Type != "primary" {
		t.Errorf("expected primary, got %s", cf.Type)
	}
	if len(cf.Uncompressed) == 0 {
		t.Error("expected non-empty uncompressed data")
	}
}

func TestReadAndVerifyCoreBadChecksum(t *testing.T) {
	now := time.Unix(1000, 0)
	_, repomd, err := BuildEmptyCoreFiles("sha256", now)
	if err != nil {
		t.Fatalf("BuildEmptyCoreFiles: %v", err)
	}

	mb := &memBackend{files: make(map[string][]byte)}
	primary, _, _ := GetCoreData(repomd)
	// Write corrupt data
	mb.files[primary.Location.Href] = gzipData([]byte("corrupted"))

	ctx := context.Background()
	_, err = ReadAndVerifyCore(ctx, mb, *primary)
	if err == nil {
		t.Fatal("expected checksum mismatch error")
	}
}

func TestReadAndVerifyCoreMissingFile(t *testing.T) {
	d := RepoData{
		Type:     "primary",
		Location: Location{Href: "repodata/missing.xml.gz"},
		Checksum: Checksum{Type: "sha256", Value: "abc"},
		OpenChecksum: &Checksum{Type: "sha256", Value: "def"},
	}

	mb := &memBackend{files: make(map[string][]byte)}
	ctx := context.Background()
	_, err := ReadAndVerifyCore(ctx, mb, d)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestReadAndVerifyCoreMissingHref(t *testing.T) {
	d := RepoData{Type: "primary"}
	mb := &memBackend{files: make(map[string][]byte)}
	ctx := context.Background()
	_, err := ReadAndVerifyCore(ctx, mb, d)
	if err == nil {
		t.Fatal("expected error for missing href")
	}
}

func TestGunzip(t *testing.T) {
	original := []byte("test data for gzip")
	compressed := gzipData(original)

	result, err := gunzip(compressed)
	if err != nil {
		t.Fatalf("gunzip: %v", err)
	}
	if string(result) != string(original) {
		t.Fatalf("got %q, want %q", result, original)
	}
}

func TestGunzipInvalid(t *testing.T) {
	_, err := gunzip([]byte("not gzip data"))
	if err == nil {
		t.Fatal("expected error for invalid gzip data")
	}
}

func gzipData(data []byte) []byte {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	_, _ = w.Write(data)
	_ = w.Close()
	return buf.Bytes()
}
