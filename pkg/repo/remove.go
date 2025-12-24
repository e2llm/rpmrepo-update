package repo

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/e2llm/rpmrepo-update/pkg/metadata"
)

// RemoveRPMs removes packages identified by filename (default) or NEVRA. Optionally deletes RPM files.
func (r *Repo) RemoveRPMs(ctx context.Context, identifiers []string, byNEVRA bool, deleteFiles bool, dryRun bool) error {
	if len(identifiers) == 0 {
		return fmt.Errorf("no identifiers provided")
	}
	md, pkgs, checksumAlg, err := r.loadPackages(ctx)
	if err != nil {
		return err
	}

	index := make(map[string]int, len(pkgs))
	nameIndex := make(map[string]int, len(pkgs))
	for i := range pkgs {
		index[pkgs[i].NEVRA()] = i
		nameIndex[filepath.Base(pkgs[i].Location)] = i
	}

	toDelete := make(map[int]struct{})
	for _, id := range identifiers {
		var idx int
		var ok bool
		if byNEVRA {
			idx, ok = index[id]
		} else {
			idx, ok = nameIndex[id]
		}
		if !ok {
			return fmt.Errorf("package %s not found", id)
		}
		toDelete[idx] = struct{}{}
	}

	var kept []metadata.Package
	var deletePaths []string
	for i, p := range pkgs {
		if _, drop := toDelete[i]; drop {
			deletePaths = append(deletePaths, p.Location)
			continue
		}
		kept = append(kept, p)
	}

	if deleteFiles && !dryRun {
		for _, path := range deletePaths {
			if err := r.backend.DeleteFile(ctx, path); err != nil {
				return fmt.Errorf("delete %s: %w", path, err)
			}
		}
	}

	now := time.Now().UTC()
	if dryRun {
		return nil
	}
	return r.writeMetadata(ctx, md, kept, checksumAlg, now)
}
