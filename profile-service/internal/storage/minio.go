package storage

import (
	"context"
	"fmt"
	"io"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

const Bucket = "drizzy-photos"

type MinIO struct {
	client *minio.Client
}

func New(endpoint, accessKey, secretKey string, useSSL bool) (*MinIO, error) {
	mc, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("minio client: %w", err)
	}
	return &MinIO{client: mc}, nil
}

func (m *MinIO) EnsureBucket(ctx context.Context) error {
	exists, err := m.client.BucketExists(ctx, Bucket)
	if err != nil {
		return fmt.Errorf("bucket exists check: %w", err)
	}
	if exists {
		return nil
	}
	if err = m.client.MakeBucket(ctx, Bucket, minio.MakeBucketOptions{}); err != nil {
		return fmt.Errorf("make bucket: %w", err)
	}
	return nil
}

func (m *MinIO) PutObject(ctx context.Context, key, contentType string, size int64, r io.Reader) error {
	_, err := m.client.PutObject(ctx, Bucket, key, r, size,
		minio.PutObjectOptions{ContentType: contentType},
	)
	if err != nil {
		return fmt.Errorf("put object %s: %w", key, err)
	}
	return nil
}

func (m *MinIO) GetObject(ctx context.Context, key string) (io.ReadCloser, int64, string, error) {
	obj, err := m.client.GetObject(ctx, Bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, 0, "", fmt.Errorf("get object %s: %w", key, err)
	}
	info, err := obj.Stat()
	if err != nil {
		obj.Close()
		return nil, 0, "", fmt.Errorf("stat object %s: %w", key, err)
	}
	return obj, info.Size, info.ContentType, nil
}
