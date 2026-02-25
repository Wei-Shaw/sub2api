package service

import (
	"context"
	"fmt"
	"net/http"
)

// SoraClient 定义直连 Sora 的任务操作接口。
type SoraClient interface {
	Enabled() bool
	UploadImage(ctx context.Context, account *Account, data []byte, filename string) (string, error)
	CreateImageTask(ctx context.Context, account *Account, req SoraImageRequest) (string, error)
	CreateVideoTask(ctx context.Context, account *Account, req SoraVideoRequest) (string, error)
	CreateStoryboardTask(ctx context.Context, account *Account, req SoraStoryboardRequest) (string, error)
	UploadCharacterVideo(ctx context.Context, account *Account, data []byte) (string, error)
	GetCameoStatus(ctx context.Context, account *Account, cameoID string) (*SoraCameoStatus, error)
	DownloadCharacterImage(ctx context.Context, account *Account, imageURL string) ([]byte, error)
	UploadCharacterImage(ctx context.Context, account *Account, data []byte) (string, error)
	FinalizeCharacter(ctx context.Context, account *Account, req SoraCharacterFinalizeRequest) (string, error)
	SetCharacterPublic(ctx context.Context, account *Account, cameoID string) error
	DeleteCharacter(ctx context.Context, account *Account, characterID string) error
	PostVideoForWatermarkFree(ctx context.Context, account *Account, generationID string) (string, error)
	DeletePost(ctx context.Context, account *Account, postID string) error
	GetWatermarkFreeURLCustom(ctx context.Context, account *Account, parseURL, parseToken, postID string) (string, error)
	EnhancePrompt(ctx context.Context, account *Account, prompt, expansionLevel string, durationS int) (string, error)
	GetImageTask(ctx context.Context, account *Account, taskID string) (*SoraImageTaskStatus, error)
	GetVideoTask(ctx context.Context, account *Account, taskID string) (*SoraVideoTaskStatus, error)
REDACTED

// SoraImageRequest 图片生成请求参数
type SoraImageRequest struct {
	Prompt  string
	Width   int
	Height  int
	MediaID string
REDACTED

// SoraVideoRequest 视频生成请求参数
type SoraVideoRequest struct {
	Prompt        string
	Orientation   string
	Frames        int
	Model         string
	Size          string
	MediaID       string
	RemixTargetID string
	CameoIDs      []string
REDACTED

// SoraStoryboardRequest 分镜视频生成请求参数
type SoraStoryboardRequest struct {
	Prompt      string
	Orientation string
	Frames      int
	Model       string
	Size        string
	MediaID     string
REDACTED

// SoraImageTaskStatus 图片任务状态
type SoraImageTaskStatus struct {
	ID          string
	Status      string
	ProgressPct float64
	URLs        []string
	ErrorMsg    string
REDACTED

// SoraVideoTaskStatus 视频任务状态
type SoraVideoTaskStatus struct {
	ID           string
	Status       string
	ProgressPct  int
	URLs         []string
	GenerationID string
	ErrorMsg     string
REDACTED

// SoraCameoStatus 角色处理中间态
type SoraCameoStatus struct {
	Status             string
	StatusMessage      string
	DisplayNameHint    string
	UsernameHint       string
	ProfileAssetURL    string
	InstructionSetHint any
	InstructionSet     any
REDACTED

// SoraCharacterFinalizeRequest 角色定稿请求参数
type SoraCharacterFinalizeRequest struct {
	CameoID             string
	Username            string
	DisplayName         string
	ProfileAssetPointer string
	InstructionSet      any
REDACTED

// SoraUpstreamError 上游错误
type SoraUpstreamError struct {
	StatusCode int
	Message    string
	Headers    http.Header
	Body       []byte
REDACTED

func (e *SoraUpstreamError) Error() string {
	if e == nil {
		return "sora upstream error"
REDACTED
	if e.Message != "" {
		return fmt.Sprintf("sora upstream error: %d %s", e.StatusCode, e.Message)
REDACTED
	return fmt.Sprintf("sora upstream error: %d", e.StatusCode)
REDACTED
