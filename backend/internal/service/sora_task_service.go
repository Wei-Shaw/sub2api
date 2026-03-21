package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

// SoraTaskService manages Sora async task creation and status queries.
type SoraTaskService struct {
	repo         SoraTaskRepository
	accountRepo  AccountRepository
	soraClient   SoraClient
	httpUpstream HTTPUpstream
}

func NewSoraTaskService(
	repo SoraTaskRepository,
	accountRepo AccountRepository,
	soraClient SoraClient,
	httpUpstream HTTPUpstream,
) *SoraTaskService {
	return &SoraTaskService{
		repo:         repo,
		accountRepo:  accountRepo,
		soraClient:   soraClient,
		httpUpstream: httpUpstream,
	}
}

// ── Request structs ──

type CreateVideoRequest struct {
	Model         string `json:"model"`
	Prompt        string `json:"prompt"`
	StyleID       string `json:"style_id,omitempty"`
	Orientation   string `json:"orientation,omitempty"`
	Image         string `json:"image,omitempty"`
	Video         string `json:"video,omitempty"`
	RemixTargetID string `json:"remix_target_id,omitempty"`
	Size          string `json:"size,omitempty"`
}

type CreateImageRequest struct {
	Model          string `json:"model,omitempty"`
	Prompt         string `json:"prompt"`
	Image          string `json:"image,omitempty"`
	Size           string `json:"size,omitempty"`
	ResponseFormat string `json:"response_format,omitempty"`
	N              int    `json:"n,omitempty"`
}

type EditImageRequest struct {
	Image          string `json:"image"`
	Prompt         string `json:"prompt"`
	Model          string `json:"model,omitempty"`
	Size           string `json:"size,omitempty"`
	ResponseFormat string `json:"response_format,omitempty"`
}

type RemixRequest struct {
	Prompt string `json:"prompt"`
}

// ── Response structs ──

type SoraTaskResponse struct {
	ID          string         `json:"id"`
	Object      string         `json:"object"`
	CreatedAt   int64          `json:"created_at"`
	Status      string         `json:"status"`
	Model       string         `json:"model,omitempty"`
	Prompt      string         `json:"prompt,omitempty"`
	Progress    int            `json:"progress"`
	VideoURL    string         `json:"video_url,omitempty"`
	ShareID     string         `json:"share_id,omitempty"`
	Seconds     string         `json:"seconds,omitempty"`
	Size        string         `json:"size,omitempty"`
	Character   *SoraCharacter `json:"character,omitempty"`
	URL         string         `json:"url,omitempty"`
	CompletedAt *int64         `json:"completed_at,omitempty"`
	Error       *SoraTaskError `json:"error,omitempty"`
}

type SoraTaskError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}

func TaskToResponse(t *SoraTask) *SoraTaskResponse {
	resp := &SoraTaskResponse{
		ID:        t.ID,
		Object:    t.ObjectType,
		CreatedAt: t.CreatedAt.Unix(),
		Status:    t.Status,
		Model:     t.Model,
		Prompt:    t.Prompt,
		Progress:  t.Progress,
		Seconds:   t.Seconds,
		Size:      t.Size,
	}
	if t.VideoURL != "" {
		resp.VideoURL = t.VideoURL
	}
	if t.ShareID != "" {
		resp.ShareID = t.ShareID
	}
	if t.CharacterInfo != nil {
		resp.Character = t.CharacterInfo
	}
	if t.CompletedAt != nil {
		ts := t.CompletedAt.Unix()
		resp.CompletedAt = &ts
	}
	if t.Status == SoraTaskFailed && t.ErrorMessage != "" {
		resp.Error = &SoraTaskError{
			Message: t.ErrorMessage,
			Type:    t.ErrorType,
		}
	}
	return resp
}

// ── Task creation ──

func (s *SoraTaskService) CreateVideoTask(
	ctx context.Context,
	apiKeyID int64,
	account *Account,
	req *CreateVideoRequest,
	body []byte,
) (*SoraTask, error) {
	modelCfg, ok := GetSoraModelConfig(req.Model)

	objectType := SoraObjectVideo
	if req.Video != "" && req.Prompt == "" {
		objectType = SoraObjectCharacter
	}

	taskID := generateTaskID(objectType)

	seconds := ""
	size := ""
	if ok && modelCfg.Type == "video" {
		seconds = fmt.Sprintf("%d", modelCfg.Frames/30)
		if modelCfg.Orientation == "landscape" {
			size = "1920x1080"
		} else {
			size = "1080x1920"
		}
	}

	task := &SoraTask{
		ID:         taskID,
		AccountID:  account.ID,
		APIKeyID:   &apiKeyID,
		ObjectType: objectType,
		Model:      req.Model,
		Prompt:     req.Prompt,
		Status:     SoraTaskQueued,
		Progress:   0,
		Seconds:    seconds,
		Size:       size,
		CreatedAt:  time.Now(),
	}

	if account.Type == AccountTypeAPIKey {
		task.RequestBody = body
		upstreamResp, err := s.forwardCreateToUpstream(ctx, account, "/v1/videos", body)
		if err != nil {
			return nil, fmt.Errorf("forward to upstream: %w", err)
		}
		s.applyUpstreamResponse(task, upstreamResp)
	} else {
		if s.soraClient == nil || !s.soraClient.Enabled() {
			return nil, fmt.Errorf("sora SDK client not configured")
		}
		upstreamID, err := s.createViaSdk(ctx, account, req, modelCfg, objectType)
		if err != nil {
			return nil, fmt.Errorf("sdk create task: %w", err)
		}
		task.UpstreamTaskID = upstreamID
	}

	if err := s.repo.Create(ctx, task); err != nil {
		return nil, fmt.Errorf("save task: %w", err)
	}
	return task, nil
}

func (s *SoraTaskService) CreateImageGeneration(
	ctx context.Context,
	apiKeyID int64,
	account *Account,
	req *CreateImageRequest,
	body []byte,
) (*SoraTask, error) {
	taskID := generateTaskID(SoraObjectImage)

	task := &SoraTask{
		ID:         taskID,
		AccountID:  account.ID,
		APIKeyID:   &apiKeyID,
		ObjectType: SoraObjectImage,
		Model:      req.Model,
		Prompt:     req.Prompt,
		Status:     SoraTaskQueued,
		Progress:   0,
		Size:       req.Size,
		CreatedAt:  time.Now(),
	}

	if account.Type == AccountTypeAPIKey {
		task.RequestBody = body
		upstreamResp, err := s.forwardCreateToUpstream(ctx, account, "/v1/images/generations", body)
		if err != nil {
			return nil, fmt.Errorf("forward to upstream: %w", err)
		}
		s.applyUpstreamResponse(task, upstreamResp)
	} else {
		if s.soraClient == nil || !s.soraClient.Enabled() {
			return nil, fmt.Errorf("sora SDK client not configured")
		}
		modelCfg, _ := GetSoraModelConfig(req.Model)
		width, height := modelCfg.Width, modelCfg.Height
		if width == 0 {
			width = 360
		}
		if height == 0 {
			height = 360
		}
		upstreamID, err := s.soraClient.CreateImageTask(ctx, account, SoraImageRequest{
			Prompt: req.Prompt,
			Width:  width,
			Height: height,
		})
		if err != nil {
			return nil, fmt.Errorf("sdk create image task: %w", err)
		}
		task.UpstreamTaskID = upstreamID
	}

	if err := s.repo.Create(ctx, task); err != nil {
		return nil, fmt.Errorf("save task: %w", err)
	}
	return task, nil
}

func (s *SoraTaskService) GetTask(ctx context.Context, taskID string, apiKeyID int64) (*SoraTask, error) {
	return s.repo.GetByIDAndAPIKey(ctx, taskID, apiKeyID)
}

func (s *SoraTaskService) GetTaskByID(ctx context.Context, taskID string) (*SoraTask, error) {
	return s.repo.GetByID(ctx, taskID)
}

// ListPendingTasks returns tasks in queued or in_progress status.
func (s *SoraTaskService) ListPendingTasks(ctx context.Context) ([]*SoraTask, error) {
	return s.repo.ListPending(ctx)
}

// UpdateTask persists task state changes.
func (s *SoraTaskService) UpdateTask(ctx context.Context, task *SoraTask) error {
	return s.repo.Update(ctx, task)
}

func (s *SoraTaskService) GetAccountByID(ctx context.Context, accountID int64) (*Account, error) {
	return s.accountRepo.GetByID(ctx, accountID)
}

// ── Internal methods ──

func (s *SoraTaskService) createViaSdk(
	ctx context.Context,
	account *Account,
	req *CreateVideoRequest,
	modelCfg SoraModelConfig,
	objectType string,
) (string, error) {
	if objectType == SoraObjectCharacter {
		return "", fmt.Errorf("character creation via /v1/videos is not yet supported for OAuth accounts")
	}

	orientation := modelCfg.Orientation
	if req.Orientation != "" {
		orientation = req.Orientation
	}

	videoReq := SoraVideoRequest{
		Prompt:        req.Prompt,
		Orientation:   orientation,
		Frames:        modelCfg.Frames,
		Model:         modelCfg.Model,
		Size:          modelCfg.Size,
		RemixTargetID: req.RemixTargetID,
	}
	return s.soraClient.CreateVideoTask(ctx, account, videoReq)
}

func (s *SoraTaskService) forwardCreateToUpstream(
	ctx context.Context,
	account *Account,
	path string,
	body []byte,
) (map[string]any, error) {
	apiKey := account.GetCredential("api_key")
	baseURL := account.GetBaseURL()
	if apiKey == "" || baseURL == "" {
		return nil, fmt.Errorf("account %d missing api_key or base_url", account.ID)
	}

	upstreamURL := strings.TrimRight(baseURL, "/") + path
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, upstreamURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}

	resp, err := s.httpUpstream.Do(req, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		return nil, fmt.Errorf("upstream request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if err != nil {
		return nil, fmt.Errorf("read upstream response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, &SoraUpstreamError{StatusCode: resp.StatusCode, Body: respBody}
	}

	var result map[string]any
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("parse upstream response: %w", err)
	}
	return result, nil
}

func (s *SoraTaskService) forwardGetToUpstream(
	ctx context.Context,
	account *Account,
	path string,
) (map[string]any, error) {
	apiKey := account.GetCredential("api_key")
	baseURL := account.GetBaseURL()
	if apiKey == "" || baseURL == "" {
		return nil, fmt.Errorf("account %d missing api_key or base_url", account.ID)
	}

	upstreamURL := strings.TrimRight(baseURL, "/") + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, upstreamURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}

	resp, err := s.httpUpstream.Do(req, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		return nil, fmt.Errorf("upstream request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if err != nil {
		return nil, fmt.Errorf("read upstream response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("upstream error %d: %s", resp.StatusCode, string(respBody))
	}

	var result map[string]any
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("parse upstream response: %w", err)
	}
	return result, nil
}

func (s *SoraTaskService) applyUpstreamResponse(task *SoraTask, resp map[string]any) {
	if id, ok := resp["id"].(string); ok && id != "" {
		task.UpstreamTaskID = id
	}
	if status, ok := resp["status"].(string); ok && status != "" {
		task.Status = status
	}
	if progress, ok := resp["progress"].(float64); ok {
		task.Progress = int(progress)
	}
	if videoURL, ok := resp["video_url"].(string); ok {
		task.VideoURL = videoURL
	}
	if shareID, ok := resp["share_id"].(string); ok {
		task.ShareID = shareID
	}
	if seconds, ok := resp["seconds"].(string); ok {
		task.Seconds = seconds
	}
	if size, ok := resp["size"].(string); ok {
		task.Size = size
	}
	if obj, ok := resp["object"].(string); ok && obj != "" {
		task.ObjectType = obj
	}
}

// PollTask polls a single task's latest status (called by worker).
func (s *SoraTaskService) PollTask(ctx context.Context, task *SoraTask, account *Account) error {
	if account.Type == AccountTypeAPIKey {
		return s.pollUpstreamTask(ctx, task, account)
	}
	return s.pollSdkTask(ctx, task, account)
}

func (s *SoraTaskService) pollUpstreamTask(ctx context.Context, task *SoraTask, account *Account) error {
	upstreamID := task.UpstreamTaskID
	if upstreamID == "" {
		upstreamID = task.ID
	}

	path := fmt.Sprintf("/v1/videos/%s", upstreamID)
	resp, err := s.forwardGetToUpstream(ctx, account, path)
	if err != nil {
		logger.LegacyPrintf("service.sora_task", "[PollUpstream] task=%s error=%v", task.ID, err)
		return err
	}

	s.applyUpstreamResponse(task, resp)
	now := time.Now()
	if task.Status == SoraTaskCompleted || task.Status == SoraTaskFailed {
		task.CompletedAt = &now
	}

	if errObj, ok := resp["error"].(map[string]any); ok {
		if msg, ok := errObj["message"].(string); ok {
			task.ErrorMessage = msg
		}
		if typ, ok := errObj["type"].(string); ok {
			task.ErrorType = typ
		}
	}

	return s.repo.Update(ctx, task)
}

func (s *SoraTaskService) pollSdkTask(ctx context.Context, task *SoraTask, account *Account) error {
	if s.soraClient == nil {
		return fmt.Errorf("sora SDK client not configured")
	}
	upstreamID := task.UpstreamTaskID
	if upstreamID == "" {
		return fmt.Errorf("task %s has no upstream_task_id", task.ID)
	}

	now := time.Now()

	switch task.ObjectType {
	case SoraObjectImage:
		status, err := s.soraClient.GetImageTask(ctx, account, upstreamID)
		if err != nil {
			return err
		}
		task.Progress = int(status.ProgressPct)
		switch status.Status {
		case "complete", "completed":
			task.Status = SoraTaskCompleted
			task.Progress = 100
			task.CompletedAt = &now
			if len(status.URLs) > 0 {
				task.VideoURL = status.URLs[0]
			}
		case "failed":
			task.Status = SoraTaskFailed
			task.CompletedAt = &now
			task.ErrorMessage = status.ErrorMsg
			task.ErrorType = "server_error"
		default:
			task.Status = SoraTaskInProgress
		}

	case SoraObjectVideo, SoraObjectCharacter:
		status, err := s.soraClient.GetVideoTask(ctx, account, upstreamID)
		if err != nil {
			return err
		}
		task.Progress = status.ProgressPct
		switch status.Status {
		case "complete", "completed":
			task.Status = SoraTaskCompleted
			task.Progress = 100
			task.CompletedAt = &now
			if len(status.URLs) > 0 {
				task.VideoURL = status.URLs[0]
			}
			if status.GenerationID != "" {
				task.ShareID = status.GenerationID
			}
		case "failed":
			task.Status = SoraTaskFailed
			task.CompletedAt = &now
			task.ErrorMessage = status.ErrorMsg
			task.ErrorType = "server_error"
		default:
			task.Status = SoraTaskInProgress
		}

	default:
		return fmt.Errorf("unknown object type: %s", task.ObjectType)
	}

	return s.repo.Update(ctx, task)
}

func generateTaskID(objectType string) string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		b = []byte(fmt.Sprintf("%x", time.Now().UnixNano()))
	}
	switch objectType {
	case SoraObjectCharacter:
		return "char_" + hex.EncodeToString(b)
	case SoraObjectImage:
		return "img_" + hex.EncodeToString(b)
	default:
		return "video_" + hex.EncodeToString(b)
	}
}
