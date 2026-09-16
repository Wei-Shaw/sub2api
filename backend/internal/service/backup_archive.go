package service

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
)

const defaultBackupPartSizeBytes int64 = 4 * 1024 * 1024 * 1024

// BackupPart 描述一个 gzip 字节分卷。
type BackupPart struct {
	Index     int    `json:"index"`
	S3Key     string `json:"s3_key"`
	SizeBytes int64  `json:"size_bytes"`
	SHA256    string `json:"sha256,omitempty"`
}

type localBackupPart struct {
	Index     int
	Path      string
	SizeBytes int64
	SHA256    string
}

func splitBackupFile(srcPath string, partSize int64) (parts []localBackupPart, err error) {
	if partSize <= 0 {
		return nil, fmt.Errorf("backup part size must be positive")
	}

	src, err := os.Open(srcPath)
	if err != nil {
		return nil, fmt.Errorf("open backup archive: %w", err)
	}
	defer func() {
		if closeErr := src.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("close backup archive: %w", closeErr)
		}
		if err != nil {
			paths := make([]string, 0, len(parts))
			for _, part := range parts {
				paths = append(paths, part.Path)
			}
			_ = cleanupBackupFiles(paths...)
		}
	}()

	info, err := src.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat backup archive: %w", err)
	}
	if info.Size() <= 0 {
		return nil, errors.New("backup archive is empty")
	}

	remaining := info.Size()
	for index := 1; remaining > 0; index++ {
		partFile, createErr := os.CreateTemp("", "sub2api-backup-part-*")
		if createErr != nil {
			return nil, fmt.Errorf("create backup part: %w", createErr)
		}
		partPath := partFile.Name()
		partBytes := partSize
		if remaining < partBytes {
			partBytes = remaining
		}

		hash := sha256.New()
		written, copyErr := io.CopyN(io.MultiWriter(partFile, hash), src, partBytes)
		closeErr := partFile.Close()
		if copyErr != nil {
			_ = os.Remove(partPath)
			return nil, fmt.Errorf("write backup part %d: %w", index, copyErr)
		}
		if closeErr != nil {
			_ = os.Remove(partPath)
			return nil, fmt.Errorf("close backup part %d: %w", index, closeErr)
		}

		parts = append(parts, localBackupPart{
			Index:     index,
			Path:      partPath,
			SizeBytes: written,
			SHA256:    hex.EncodeToString(hash.Sum(nil)),
		})
		remaining -= written
	}

	return parts, nil
}

func cleanupBackupFiles(paths ...string) error {
	var errs []error
	for _, path := range paths {
		if path == "" {
			continue
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, fmt.Errorf("remove %s: %w", path, err))
		}
	}
	return errors.Join(errs...)
}
