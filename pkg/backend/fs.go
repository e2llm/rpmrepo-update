package backend

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type FSBackend struct {
	root string
}

func NewFSBackend(root string) *FSBackend {
	return &FSBackend{root: root}
}

func (b *FSBackend) RepoRoot() string {
	return b.root
}

func (b *FSBackend) ListRepodata(ctx context.Context) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	dirPath := filepath.Join(b.root, "repodata")
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		paths = append(paths, filepath.ToSlash(filepath.Join("repodata", entry.Name())))
	}
	return paths, nil
}

func (b *FSBackend) ReadFile(ctx context.Context, path string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return os.ReadFile(filepath.Join(b.root, filepath.FromSlash(path)))
}

func (b *FSBackend) Exists(ctx context.Context, path string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	_, err := os.Stat(filepath.Join(b.root, filepath.FromSlash(path)))
	if err == nil {
		return true, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return false, err
}

func (b *FSBackend) ListRPMs(ctx context.Context) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var rpms []string
	err := filepath.WalkDir(b.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(b.root, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}
		// Skip metadata directory when looking for RPMs.
		if d.IsDir() && rel == "repodata" {
			return filepath.SkipDir
		}
		if !d.IsDir() && strings.HasSuffix(d.Name(), ".rpm") {
			rpms = append(rpms, rel)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return rpms, nil
}

func (b *FSBackend) WriteFile(ctx context.Context, path string, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	absPath := filepath.Join(b.root, filepath.FromSlash(path))
	dir := filepath.Dir(absPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".tmp-rpmrepo-*")
	if err != nil {
		return err
	}
	cleanup := func() {
		_ = os.Remove(tmp.Name())
	}
	defer func() {
		if tmp != nil {
			cleanup()
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	tmpName := tmp.Name()
	tmp = nil // avoid double cleanup after rename succeeds
	if err := os.Rename(tmpName, absPath); err != nil {
		return err
	}
	return nil
}

func (b *FSBackend) DeleteFile(ctx context.Context, path string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	err := os.Remove(filepath.Join(b.root, filepath.FromSlash(path)))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}
