package backend

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type S3Backend struct {
	client      *s3.Client
	uploader    *manager.Uploader
	bucket      string
	prefix      string
	repomdETag  string
	repomdKey   string
	disableETag bool
	tempPrefix  string
	ifMatchETag string
}

// NewS3Backend creates an S3 backend for the provided s3://bucket/prefix root.
// If endpoint is non-empty, it configures the client for S3-compatible storage
// (e.g., MinIO) with path-style addressing.
func NewS3Backend(ctx context.Context, root, endpoint string) (*S3Backend, error) {
	bucket, prefix, err := parseS3URI(root)
	if err != nil {
		return nil, err
	}
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("load AWS config: %w", err)
	}

	// Configure client options for S3-compatible storage (MinIO, etc.)
	var clientOpts []func(*s3.Options)
	if endpoint != "" {
		clientOpts = append(clientOpts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(endpoint)
			o.UsePathStyle = true // Required for MinIO and most S3-compatible storage
		})
	}

	client := s3.NewFromConfig(cfg, clientOpts...)
	uploader := manager.NewUploader(client)
	return &S3Backend{
		client:     client,
		uploader:   uploader,
		bucket:     bucket,
		prefix:     prefix,
		repomdKey:  keyJoin(prefix, "repodata/repomd.xml"),
		tempPrefix: keyJoin(prefix, "repodata/.tmp"),
	}, nil
}

func (b *S3Backend) RepoRoot() string {
	if b.prefix == "" {
		return fmt.Sprintf("s3://%s", b.bucket)
	}
	return fmt.Sprintf("s3://%s/%s", b.bucket, b.prefix)
}

func (b *S3Backend) key(path string) string {
	return keyJoin(b.prefix, path)
}

func keyJoin(prefix, p string) string {
	if p == "" {
		return strings.TrimSuffix(prefix, "/")
	}
	p = path.Clean(p)
	if p == "." {
		return strings.TrimSuffix(prefix, "/")
	}
	p = strings.TrimPrefix(p, "/")
	if prefix == "" {
		return p
	}
	return strings.TrimSuffix(prefix, "/") + "/" + p
}

func parseS3URI(uri string) (bucket, prefix string, err error) {
	if !strings.HasPrefix(uri, "s3://") {
		return "", "", fmt.Errorf("invalid s3 uri %q", uri)
	}
	trim := strings.TrimPrefix(uri, "s3://")
	parts := strings.SplitN(trim, "/", 2)
	bucket = parts[0]
	if bucket == "" {
		return "", "", fmt.Errorf("missing bucket in uri %q", uri)
	}
	if len(parts) == 2 {
		prefix = strings.Trim(parts[1], "/")
	}
	return bucket, prefix, nil
}

func (b *S3Backend) ListRepodata(ctx context.Context) ([]string, error) {
	var out []string
	prefix := keyJoin(b.prefix, "repodata/")
	paginator := s3.NewListObjectsV2Paginator(b.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(b.bucket),
		Prefix: aws.String(prefix),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, obj := range page.Contents {
			if obj.Key == nil {
				continue
			}
			rel := strings.TrimPrefix(*obj.Key, keyJoin(b.prefix, ""))
			rel = strings.TrimPrefix(rel, "/")
			out = append(out, rel)
		}
	}
	return out, nil
}

func (b *S3Backend) ReadFile(ctx context.Context, path string) ([]byte, error) {
	key := b.key(path)
	obj, err := b.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, err
	}
	defer obj.Body.Close()
	data, err := io.ReadAll(obj.Body)
	if err != nil {
		return nil, err
	}
	if key == b.repomdKey && obj.ETag != nil {
		b.repomdETag = strings.Trim(*obj.ETag, "\"")
		b.ifMatchETag = b.repomdETag
	}
	return data, nil
}

func (b *S3Backend) WriteFile(ctx context.Context, path string, data []byte) error {
	key := b.key(path)
	// If writing repodata assets, stage under temp prefix before final put.
	if strings.HasPrefix(path, "repodata/") && !strings.HasSuffix(path, "repomd.xml") {
		tmpKey := b.stageKey(path)
		if err := b.putObject(ctx, tmpKey, data); err != nil {
			return err
		}
		if err := b.copyObject(ctx, tmpKey, key); err != nil {
			return err
		}
		// Clean up temp file after successful copy
		_, _ = b.client.DeleteObject(ctx, &s3.DeleteObjectInput{
			Bucket: aws.String(b.bucket),
			Key:    aws.String(tmpKey),
		})
		return nil
	}
	// For repomd.xml apply conditional put if we have an ETag from read.
	if strings.HasSuffix(path, "repomd.xml") && b.ifMatchETag != "" {
		_, err := b.client.PutObject(ctx, &s3.PutObjectInput{
			Bucket:  aws.String(b.bucket),
			Key:     aws.String(key),
			Body:    bytes.NewReader(data),
			IfMatch: aws.String(b.ifMatchETag),
		})
		return err
	}
	return b.putObject(ctx, key, data)
}

func (b *S3Backend) DeleteFile(ctx context.Context, path string) error {
	key := b.key(path)
	_, err := b.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(key),
	})
	return err
}

func (b *S3Backend) Exists(ctx context.Context, path string) (bool, error) {
	key := b.key(path)
	_, err := b.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(key),
	})
	if err == nil {
		return true, nil
	}
	var nfe *s3types.NotFound
	if errors.As(err, &nfe) {
		return false, nil
	}
	return false, err
}

func (b *S3Backend) ListRPMs(ctx context.Context) ([]string, error) {
	var out []string
	paginator := s3.NewListObjectsV2Paginator(b.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(b.bucket),
		Prefix: aws.String(keyJoin(b.prefix, "")),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, obj := range page.Contents {
			if obj.Key == nil {
				continue
			}
			key := *obj.Key
			rel := strings.TrimPrefix(key, keyJoin(b.prefix, ""))
			rel = strings.TrimPrefix(rel, "/")
			if strings.HasPrefix(rel, "repodata/") {
				continue
			}
			if strings.HasSuffix(rel, ".rpm") {
				out = append(out, rel)
			}
		}
	}
	return out, nil
}

// CheckRepomdUnchanged compares the current repomd ETag with the cached one.
func (b *S3Backend) CheckRepomdUnchanged(ctx context.Context) error {
	if b.disableETag || b.repomdETag == "" {
		return nil
	}
	head, err := b.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(b.repomdKey),
	})
	if err != nil {
		return err
	}
	current := strings.Trim(aws.ToString(head.ETag), "\"")
	if current != b.repomdETag {
		return fmt.Errorf("conflict: repomd.xml changed since read (etag %s -> %s)", b.repomdETag, current)
	}
	return nil
}

func (b *S3Backend) putObject(ctx context.Context, key string, data []byte) error {
	_, err := b.uploader.Upload(ctx, &s3.PutObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(key),
		Body:   bytes.NewReader(data),
	})
	return err
}

func (b *S3Backend) copyObject(ctx context.Context, srcKey, dstKey string) error {
	_, err := b.client.CopyObject(ctx, &s3.CopyObjectInput{
		Bucket:     aws.String(b.bucket),
		CopySource: aws.String(path.Join("/", b.bucket, srcKey)),
		Key:        aws.String(dstKey),
	})
	return err
}

func (b *S3Backend) stageKey(path string) string {
	base := strings.TrimPrefix(path, "repodata/")
	return keyJoin(b.tempPrefix, base)
}
