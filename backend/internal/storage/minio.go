package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime"
	"path"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type MinioStore struct {
	Client *minio.Client
	Bucket string
}

func NewMinioStore(endpoint, accessKey, secretKey string, secure bool, bucket string) (*MinioStore, error) {
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: secure,
	})
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	exists, err := client.BucketExists(ctx, bucket)
	if err != nil {
		return nil, err
	}
	if !exists {
		if err := client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
			return nil, err
		}
	}

	return &MinioStore{Client: client, Bucket: bucket}, nil
}

func (s *MinioStore) PutBytes(ctx context.Context, objectPath string, data []byte, contentType string) error {
	reader := bytes.NewReader(data)
	_, err := s.Client.PutObject(ctx, s.Bucket, objectPath, reader, int64(len(data)), minio.PutObjectOptions{
		ContentType: contentType,
	})
	return err
}

func (s *MinioStore) PutStream(ctx context.Context, objectPath string, r io.Reader, size int64, contentType string) error {
	_, err := s.Client.PutObject(ctx, s.Bucket, objectPath, r, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	return err
}

func (s *MinioStore) Get(ctx context.Context, objectPath string) (*minio.Object, error) {
	return s.Client.GetObject(ctx, s.Bucket, objectPath, minio.GetObjectOptions{})
}

func GuessContentType(filename string, fallback string) string {
	if ext := path.Ext(filename); ext != "" {
		if ct := mime.TypeByExtension(ext); ct != "" {
			return ct
		}
	}
	if strings.TrimSpace(fallback) != "" {
		return fallback
	}
	return "application/octet-stream"
}

func ArchivePrefix(archiveID string) string {
	return fmt.Sprintf("archives/%s", archiveID)
}
