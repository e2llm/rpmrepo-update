package repo

import (
	"context"
	"os"
	"strings"
)

// memBackend is a simple in-memory backend for tests.
type memBackend struct {
	files   map[string][]byte
	deleted []string
}

func newMemBackend() *memBackend {
	return &memBackend{files: make(map[string][]byte)}
}

func (m *memBackend) ListRepodata(ctx context.Context) ([]string, error) {
	var out []string
	for k := range m.files {
		if strings.HasPrefix(k, "repodata/") {
			out = append(out, k)
		}
	}
	return out, nil
}

func (m *memBackend) ReadFile(ctx context.Context, path string) ([]byte, error) {
	if d, ok := m.files[path]; ok {
		return d, nil
	}
	return nil, os.ErrNotExist
}

func (m *memBackend) WriteFile(ctx context.Context, path string, data []byte) error {
	m.files[path] = data
	return nil
}

func (m *memBackend) DeleteFile(ctx context.Context, path string) error {
	delete(m.files, path)
	m.deleted = append(m.deleted, path)
	return nil
}

func (m *memBackend) Exists(ctx context.Context, path string) (bool, error) {
	_, ok := m.files[path]
	return ok, nil
}

func (m *memBackend) ListRPMs(ctx context.Context) ([]string, error) {
	var out []string
	for k := range m.files {
		if strings.HasPrefix(k, "repodata/") {
			continue
		}
		if strings.HasSuffix(k, ".rpm") {
			out = append(out, k)
		}
	}
	return out, nil
}

func (m *memBackend) RepoRoot() string { return "mem" }
