package service

import (
	"context"
	"fmt"
	"io"
	"mime"
	"path"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const DeveloperFileMaxBytes int64 = 512 << 20

var ErrDeveloperFileForbidden = infraerrors.Forbidden("DEVELOPER_FILE_FORBIDDEN", "file does not belong to the authenticated user")

func ErrDeveloperFileTooLarge() error {
	return infraerrors.BadRequest("FILE_TOO_LARGE", "file exceeds the 512 MiB limit")
}

type DeveloperFile struct {
	URL         string `json:"url"`
	SizeBytes   int64  `json:"size"`
	ContentType string `json:"content_type"`
}

type DeveloperFileService struct {
	cos       *COSImageTransferService
	materials *UserMaterialService
}

func NewDeveloperFileService(cos *COSImageTransferService, materials *UserMaterialService) *DeveloperFileService {
	return &DeveloperFileService{cos: cos, materials: materials}
}

func (s *DeveloperFileService) Upload(ctx context.Context, userID int64, filename, contentType string, size int64, body io.Reader) (*DeveloperFile, error) {
	if s == nil || s.cos == nil || s.materials == nil {
		return nil, fmt.Errorf("developer file: nil dependencies")
	}
	if userID <= 0 {
		return nil, ErrDeveloperKeyInvalid
	}
	if size <= 0 {
		return nil, infraerrors.BadRequest("EMPTY_FILE", "uploaded file is empty")
	}
	if size > DeveloperFileMaxBytes {
		return nil, ErrDeveloperFileTooLarge()
	}
	prefix, err := s.userUploadPrefix(ctx, userID)
	if err != nil {
		return nil, err
	}
	ct := strings.TrimSpace(strings.SplitN(contentType, ";", 2)[0])
	if ct == "" {
		ct = mime.TypeByExtension(path.Ext(filename))
	}
	if ct == "" {
		ct = "application/octet-stream"
	}
	key := fmt.Sprintf("%s/%s/%s%s", prefix, developerFileNow().Format("2006/01"), uuid.NewString(), safeDeveloperFileExtension(filename))
	publicURL, err := s.cos.UploadReaderWithKey(ctx, key, body, size, ct)
	if err != nil {
		return nil, err
	}
	return &DeveloperFile{URL: publicURL, SizeBytes: size, ContentType: ct}, nil
}

func (s *DeveloperFileService) Delete(ctx context.Context, userID int64, rawURL string) error {
	if s == nil || s.cos == nil || s.materials == nil {
		return fmt.Errorf("developer file: nil dependencies")
	}
	key, err := s.cos.PublicURLToKey(ctx, rawURL)
	if err != nil {
		return err
	}
	prefix, err := s.userUploadPrefix(ctx, userID)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(key, prefix+"/") {
		return ErrDeveloperFileForbidden
	}
	return s.cos.DeleteObject(ctx, key)
}

func (s *DeveloperFileService) userUploadPrefix(ctx context.Context, userID int64) (string, error) {
	cfg, err := s.cos.GetConfig(ctx)
	if err != nil {
		return "", err
	}
	if cfg == nil || !cfg.Enabled || !cfg.IsConfigured() {
		return "", ErrCOSNotConfigured
	}
	userPrefix, err := s.materials.UserStoragePrefix(ctx, userID)
	if err != nil {
		return "", err
	}
	return developerFileUserPrefix(cfg, userPrefix)
}

func developerFileRoot(cfg *COSImageConfig) string {
	prefix := strings.Trim(strings.TrimSpace(cfg.Prefix), "/")
	if prefix == "" {
		return "file_uploads"
	}
	return prefix + "/file_uploads"
}

func developerFileUserPrefix(cfg *COSImageConfig, userPrefix string) (string, error) {
	return sanitizeObjectKey(developerFileRoot(cfg) + "/" + strings.Trim(userPrefix, "/"))
}

func safeDeveloperFileExtension(filename string) string {
	ext := strings.ToLower(path.Ext(path.Base(strings.ReplaceAll(filename, "\\", "/"))))
	if len(ext) < 2 || len(ext) > 17 {
		return ".bin"
	}
	for _, r := range ext[1:] {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return ".bin"
		}
	}
	return ext
}

var developerFileNow = func() time.Time { return time.Now().UTC() }
