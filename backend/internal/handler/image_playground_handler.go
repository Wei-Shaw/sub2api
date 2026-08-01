package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
)

const (
	ImagePlaygroundMaxRequestBodyBytes int64 = 42 << 20
	imagePlaygroundMaxInputImages            = 4
	imagePlaygroundMaxInputImageBytes  int64 = 10 << 20
	imagePlaygroundMultipartMemory     int64 = 4 << 20
	imagePlaygroundDefaultEditPrompt         = "Create a high-quality variation based on the provided reference image or images."
	imagePlaygroundSubmitContextKey          = "image_playground_submit_request"
)

type ImagePlaygroundSubmitRequest struct {
	GroupID      int64  `json:"group_id" form:"group_id"`
	Model        string `json:"model" form:"model"`
	Prompt       string `json:"prompt" form:"prompt"`
	Size         string `json:"size,omitempty" form:"size"`
	Quality      string `json:"quality,omitempty" form:"quality"`
	N            int    `json:"n,omitempty" form:"n"`
	OutputFormat string `json:"output_format,omitempty" form:"output_format"`
	Background   string `json:"background,omitempty" form:"background"`
}

type imagePlaygroundParsedSubmit struct {
	Request ImagePlaygroundSubmitRequest
	Images  []*multipart.FileHeader
}

type imagePlaygroundUpload struct {
	FileName    string
	ContentType string
	Data        []byte
}

type imagePlaygroundApplication interface {
	Enabled(ctx context.Context) bool
	Options(ctx context.Context, userID int64) (*service.ImagePlaygroundOptions, error)
	ResolveAPIKey(ctx context.Context, userID, groupID int64) (*service.APIKey, error)
	ValidateGenerationOptions(ctx context.Context, group *service.Group, model, size, quality, outputFormat, background string, n int) error
}

type imagePlaygroundTasks interface {
	ListForUser(ctx context.Context, userID int64, limit int) ([]*service.ImageTask, error)
	GetForUser(ctx context.Context, userID int64, id string) (*service.ImageTask, error)
	DownloadForUser(ctx context.Context, userID int64, id string, imageIndex int) (*service.ImageTaskDownload, error)
	DeleteForUser(ctx context.Context, userID int64, id string) error
}

const imagePlaygroundHistoryLimit = 24

type ImagePlaygroundHandler struct {
	playground imagePlaygroundApplication
	tasks      imagePlaygroundTasks
	async      *AsyncImageHandler
}

func NewImagePlaygroundHandler(playground *service.ImagePlaygroundService, tasks *service.ImageTaskService, async *AsyncImageHandler) *ImagePlaygroundHandler {
	return &ImagePlaygroundHandler{playground: playground, tasks: tasks, async: async}
}

type imagePlaygroundImage struct {
	Index       int    `json:"index"`
	URL         string `json:"url"`
	DownloadURL string `json:"download_url"`
}

type imagePlaygroundTaskResponse struct {
	ID              string                 `json:"id"`
	Object          string                 `json:"object"`
	Status          string                 `json:"status"`
	GroupID         int64                  `json:"group_id"`
	Platform        string                 `json:"platform"`
	Model           string                 `json:"model"`
	PromptPreview   string                 `json:"prompt_preview,omitempty"`
	InputImageCount int                    `json:"input_image_count,omitempty"`
	Images          []imagePlaygroundImage `json:"images"`
	Error           json.RawMessage        `json:"error,omitempty"`
	CreatedAt       int64                  `json:"created_at"`
	CompletedAt     *int64                 `json:"completed_at,omitempty"`
	ExpiresAt       int64                  `json:"expires_at"`
	PollURL         string                 `json:"poll_url"`
}

func (h *ImagePlaygroundHandler) Options(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	options, err := h.playground.Options(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, options)
}

// ResolveAPIKey runs after JWT authentication and before the existing API-key
// middleware. It replaces the already-consumed JWT Authorization header with a
// server-selected key that belongs to the same user and selected group.
func (h *ImagePlaygroundHandler) ResolveAPIKey(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		c.Abort()
		return
	}
	parsed, err := parseImagePlaygroundSubmit(c)
	if err != nil || parsed.Request.GroupID <= 0 {
		response.BadRequest(c, "group_id is required")
		c.Abort()
		return
	}
	if c.Request.MultipartForm != nil {
		defer func() {
			_ = c.Request.MultipartForm.RemoveAll()
		}()
	}
	key, err := h.playground.ResolveAPIKey(c.Request.Context(), subject.UserID, parsed.Request.GroupID)
	if err != nil {
		response.ErrorFrom(c, err)
		c.Abort()
		return
	}
	if key == nil || strings.TrimSpace(key.Key) == "" {
		response.ErrorFrom(c, service.ErrImagePlaygroundAPIKeyRequired)
		c.Abort()
		return
	}
	c.Request.Header.Set("Authorization", "Bearer "+key.Key)
	c.Request.Header.Del("x-api-key")
	c.Request.Header.Del("x-goog-api-key")
	c.Next()
}

func (h *ImagePlaygroundHandler) Submit(c *gin.Context) {
	parsed, err := parseImagePlaygroundSubmit(c)
	if err != nil {
		response.BadRequest(c, "invalid image playground request")
		return
	}
	req := parsed.Request
	if err := validateImagePlaygroundSubmitRequest(&req, len(parsed.Images)); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok || apiKey == nil || apiKey.Group == nil || apiKey.GroupID == nil || *apiKey.GroupID != req.GroupID {
		response.Forbidden(c, "selected group is not available")
		return
	}
	if len(parsed.Images) > 0 && apiKey.Group.Platform != service.PlatformOpenAI {
		response.ErrorFrom(c, infraerrors.New(
			http.StatusBadRequest,
			"IMAGE_PLAYGROUND_IMAGE_INPUT_UNSUPPORTED",
			"reference images are currently supported only for OpenAI image models",
		))
		return
	}
	if err := h.playground.ValidateGenerationOptions(
		c.Request.Context(),
		apiKey.Group,
		req.Model,
		req.Size,
		req.Quality,
		req.OutputFormat,
		req.Background,
		req.N,
	); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	body, contentType, targetPath, err := buildImagePlaygroundGatewayRequest(req, parsed.Images)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	recorder := httptest.NewRecorder()
	child, _ := gin.CreateTestContext(recorder)
	request := c.Request.Clone(c.Request.Context())
	request.Method = http.MethodPost
	request.URL.Path = targetPath
	request.RequestURI = request.URL.RequestURI()
	request.Body = io.NopCloser(bytes.NewReader(body))
	request.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(body)), nil }
	request.ContentLength = int64(len(body))
	// The dashboard request was already parsed to collect uploads. The internal
	// gateway request has a newly encoded multipart body and must be parsed from
	// scratch just like a direct /v1/images/edits request.
	request.MultipartForm = nil
	request.Form = nil
	request.PostForm = nil
	request.Header.Set("Content-Type", contentType)
	child.Request = request
	for key, value := range c.Copy().Keys {
		child.Set(key, value)
	}

	if h.async == nil {
		response.ErrorFrom(c, service.ErrImagePlaygroundUnavailable)
		return
	}
	h.async.Submit(child)
	if recorder.Code != http.StatusAccepted {
		relayImagePlaygroundSubmitError(c, recorder)
		return
	}
	var accepted struct {
		TaskID string `json:"task_id"`
	}
	if json.Unmarshal(recorder.Body.Bytes(), &accepted) != nil || strings.TrimSpace(accepted.TaskID) == "" {
		response.InternalError(c, "image task submission returned an invalid response")
		return
	}
	subject, _ := middleware2.GetAuthSubjectFromContext(c)
	task, err := h.tasks.GetForUser(c.Request.Context(), subject.UserID, accepted.TaskID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	pollURL := imagePlaygroundPollURL(task.ID)
	c.Header("Cache-Control", "no-store")
	c.Header("Location", pollURL)
	c.Header("Retry-After", "3")
	response.Accepted(c, imagePlaygroundTaskToResponse(task))
}

func (h *ImagePlaygroundHandler) GetTask(c *gin.Context) {
	if h.playground == nil || !h.playground.Enabled(c.Request.Context()) {
		response.ErrorFrom(c, service.ErrImagePlaygroundDisabled)
		return
	}
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	task, err := h.tasks.GetForUser(c.Request.Context(), subject.UserID, c.Param("task_id"))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	if task.Status == service.ImageTaskStatusProcessing {
		c.Header("Retry-After", "3")
	}
	response.Success(c, imagePlaygroundTaskToResponse(task))
}

func (h *ImagePlaygroundHandler) ListTasks(c *gin.Context) {
	if h.playground == nil || !h.playground.Enabled(c.Request.Context()) {
		response.ErrorFrom(c, service.ErrImagePlaygroundDisabled)
		return
	}
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	tasks, err := h.tasks.ListForUser(c.Request.Context(), subject.UserID, imagePlaygroundHistoryLimit)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]imagePlaygroundTaskResponse, 0, len(tasks))
	for _, task := range tasks {
		out = append(out, imagePlaygroundTaskToResponse(task))
	}
	c.Header("Cache-Control", "no-store")
	response.Success(c, out)
}

func (h *ImagePlaygroundHandler) Download(c *gin.Context) {
	if h.playground == nil || !h.playground.Enabled(c.Request.Context()) {
		response.ErrorFrom(c, service.ErrImagePlaygroundDisabled)
		return
	}
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	imageIndex, err := strconv.Atoi(c.Param("image_index"))
	if err != nil || imageIndex < 0 {
		response.BadRequest(c, "invalid image index")
		return
	}
	download, err := h.tasks.DownloadForUser(c.Request.Context(), subject.UserID, c.Param("task_id"), imageIndex)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": download.Filename}))
	c.Data(http.StatusOK, download.ContentType, download.Data)
}

func (h *ImagePlaygroundHandler) Preview(c *gin.Context) {
	if h.playground == nil || !h.playground.Enabled(c.Request.Context()) {
		response.ErrorFrom(c, service.ErrImagePlaygroundDisabled)
		return
	}
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	imageIndex, err := strconv.Atoi(c.Param("image_index"))
	if err != nil || imageIndex < 0 {
		response.BadRequest(c, "invalid image index")
		return
	}
	preview, err := h.tasks.DownloadForUser(c.Request.Context(), subject.UserID, c.Param("task_id"), imageIndex)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.Data(http.StatusOK, preview.ContentType, preview.Data)
}

func (h *ImagePlaygroundHandler) DeleteTask(c *gin.Context) {
	if h.playground == nil || !h.playground.Enabled(c.Request.Context()) {
		response.ErrorFrom(c, service.ErrImagePlaygroundDisabled)
		return
	}
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	if err := h.tasks.DeleteForUser(c.Request.Context(), subject.UserID, c.Param("task_id")); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func validateImagePlaygroundSubmitRequest(req *ImagePlaygroundSubmitRequest, inputImageCount int) error {
	if req == nil || req.GroupID <= 0 {
		return infraerrors.New(http.StatusBadRequest, "IMAGE_PLAYGROUND_GROUP_REQUIRED", "group_id is required")
	}
	req.Model = strings.TrimSpace(req.Model)
	req.Prompt = strings.TrimSpace(req.Prompt)
	req.Size = strings.TrimSpace(req.Size)
	req.Quality = strings.TrimSpace(req.Quality)
	req.OutputFormat = strings.ToLower(strings.TrimSpace(req.OutputFormat))
	req.Background = strings.ToLower(strings.TrimSpace(req.Background))
	if req.Model == "" || len(req.Model) > 200 {
		return infraerrors.New(http.StatusBadRequest, "IMAGE_PLAYGROUND_MODEL_REQUIRED", "model is required")
	}
	if req.Prompt == "" && inputImageCount == 0 {
		return infraerrors.New(http.StatusBadRequest, "IMAGE_PLAYGROUND_PROMPT_REQUIRED", "prompt or reference image is required")
	}
	if req.Prompt == "" {
		req.Prompt = imagePlaygroundDefaultEditPrompt
	}
	if utf8.RuneCountInString(req.Prompt) > 32000 {
		return infraerrors.New(http.StatusBadRequest, "IMAGE_PLAYGROUND_PROMPT_TOO_LONG", "prompt is too long")
	}
	if req.N == 0 {
		req.N = 1
	}
	if req.N < 1 || req.N > 10 {
		return infraerrors.New(http.StatusBadRequest, "IMAGE_PLAYGROUND_INVALID_COUNT", "n must be between 1 and 10")
	}
	if inputImageCount > imagePlaygroundMaxInputImages {
		return infraerrors.New(http.StatusBadRequest, "IMAGE_PLAYGROUND_TOO_MANY_INPUT_IMAGES", "no more than 4 reference images are allowed")
	}
	if len(req.Size) > 32 || len(req.Quality) > 32 || len(req.OutputFormat) > 16 || len(req.Background) > 16 {
		return infraerrors.New(http.StatusBadRequest, "IMAGE_PLAYGROUND_INVALID_OPTION", "an image option is invalid")
	}
	if req.OutputFormat != "" && req.OutputFormat != "png" && req.OutputFormat != "jpeg" && req.OutputFormat != "webp" {
		return infraerrors.New(http.StatusBadRequest, "IMAGE_PLAYGROUND_INVALID_FORMAT", "output_format must be png, jpeg, or webp")
	}
	return nil
}

func parseImagePlaygroundSubmit(c *gin.Context) (*imagePlaygroundParsedSubmit, error) {
	if c == nil || c.Request == nil {
		return nil, fmt.Errorf("missing request")
	}
	if cached, ok := c.Get(imagePlaygroundSubmitContextKey); ok {
		if parsed, ok := cached.(*imagePlaygroundParsedSubmit); ok && parsed != nil {
			return parsed, nil
		}
	}

	parsed := &imagePlaygroundParsedSubmit{}
	mediaType, _, _ := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if strings.EqualFold(mediaType, "multipart/form-data") {
		if err := c.Request.ParseMultipartForm(imagePlaygroundMultipartMemory); err != nil {
			return nil, err
		}
		form := c.Request.MultipartForm
		parsed.Request = ImagePlaygroundSubmitRequest{
			GroupID:      parseImagePlaygroundInt64(formValue(form, "group_id")),
			Model:        formValue(form, "model"),
			Prompt:       formValue(form, "prompt"),
			Size:         formValue(form, "size"),
			Quality:      formValue(form, "quality"),
			N:            int(parseImagePlaygroundInt64(formValue(form, "n"))),
			OutputFormat: formValue(form, "output_format"),
			Background:   formValue(form, "background"),
		}
		for _, field := range []string{"images", "image", "image[]"} {
			parsed.Images = append(parsed.Images, form.File[field]...)
		}
	} else if err := c.ShouldBindBodyWith(&parsed.Request, binding.JSON); err != nil {
		return nil, err
	}
	c.Set(imagePlaygroundSubmitContextKey, parsed)
	return parsed, nil
}

func formValue(form *multipart.Form, name string) string {
	if form == nil || len(form.Value[name]) == 0 {
		return ""
	}
	return form.Value[name][0]
}

func parseImagePlaygroundInt64(value string) int64 {
	parsed, _ := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	return parsed
}

func buildImagePlaygroundGatewayRequest(req ImagePlaygroundSubmitRequest, imageHeaders []*multipart.FileHeader) ([]byte, string, string, error) {
	if len(imageHeaders) == 0 {
		body, err := json.Marshal(imagePlaygroundGatewayPayload(req))
		if err != nil {
			return nil, "", "", imagePlaygroundBuildError(err)
		}
		return body, "application/json", "/v1/images/generations/async", nil
	}

	uploads, err := readImagePlaygroundUploads(imageHeaders)
	if err != nil {
		return nil, "", "", err
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, field := range imagePlaygroundGatewayFields(req) {
		if field.Value == "" {
			continue
		}
		if err := writer.WriteField(field.Name, field.Value); err != nil {
			return nil, "", "", imagePlaygroundBuildError(err)
		}
	}
	for _, upload := range uploads {
		header := make(textproto.MIMEHeader)
		header.Set("Content-Disposition", mime.FormatMediaType("form-data", map[string]string{
			"name": "image[]", "filename": upload.FileName,
		}))
		header.Set("Content-Type", upload.ContentType)
		part, err := writer.CreatePart(header)
		if err != nil {
			return nil, "", "", imagePlaygroundBuildError(err)
		}
		if _, err := part.Write(upload.Data); err != nil {
			return nil, "", "", imagePlaygroundBuildError(err)
		}
	}
	if err := writer.Close(); err != nil {
		return nil, "", "", imagePlaygroundBuildError(err)
	}
	return body.Bytes(), writer.FormDataContentType(), "/v1/images/edits/async", nil
}

type imagePlaygroundGatewayField struct{ Name, Value string }

func imagePlaygroundGatewayFields(req ImagePlaygroundSubmitRequest) []imagePlaygroundGatewayField {
	return []imagePlaygroundGatewayField{
		{Name: "model", Value: req.Model},
		{Name: "prompt", Value: req.Prompt},
		{Name: "n", Value: strconv.Itoa(req.N)},
		{Name: "size", Value: req.Size},
		{Name: "quality", Value: req.Quality},
		{Name: "output_format", Value: req.OutputFormat},
		{Name: "background", Value: req.Background},
	}
}

func readImagePlaygroundUploads(headers []*multipart.FileHeader) ([]imagePlaygroundUpload, error) {
	if len(headers) == 0 || len(headers) > imagePlaygroundMaxInputImages {
		return nil, infraerrors.New(http.StatusBadRequest, "IMAGE_PLAYGROUND_TOO_MANY_INPUT_IMAGES", "between 1 and 4 reference images are required")
	}
	uploads := make([]imagePlaygroundUpload, 0, len(headers))
	for index, header := range headers {
		file, err := header.Open()
		if err != nil {
			return nil, infraerrors.New(http.StatusBadRequest, "IMAGE_PLAYGROUND_INVALID_INPUT_IMAGE", "failed to read a reference image").WithCause(err)
		}
		data, readErr := io.ReadAll(io.LimitReader(file, imagePlaygroundMaxInputImageBytes+1))
		_ = file.Close()
		if readErr != nil {
			return nil, infraerrors.New(http.StatusBadRequest, "IMAGE_PLAYGROUND_INVALID_INPUT_IMAGE", "failed to read a reference image").WithCause(readErr)
		}
		if len(data) == 0 {
			return nil, infraerrors.New(http.StatusBadRequest, "IMAGE_PLAYGROUND_INVALID_INPUT_IMAGE", "reference images cannot be empty")
		}
		if int64(len(data)) > imagePlaygroundMaxInputImageBytes {
			return nil, infraerrors.New(http.StatusRequestEntityTooLarge, "IMAGE_PLAYGROUND_INPUT_IMAGE_TOO_LARGE", "each reference image must be 10 MB or smaller")
		}
		contentType := http.DetectContentType(data)
		if contentType != "image/png" && contentType != "image/jpeg" && contentType != "image/webp" {
			return nil, infraerrors.New(http.StatusBadRequest, "IMAGE_PLAYGROUND_INVALID_INPUT_IMAGE", "reference images must be PNG, JPEG, or WebP")
		}
		fileName := filepath.Base(strings.TrimSpace(header.Filename))
		if fileName == "" || fileName == "." {
			fileName = fmt.Sprintf("reference-%d%s", index+1, extensionForImageContentType(contentType))
		}
		uploads = append(uploads, imagePlaygroundUpload{FileName: fileName, ContentType: contentType, Data: data})
	}
	return uploads, nil
}

func imagePlaygroundBuildError(err error) error {
	return infraerrors.New(http.StatusInternalServerError, "IMAGE_PLAYGROUND_MULTIPART_FAILED", "failed to build image request").WithCause(err)
}

func extensionForImageContentType(contentType string) string {
	switch contentType {
	case "image/jpeg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	default:
		return ".png"
	}
}

func imagePlaygroundGatewayPayload(req ImagePlaygroundSubmitRequest) map[string]any {
	payload := map[string]any{"model": req.Model, "prompt": req.Prompt, "n": req.N}
	if req.Size != "" {
		payload["size"] = req.Size
	}
	if req.Quality != "" {
		payload["quality"] = req.Quality
	}
	if req.OutputFormat != "" {
		payload["output_format"] = req.OutputFormat
	}
	if req.Background != "" {
		payload["background"] = req.Background
	}
	return payload
}

func imagePlaygroundTaskToResponse(task *service.ImageTask) imagePlaygroundTaskResponse {
	result := imagePlaygroundTaskResponse{Images: []imagePlaygroundImage{}}
	if task == nil {
		return result
	}
	result.ID = task.ID
	result.Object = "image.playground.task"
	result.Status = task.Status
	result.GroupID = task.GroupID
	result.Platform = task.Platform
	result.Model = task.Model
	result.PromptPreview = task.PromptPreview
	result.InputImageCount = task.InputImageCount
	result.Error = task.Error
	result.CreatedAt = task.CreatedAt
	result.CompletedAt = task.CompletedAt
	result.ExpiresAt = task.ExpiresAt
	result.PollURL = imagePlaygroundPollURL(task.ID)
	for index := 0; index < task.ImageCount; index++ {
		previewURL := fmt.Sprintf("%s/images/%d", result.PollURL, index)
		result.Images = append(result.Images, imagePlaygroundImage{
			Index:       index,
			URL:         previewURL,
			DownloadURL: fmt.Sprintf("%s/images/%d/download", result.PollURL, index),
		})
	}
	return result
}

func imagePlaygroundPollURL(taskID string) string {
	return "/api/v1/image-playground/tasks/" + taskID
}

func relayImagePlaygroundSubmitError(c *gin.Context, recorder *httptest.ResponseRecorder) {
	status := recorder.Code
	if status <= 0 {
		status = http.StatusInternalServerError
	}
	var envelope struct {
		Error struct {
			Type    string `json:"type"`
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.Unmarshal(recorder.Body.Bytes(), &envelope)
	reason := strings.TrimSpace(envelope.Error.Code)
	if reason == "" {
		reason = strings.TrimSpace(envelope.Error.Type)
	}
	message := strings.TrimSpace(envelope.Error.Message)
	if message == "" {
		message = http.StatusText(status)
	}
	response.ErrorWithDetails(c, status, message, reason, nil)
}
