package metadata

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"

	"github.com/e2llm/rpmrepo-update/pkg/backend"
)

// LoadRepoMD reads and unmarshals repodata/repomd.xml from backend.
func LoadRepoMD(ctx context.Context, b backend.Backend) (RepoMD, error) {
	data, err := b.ReadFile(ctx, "repodata/repomd.xml")
	if err != nil {
		return RepoMD{}, err
	}
	return ParseRepoMD(data)
}

// ParseRepoMD unmarshals repomd XML from raw bytes.
func ParseRepoMD(data []byte) (RepoMD, error) {
	var md RepoMD
	if err := xml.Unmarshal(data, &md); err != nil {
		return RepoMD{}, err
	}
	return md, nil
}

// GetCoreData returns the RepoData entries for primary, filelists, and other.
func GetCoreData(md RepoMD) (primary, filelists, other *RepoData) {
	for i := range md.Data {
		d := &md.Data[i]
		switch d.Type {
		case "primary":
			primary = d
		case "filelists":
			filelists = d
		case "other":
			other = d
		}
	}
	return
}

// ReadAndVerifyCore downloads and decompresses a core metadata file and verifies checksums.
func ReadAndVerifyCore(ctx context.Context, b backend.Backend, d RepoData) (CoreFile, error) {
	if d.Location.Href == "" {
		return CoreFile{}, errors.New("missing location href")
	}
	compressed, err := b.ReadFile(ctx, d.Location.Href)
	if err != nil {
		return CoreFile{}, fmt.Errorf("read %s: %w", d.Location.Href, err)
	}
	uncompressed, err := gunzip(compressed)
	if err != nil {
		return CoreFile{}, fmt.Errorf("decompress %s: %w", d.Location.Href, err)
	}

	if d.Checksum.Type == "" || d.OpenChecksum == nil || d.OpenChecksum.Type == "" {
		return CoreFile{}, errors.New("missing checksum metadata")
	}
	if !SupportedChecksum(d.Checksum.Type) {
		return CoreFile{}, fmt.Errorf("unsupported checksum type %q", d.Checksum.Type)
	}

	sum, err := ComputeChecksum(compressed, d.Checksum.Type)
	if err != nil {
		return CoreFile{}, err
	}
	if sum != d.Checksum.Value {
		return CoreFile{}, fmt.Errorf("checksum mismatch for %s: expected %s got %s", d.Type, d.Checksum.Value, sum)
	}

	openSum, err := ComputeChecksum(uncompressed, d.OpenChecksum.Type)
	if err != nil {
		return CoreFile{}, err
	}
	if openSum != d.OpenChecksum.Value {
		return CoreFile{}, fmt.Errorf("open-checksum mismatch for %s: expected %s got %s", d.Type, d.OpenChecksum.Value, openSum)
	}

	return CoreFile{
		Type:         d.Type,
		Path:         d.Location.Href,
		Compressed:   compressed,
		Uncompressed: uncompressed,
		Checksum:     sum,
		OpenChecksum: openSum,
		Size:         int64(len(compressed)),
		OpenSize:     int64(len(uncompressed)),
		Timestamp:    d.Timestamp,
	}, nil
}

func gunzip(data []byte) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
