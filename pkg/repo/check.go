package repo

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/e2llm/rpmrepo-update/pkg/metadata"
)

// CheckResult captures warnings and an optional terminal error.
type CheckResult struct {
	Warnings []string `json:"warnings"`
	Err      error    `json:"-"`
}

// CheckDetailed performs checks and returns warnings/errors without writing output.
func (r *Repo) CheckDetailed(ctx context.Context) CheckResult {
	warnings, err := r.checkCollect(ctx)
	return CheckResult{Warnings: warnings, Err: err}
}

// Check validates that core metadata files exist, decompress, and match checksums recorded in repomd.xml.
func (r *Repo) Check(ctx context.Context) error {
	warnings, err := r.checkCollect(ctx)
	for _, w := range warnings {
		r.logger.Printf("warn: %s", w)
	}
	return err
}

func (r *Repo) checkCollect(ctx context.Context) ([]string, error) {
	if r.backend == nil {
		return nil, fmt.Errorf("backend is required")
	}
	md, err := metadata.LoadRepoMD(ctx, r.backend)
	if err != nil {
		return nil, fmt.Errorf("load repomd.xml: %w", err)
	}
	primary, filelists, other := metadata.GetCoreData(md)
	var errs []error
	if primary == nil {
		errs = append(errs, errors.New("missing primary metadata in repomd.xml"))
	}
	if filelists == nil {
		errs = append(errs, errors.New("missing filelists metadata in repomd.xml"))
	}
	if other == nil {
		errs = append(errs, errors.New("missing other metadata in repomd.xml"))
	}
	for _, d := range []*metadata.RepoData{primary, filelists, other} {
		if d == nil {
			continue
		}
		core, err := metadata.ReadAndVerifyCore(ctx, r.backend, *d)
		if err != nil {
			errs = append(errs, fmt.Errorf("core %s: %w", d.Type, err))
			continue
		}
		if d.Size != 0 && d.Size != core.Size {
			errs = append(errs, fmt.Errorf("core %s size mismatch: repomd=%d actual=%d", d.Type, d.Size, core.Size))
		}
		if d.OpenSize != 0 && d.OpenSize != core.OpenSize {
			errs = append(errs, fmt.Errorf("core %s open-size mismatch: repomd=%d actual=%d", d.Type, d.OpenSize, core.OpenSize))
		}
	}

	// Parse packages for deeper checks.
	var pkgs []metadata.Package
	if len(errs) == 0 && primary != nil && filelists != nil && other != nil {
		primaryCore, err := metadata.ReadAndVerifyCore(ctx, r.backend, *primary)
		if err != nil {
			errs = append(errs, fmt.Errorf("primary parse: %w", err))
		} else {
			filelistsCore, err := metadata.ReadAndVerifyCore(ctx, r.backend, *filelists)
			if err != nil {
				errs = append(errs, fmt.Errorf("filelists parse: %w", err))
			} else {
				otherCore, err := metadata.ReadAndVerifyCore(ctx, r.backend, *other)
				if err != nil {
					errs = append(errs, fmt.Errorf("other parse: %w", err))
				} else {
					pkgs, err = metadata.ParsePackagesFromXML(primaryCore.Uncompressed, filelistsCore.Uncompressed, otherCore.Uncompressed)
					if err != nil {
						errs = append(errs, fmt.Errorf("parse packages: %w", err))
					}
				}
			}
		}
	}

	if len(pkgs) > 0 {
		rpmList, err := r.backend.ListRPMs(ctx)
		if err != nil {
			errs = append(errs, fmt.Errorf("list rpms: %w", err))
		} else {
			expected := make(map[string]struct{}, len(pkgs))
			for _, p := range pkgs {
				expected[p.Location] = struct{}{}
				if p.Location == "" {
					errs = append(errs, fmt.Errorf("package %s missing location", p.NEVRA()))
					continue
				}
				exists, err := r.backend.Exists(ctx, p.Location)
				if err != nil {
					errs = append(errs, fmt.Errorf("exists %s: %w", p.Location, err))
					continue
				}
				if !exists {
					errs = append(errs, fmt.Errorf("rpm missing for %s (%s)", p.NEVRA(), p.Location))
				}
			}
			for _, rpmPath := range rpmList {
				base := filepath.ToSlash(rpmPath)
				if _, ok := expected[base]; !ok {
					errs = append(errs, fmt.Errorf("rpm present but not referenced: %s", base))
				}
			}
		}
	}

	var warnings []string
	for _, d := range md.Data {
		if d.Type != "primary" && d.Type != "filelists" && d.Type != "other" && d.Type != "modules" {
			warnings = append(warnings, fmt.Sprintf("preserving unknown metadata type '%s' from repomd.xml; checksum not verified", d.Type))
		}
	}

	return warnings, errors.Join(errs...)
}
