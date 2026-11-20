package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/tidwall/gjson"
)

const (
	DeepseekASRFileLimit      = 8 << 20
	DeepseekDocumentFileLimit = 100 << 20
	DeepseekMultipartOverhead = 1 << 20
)

func checkDeepseekFilePart(part *multipart.Part, limit int64) (int64, error) {
	size, err := io.Copy(io.Discard, io.LimitReader(part, limit+1))
	if err != nil {
		return 0, invalidFileRequest("Invalid multipart file upload")
	}
	if size > limit {
		return 0, infraerrors.New(413, "request_too_large", "File exceeds the DeepSeek file size limit")
	}
	return size, nil
}

func validateDeepseekMultipartSize(body []byte, contentType, endpoint string) error {
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil || mediaType != "multipart/form-data" {
		return nil // The upstream validates the request format.
	}
	limit := int64(DeepseekDocumentFileLimit)
	if endpoint == "asr" {
		limit = DeepseekASRFileLimit
	}
	reader := multipart.NewReader(bytes.NewReader(body), params["boundary"])
	for {
		part, err := reader.NextRawPart()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return invalidFileRequest("Invalid multipart file upload")
		}
		if part.FormName() == "file" {
			if _, err := checkDeepseekFilePart(part, limit); err != nil {
				return err
			}
		}
	}
}

type deepseekUploadMetadata struct {
	purpose  string
	mimeType string
	filename string
	bytes    int64
}

func invalidFileRequest(message string) error {
	return infraerrors.BadRequest("invalid_request_error", message)
}

func (s *OpenAIGatewayService) checkDeepseekUploadPurpose(purpose string) error {
	if (purpose == "text_recognition" || purpose == "rich_layout_recognition") &&
		(s.cfg == nil || !s.cfg.Gateway.AllowUnbilledDeepseekDocumentParsing) {
		return invalidFileRequest("Document parsing uploads are disabled. Use /v1/tools/ocr or upload with purpose=user_data")
	}
	return nil
}

func (s *OpenAIGatewayService) deepseekUploadMetadata(body []byte, contentType string) (deepseekUploadMetadata, error) {
	meta := deepseekUploadMetadata{purpose: "user_data", mimeType: "application/octet-stream"}
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil || mediaType != "multipart/form-data" || params["boundary"] == "" {
		return meta, invalidFileRequest("File uploads require multipart/form-data with a boundary")
	}
	reader := multipart.NewReader(bytes.NewReader(body), params["boundary"])
	havePurpose := false
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			return meta, nil
		}
		if err != nil {
			return meta, invalidFileRequest("Invalid multipart file upload")
		}
		switch part.FormName() {
		case "file":
			meta.filename = part.FileName()
			meta.bytes, err = checkDeepseekFilePart(part, DeepseekDocumentFileLimit)
			if err != nil {
				return meta, err
			}
			if t, _, err := mime.ParseMediaType(part.Header.Get("Content-Type")); err == nil {
				meta.mimeType = t
			}
		case "purpose":
			value, err := io.ReadAll(io.LimitReader(part, 65))
			if err != nil || len(value) > 64 || havePurpose {
				return meta, invalidFileRequest("Invalid or duplicate file purpose")
			}
			havePurpose = true
			meta.purpose = strings.TrimSpace(string(value))
			if err := s.checkDeepseekUploadPurpose(meta.purpose); err != nil {
				return meta, err
			}
		}
	}
}

func deepseekFileTime(value gjson.Result) (time.Time, error) {
	if value.Type == gjson.Number {
		seconds, err := strconv.ParseInt(value.Raw, 10, 64)
		return time.Unix(seconds, 0).UTC(), err
	}
	t, err := time.Parse(time.RFC3339Nano, value.String())
	return t.UTC(), err
}

func deepseekUploadedFile(data []byte, accountID int64, anthropic bool, upload deepseekUploadMetadata) (DeepseekFile, error) {
	obj := gjson.ParseBytes(data)
	file := DeepseekFile{ID: obj.Get("id").String(), AccountID: accountID, Filename: upload.filename,
		Purpose: upload.purpose, MIMEType: upload.mimeType, Bytes: upload.bytes, Format: "openai"}
	kind, size := obj.Get("object").String(), obj.Get("bytes")
	if anthropic {
		file.Format = "anthropic"
		kind, size = obj.Get("type").String(), obj.Get("size_bytes")
		if mimeType := obj.Get("mime_type").String(); mimeType != "" {
			file.MIMEType = mimeType
		}
	}
	if size.Exists() {
		file.Bytes = size.Int()
	}
	if filename := obj.Get("filename").String(); filename != "" {
		file.Filename = filename
	}
	if purpose := obj.Get("purpose").String(); purpose != "" {
		file.Purpose = purpose
	}
	if !isValidDeepseekFileID(file.ID) || kind != "file" {
		return file, errors.New("invalid uploaded file object")
	}
	if created := obj.Get("created_at"); created.Exists() {
		t, err := deepseekFileTime(created)
		if err != nil {
			return file, err
		}
		file.CreatedAt = t
	} else {
		file.CreatedAt = time.Now().UTC()
	}
	if exp := obj.Get("expires_at"); exp.Exists() && exp.Type != gjson.Null {
		t, err := deepseekFileTime(exp)
		if err != nil {
			return file, err
		}
		file.ExpiresAt = t.Unix()
	}
	return file, nil
}

func deepseekMissingFileResponse(data []byte) bool {
	code := gjson.GetBytes(data, "error.code").String()
	kind := gjson.GetBytes(data, "error.type").String()
	message := strings.ToLower(gjson.GetBytes(data, "error.message").String())
	return code == "file_not_found" || kind == "file_not_found" ||
		(strings.Contains(message, "file") && (strings.Contains(message, "not found") || strings.Contains(message, "does not exist")))
}

func deepseekFileDeleted(data []byte, id string, anthropic bool) bool {
	result := gjson.ParseBytes(data)
	if result.Get("id").String() != id || result.Get("error").Exists() {
		return false
	}
	if anthropic {
		return result.Get("type").String() == "file_deleted"
	}
	return result.Get("deleted").Type == gjson.True
}

func (f DeepseekFile) publicMetadata(anthropic bool) map[string]any {
	if anthropic {
		return map[string]any{"id": f.ID, "type": "file", "filename": f.Filename,
			"size_bytes": f.Bytes, "mime_type": f.MIMEType, "created_at": f.CreatedAt.Format(time.RFC3339Nano)}
	}
	obj := map[string]any{"id": f.ID, "object": "file", "filename": f.Filename,
		"bytes": f.Bytes, "created_at": f.CreatedAt.Unix(), "purpose": f.Purpose}
	if f.ExpiresAt != 0 {
		obj["expires_at"] = f.ExpiresAt
	}
	return obj
}

// ListDeepseekFiles reads up to Limit+1 live records in scan direction.
// Before pages scan backwards and are reversed before returning to the client.
type DeepseekFilePage struct {
	Limit              int
	Descending, Before bool
	Cursor, Purpose    string
}

var ErrDeepseekFileCursor = errors.New("file list cursor not found")

func (s *OpenAIGatewayService) ListDeepseekFiles(ctx context.Context, userID int64, rawQuery string, anthropic bool) (map[string]any, error) {
	q, err := url.ParseQuery(rawQuery)
	if err != nil {
		return nil, invalidFileRequest("Invalid file list query")
	}
	limit := 1000
	desc := anthropic
	after, before := q.Get("after"), ""
	if anthropic {
		limit = 20
		after, before = q.Get("after_id"), q.Get("before_id")
	}
	for k, values := range q {
		allowed := k == "limit" || (anthropic && (k == "after_id" || k == "before_id")) ||
			(!anthropic && (k == "after" || k == "order" || k == "purpose"))
		if !allowed || len(values) != 1 {
			return nil, invalidFileRequest("Unsupported or duplicate file list parameter")
		}
	}
	if q.Has("limit") {
		limit, err = strconv.Atoi(q.Get("limit"))
		if err != nil || limit < 1 || limit > 1000 {
			return nil, invalidFileRequest("limit must be between 1 and 1000")
		}
	}
	if q.Has("after_id") && q.Has("before_id") {
		return nil, invalidFileRequest("after_id and before_id are mutually exclusive")
	}
	if q.Has("purpose") {
		switch q.Get("purpose") {
		case "user_data", "text_recognition", "rich_layout_recognition":
		default:
			return nil, invalidFileRequest("Unknown file purpose")
		}
	}
	if q.Has("order") {
		switch q.Get("order") {
		case "asc", "desc":
			desc = q.Get("order") == "desc"
		default:
			return nil, invalidFileRequest("order must be asc or desc")
		}
	}
	if userID <= 0 {
		return nil, errDeepseekFileNotFound
	}
	store, err := s.deepseekFileStore()
	if err != nil {
		return nil, err
	}
	cursor := after
	if before != "" {
		cursor = before
	}
	if cursor != "" && !isValidDeepseekFileID(cursor) {
		return nil, invalidFileRequest("Invalid file list cursor")
	}
	files, err := store.ListDeepseekFiles(ctx, userID, DeepseekFilePage{Limit: limit, Descending: desc, Before: before != "", Cursor: cursor, Purpose: q.Get("purpose")})
	if errors.Is(err, ErrDeepseekFileCursor) {
		return nil, invalidFileRequest("File list cursor not found")
	}
	if err != nil {
		return nil, errDeepseekFileStore.WithCause(err)
	}
	hasMore := len(files) > limit
	if hasMore {
		files = files[:limit]
	}
	if before != "" {
		slices.Reverse(files)
	}
	data := make([]map[string]any, 0, len(files))
	for _, f := range files {
		data = append(data, f.publicMetadata(anthropic))
	}
	page := map[string]any{"data": data, "has_more": hasMore, "first_id": nil, "last_id": nil}
	if len(files) > 0 {
		page["first_id"], page["last_id"] = files[0].ID, files[len(files)-1].ID
	}
	if !anthropic {
		page["object"] = "list"
	}
	return page, nil
}
