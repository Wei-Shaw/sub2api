//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

var _ SoraClient = (*stubSoraClientForPoll)(nil)

type stubSoraClientForPoll struct {
	imageStatus *SoraImageTaskStatus
	videoStatus *SoraVideoTaskStatus
	imageCalls  int
	videoCalls  int
	enhanced    string
	enhanceErr  error
	storyboard  bool
	videoReq    SoraVideoRequest
	parseErr    error
	postCalls   int
	deleteCalls int
REDACTED

func (s *stubSoraClientForPoll) Enabled() bool { return true REDACTED
func (s *stubSoraClientForPoll) UploadImage(ctx context.Context, account *Account, data []byte, filename string) (string, error) {
	return "", nil
REDACTED
func (s *stubSoraClientForPoll) CreateImageTask(ctx context.Context, account *Account, req SoraImageRequest) (string, error) {
	return "task-image", nil
REDACTED
func (s *stubSoraClientForPoll) CreateVideoTask(ctx context.Context, account *Account, req SoraVideoRequest) (string, error) {
	s.videoReq = req
	return "task-video", nil
REDACTED
func (s *stubSoraClientForPoll) CreateStoryboardTask(ctx context.Context, account *Account, req SoraStoryboardRequest) (string, error) {
	s.storyboard = true
	return "task-video", nil
REDACTED
func (s *stubSoraClientForPoll) UploadCharacterVideo(ctx context.Context, account *Account, data []byte) (string, error) {
	return "cameo-1", nil
REDACTED
func (s *stubSoraClientForPoll) GetCameoStatus(ctx context.Context, account *Account, cameoID string) (*SoraCameoStatus, error) {
	return &SoraCameoStatus{
		Status:          "finalized",
		StatusMessage:   "Completed",
		DisplayNameHint: "Character",
		UsernameHint:    "user.character",
		ProfileAssetURL: "https://example.com/avatar.webp",
REDACTED, nil
REDACTED
func (s *stubSoraClientForPoll) DownloadCharacterImage(ctx context.Context, account *Account, imageURL string) ([]byte, error) {
	return []byte("avatar"), nil
REDACTED
func (s *stubSoraClientForPoll) UploadCharacterImage(ctx context.Context, account *Account, data []byte) (string, error) {
	return "asset-pointer", nil
REDACTED
func (s *stubSoraClientForPoll) FinalizeCharacter(ctx context.Context, account *Account, req SoraCharacterFinalizeRequest) (string, error) {
	return "character-1", nil
REDACTED
func (s *stubSoraClientForPoll) SetCharacterPublic(ctx context.Context, account *Account, cameoID string) error {
	return nil
REDACTED
func (s *stubSoraClientForPoll) DeleteCharacter(ctx context.Context, account *Account, characterID string) error {
	return nil
REDACTED
func (s *stubSoraClientForPoll) PostVideoForWatermarkFree(ctx context.Context, account *Account, generationID string) (string, error) {
	s.postCalls++
	return "s_post", nil
REDACTED
func (s *stubSoraClientForPoll) DeletePost(ctx context.Context, account *Account, postID string) error {
	s.deleteCalls++
	return nil
REDACTED
func (s *stubSoraClientForPoll) GetWatermarkFreeURLCustom(ctx context.Context, account *Account, parseURL, parseToken, postID string) (string, error) {
	if s.parseErr != nil {
		return "", s.parseErr
REDACTED
	return "https://example.com/no-watermark.mp4", nil
REDACTED
func (s *stubSoraClientForPoll) EnhancePrompt(ctx context.Context, account *Account, prompt, expansionLevel string, durationS int) (string, error) {
	if s.enhanced != "" {
		return s.enhanced, s.enhanceErr
REDACTED
	return "enhanced prompt", s.enhanceErr
REDACTED
func (s *stubSoraClientForPoll) GetImageTask(ctx context.Context, account *Account, taskID string) (*SoraImageTaskStatus, error) {
	s.imageCalls++
	return s.imageStatus, nil
REDACTED
func (s *stubSoraClientForPoll) GetVideoTask(ctx context.Context, account *Account, taskID string) (*SoraVideoTaskStatus, error) {
	s.videoCalls++
	return s.videoStatus, nil
REDACTED

func TestSoraGatewayService_PollImageTaskCompleted(t *testing.T) {
	client := &stubSoraClientForPoll{
		imageStatus: &SoraImageTaskStatus{
			Status: "completed",
			URLs:   []string{"https://example.com/a.png"REDACTED,
	REDACTED,
REDACTED
	cfg := &config.Config{
		Sora: config.SoraConfig{
			Client: config.SoraClientConfig{
				PollIntervalSeconds: 1,
				MaxPollAttempts:     1,
		REDACTED,
	REDACTED,
REDACTED
	service := NewSoraGatewayService(client, nil, nil, cfg)

	urls, err := service.pollImageTask(context.Background(), nil, &Account{ID: 1REDACTED, "task", false)
REDACTED
	require.Equal(t, []string{"https://example.com/a.png"REDACTED, urls)
	require.Equal(t, 1, client.imageCalls)
REDACTED

func TestSoraGatewayService_ForwardPromptEnhance(t *testing.T) {
	client := &stubSoraClientForPoll{
		enhanced: "cinematic prompt",
REDACTED
	cfg := &config.Config{
		Sora: config.SoraConfig{
			Client: config.SoraClientConfig{
				PollIntervalSeconds: 1,
				MaxPollAttempts:     1,
		REDACTED,
	REDACTED,
REDACTED
	svc := NewSoraGatewayService(client, nil, nil, cfg)
	account := &Account{
		ID:       1,
		Platform: PlatformSora,
		Status:   StatusActive,
REDACTED
	body := []byte(`{"model":"prompt-enhance-short-10s","messages":[{"role":"user","content":"cat running"REDACTED],"stream":falseREDACTED`)

	result, err := svc.Forward(context.Background(), nil, account, body, false)
REDACTED
	require.NotNil(t, result)
	require.Equal(t, "prompt", result.MediaType)
	require.Equal(t, "prompt-enhance-short-10s", result.Model)
REDACTED

func TestSoraGatewayService_ForwardStoryboardPrompt(t *testing.T) {
	client := &stubSoraClientForPoll{
		videoStatus: &SoraVideoTaskStatus{
			Status: "completed",
			URLs:   []string{"https://example.com/v.mp4"REDACTED,
	REDACTED,
REDACTED
	cfg := &config.Config{
		Sora: config.SoraConfig{
			Client: config.SoraClientConfig{
				PollIntervalSeconds: 1,
				MaxPollAttempts:     1,
		REDACTED,
	REDACTED,
REDACTED
	svc := NewSoraGatewayService(client, nil, nil, cfg)
	account := &Account{ID: 1, Platform: PlatformSora, Status: StatusActiveREDACTED
	body := []byte(`{"model":"sora2-landscape-10s","messages":[{"role":"user","content":"[5.0s]猫猫跳伞 [5.0s]猫猫落地"REDACTED],"stream":falseREDACTED`)

	result, err := svc.Forward(context.Background(), nil, account, body, false)
REDACTED
	require.NotNil(t, result)
	require.True(t, client.storyboard)
REDACTED

func TestSoraGatewayService_ForwardCharacterOnly(t *testing.T) {
	client := &stubSoraClientForPoll{REDACTED
	cfg := &config.Config{
		Sora: config.SoraConfig{
			Client: config.SoraClientConfig{
				PollIntervalSeconds: 1,
				MaxPollAttempts:     1,
		REDACTED,
	REDACTED,
REDACTED
	svc := NewSoraGatewayService(client, nil, nil, cfg)
	account := &Account{ID: 1, Platform: PlatformSora, Status: StatusActiveREDACTED
	body := []byte(`{"model":"sora2-landscape-10s","video":"aGVsbG8=","stream":falseREDACTED`)

	result, err := svc.Forward(context.Background(), nil, account, body, false)
REDACTED
	require.NotNil(t, result)
	require.Equal(t, "prompt", result.MediaType)
	require.Equal(t, 0, client.videoCalls)
REDACTED

func TestSoraGatewayService_ForwardWatermarkFallback(t *testing.T) {
	client := &stubSoraClientForPoll{
		videoStatus: &SoraVideoTaskStatus{
			Status:       "completed",
			URLs:         []string{"https://example.com/original.mp4"REDACTED,
			GenerationID: "gen_1",
	REDACTED,
		parseErr: errors.New("parse failed"),
REDACTED
	cfg := &config.Config{
		Sora: config.SoraConfig{
			Client: config.SoraClientConfig{
				PollIntervalSeconds: 1,
				MaxPollAttempts:     1,
		REDACTED,
	REDACTED,
REDACTED
	svc := NewSoraGatewayService(client, nil, nil, cfg)
	account := &Account{ID: 1, Platform: PlatformSora, Status: StatusActiveREDACTED
	body := []byte(`{"model":"sora2-landscape-10s","messages":[{"role":"user","content":"cat running"REDACTED],"stream":false,"watermark_free":true,"watermark_parse_method":"custom","watermark_parse_url":"https://parser.example.com","watermark_parse_token":"token","watermark_fallback_on_failure":trueREDACTED`)

	result, err := svc.Forward(context.Background(), nil, account, body, false)
REDACTED
	require.NotNil(t, result)
	require.Equal(t, "https://example.com/original.mp4", result.MediaURL)
	require.Equal(t, 1, client.postCalls)
	require.Equal(t, 0, client.deleteCalls)
REDACTED

func TestSoraGatewayService_ForwardWatermarkCustomSuccessAndDelete(t *testing.T) {
	client := &stubSoraClientForPoll{
		videoStatus: &SoraVideoTaskStatus{
			Status:       "completed",
			URLs:         []string{"https://example.com/original.mp4"REDACTED,
			GenerationID: "gen_1",
	REDACTED,
REDACTED
	cfg := &config.Config{
		Sora: config.SoraConfig{
			Client: config.SoraClientConfig{
				PollIntervalSeconds: 1,
				MaxPollAttempts:     1,
		REDACTED,
	REDACTED,
REDACTED
	svc := NewSoraGatewayService(client, nil, nil, cfg)
	account := &Account{ID: 1, Platform: PlatformSora, Status: StatusActiveREDACTED
	body := []byte(`{"model":"sora2-landscape-10s","messages":[{"role":"user","content":"cat running"REDACTED],"stream":false,"watermark_free":true,"watermark_parse_method":"custom","watermark_parse_url":"https://parser.example.com","watermark_parse_token":"token","watermark_delete_post":trueREDACTED`)

	result, err := svc.Forward(context.Background(), nil, account, body, false)
REDACTED
	require.NotNil(t, result)
	require.Equal(t, "https://example.com/no-watermark.mp4", result.MediaURL)
	require.Equal(t, 1, client.postCalls)
	require.Equal(t, 1, client.deleteCalls)
REDACTED

func TestSoraGatewayService_PollVideoTaskFailed(t *testing.T) {
	client := &stubSoraClientForPoll{
		videoStatus: &SoraVideoTaskStatus{
			Status:   "failed",
			ErrorMsg: "reject",
	REDACTED,
REDACTED
	cfg := &config.Config{
		Sora: config.SoraConfig{
			Client: config.SoraClientConfig{
				PollIntervalSeconds: 1,
				MaxPollAttempts:     1,
		REDACTED,
	REDACTED,
REDACTED
	service := NewSoraGatewayService(client, nil, nil, cfg)

	status, err := service.pollVideoTaskDetailed(context.Background(), nil, &Account{ID: 1REDACTED, "task", false)
REDACTED
	require.Nil(t, status)
	require.Contains(t, err.Error(), "reject")
	require.Equal(t, 1, client.videoCalls)
REDACTED

func TestSoraGatewayService_BuildSoraMediaURLSigned(t *testing.T) {
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			SoraMediaSigningKey:          "test-key",
			SoraMediaSignedURLTTLSeconds: 600,
	REDACTED,
REDACTED
	service := NewSoraGatewayService(nil, nil, nil, cfg)

	url := service.buildSoraMediaURL("/image/2025/01/01/a.png", "")
	require.Contains(t, url, "/sora/media-signed")
	require.Contains(t, url, "expires=")
	require.Contains(t, url, "sig=")
REDACTED

func TestNormalizeSoraMediaURLs_Empty(t *testing.T) {
	svc := NewSoraGatewayService(nil, nil, nil, &config.Config{REDACTED)
	result := svc.normalizeSoraMediaURLs(nil)
	require.Empty(t, result)

	result = svc.normalizeSoraMediaURLs([]string{REDACTED)
	require.Empty(t, result)
REDACTED

func TestNormalizeSoraMediaURLs_HTTPUrls(t *testing.T) {
	svc := NewSoraGatewayService(nil, nil, nil, &config.Config{REDACTED)
	urls := []string{"https://example.com/a.png", "http://example.com/b.mp4"REDACTED
	result := svc.normalizeSoraMediaURLs(urls)
	require.Equal(t, urls, result)
REDACTED

func TestNormalizeSoraMediaURLs_LocalPaths(t *testing.T) {
	cfg := &config.Config{REDACTED
	svc := NewSoraGatewayService(nil, nil, nil, cfg)
	urls := []string{"/image/2025/01/a.png", "video/2025/01/b.mp4"REDACTED
	result := svc.normalizeSoraMediaURLs(urls)
	require.Len(t, result, 2)
	require.Contains(t, result[0], "/sora/media")
	require.Contains(t, result[1], "/sora/media")
REDACTED

func TestNormalizeSoraMediaURLs_SkipsBlank(t *testing.T) {
	svc := NewSoraGatewayService(nil, nil, nil, &config.Config{REDACTED)
	urls := []string{"https://example.com/a.png", "", "  ", "https://example.com/b.png"REDACTED
	result := svc.normalizeSoraMediaURLs(urls)
	require.Len(t, result, 2)
REDACTED

func TestBuildSoraContent_Image(t *testing.T) {
	content := buildSoraContent("image", []string{"https://a.com/1.png", "https://a.com/2.png"REDACTED)
	require.Contains(t, content, "![image](https://a.com/1.png)")
	require.Contains(t, content, "![image](https://a.com/2.png)")
REDACTED

func TestBuildSoraContent_Video(t *testing.T) {
	content := buildSoraContent("video", []string{"https://a.com/v.mp4"REDACTED)
	require.Contains(t, content, "<video src='https://a.com/v.mp4'")
REDACTED

func TestBuildSoraContent_VideoEmpty(t *testing.T) {
	content := buildSoraContent("video", nil)
	require.Empty(t, content)
REDACTED

func TestBuildSoraContent_Prompt(t *testing.T) {
	content := buildSoraContent("prompt", nil)
	require.Empty(t, content)
REDACTED

func TestSoraImageSizeFromModel(t *testing.T) {
	require.Equal(t, "360", soraImageSizeFromModel("gpt-image"))
	require.Equal(t, "540", soraImageSizeFromModel("gpt-image-landscape"))
	require.Equal(t, "540", soraImageSizeFromModel("gpt-image-portrait"))
	require.Equal(t, "540", soraImageSizeFromModel("something-landscape"))
	require.Equal(t, "360", soraImageSizeFromModel("unknown-model"))
REDACTED

func TestFirstMediaURL(t *testing.T) {
	require.Equal(t, "", firstMediaURL(nil))
	require.Equal(t, "", firstMediaURL([]string{REDACTED))
	require.Equal(t, "a", firstMediaURL([]string{"a", "b"REDACTED))
REDACTED

func TestSoraProErrorMessage(t *testing.T) {
	require.Contains(t, soraProErrorMessage("sora2pro-hd", ""), "Pro-HD")
	require.Contains(t, soraProErrorMessage("sora2pro", ""), "Pro")
	require.Empty(t, soraProErrorMessage("sora-basic", ""))
REDACTED

func TestSoraGatewayService_WriteSoraError_StreamEscapesJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	svc := NewSoraGatewayService(nil, nil, nil, &config.Config{REDACTED)
	svc.writeSoraError(c, http.StatusBadGateway, "upstream_error", "invalid \"prompt\"\nline2", true)

	body := rec.Body.String()
	require.Contains(t, body, "event: error\n")
	require.Contains(t, body, "data: [DONE]\n\n")

	lines := strings.Split(body, "\n")
	require.GreaterOrEqual(t, len(lines), 2)
	require.Equal(t, "event: error", lines[0])
	require.True(t, strings.HasPrefix(lines[1], "data: "))

	data := strings.TrimPrefix(lines[1], "data: ")
	var parsed map[string]any
	require.NoError(t, json.Unmarshal([]byte(data), &parsed))
	errObj, ok := parsed["error"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "upstream_error", errObj["type"])
	require.Equal(t, "invalid \"prompt\"\nline2", errObj["message"])
REDACTED

func TestShouldFailoverUpstreamError(t *testing.T) {
	svc := NewSoraGatewayService(nil, nil, nil, &config.Config{REDACTED)
	require.True(t, svc.shouldFailoverUpstreamError(401))
	require.True(t, svc.shouldFailoverUpstreamError(404))
	require.True(t, svc.shouldFailoverUpstreamError(429))
	require.True(t, svc.shouldFailoverUpstreamError(500))
	require.True(t, svc.shouldFailoverUpstreamError(502))
	require.False(t, svc.shouldFailoverUpstreamError(200))
	require.False(t, svc.shouldFailoverUpstreamError(400))
REDACTED

func TestWithSoraTimeout_NilService(t *testing.T) {
	var svc *SoraGatewayService
	ctx, cancel := svc.withSoraTimeout(context.Background(), false)
	require.NotNil(t, ctx)
	require.Nil(t, cancel)
REDACTED

func TestWithSoraTimeout_ZeroTimeout(t *testing.T) {
	cfg := &config.Config{REDACTED
	svc := NewSoraGatewayService(nil, nil, nil, cfg)
	ctx, cancel := svc.withSoraTimeout(context.Background(), false)
	require.NotNil(t, ctx)
	require.Nil(t, cancel)
REDACTED

func TestWithSoraTimeout_PositiveTimeout(t *testing.T) {
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			SoraRequestTimeoutSeconds: 30,
	REDACTED,
REDACTED
	svc := NewSoraGatewayService(nil, nil, nil, cfg)
	ctx, cancel := svc.withSoraTimeout(context.Background(), false)
	require.NotNil(t, ctx)
	require.NotNil(t, cancel)
	cancel()
REDACTED

func TestPollInterval(t *testing.T) {
	cfg := &config.Config{
		Sora: config.SoraConfig{
			Client: config.SoraClientConfig{
				PollIntervalSeconds: 5,
		REDACTED,
	REDACTED,
REDACTED
	svc := NewSoraGatewayService(nil, nil, nil, cfg)
	require.Equal(t, 5*time.Second, svc.pollInterval())

	// 默认值
	svc2 := NewSoraGatewayService(nil, nil, nil, &config.Config{REDACTED)
	require.True(t, svc2.pollInterval() > 0)
REDACTED

func TestPollMaxAttempts(t *testing.T) {
	cfg := &config.Config{
		Sora: config.SoraConfig{
			Client: config.SoraClientConfig{
				MaxPollAttempts: 100,
		REDACTED,
	REDACTED,
REDACTED
	svc := NewSoraGatewayService(nil, nil, nil, cfg)
	require.Equal(t, 100, svc.pollMaxAttempts())

	// 默认值
	svc2 := NewSoraGatewayService(nil, nil, nil, &config.Config{REDACTED)
	require.True(t, svc2.pollMaxAttempts() > 0)
REDACTED

func TestDecodeSoraImageInput_BlockPrivateURL(t *testing.T) {
	_, _, err := decodeSoraImageInput(context.Background(), "http://127.0.0.1/internal.png")
REDACTED
REDACTED

func TestDecodeSoraImageInput_DataURL(t *testing.T) {
	encoded := "data:image/png;base64,aGVsbG8="
	data, filename, err := decodeSoraImageInput(context.Background(), encoded)
REDACTED
	require.NotEmpty(t, data)
	require.Contains(t, filename, ".png")
REDACTED

func TestDecodeBase64WithLimit_ExceedLimit(t *testing.T) {
	data, err := decodeBase64WithLimit("aGVsbG8=", 3)
REDACTED
	require.Nil(t, data)
REDACTED

func TestParseSoraWatermarkOptions_NumericBool(t *testing.T) {
	body := map[string]any{
		"watermark_free":                float64(1),
		"watermark_fallback_on_failure": float64(0),
REDACTED
	opts := parseSoraWatermarkOptions(body)
	require.True(t, opts.Enabled)
	require.False(t, opts.FallbackOnFailure)
REDACTED
