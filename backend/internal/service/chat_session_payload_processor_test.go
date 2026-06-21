package service

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/config"
)

func TestChatSessionPayloadProcessorProcessTaskMarksPathProcessed(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectExec("UPDATE chat_messages").
		WithArgs("2026-06-11/abc-message.json.gz", "sha-1", "failed", sqlmock.AnyArg(), "", nil, nil).
		WillReturnResult(sqlmock.NewResult(0, 1))

	processor := NewChatSessionPayloadProcessor(db, nil, nil)
	err = processor.processTask(context.Background(), ChatSessionPayloadTask{
		Path:   "2026-06-11/abc-message.json.gz",
		SHA256: "sha-1",
	})
	if err != nil {
		t.Fatalf("processTask: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestChatSessionPayloadProcessorProcessTaskIgnoresEmptyPath(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	processor := NewChatSessionPayloadProcessor(db, nil, nil)
	err = processor.processTask(context.Background(), ChatSessionPayloadTask{})
	if err != nil {
		t.Fatalf("processTask: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestChatSessionPayloadProcessorProcessTaskNilDB(t *testing.T) {
	processor := NewChatSessionPayloadProcessor((*sql.DB)(nil), nil, nil)
	err := processor.processTask(context.Background(), ChatSessionPayloadTask{
		Path: "2026-06-11/abc-message.json.gz",
	})
	if err != nil {
		t.Fatalf("processTask: %v", err)
	}
}

func TestChatSessionPayloadProcessorRewritesBase64ImagesAndKeepsPayloadPath(t *testing.T) {
	baseDir := t.TempDir()
	day := "2026-06-11"
	payloadRelPath := filepath.ToSlash(filepath.Join(day, "payload-message.json.gz"))
	payloadFullPath := filepath.Join(baseDir, day, "payload-message.json.gz")
	if err := os.MkdirAll(filepath.Dir(payloadFullPath), 0750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	imageBytes := append([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}, bytes.Repeat([]byte{1}, 20*1024)...)
	imageDataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(imageBytes)
	payload := map[string]any{
		"input": []any{
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{"type": "input_text", "text": "hello"},
					map[string]any{"type": "input_image", "image_url": imageDataURL},
					map[string]any{"type": "input_image", "image_url": imageDataURL},
				},
			},
		},
	}
	rawPayload, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	compressedPayload, err := gzipPayloadForTest(rawPayload)
	if err != nil {
		t.Fatalf("gzip payload: %v", err)
	}
	if err := os.WriteFile(payloadFullPath, compressedPayload, 0640); err != nil {
		t.Fatalf("write payload: %v", err)
	}
	sum := sha256.Sum256(rawPayload)

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectExec("UPDATE chat_messages").
		WithArgs(payloadRelPath, hex.EncodeToString(sum[:]), "processed", nil, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))

	processor := NewChatSessionPayloadProcessor(db, nil, &config.Config{
		ChatSessionRetention: config.ChatSessionRetentionConfig{
			PayloadDir: baseDir,
		},
	})
	err = processor.processTask(context.Background(), ChatSessionPayloadTask{
		Path:         payloadRelPath,
		SHA256:       hex.EncodeToString(sum[:]),
		Compression:  "gzip",
		CreatedAtUTC: time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatalf("processTask: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}

	rewrittenGzip, err := os.ReadFile(payloadFullPath)
	if err != nil {
		t.Fatalf("read rewritten payload: %v", err)
	}
	rewrittenPayload, err := gunzipPayloadForTest(rewrittenGzip)
	if err != nil {
		t.Fatalf("gunzip rewritten payload: %v", err)
	}
	if bytes.Contains(rewrittenPayload, []byte("data:image/png;base64")) {
		t.Fatalf("rewritten payload still contains data URL")
	}
	if !bytes.Contains(rewrittenPayload, []byte(chatSessionMediaReferenceType)) {
		t.Fatalf("rewritten payload does not contain media refs: %s", rewrittenPayload)
	}
	imageSum := sha256.Sum256(imageBytes)
	imagePath := filepath.Join(baseDir, day, "media", hex.EncodeToString(imageSum[:])+".png")
	if _, err := os.Stat(imagePath); err != nil {
		t.Fatalf("expected image file: %v", err)
	}
}

func gzipPayloadForTest(raw []byte) ([]byte, error) {
	var buf bytes.Buffer
	writer := gzip.NewWriter(&buf)
	if _, err := writer.Write(raw); err != nil {
		_ = writer.Close()
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func gunzipPayloadForTest(raw []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(reader)
}
