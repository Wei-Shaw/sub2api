package repository

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/Wei-Shaw/sub2api/internal/pkg/servertiming"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// S3BackupStore implements service.BackupObjectStore using AWS S3 compatible storage
type S3BackupStore struct {
	client *s3.Client
	bucket string
}

// NewS3BackupStoreFactory returns a BackupObjectStoreFactory that creates S3-backed stores
func NewS3BackupStoreFactory() service.BackupObjectStoreFactory {
	return func(ctx context.Context, cfg *service.BackupS3Config) (service.BackupObjectStore, error) {
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
		return &S3BackupStore{client: client, bucket: cfg.Bucket}, nil
	}
}

func (s *S3BackupStore) Upload(ctx context.Context, key string, body io.Reader, contentType string) (int64, error) {
	// 读取全部内容以获取大小（S3 PutObject 需要知道内容长度）
	// 注意：阿里云 OSS 不兼容 s3manager 分片上传的签名方式，因此使用 PutObject
	data, err := io.ReadAll(body)
	if err != nil {
		return 0, fmt.Errorf("read body: %w", err)
	}

	finish := servertiming.ObserveDependency(ctx, "s3")
	_, err = s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      &s.bucket,
		Key:         &key,
		Body:        bytes.NewReader(data),
		ContentType: &contentType,
	})
	finish()
	if err != nil {
		return 0, fmt.Errorf("S3 PutObject: %w", err)
	}
	return int64(len(data)), nil
}

func (s *S3BackupStore) UploadFile(ctx context.Context, key string, filePath string, contentType string) (int64, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return 0, fmt.Errorf("open upload file: %w", err)
	}
	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil {
		return 0, fmt.Errorf("stat upload file: %w", err)
	}
	sizeBytes := info.Size()

	finish := servertiming.ObserveDependency(ctx, "s3")
	_, err = s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        &s.bucket,
		Key:           &key,
		Body:          file,
		ContentLength: &sizeBytes,
		ContentType:   &contentType,
	})
	finish()
	if err != nil {
		return 0, fmt.Errorf("S3 PutObject file: %w", err)
	}
	return sizeBytes, nil
}

func (s *S3BackupStore) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	finish := servertiming.ObserveDependency(ctx, "s3")
	result, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: &s.bucket,
		Key:    &key,
	})
	finish()
	if err != nil {
		return nil, fmt.Errorf("S3 GetObject: %w", err)
	}
	return result.Body, nil
}

func (s *S3BackupStore) Delete(ctx context.Context, key string) error {
	finish := servertiming.ObserveDependency(ctx, "s3")
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: &s.bucket,
		Key:    &key,
	})
	finish()
	return err
}

func (s *S3BackupStore) PresignURL(ctx context.Context, key string, expiry time.Duration) (string, error) {
	presignClient := s3.NewPresignClient(s.client)
	// 强制 attachment disposition：浏览器同页导航该 URL 时直接触发下载而非渲染，
	// 前端无需依赖会被弹窗拦截的新标签页。
	disposition := fmt.Sprintf("attachment; filename=%q", path.Base(key))
	result, err := presignClient.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket:                     &s.bucket,
		Key:                        &key,
		ResponseContentDisposition: &disposition,
	}, s3.WithPresignExpires(expiry))
	if err != nil {
		return "", fmt.Errorf("presign url: %w", err)
	}
	return result.URL, nil
}

func (s *S3BackupStore) HeadBucket(ctx context.Context) error {
	finish := servertiming.ObserveDependency(ctx, "s3")
	_, err := s.client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: &s.bucket,
	})
	finish()
	if err != nil {
		return fmt.Errorf("S3 HeadBucket failed: %w", err)
	}
	return nil
}

// 编译期断言：S3BackupStore 实现了两个可选扩展接口。
// 少了这两行，接口签名改动时只会在运行时的类型断言处静默降级成"不支持"。
var (
	_ service.BackupObjectLister = (*S3BackupStore)(nil)
	_ service.BackupObjectCopier = (*S3BackupStore)(nil)
)

// ListObjects 实现 service.BackupObjectLister。
//
// delimiter 非空时（通常传 "/"）走 S3 的 CommonPrefixes 聚合，返回"目录 + 本层
// 文件"，这样管理端可以逐层浏览，而不是一次把几万个对象全拉下来。
//
// 分页用 ContinuationToken（S3 只提供游标式分页，没有 offset 跳页）。
func (s *S3BackupStore) ListObjects(
	ctx context.Context,
	prefix, delimiter, token string,
	limit int32,
) (*service.ObjectPage, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		// S3 单次 MaxKeys 上限即 1000，传更大值不会生效，这里直接夹住。
		limit = 1000
	}
	in := &s3.ListObjectsV2Input{
		Bucket:  &s.bucket,
		MaxKeys: &limit,
	}
	if prefix != "" {
		in.Prefix = &prefix
	}
	if delimiter != "" {
		in.Delimiter = &delimiter
	}
	if token != "" {
		in.ContinuationToken = &token
	}

	finish := servertiming.ObserveDependency(ctx, "s3")
	out, err := s.client.ListObjectsV2(ctx, in)
	finish()
	if err != nil {
		return nil, fmt.Errorf("S3 ListObjectsV2: %w", err)
	}

	page := &service.ObjectPage{
		Entries: make([]service.ObjectEntry, 0, len(out.Contents)+len(out.CommonPrefixes)),
	}
	// 目录（CommonPrefixes）排在前面，符合文件管理器的习惯。
	for _, cp := range out.CommonPrefixes {
		if cp.Prefix == nil {
			continue
		}
		page.Entries = append(page.Entries, service.ObjectEntry{
			Key:   *cp.Prefix,
			IsDir: true,
		})
	}
	for _, obj := range out.Contents {
		if obj.Key == nil {
			continue
		}
		// 跳过"目录占位对象"：某些工具会创建 key 以 "/" 结尾、大小为 0 的对象来
		// 模拟空目录。把它当文件列出来会让用户看到一条无法下载的怪条目。
		if strings.HasSuffix(*obj.Key, "/") {
			continue
		}
		e := service.ObjectEntry{Key: *obj.Key}
		if obj.Size != nil {
			e.Size = *obj.Size
		}
		if obj.LastModified != nil {
			e.LastModified = *obj.LastModified
		}
		if obj.ETag != nil {
			e.ETag = strings.Trim(*obj.ETag, `"`)
		}
		page.Entries = append(page.Entries, e)
	}
	if out.IsTruncated != nil && *out.IsTruncated && out.NextContinuationToken != nil {
		page.NextToken = *out.NextContinuationToken
	}
	return page, nil
}

// CopyObject 实现 service.BackupObjectCopier（服务端复制，用于重命名/移动）。
//
// CopySource 必须是 "bucket/key" 且经过 URL 转义 —— key 里出现空格、中文、
// "+"、"?" 时不转义会被上游解析成别的对象或直接 400。
func (s *S3BackupStore) CopyObject(ctx context.Context, srcKey, dstKey string) error {
	source := url.PathEscape(s.bucket + "/" + srcKey)
	// PathEscape 会把 "/" 也转义成 %2F。S3 要求 bucket 与 key 之间保留真实的 "/"，
	// 而 key 内部的 "/" 同样应保留（层级语义），因此把 %2F 还原回来。
	source = strings.ReplaceAll(source, "%2F", "/")

	finish := servertiming.ObserveDependency(ctx, "s3")
	_, err := s.client.CopyObject(ctx, &s3.CopyObjectInput{
		Bucket:     &s.bucket,
		Key:        &dstKey,
		CopySource: &source,
	})
	finish()
	if err != nil {
		return fmt.Errorf("S3 CopyObject: %w", err)
	}
	return nil
}
