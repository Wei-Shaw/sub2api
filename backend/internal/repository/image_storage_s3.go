package repository

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/servertiming"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// S3ImageStorage 用 S3 兼容对象存储实现 service.ImageStorage。
type S3ImageStorage struct {
	client        *s3.Client
	bucket        string
	publicBaseURL string
	presignExpiry time.Duration
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

	expiry := time.Duration(cfg.PresignExpiry) * time.Hour
	if expiry <= 0 {
		expiry = 24 * time.Hour
	}

	return &S3ImageStorage{
		client:        client,
		bucket:        cfg.Bucket,
		publicBaseURL: strings.TrimRight(cfg.PublicBaseURL, "/"),
		presignExpiry: expiry,
	}, nil
}

// Save 上传图片字节，返回可访问 URL：配了 public_base_url 则返回公开直链，否则返回 presigned 临时链接。
//
// 【2026-10-22 加 Content-Disposition: attachment】主人要求「点下载直接落到浏览器下载目录」。
//
// 为什么这一行就能解决：
//
//	图片域名 img.chenshuapi.com 没有配 CORS，所以页面里 `fetch()` 那张图会抛
//	`Failed to fetch`，前端拿不到字节，只能退化成「新开标签页让用户右键另存」。
//	配 CORS 需要 Cloudflare 后台的 bucket 级权限（现有密钥没有，实测 6 种写法全 403）。
//
//	而浏览器**导航**到一个地址不受 CORS 限制。响应里带 `attachment` 时，
//	浏览器会把它当文件下载而不是打开显示 —— 于是「点一下 → 直接进下载目录」成立。
//
// 已实测（真浏览器）：
//   - 带 attachment 的图导航过去 → 文件落到下载目录 ✅
//   - 同一张图用 <img> 加载 → 正常显示 ✅（页面看图不受影响）
//   - 同一张图 fetch → 仍然失败（CORS 依旧缺，但已经不需要它了）
//
// 只影响 images/ 前缀（走本类）；数据库备份走 backup_s3_store.go，不受影响。
func (s *S3ImageStorage) Save(ctx context.Context, key, contentType string, data []byte) (string, error) {
	finish := servertiming.ObserveDependency(ctx, "s3")
	// 文件名取 key 的最后一段（形如 imgtask_xxx-0.png），下载下来能看出是哪张图。
	disposition := "attachment"
	if name := key[strings.LastIndex(key, "/")+1:]; name != "" {
		disposition = "attachment; filename=\"" + name + "\""
	}
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      &s.bucket,
		Key:         &key,
		Body:        bytes.NewReader(data),
		ContentType: &contentType,
		// 让浏览器把图片当文件下载，而不是在标签页里打开。
		ContentDisposition: &disposition,
	})
	finish()
	if err != nil {
		return "", fmt.Errorf("S3 PutObject: %w", err)
	}

	if s.publicBaseURL != "" {
		return s.publicBaseURL + "/" + strings.TrimLeft(key, "/"), nil
	}

	presignClient := s3.NewPresignClient(s.client)
	result, err := presignClient.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: &s.bucket,
		Key:    &key,
	}, s3.WithPresignExpires(s.presignExpiry))
	if err != nil {
		return "", fmt.Errorf("presign url: %w", err)
	}
	return result.URL, nil
}
