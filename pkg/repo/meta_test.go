package repo

import (
	"context"
	"testing"
	"time"

	"github.com/e2llm/rpmrepo-update/pkg/metadata"
)

func TestNormalizeChecksum(t *testing.T) {
	if got := normalizeChecksum("sha256"); got != "sha256" {
		t.Fatalf("unexpected %s", got)
	}
	if got := normalizeChecksum("unknown"); got != "sha256" {
		t.Fatalf("fallback expected sha256, got %s", got)
	}
}

func TestAssembleRepoMDUnknownWarning(t *testing.T) {
	old := metadata.RepoMD{
		Data: []metadata.RepoData{
			{Type: "primary"},
			{Type: "filelists"},
			{Type: "other"},
			{Type: "productid"},
			{Type: "modules"},
		},
	}
	core := []metadata.CoreFile{
		{Type: "primary", Checksum: "a", OpenChecksum: "b", Path: "repodata/a-primary.xml.gz", Size: 1, OpenSize: 1},
		{Type: "filelists", Checksum: "c", OpenChecksum: "d", Path: "repodata/c-filelists.xml.gz", Size: 1, OpenSize: 1},
		{Type: "other", Checksum: "e", OpenChecksum: "f", Path: "repodata/e-other.xml.gz", Size: 1, OpenSize: 1},
	}
	_, warnings := assembleRepoMD(old, core, "sha256", time.Unix(0, 0), true)
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d: %v", len(warnings), warnings)
	}
	if warnings[0] == "" {
		t.Fatalf("warning should not be empty")
	}
}

func TestIsSqlite(t *testing.T) {
	if !isSqlite("repodata/primary.sqlite.bz2") {
		t.Fatalf("expected sqlite detection")
	}
	if isSqlite("repodata/primary.xml.gz") {
		t.Fatalf("did not expect sqlite detection")
	}
}

func TestLoadPackagesRejectsSqlite(t *testing.T) {
	md := metadata.RepoMD{
		Data: []metadata.RepoData{
			{Type: "primary", Location: metadata.Location{Href: "repodata/primary.sqlite.bz2"}},
			{Type: "filelists", Location: metadata.Location{Href: "repodata/filelists.sqlite.bz2"}},
			{Type: "other", Location: metadata.Location{Href: "repodata/other.sqlite.bz2"}},
		},
	}
	repomdBytes, err := metadata.MarshalRepoMD(md)
	if err != nil {
		t.Fatalf("marshal repomd: %v", err)
	}
	mb := newMemBackend()
	mb.files["repodata/repomd.xml"] = repomdBytes
	r := New(mb)
	if _, _, _, err := r.loadPackages(context.Background()); err == nil {
		t.Fatalf("expected sqlite error, got nil")
	}
}
