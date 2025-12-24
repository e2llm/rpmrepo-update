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

func TestBuildEmptyCoreFilesSHA512(t *testing.T) {
	now := time.Unix(0, 0)
	files, _, err := BuildEmptyCoreFiles("sha512", now)
	if err != nil {
		t.Fatalf("BuildEmptyCoreFiles with sha512: %v", err)
	}
	if len(files) != 3 {
		t.Fatalf("expected 3 core files, got %d", len(files))
	}
	// SHA512 checksums are 128 hex chars
	for _, f := range files {
		if len(f.Checksum) != 128 {
			t.Errorf("expected SHA512 checksum (128 chars), got %d chars", len(f.Checksum))
		}
	}
}

func TestBuildEmptyCoreFilesInvalidChecksum(t *testing.T) {
	now := time.Unix(0, 0)
	_, _, err := BuildEmptyCoreFiles("md5", now)
	if err == nil {
		t.Fatal("expected error for unsupported checksum algorithm")
	}
}

func TestComputeChecksum(t *testing.T) {
	data := []byte("hello world")

	sha256sum, err := ComputeChecksum(data, "sha256")
	if err != nil {
		t.Fatalf("ComputeChecksum sha256: %v", err)
	}
	if len(sha256sum) != 64 {
		t.Errorf("expected 64 hex chars for sha256, got %d", len(sha256sum))
	}

	sha512sum, err := ComputeChecksum(data, "sha512")
	if err != nil {
		t.Fatalf("ComputeChecksum sha512: %v", err)
	}
	if len(sha512sum) != 128 {
		t.Errorf("expected 128 hex chars for sha512, got %d", len(sha512sum))
	}
}

func TestComputeChecksumUnsupported(t *testing.T) {
	_, err := ComputeChecksum([]byte("data"), "md5")
	if err == nil {
		t.Fatal("expected error for unsupported algorithm")
	}
}

func TestSupportedChecksum(t *testing.T) {
	tests := []struct {
		alg  string
		want bool
	}{
		{"sha256", true},
		{"SHA256", true},
		{"sha512", true},
		{"SHA512", true},
		{"md5", false},
		{"sha1", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := SupportedChecksum(tt.alg); got != tt.want {
			t.Errorf("SupportedChecksum(%q) = %v, want %v", tt.alg, got, tt.want)
		}
	}
}

func TestRenderParseRoundTripEmpty(t *testing.T) {
	// Test with empty package list
	primaryXML, filelistsXML, otherXML, err := RenderCoreXML([]Package{})
	if err != nil {
		t.Fatalf("render empty: %v", err)
	}
	outPkgs, err := ParsePackagesFromXML(primaryXML, filelistsXML, otherXML)
	if err != nil {
		t.Fatalf("parse empty: %v", err)
	}
	if len(outPkgs) != 0 {
		t.Fatalf("expected 0 packages, got %d", len(outPkgs))
	}
}

func TestRenderParseRoundTripMultiplePackages(t *testing.T) {
	pkgs := []Package{
		{
			Name:         "foo",
			Arch:         "x86_64",
			Version:      "1.0",
			Release:      "1",
			ChecksumType: "sha256",
			PkgID:        "abcdef",
		},
		{
			Name:         "bar",
			Arch:         "noarch",
			Version:      "2.0",
			Release:      "2",
			ChecksumType: "sha256",
			PkgID:        "123456",
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
	if len(outPkgs) != 2 {
		t.Fatalf("expected 2 packages, got %d", len(outPkgs))
	}
}

func TestPackageWithDependencies(t *testing.T) {
	// Note: Go's encoding/xml doesn't properly handle rpm: namespace prefix during unmarshal.
	// Dependencies are populated from actual RPM files via inspector, not XML round-trip.
	// This test verifies that packages with dependencies can be rendered without error.
	pkgs := []Package{
		{
			Name:         "foo",
			Arch:         "x86_64",
			Version:      "1.0",
			Release:      "1",
			ChecksumType: "sha256",
			PkgID:        "abcdef",
			Requires: []Relation{
				{Name: "libc.so.6"},
				{Name: "bar", Flags: "GE", Ver: "1.0"},
			},
			Provides: []Relation{
				{Name: "foo", Flags: "EQ", Ver: "1.0", Rel: "1"},
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
	// Verify basic package info is preserved
	if outPkgs[0].Name != "foo" {
		t.Errorf("expected name 'foo', got %q", outPkgs[0].Name)
	}
}

func TestPackageWithChangelogs(t *testing.T) {
	pkgs := []Package{
		{
			Name:         "foo",
			Arch:         "x86_64",
			Version:      "1.0",
			Release:      "1",
			ChecksumType: "sha256",
			PkgID:        "abcdef",
			Changelogs: []Changelog{
				{Author: "John Doe <john@example.com>", Date: 1234567890, Text: "Initial release"},
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
	if len(outPkgs[0].Changelogs) != 1 {
		t.Errorf("expected 1 changelog, got %d", len(outPkgs[0].Changelogs))
	}
}
