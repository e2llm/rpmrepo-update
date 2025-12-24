package metadata

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"strings"
	"time"
)

const (
	CommonNamespace    = "http://linux.duke.edu/metadata/common"
	FilelistsNamespace = "http://linux.duke.edu/metadata/filelists"
	OtherNamespace     = "http://linux.duke.edu/metadata/other"
	RpmNamespace       = "http://linux.duke.edu/metadata/rpm"
)

type CoreFile struct {
	Type         string
	Path         string
	Compressed   []byte
	Uncompressed []byte
	Checksum     string
	OpenChecksum string
	Size         int64
	OpenSize     int64
	Timestamp    int64
}

type primaryRoot struct {
	XMLName  xml.Name `xml:"metadata"`
	Xmlns    string   `xml:"xmlns,attr"`
	XmlnsRpm string   `xml:"xmlns:rpm,attr,omitempty"`
	Packages int      `xml:"packages,attr"`
}

type filelistsRoot struct {
	XMLName  xml.Name `xml:"filelists"`
	Xmlns    string   `xml:"xmlns,attr"`
	Packages int      `xml:"packages,attr"`
}

type otherRoot struct {
	XMLName  xml.Name `xml:"otherdata"`
	Xmlns    string   `xml:"xmlns,attr"`
	Packages int      `xml:"packages,attr"`
}

// BuildEmptyCoreFiles creates empty primary/filelists/other XML payloads, compresses
// them, computes checksums, and prepares a repomd definition using the provided checksum algorithm.
func BuildEmptyCoreFiles(checksumAlg string, now time.Time) ([]CoreFile, RepoMD, error) {
	checksumAlg = strings.ToLower(checksumAlg)
	if !SupportedChecksum(checksumAlg) {
		return nil, RepoMD{}, fmt.Errorf("unsupported checksum algorithm %q", checksumAlg)
	}

	payloads := map[string]interface{}{
		"primary":   primaryRoot{Xmlns: CommonNamespace, XmlnsRpm: RpmNamespace, Packages: 0},
		"filelists": filelistsRoot{Xmlns: FilelistsNamespace, Packages: 0},
		"other":     otherRoot{Xmlns: OtherNamespace, Packages: 0},
	}

	var coreFiles []CoreFile
	for _, t := range []string{"primary", "filelists", "other"} {
		xmlBytes, err := marshalWithHeader(payloads[t])
		if err != nil {
			return nil, RepoMD{}, err
		}
		compressed, err := gzipBytes(xmlBytes)
		if err != nil {
			return nil, RepoMD{}, err
		}
		sum, err := ComputeChecksum(compressed, checksumAlg)
		if err != nil {
			return nil, RepoMD{}, err
		}
		openSum, err := ComputeChecksum(xmlBytes, checksumAlg)
		if err != nil {
			return nil, RepoMD{}, err
		}
		path := fmt.Sprintf("repodata/%s-%s.xml.gz", sum, t)
		coreFiles = append(coreFiles, CoreFile{
			Type:         t,
			Path:         path,
			Compressed:   compressed,
			Uncompressed: xmlBytes,
			Checksum:     sum,
			OpenChecksum: openSum,
			Size:         int64(len(compressed)),
			OpenSize:     int64(len(xmlBytes)),
			Timestamp:    now.Unix(),
		})
	}

	repomd := RepoMD{
		Revision: fmt.Sprintf("%d", now.Unix()),
	}
	for _, f := range coreFiles {
		openChecksum := f.OpenChecksum
		repomd.Data = append(repomd.Data, RepoData{
			Type:         f.Type,
			Checksum:     Checksum{Type: checksumAlg, Value: f.Checksum},
			OpenChecksum: &Checksum{Type: checksumAlg, Value: openChecksum},
			Location:     Location{Href: f.Path},
			Timestamp:    f.Timestamp,
			Size:         f.Size,
			OpenSize:     f.OpenSize,
		})
	}
	return coreFiles, repomd, nil
}

func marshalWithHeader(v interface{}) ([]byte, error) {
	body, err := xml.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	return append([]byte(xml.Header), body...), nil
}

func gzipBytes(content []byte) ([]byte, error) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(content); err != nil {
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func ComputeChecksum(data []byte, alg string) (string, error) {
	switch strings.ToLower(alg) {
	case "sha256":
		sum := sha256.Sum256(data)
		return hex.EncodeToString(sum[:]), nil
	case "sha512":
		sum := sha512.Sum512(data)
		return hex.EncodeToString(sum[:]), nil
	default:
		return "", fmt.Errorf("unsupported checksum algorithm %q", alg)
	}
}

// SupportedChecksum reports whether the algorithm is one of the allowed types.
func SupportedChecksum(alg string) bool {
	switch strings.ToLower(alg) {
	case "sha256", "sha512":
		return true
	default:
		return false
	}
}
