package service

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/imroc/req/v3"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type audioTranscriptionField struct {
	name  string
	value string
}

func audioTranscriptionTestWAV(seconds float64) []byte {
	const byteRate = 16000 * 2
	dataBytes := int(seconds * byteRate)
	le := binary.LittleEndian
	body := []byte("WAVEfmt ")
	body = le.AppendUint32(body, 16)
	body = le.AppendUint16(body, 1)
	body = le.AppendUint16(body, 1)
	body = le.AppendUint32(body, 16000)
	body = le.AppendUint32(body, byteRate)
	body = le.AppendUint16(body, 2)
	body = le.AppendUint16(body, 16)
	body = append(body, "data"...)
	body = le.AppendUint32(body, uint32(dataBytes))
	body = append(body, make([]byte, dataBytes)...)
	out := le.AppendUint32([]byte("RIFF"), uint32(len(body)))
	return append(out, body...)
}

func buildAudioTranscriptionMultipart(t *testing.T, audio []byte, fields ...audioTranscriptionField) ([]byte, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if audio != nil {
		part, err := writer.CreateFormFile("file", "audio.wav")
		require.NoError(t, err)
		_, err = part.Write(audio)
		require.NoError(t, err)
	}
	for _, field := range fields {
		require.NoError(t, writer.WriteField(field.name, field.value))
	}
	require.NoError(t, writer.Close())
	return body.Bytes(), writer.FormDataContentType()
}

func TestParseOpenAIAudioTranscriptionRequest(t *testing.T) {
	audio := audioTranscriptionTestWAV(1.5)
	body, contentType := buildAudioTranscriptionMultipart(t, audio,
		audioTranscriptionField{"model", "gpt-4o-mini-transcribe"},
		audioTranscriptionField{"prompt", " sub2api, openless "},
		audioTranscriptionField{"hotwords", `["sub2api"]`},
		audioTranscriptionField{"language", "zh"},
		audioTranscriptionField{"response_format", "VERBOSE_JSON"},
		audioTranscriptionField{"temperature", "0"},
	)

	parsed, err := ParseOpenAIAudioTranscriptionRequest(contentType, body)
	require.NoError(t, err)
	require.Equal(t, "gpt-4o-mini-transcribe", parsed.Model)
	require.Equal(t, "sub2api, openless", parsed.Prompt)
	require.Equal(t, "zh", parsed.Language)
	require.Equal(t, "verbose_json", parsed.ResponseFormat)
	require.Equal(t, "audio.wav", parsed.FileName)
	require.Equal(t, "application/octet-stream", parsed.FileContentType)
	require.Equal(t, audio, parsed.Audio)
	require.Equal(t, body, parsed.Body)
	require.Equal(t, contentType, parsed.ContentType)
	require.True(t, parsed.DurationExact)
	require.InDelta(t, 1.5, parsed.DurationSeconds, 1e-9)
	require.Equal(t, 2, parsed.BilledSeconds())
	require.Equal(t, "sub2api, openless", gjson.GetBytes(parsed.ModerationBody(), "prompt").String())
}

func TestParseOpenAIAudioTranscriptionRequestAcceptsFileWithoutFilename(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", `form-data; name="file"`)
	part, err := writer.CreatePart(header)
	require.NoError(t, err)
	_, err = part.Write(audioTranscriptionTestWAV(0.25))
	require.NoError(t, err)
	require.NoError(t, writer.WriteField("model", "whisper-1"))
	require.NoError(t, writer.Close())

	parsed, err := ParseOpenAIAudioTranscriptionRequest(writer.FormDataContentType(), body.Bytes())
	require.NoError(t, err)
	require.Empty(t, parsed.FileName)
	require.Equal(t, 1, parsed.BilledSeconds(), "sub-second clips bill the one second floor")
}

func TestParseOpenAIAudioTranscriptionRequestRejectsInvalidInput(t *testing.T) {
	audio := audioTranscriptionTestWAV(1)
	tests := []struct {
		name        string
		contentType string
		body        []byte
		wantStatus  int
		wantMessage string
	}{
		{name: "json body", contentType: "application/json", body: []byte(`{"model":"whisper-1"}`), wantStatus: http.StatusBadRequest, wantMessage: "multipart/form-data is required"},
	}
	add := func(name string, audio []byte, wantStatus int, wantMessage string, fields ...audioTranscriptionField) {
		body, contentType := buildAudioTranscriptionMultipart(t, audio, fields...)
		tests = append(tests, struct {
			name        string
			contentType string
			body        []byte
			wantStatus  int
			wantMessage string
		}{name, contentType, body, wantStatus, wantMessage})
	}
	add("missing file", nil, http.StatusBadRequest, "file is required", audioTranscriptionField{"model", "whisper-1"})
	add("empty file", []byte{}, http.StatusBadRequest, "file is empty", audioTranscriptionField{"model", "whisper-1"})
	add("missing model", audio, http.StatusBadRequest, "model is required")
	add("stream", audio, http.StatusBadRequest, "stream is not supported", audioTranscriptionField{"model", "whisper-1"}, audioTranscriptionField{"stream", "true"})
	add("srt", audio, http.StatusBadRequest, `response_format "srt" is not supported`, audioTranscriptionField{"model", "whisper-1"}, audioTranscriptionField{"response_format", "srt"})
	add("oversized file", make([]byte, openAIAudioTranscriptionMaxFileSize+1), http.StatusRequestEntityTooLarge, "25 MB", audioTranscriptionField{"model", "whisper-1"})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseOpenAIAudioTranscriptionRequest(tt.contentType, tt.body)
			require.Error(t, err)
			var reqErr *OpenAIAudioTranscriptionRequestError
			require.True(t, errors.As(err, &reqErr))
			require.Equal(t, tt.wantStatus, reqErr.StatusCode)
			require.Contains(t, err.Error(), tt.wantMessage)
		})
	}
}

func TestOpenAIAudioTranscriptionResponse(t *testing.T) {
	body, contentType := openAIAudioTranscriptionResponse("hello", &OpenAIAudioTranscriptionRequest{}, 3)
	require.Equal(t, "application/json", contentType)
	require.Equal(t, "hello", gjson.GetBytes(body, "text").String())
	require.Equal(t, "duration", gjson.GetBytes(body, "usage.type").String())
	require.Equal(t, int64(3), gjson.GetBytes(body, "usage.seconds").Int())

	body, contentType = openAIAudioTranscriptionResponse("hello", &OpenAIAudioTranscriptionRequest{ResponseFormat: "verbose_json", Language: "en"}, 3)
	require.Equal(t, "application/json", contentType)
	require.Equal(t, "transcribe", gjson.GetBytes(body, "task").String())
	require.Equal(t, int64(3), gjson.GetBytes(body, "duration").Int())
	require.Equal(t, "en", gjson.GetBytes(body, "language").String())
	require.Equal(t, "hello", gjson.GetBytes(body, "text").String())

	body, contentType = openAIAudioTranscriptionResponse("hello", &OpenAIAudioTranscriptionRequest{ResponseFormat: "text"}, 3)
	require.Equal(t, "text/plain; charset=utf-8", contentType)
	require.Equal(t, "hello", string(body))
}

func TestOpenAIAudioTranscriptionBilledSeconds(t *testing.T) {
	require.Equal(t, 1, openAIAudioTranscriptionBilledSeconds(0))
	require.Equal(t, 1, openAIAudioTranscriptionBilledSeconds(0.2))
	require.Equal(t, 1, openAIAudioTranscriptionBilledSeconds(1))
	require.Equal(t, 2, openAIAudioTranscriptionBilledSeconds(1.01))
	require.InDelta(t, 2.0/3600, openAIAudioTranscriptionUsage(2).DurationOrUnits, 1e-12)
	require.Equal(t, "stt", openAIAudioTranscriptionUsage(2).Mode)
}

func TestAudioTranscriptionCapabilityMatrix(t *testing.T) {
	tests := []struct {
		name    string
		account *Account
		want    bool
	}{
		{name: "openai oauth", account: &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}, want: true},
		{name: "openai personal access token", account: &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"auth_mode": OpenAIAuthModePersonalAccessToken}}, want: false},
		{name: "openai agent identity", account: &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"auth_mode": OpenAIAuthModeAgentIdentity}}, want: false},
		{name: "openai api key", account: &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, want: true},
		{name: "openai setup token", account: &Account{Platform: PlatformOpenAI, Type: AccountTypeSetupToken}, want: false},
		{name: "grok oauth", account: &Account{Platform: PlatformGrok, Type: AccountTypeOAuth}, want: false},
		{name: "oauth with embeddings-only whitelist", account: &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"openai_capabilities": []any{"embeddings"}}}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.account.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityAudioTranscriptions))
		})
	}
}

type chatGPTTranscribeStub struct {
	status      int
	contentType string
	body        string
	requests    []*http.Request
	files       [][]byte
	fileNames   []string
}

func (s *chatGPTTranscribeStub) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.requests = append(s.requests, r.Clone(context.Background()))
		_, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		require.NoError(t, err)
		reader := multipart.NewReader(r.Body, params["boundary"])
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}
			require.NoError(t, err)
			data, err := io.ReadAll(part)
			require.NoError(t, err)
			if part.FormName() == "file" {
				s.files = append(s.files, data)
				s.fileNames = append(s.fileNames, part.FileName())
			}
		}
		w.Header().Set("Content-Type", s.contentType)
		w.Header().Set("x-request-id", "req-upstream-1")
		w.WriteHeader(s.status)
		_, _ = io.WriteString(w, s.body)
	}
}

func newChatGPTTranscribeTestService(t *testing.T, stub *chatGPTTranscribeStub) (*OpenAIGatewayService, *chatGPTTranscribeStub) {
	t.Helper()
	srv := httptest.NewServer(stub.handler(t))
	t.Cleanup(srv.Close)
	previous := chatgptTranscribeURL
	chatgptTranscribeURL = srv.URL + "/backend-api/transcribe"
	t.Cleanup(func() { chatgptTranscribeURL = previous })
	svc := &OpenAIGatewayService{
		cfg: &config.Config{},
		// The production factory impersonates Chrome; the same client must work
		// against a plain HTTP test server so this stays representative.
		chatGPTUploadClientFactory: func(string) (*req.Client, error) { return req.C().ImpersonateChrome(), nil },
	}
	return svc, stub
}

func chatGPTTranscribeTestAccount() *Account {
	return &Account{
		ID:       41,
		Name:     "codex-oauth",
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token":       "tok-41",
			"chatgpt_account_id": "acct-41",
		},
	}
}

func audioTranscriptionTestContext(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/audio/transcriptions", nil)
	return c, rec
}

func parsedAudioTranscription(t *testing.T, seconds float64, fields ...audioTranscriptionField) *OpenAIAudioTranscriptionRequest {
	t.Helper()
	if len(fields) == 0 {
		fields = []audioTranscriptionField{{"model", "gpt-4o-mini-transcribe"}}
	}
	body, contentType := buildAudioTranscriptionMultipart(t, audioTranscriptionTestWAV(seconds), fields...)
	parsed, err := ParseOpenAIAudioTranscriptionRequest(contentType, body)
	require.NoError(t, err)
	return parsed
}

func TestForwardAudioTranscriptionChatGPTMapsResponseAndBillsDuration(t *testing.T) {
	svc, stub := newChatGPTTranscribeTestService(t, &chatGPTTranscribeStub{
		status:      http.StatusOK,
		contentType: "application/json",
		body:        `{"text":"hello world","asset_pointer":"sediment://file_1","asset_ttl":"30d","asset_format":"wav"}`,
	})
	c, rec := audioTranscriptionTestContext(t)
	parsed := parsedAudioTranscription(t, 1.5)

	result, err := svc.ForwardAudioTranscription(context.Background(), c, chatGPTTranscribeTestAccount(), parsed, "")

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	require.Equal(t, "hello world", gjson.GetBytes(rec.Body.Bytes(), "text").String())
	require.False(t, gjson.GetBytes(rec.Body.Bytes(), "asset_pointer").Exists())
	require.Equal(t, int64(2), gjson.GetBytes(rec.Body.Bytes(), "usage.seconds").Int())

	require.Len(t, stub.requests, 1)
	upstream := stub.requests[0]
	require.Equal(t, "/backend-api/transcribe", upstream.URL.Path)
	require.Equal(t, "Bearer tok-41", upstream.Header.Get("Authorization"))
	require.Equal(t, "acct-41", upstream.Header.Get("Chatgpt-Account-Id"))
	require.Equal(t, "application/json", upstream.Header.Get("Accept"))
	require.Equal(t, "cors", upstream.Header.Get("Sec-Fetch-Mode"))
	require.Equal(t, "https://chatgpt.com", upstream.Header.Get("Origin"))
	require.Equal(t, []string{"audio.wav"}, stub.fileNames)
	require.Equal(t, parsed.Audio, stub.files[0])

	require.Equal(t, "openai_audio:req-upstream-1", result.RequestID)
	require.Equal(t, "gpt-4o-mini-transcribe", result.Model)
	require.Equal(t, "/backend-api/transcribe", result.UpstreamEndpoint)
	require.NotNil(t, result.AudioUsage)
	require.Equal(t, "stt", result.AudioUsage.Mode)
	require.InDelta(t, 2.0/3600, result.AudioUsage.DurationOrUnits, 1e-12)
}

func TestForwardAudioTranscriptionChatGPTHonorsResponseFormat(t *testing.T) {
	svc, _ := newChatGPTTranscribeTestService(t, &chatGPTTranscribeStub{status: http.StatusOK, contentType: "application/json", body: `{"text":"hello"}`})

	c, rec := audioTranscriptionTestContext(t)
	parsed := parsedAudioTranscription(t, 1, audioTranscriptionField{"model", "whisper-1"}, audioTranscriptionField{"response_format", "text"})
	_, err := svc.ForwardAudioTranscription(context.Background(), c, chatGPTTranscribeTestAccount(), parsed, "")
	require.NoError(t, err)
	require.Equal(t, "text/plain; charset=utf-8", rec.Header().Get("Content-Type"))
	require.Equal(t, "hello", rec.Body.String())

	c, rec = audioTranscriptionTestContext(t)
	parsed = parsedAudioTranscription(t, 1, audioTranscriptionField{"model", "whisper-1"}, audioTranscriptionField{"response_format", "verbose_json"}, audioTranscriptionField{"language", "zh"})
	_, err = svc.ForwardAudioTranscription(context.Background(), c, chatGPTTranscribeTestAccount(), parsed, "")
	require.NoError(t, err)
	require.Equal(t, "transcribe", gjson.GetBytes(rec.Body.Bytes(), "task").String())
	require.Equal(t, "zh", gjson.GetBytes(rec.Body.Bytes(), "language").String())
	require.Equal(t, int64(1), gjson.GetBytes(rec.Body.Bytes(), "duration").Int())
}

func TestForwardAudioTranscriptionChatGPTEmptyTranscriptStillBills(t *testing.T) {
	svc, _ := newChatGPTTranscribeTestService(t, &chatGPTTranscribeStub{status: http.StatusOK, contentType: "application/json", body: `{"text":""}`})
	c, rec := audioTranscriptionTestContext(t)

	result, err := svc.ForwardAudioTranscription(context.Background(), c, chatGPTTranscribeTestAccount(), parsedAudioTranscription(t, 4), "")

	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "", gjson.GetBytes(rec.Body.Bytes(), "text").String())
	require.NotNil(t, result.AudioUsage)
	require.InDelta(t, 4.0/3600, result.AudioUsage.DurationOrUnits, 1e-12)
}

func TestForwardAudioTranscriptionChatGPTCloudflareChallengeIsNotFailover(t *testing.T) {
	svc, _ := newChatGPTTranscribeTestService(t, &chatGPTTranscribeStub{
		status:      http.StatusForbidden,
		contentType: "text/html; charset=UTF-8",
		body:        `<!DOCTYPE html><html><head><title>Just a moment...</title></head><body>cloudflare challenge-platform</body></html>`,
	})
	c, rec := audioTranscriptionTestContext(t)

	result, err := svc.ForwardAudioTranscription(context.Background(), c, chatGPTTranscribeTestAccount(), parsedAudioTranscription(t, 1), "")

	require.Error(t, err)
	require.Nil(t, result)
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr), "a fingerprint challenge must not trigger account failover")
	require.Equal(t, http.StatusBadGateway, rec.Code)
	require.Contains(t, gjson.GetBytes(rec.Body.Bytes(), "error.message").String(), "Cloudflare")
}

func TestForwardAudioTranscriptionChatGPTUnauthorizedFailsOverWithoutWriting(t *testing.T) {
	svc, _ := newChatGPTTranscribeTestService(t, &chatGPTTranscribeStub{
		status:      http.StatusUnauthorized,
		contentType: "application/json",
		body:        `{"error":{"message":"Unauthorized - Invalid access token","code":"unauthorized_unknown"},"status":401}`,
	})
	c, rec := audioTranscriptionTestContext(t)

	result, err := svc.ForwardAudioTranscription(context.Background(), c, chatGPTTranscribeTestAccount(), parsedAudioTranscription(t, 1), "")

	require.Nil(t, result)
	var failoverErr *UpstreamFailoverError
	require.True(t, errors.As(err, &failoverErr))
	require.Equal(t, http.StatusUnauthorized, failoverErr.StatusCode)
	require.Zero(t, rec.Body.Len(), "failover leaves the response to the handler")
}

func TestForwardAudioTranscriptionChatGPTPayloadTooLargeIsPassedThrough(t *testing.T) {
	svc, _ := newChatGPTTranscribeTestService(t, &chatGPTTranscribeStub{status: http.StatusRequestEntityTooLarge, contentType: "application/json", body: `{"detail":"too large"}`})
	c, rec := audioTranscriptionTestContext(t)

	_, err := svc.ForwardAudioTranscription(context.Background(), c, chatGPTTranscribeTestAccount(), parsedAudioTranscription(t, 1), "")

	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr))
	require.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
}

func TestForwardAudioTranscriptionChatGPTUnexpectedBodyIsBadGateway(t *testing.T) {
	svc, _ := newChatGPTTranscribeTestService(t, &chatGPTTranscribeStub{status: http.StatusOK, contentType: "text/html", body: `<html>login</html>`})
	c, rec := audioTranscriptionTestContext(t)

	result, err := svc.ForwardAudioTranscription(context.Background(), c, chatGPTTranscribeTestAccount(), parsedAudioTranscription(t, 1), "")

	require.Error(t, err)
	require.Nil(t, result)
	require.Equal(t, http.StatusBadGateway, rec.Code)
}

func TestForwardAudioTranscriptionChatGPTRejectsAgentIdentity(t *testing.T) {
	svc, stub := newChatGPTTranscribeTestService(t, &chatGPTTranscribeStub{status: http.StatusOK, contentType: "application/json", body: `{"text":"x"}`})
	c, _ := audioTranscriptionTestContext(t)
	account := chatGPTTranscribeTestAccount()
	account.Credentials["auth_mode"] = OpenAIAuthModeAgentIdentity

	_, err := svc.ForwardAudioTranscription(context.Background(), c, account, parsedAudioTranscription(t, 1), "")

	require.Error(t, err)
	require.Empty(t, stub.requests, "no bearer token means no upstream call")
}

func TestForwardAudioTranscriptionAPIKeyRewritesModelAndBillsUpstreamDuration(t *testing.T) {
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}, "X-Request-Id": []string{"req-api-1"}},
		Body:       io.NopCloser(strings.NewReader(`{"text":"hi","usage":{"type":"duration","seconds":3}}`)),
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{
		ID:       7,
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":       "sk-test",
			"base_url":      "https://compat.example/v4",
			"model_mapping": map[string]any{"whisper-1": "whisper-large"},
		},
	}
	c, rec := audioTranscriptionTestContext(t)
	parsed := parsedAudioTranscription(t, 1, audioTranscriptionField{"model", "whisper-1"}, audioTranscriptionField{"prompt", "names"})

	result, err := svc.ForwardAudioTranscription(context.Background(), c, account, parsed, "")

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "https://compat.example/v4/audio/transcriptions", upstream.lastReq.URL.String())
	require.Equal(t, "Bearer sk-test", upstream.lastReq.Header.Get("Authorization"))
	_, params, err := mime.ParseMediaType(upstream.lastReq.Header.Get("Content-Type"))
	require.NoError(t, err)
	fields := map[string]string{}
	var forwardedAudio []byte
	reader := multipart.NewReader(bytes.NewReader(upstream.lastBody), params["boundary"])
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		data, err := io.ReadAll(part)
		require.NoError(t, err)
		if part.FormName() == "file" {
			forwardedAudio = data
			continue
		}
		fields[part.FormName()] = string(data)
	}
	require.Equal(t, "whisper-large", fields["model"])
	require.Equal(t, "names", fields["prompt"])
	require.Equal(t, parsed.Audio, forwardedAudio)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "hi", gjson.GetBytes(rec.Body.Bytes(), "text").String())
	require.Equal(t, "whisper-1", result.Model)
	require.Equal(t, "whisper-large", result.UpstreamModel)
	require.Equal(t, "openai_audio:req-api-1", result.RequestID)
	require.NotNil(t, result.AudioUsage)
	require.InDelta(t, 3.0/3600, result.AudioUsage.DurationOrUnits, 1e-12)
}

func TestForwardAudioTranscriptionAPIKeyForwardsBodyVerbatimWithoutMapping(t *testing.T) {
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"text":"hi"}`)),
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{ID: 8, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "sk-test"}}
	c, _ := audioTranscriptionTestContext(t)
	parsed := parsedAudioTranscription(t, 2)

	result, err := svc.ForwardAudioTranscription(context.Background(), c, account, parsed, "")

	require.NoError(t, err)
	require.Equal(t, "https://api.openai.com/v1/audio/transcriptions", upstream.lastReq.URL.String())
	require.Equal(t, parsed.ContentType, upstream.lastReq.Header.Get("Content-Type"))
	require.True(t, bytes.Equal(parsed.Body, upstream.lastBody))
	require.InDelta(t, 2.0/3600, result.AudioUsage.DurationOrUnits, 1e-12, "no upstream duration falls back to the local measurement")
}

func TestForwardAudioTranscriptionAPIKeyPassesThroughClientErrors(t *testing.T) {
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"error":{"type":"invalid_request_error","message":"bad audio"}}`)),
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{ID: 9, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "sk-test"}}
	c, rec := audioTranscriptionTestContext(t)

	result, err := svc.ForwardAudioTranscription(context.Background(), c, account, parsedAudioTranscription(t, 1), "")

	require.Error(t, err)
	require.Nil(t, result)
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr))
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, "bad audio", gjson.GetBytes(rec.Body.Bytes(), "error.message").String())
}
