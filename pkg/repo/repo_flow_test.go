package repo

import (
	"context"
	"fmt"
	"io"
	"log"
	"testing"
	"time"

	"github.com/e2llm/rpmrepo-update/pkg/metadata"
)

func TestRemoveRPMsMetadataOnly(t *testing.T) {
	ctx := context.Background()
	mb := newMemBackend()

	pkgs := []metadata.Package{
		{
			Name:         "foo",
			Arch:         "x86_64",
			Version:      "1.0",
			Release:      "1",
			ChecksumType: "sha256",
			PkgID:        "pkgid",
			Location:     "foo-1.0-1.x86_64.rpm",
		},
	}
	now := time.Unix(0, 0)
	core, err := metadata.BuildCoreFilesFromPackages(pkgs, "sha256", now)
	if err != nil {
		t.Fatalf("build core: %v", err)
	}
	repomd := metadata.UpdateRepoMDWithCore(metadata.RepoMD{}, core, "sha256", now)
	repomdBytes, err := metadata.MarshalRepoMD(repomd)
	if err != nil {
		t.Fatalf("marshal repomd: %v", err)
	}
	for _, cf := range core {
		mb.files[cf.Path] = cf.Compressed
	}
	mb.files["repodata/repomd.xml"] = repomdBytes
	mb.files["foo-1.0-1.x86_64.rpm"] = []byte("rpmdata")

	r := New(mb)
	if err := r.RemoveRPMs(ctx, []string{"foo-1.0-1.x86_64.rpm"}, false, true, false); err != nil {
		t.Fatalf("RemoveRPMs: %v", err)
	}
	_, pkgsOut, _, err := r.loadPackages(ctx)
	if err != nil {
		t.Fatalf("loadPackages: %v", err)
	}
	if len(pkgsOut) != 0 {
		t.Fatalf("expected 0 packages, got %d", len(pkgsOut))
	}
	if exists, _ := mb.Exists(ctx, "foo-1.0-1.x86_64.rpm"); exists {
		t.Fatalf("expected rpm deleted")
	}
}

type conflictBackend struct {
	memBackend
}

func (c *conflictBackend) CheckRepomdUnchanged(ctx context.Context) error {
	return fmt.Errorf("etag conflict")
}

func TestWriteMetadataConflict(t *testing.T) {
	ctx := context.Background()
	cb := &conflictBackend{memBackend{files: make(map[string][]byte)}}
	pkgs := []metadata.Package{}
	now := time.Unix(0, 0)
	md := metadata.RepoMD{}
	err := (&Repo{backend: cb, logger: newTestLogger(t)}).writeMetadata(ctx, md, pkgs, "sha256", now)
	if err == nil {
		t.Fatalf("expected conflict error")
	}
}

// newTestLogger suppresses output in tests.
func newTestLogger(t *testing.T) *log.Logger {
	t.Helper()
	return log.New(io.Discard, "", 0)
}
