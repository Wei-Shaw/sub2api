package repository

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/servertiming"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// S3ImageStorage 用 S3 兼容对象存储实现 service.ImageStorage。
type S3ImageStorage struct {
	client       *s3.Client
	getObject    func(context.Context, *s3.GetObjectInput, ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	deleteObject func(context.Context, *s3.DeleteObjectInput, ...func(*s3.Options)) (*s3.DeleteObjectOutput, error)
	bucket       string
}

var _ service.ImageStorage = (*S3ImageStorage)(nil)

// NewS3ImageStorage 依据配置构造 S3 图片存储（调用方应先确认 cfg.Active()）。
func NewS3ImageStorage(ctx context.Context, cfg *config.ImageStorageConfig) (*S3ImageStorage, error) {
	client, err := newS3Client(ctx, s3ClientParams{
		Endpoint:        cfg.Endpoint,
		Region:          cfg.Region,
		AccessKeyID:     cfg.AccessKeyID,
		SecretAccessKey: cfg.SecretAccessKey,
		ForcePathStyle:  cfg.ForcePathStyle,
	})
	if err != nil {
		return nil, err
	}

	return &S3ImageStorage{
		client:       client,
		getObject:    client.GetObject,
		deleteObject: client.DeleteObject,
		bucket:       cfg.Bucket,
	}, nil
}

func (s *S3ImageStorage) Save(ctx context.Context, key, contentType string, data []byte) error {
	finish := servertiming.ObserveDependency(ctx, "s3")
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      &s.bucket,
		Key:         &key,
		Body:        bytes.NewReader(data),
		ContentType: &contentType,
	})
	finish()
	if err != nil {
		return fmt.Errorf("S3 PutObject: %w", err)
	}
	return nil
}

func (s *S3ImageStorage) Load(ctx context.Context, key string, maxBytes int64) ([]byte, string, error) {
	if maxBytes <= 0 {
		return nil, "", fmt.Errorf("S3 GetObject: invalid maximum size")
	}
	finish := servertiming.ObserveDependency(ctx, "s3")
	getObject := s.getObject
	if getObject == nil && s.client != nil {
		getObject = s.client.GetObject
	}
	if getObject == nil {
		finish()
		return nil, "", fmt.Errorf("S3 GetObject: client is unavailable")
	}
	output, err := getObject(ctx, &s3.GetObjectInput{Bucket: &s.bucket, Key: &key})
	finish()
	if err != nil {
		return nil, "", fmt.Errorf("S3 GetObject: %w", err)
	}
	if output == nil || output.Body == nil {
		return nil, "", fmt.Errorf("S3 GetObject: empty body")
	}
	defer func() { _ = output.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(output.Body, maxBytes+1))
	if err != nil {
		return nil, "", fmt.Errorf("S3 GetObject read: %w", err)
	}
	if int64(len(data)) > maxBytes {
		return nil, "", fmt.Errorf("S3 GetObject exceeds %d bytes", maxBytes)
	}
	contentType := strings.ToLower(http.DetectContentType(data))
	if !strings.HasPrefix(contentType, "image/") {
		return nil, "", fmt.Errorf("S3 GetObject content type %q is not an image", contentType)
	}
	return data, contentType, nil
}

func (s *S3ImageStorage) Delete(ctx context.Context, key string) error {
	finish := servertiming.ObserveDependency(ctx, "s3")
	deleteObject := s.deleteObject
	if deleteObject == nil && s.client != nil {
		deleteObject = s.client.DeleteObject
	}
	if deleteObject == nil {
		finish()
		return fmt.Errorf("S3 DeleteObject: client is unavailable")
	}
	_, err := deleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: &s.bucket,
		Key:    &key,
	})
	finish()
	if err != nil {
		return fmt.Errorf("S3 DeleteObject: %w", err)
	}
	return nil
}
