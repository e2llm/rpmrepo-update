package metadata

import (
	"testing"
	"time"
)

func TestRenderParseRoundTrip(t *testing.T) {
	pkgs := []Package{
		{
			Name:         "foo",
			Arch:         "x86_64",
			Version:      "1.0",
			Release:      "1",
			ChecksumType: "sha256",
			PkgID:        "abcdef",
			Files: []File{
				{Path: "/usr/bin/foo"},
			},
		},
	}
	primaryXML, filelistsXML, otherXML, err := RenderCoreXML(pkgs)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	outPkgs, err := ParsePackagesFromXML(primaryXML, filelistsXML, otherXML)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(outPkgs) != 1 {
		t.Fatalf("expected 1 package, got %d", len(outPkgs))
	}
	got := outPkgs[0]
	if got.Name != "foo" || got.Arch != "x86_64" {
		t.Fatalf("unexpected package: %+v", got)
	}
	if len(got.Files) != 1 || got.Files[0].Path != "/usr/bin/foo" {
		t.Fatalf("unexpected files: %+v", got.Files)
	}
}

func TestBuildEmptyCoreFiles(t *testing.T) {
	now := time.Unix(0, 0)
	files, repomd, err := BuildEmptyCoreFiles("sha256", now)
	if err != nil {
		t.Fatalf("BuildEmptyCoreFiles: %v", err)
	}
	if len(files) != 3 {
		t.Fatalf("expected 3 core files, got %d", len(files))
	}
	if repomd.Revision == "" {
		t.Fatalf("expected revision to be set")
	}
}
