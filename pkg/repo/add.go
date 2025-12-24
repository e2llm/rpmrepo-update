package repo

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/e2llm/rpmrepo-update/pkg/inspector"
)

// AddRPMs adds RPMs to the repository, updating core metadata. Only filesystem/S3 backends are supported in v1.
func (r *Repo) AddRPMs(ctx context.Context, rpmPaths []string, replaceExisting bool, dryRun bool, signRPMs bool, gpgKey string) error {
	if r.backend == nil {
		return fmt.Errorf("backend is required")
	}
	if len(rpmPaths) == 0 {
		return fmt.Errorf("no RPM paths provided")
	}

	md, pkgs, checksumAlg, err := r.loadPackages(ctx)
	if err != nil {
		return err
	}

	index := make(map[string]int, len(pkgs))
	for i := range pkgs {
		index[pkgs[i].NEVRA()] = i
	}

	// detect duplicates in existing metadata
	if len(index) != len(pkgs) {
		return fmt.Errorf("metadata contains duplicate NEVRA entries")
	}

	now := time.Now().UTC()

	for _, path := range rpmPaths {
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("stat %s: %w", path, err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		destRel := filepath.Base(path)
		if r.DestPrefix != "" {
			destRel = filepath.ToSlash(filepath.Join(r.DestPrefix, destRel))
		}
		pkgMeta, err := inspector.InspectRPM(path, data, info, checksumAlg, destRel)
		if err != nil {
			return err
		}
		if signRPMs && !dryRun {
			signed, err := r.signRPM(ctx, data, gpgKey)
			if err != nil {
				return fmt.Errorf("sign rpm %s: %w", path, err)
			}
			data = signed
		}

		key := pkgMeta.NEVRA()
		if idx, ok := index[key]; ok {
			if !replaceExisting {
				return fmt.Errorf("package %s already exists (use --replace-existing)", key)
			}
			pkgs[idx] = pkgMeta
		} else {
			pkgs = append(pkgs, pkgMeta)
			index[key] = len(pkgs) - 1
		}

		if !dryRun {
			if err := r.backend.WriteFile(ctx, destRel, data); err != nil {
				return fmt.Errorf("write rpm %s: %w", destRel, err)
			}
		}
	}

	if dryRun {
		return nil
	}
	return r.writeMetadata(ctx, md, pkgs, checksumAlg, now)
}
