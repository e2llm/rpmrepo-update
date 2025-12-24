package repo

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/e2llm/rpmrepo-update/pkg/metadata"
)

// loadPackages loads repomd and core metadata, returning parsed packages and the checksum algorithm.
func (r *Repo) loadPackages(ctx context.Context) (metadata.RepoMD, []metadata.Package, string, error) {
	md, err := metadata.LoadRepoMD(ctx, r.backend)
	if err != nil {
		return metadata.RepoMD{}, nil, "", fmt.Errorf("load repomd.xml: %w", err)
	}
	primaryData, filelistsData, otherData := metadata.GetCoreData(md)
	if primaryData == nil || filelistsData == nil || otherData == nil {
		return metadata.RepoMD{}, nil, "", fmt.Errorf("repo missing core metadata (primary/filelists/other)")
	}
	if isSqlite(primaryData.Location.Href) || isSqlite(filelistsData.Location.Href) || isSqlite(otherData.Location.Href) {
		return metadata.RepoMD{}, nil, "", fmt.Errorf("unsupported: sqlite-only metadata in v1")
	}

	primaryCore, err := metadata.ReadAndVerifyCore(ctx, r.backend, *primaryData)
	if err != nil {
		return metadata.RepoMD{}, nil, "", fmt.Errorf("read primary: %w", err)
	}
	filelistsCore, err := metadata.ReadAndVerifyCore(ctx, r.backend, *filelistsData)
	if err != nil {
		return metadata.RepoMD{}, nil, "", fmt.Errorf("read filelists: %w", err)
	}
	otherCore, err := metadata.ReadAndVerifyCore(ctx, r.backend, *otherData)
	if err != nil {
		return metadata.RepoMD{}, nil, "", fmt.Errorf("read other: %w", err)
	}

	pkgs, err := metadata.ParsePackagesFromXML(primaryCore.Uncompressed, filelistsCore.Uncompressed, otherCore.Uncompressed)
	if err != nil {
		return metadata.RepoMD{}, nil, "", fmt.Errorf("parse metadata: %w", err)
	}

	checksumAlg := primaryData.Checksum.Type
	if checksumAlg == "" {
		checksumAlg = "sha256"
	}
	return md, pkgs, checksumAlg, nil
}

// writeMetadata regenerates core metadata and repomd.xml, writing via backend.
func (r *Repo) writeMetadata(ctx context.Context, md metadata.RepoMD, pkgs []metadata.Package, checksumAlg string, now time.Time) error {
	if validator, ok := r.backend.(RepomdValidator); ok {
		if err := validator.CheckRepomdUnchanged(ctx); err != nil {
			return err
		}
	}
	checksumAlg = normalizeChecksum(checksumAlg)

	coreFiles, err := metadata.BuildCoreFilesFromPackages(pkgs, checksumAlg, now)
	if err != nil {
		return fmt.Errorf("build core metadata: %w", err)
	}
	newRepoMD, warnings := assembleRepoMD(md, coreFiles, checksumAlg, now, r.AllowUnknown)
	repomdBytes, err := metadata.MarshalRepoMD(newRepoMD)
	if err != nil {
		return fmt.Errorf("marshal repomd.xml: %w", err)
	}
	for _, w := range warnings {
		r.logger.Printf("warn: %s", w)
	}

	for _, cf := range coreFiles {
		if err := r.backend.WriteFile(ctx, cf.Path, cf.Compressed); err != nil {
			return fmt.Errorf("write %s: %w", cf.Path, err)
		}
	}
	if err := r.backend.WriteFile(ctx, "repodata/repomd.xml", repomdBytes); err != nil {
		return fmt.Errorf("write repodata/repomd.xml: %w", err)
	}

	// Clean up old metadata files no longer referenced
	if err := r.cleanupOldMetadata(ctx, newRepoMD); err != nil {
		r.logger.Printf("warn: cleanup old metadata: %v", err)
	}
	return nil
}

// cleanupOldMetadata removes metadata files not referenced in current repomd.xml
func (r *Repo) cleanupOldMetadata(ctx context.Context, md metadata.RepoMD) error {
	// Build set of referenced files
	referenced := make(map[string]struct{})
	referenced["repodata/repomd.xml"] = struct{}{}
	referenced["repodata/repomd.xml.asc"] = struct{}{}
	for _, d := range md.Data {
		referenced[d.Location.Href] = struct{}{}
	}

	// List current repodata files
	files, err := r.backend.ListRepodata(ctx)
	if err != nil {
		return fmt.Errorf("list repodata: %w", err)
	}

	// Delete unreferenced files
	for _, f := range files {
		if _, ok := referenced[f]; ok {
			continue
		}
		// Skip .tmp directory
		if strings.HasPrefix(f, "repodata/.tmp") {
			continue
		}
		if err := r.backend.DeleteFile(ctx, f); err != nil {
			r.logger.Printf("warn: delete %s: %v", f, err)
		}
	}
	return nil
}

// RepomdValidator optionally protects writes with ETag checks.
type RepomdValidator interface {
	CheckRepomdUnchanged(ctx context.Context) error
}

func assembleRepoMD(old metadata.RepoMD, core []metadata.CoreFile, checksumAlg string, now time.Time, allowUnknown bool) (metadata.RepoMD, []string) {
	newMD := metadata.RepoMD{
		Xmlns:    old.Xmlns,
		Revision: fmt.Sprintf("%d", now.Unix()),
	}
	if newMD.Xmlns == "" {
		newMD.Xmlns = metadata.RepoNamespace
	}

	unknownTypes := make(map[string]struct{})
	for _, d := range old.Data {
		switch d.Type {
		case "primary", "filelists", "other", "prestodelta":
			continue
		case "modules":
			newMD.Data = append(newMD.Data, d)
		default:
			if allowUnknown {
				newMD.Data = append(newMD.Data, d)
				unknownTypes[d.Type] = struct{}{}
			} else {
				unknownTypes[d.Type] = struct{}{}
			}
		}
	}

	for _, cf := range core {
		alg := checksumAlg
		if alg == "" {
			alg = "sha256"
		}
		openChecksum := cf.OpenChecksum
		newMD.Data = append(newMD.Data, metadata.RepoData{
			Type:         cf.Type,
			Checksum:     metadata.Checksum{Type: alg, Value: cf.Checksum},
			OpenChecksum: &metadata.Checksum{Type: alg, Value: openChecksum},
			Location:     metadata.Location{Href: cf.Path},
			Timestamp:    cf.Timestamp,
			Size:         cf.Size,
			OpenSize:     cf.OpenSize,
		})
	}

	var warnings []string
	for t := range unknownTypes {
		warnings = append(warnings, fmt.Sprintf("preserving unknown metadata type '%s' from repomd.xml; checksum not verified", t))
	}
	return newMD, warnings
}

func normalizeChecksum(alg string) string {
	switch alg {
	case "sha256", "sha512":
		return alg
	default:
		return "sha256"
	}
}

func isSqlite(path string) bool {
	return strings.Contains(path, ".sqlite")
}
