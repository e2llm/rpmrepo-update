package backend

import "context"

// Backend abstracts storage for a single repository root.
// Paths are always relative to the repository root (e.g. "repodata/repomd.xml").
type Backend interface {
	ListRepodata(ctx context.Context) ([]string, error)
	ReadFile(ctx context.Context, path string) ([]byte, error)
	WriteFile(ctx context.Context, path string, data []byte) error
	DeleteFile(ctx context.Context, path string) error
	Exists(ctx context.Context, path string) (bool, error)
	ListRPMs(ctx context.Context) ([]string, error)
	RepoRoot() string
}
