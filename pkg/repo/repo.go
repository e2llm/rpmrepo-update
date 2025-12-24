package repo

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/e2llm/rpmrepo-update/pkg/backend"
	"github.com/e2llm/rpmrepo-update/pkg/metadata"
)

type Repo struct {
	backend backend.Backend
	logger  *log.Logger
	// AllowUnknown controls whether unknown metadata types in repomd.xml are preserved with warnings (true) or cause an error (false).
	AllowUnknown bool
	// DestPrefix sets a destination prefix under the repo root for RPM writes.
	DestPrefix string
}

func New(backend backend.Backend) *Repo {
	return &Repo{
		backend: backend,
		logger:  log.New(os.Stderr, "", 0),
	}
}

// WithLogger overrides the logger used for warnings/info.
func (r *Repo) WithLogger(w io.Writer) {
	r.logger = log.New(w, "", 0)
}

// InitRepo creates an empty repository layout with core metadata files.
func (r *Repo) InitRepo(ctx context.Context, checksumAlg string, force bool, signRepodata bool, gpgKey string) error {
	if r.backend == nil {
		return fmt.Errorf("backend is required")
	}
	exists, err := r.backend.Exists(ctx, "repodata/repomd.xml")
	if err != nil {
		return err
	}
	if exists && !force {
		return fmt.Errorf("repodata/repomd.xml already exists (use --force to overwrite)")
	}

	now := time.Now().UTC()
	coreFiles, repomd, err := metadata.BuildEmptyCoreFiles(checksumAlg, now)
	if err != nil {
		return err
	}
	repomdBytes, err := metadata.MarshalRepoMD(repomd)
	if err != nil {
		return err
	}

	for _, file := range coreFiles {
		if err := r.backend.WriteFile(ctx, file.Path, file.Compressed); err != nil {
			return fmt.Errorf("write %s: %w", file.Path, err)
		}
	}
	if err := r.backend.WriteFile(ctx, "repodata/repomd.xml", repomdBytes); err != nil {
		return fmt.Errorf("write repodata/repomd.xml: %w", err)
	}
	if signRepodata {
		if err := r.signRepomd(ctx, repomdBytes, gpgKey); err != nil {
			return fmt.Errorf("sign repomd.xml: %w", err)
		}
	}
	return nil
}
