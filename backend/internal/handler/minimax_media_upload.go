package handler

import (
	"bytes"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// Voice samples are buffered within a strict bound and never persisted locally.
func (h *OpenAIGatewayHandler) MiniMaxUpload(c *gin.Context) {
	key, ok := h.miniMaxMediaKey(c)
	if !ok {
		return
	}
	typ, params, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || typ != "multipart/form-data" {
		h.errorResponse(c, 400, "invalid_request_error", "multipart/form-data is required")
		return
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, (21<<20)+1))
	if err != nil || len(body) > 21<<20 {
		h.errorResponse(c, 413, "invalid_request_error", "Voice upload exceeds 20 MB")
		return
	}
	reader := multipart.NewReader(bytes.NewReader(body), params["boundary"])
	purpose, files, parts := "", 0, 0
	for {
		part, e := reader.NextPart()
		if e == io.EOF {
			break
		}
		if e != nil {
			h.errorResponse(c, 400, "invalid_request_error", "Invalid multipart upload")
			return
		}
		parts++
		switch part.FormName() {
		case "purpose":
			if purpose != "" {
				h.errorResponse(c, 400, "invalid_request_error", "Duplicate purpose")
				return
			}
			v, _ := io.ReadAll(io.LimitReader(part, 64))
			purpose = string(v)
		case "file":
			files++
			ext := strings.ToLower(filepath.Ext(part.FileName()))
			if ext != ".mp3" && ext != ".m4a" && ext != ".wav" {
				h.errorResponse(c, 400, "invalid_request_error", "Voice sample must be mp3, m4a or wav")
				return
			}
			n, e := io.Copy(io.Discard, io.LimitReader(part, (20<<20)+1))
			if e != nil || n == 0 || n > 20<<20 {
				h.errorResponse(c, 413, "invalid_request_error", "Voice sample must be nonempty and at most 20 MB")
				return
			}
		default:
			h.errorResponse(c, 400, "invalid_request_error", "Only purpose and file are supported")
			return
		}
	}
	if files != 1 || parts != 2 || (purpose != "voice_clone" && purpose != "prompt_audio") {
		h.errorResponse(c, 400, "invalid_request_error", "Upload requires one file and purpose voice_clone or prompt_audio")
		return
	}
	slot, err := h.minimaxMedia.AcquireUser(c.Request.Context(), key.UserID, key.User.Concurrency)
	if err != nil || slot == nil || !slot.Acquired {
		h.errorResponse(c, 429, "rate_limit_error", "User concurrency limit reached")
		return
	}
	defer slot.ReleaseFunc()
	account, release, err := h.minimaxMedia.Select(c.Request.Context(), *key.GroupID)
	if err != nil {
		h.errorResponse(c, 503, "api_error", err.Error())
		return
	}
	defer release()
	origin, err := service.MiniMaxMediaOrigin(account)
	if err != nil {
		h.errorResponse(c, 400, "invalid_request_error", err.Error())
		return
	}
	ctx, cancel := h.minimaxMedia.Detached(90 * time.Second)
	defer cancel()
	resp, err := h.minimaxMedia.DoContentType(ctx, account, origin, http.MethodPost, "/v1/files/upload", body, c.GetHeader("Content-Type"))
	if err != nil {
		h.errorResponse(c, 502, "upstream_error", "Upload outcome is unknown")
		return
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	id := gjson.GetBytes(data, "file.file_id").String()
	if err != nil || resp.StatusCode != 200 || !gjson.GetBytes(data, "base_resp.status_code").Exists() || gjson.GetBytes(data, "base_resp.status_code").Int() != 0 || id == "" {
		h.errorResponse(c, 502, "upstream_error", "MiniMax upload failed")
		return
	}
	err = h.minimaxMedia.SaveAsset(ctx, &service.MiniMaxMediaAsset{Kind: purpose + "_file", ID: id, AccountID: account.ID, UserID: key.UserID, APIKeyID: key.ID})
	if err != nil {
		h.errorResponse(c, 503, "api_error", "Upload succeeded but ownership receipt could not be saved")
		return
	}
	c.Data(200, "application/json", data)
}
