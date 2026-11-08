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
// 【2026-10-24 撤掉了 Content-Disposition: attachment】
//
// 历史：2026-10-22 为了让「点下载直接落到浏览器下载目录」，
// 这里给每个对象写死了 `attachment`。当时图片域名没有 CORS、页面读不到字节，
// 只能靠「导航 + attachment」让浏览器下载，这是唯一可行的办法。
//
// 为什么现在撤掉：
//   - 2026-10-23 加了同源反代 `chenshuapi.com/img-proxy/*`，页面能从同源地址
//     读到字节，下载改用 blob + <a download>，**不再依赖这个头**；
//   - 主人反馈「Ozon 上架时粘贴图片链接老是识别不了、要反复粘贴」，而那张图是
//     刚生成的、传输正常（实测 1.6MB / 0.15s）。响应里唯一不正常的就是这个头：
//     `attachment` 是**文件下载**语义，第三方抓图程序可能因此把它当附件而不是图片；
//   - 图片直链是给外部系统用的，应该按图片的正常语义返回。
//
// 现在图片直链是一个干净的图片响应（只有 Content-Type）。
// 想「点了直接下载」走的是 /img-proxy（那一条在 Caddy 里单独加了 attachment）。
//
// 只影响 images/ 前缀（走本类）；数据库备份走 backup_s3_store.go，不受影响。
func (s *S3ImageStorage) Save(ctx context.Context, key, contentType string, data []byte) (string, error) {
	finish := servertiming.ObserveDependency(ctx, "s3")
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      &s.bucket,
		Key:         &key,
		Body:        bytes.NewReader(data),
		ContentType: &contentType,
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
