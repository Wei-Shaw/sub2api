//go:build unit

package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/chatgptwebimage"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func webImageTestAccount() *Account {
	return &Account{
		Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Credentials: map[string]any{"plan_type": "free", "model_mapping": map[string]any{"gpt-image-2": "gpt-image-2"}},
		Extra:       map[string]any{AccountExtraChatGPTWebImageEnabled: true, AccountExtraChatGPTWebImageRegularChat: true},
	}
}

func webImageTestRequest() *OpenAIImagesRequest {
	return &OpenAIImagesRequest{Endpoint: openAIImagesGenerationsEndpoint, Model: "gpt-image-2", Prompt: "A blue square", N: 1}
}

func webImageTestContext() (*gin.Context, *httptest.ResponseRecorder) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	return c, rec
}

func TestChatGPTWebImageAccountRequiresExplicitConsentAndFreeOAuth(t *testing.T) {
	require.False(t, chatGPTWebImageEnabled(nil))
	require.Nil(t, validateChatGPTWebImageAccount(webImageTestAccount()))
	cases := []struct {
		name string
		edit func(*Account)
		code string
	}{
		{"missing_enable", func(a *Account) { delete(a.Extra, AccountExtraChatGPTWebImageEnabled) }, "chatgpt_web_image_disabled"},
		{"string_enable", func(a *Account) { a.Extra[AccountExtraChatGPTWebImageEnabled] = "true" }, "chatgpt_web_image_disabled"},
		{"missing_history_consent", func(a *Account) { delete(a.Extra, AccountExtraChatGPTWebImageRegularChat) }, "chatgpt_web_image_history_consent_required"},
		{"string_history_consent", func(a *Account) { a.Extra[AccountExtraChatGPTWebImageRegularChat] = "true" }, "chatgpt_web_image_history_consent_required"},
		{"plus", func(a *Account) { a.Credentials["plan_type"] = "plus" }, "chatgpt_web_image_account_unsupported"},
		{"unknown_plan", func(a *Account) { delete(a.Credentials, "plan_type") }, "chatgpt_web_image_account_unsupported"},
		{"api_key", func(a *Account) { a.Type = AccountTypeAPIKey }, "chatgpt_web_image_account_unsupported"},
		{"setup_token", func(a *Account) { a.Type = AccountTypeSetupToken }, "chatgpt_web_image_account_unsupported"},
		{"other_platform", func(a *Account) { a.Platform = PlatformAnthropic }, "chatgpt_web_image_account_unsupported"},
		{"shadow", func(a *Account) { id := int64(7); a.ParentAccountID = &id }, "chatgpt_web_image_account_unsupported"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			account := webImageTestAccount()
			tc.edit(account)
			// No service dependencies or access token: validation must run first.
			_, err := (&OpenAIGatewayService{}).generateWebImage(context.Background(), account, "A blue square")
			var typed *OpenAIImagesUpstreamError
			require.ErrorAs(t, err, &typed)
			require.Equal(t, http.StatusBadRequest, typed.StatusCode)
			require.Equal(t, tc.code, typed.Code)
		})
	}
}

func TestChatGPTWebImagesRejectUnsupportedOptionsAndPrivacyFields(t *testing.T) {
	require.Nil(t, validateChatGPTWebImagesRequest(webImageTestRequest()))
	cases := []struct {
		name, param string
		edit        func(*OpenAIImagesRequest)
	}{
		{"edit", "image", func(r *OpenAIImagesRequest) { r.Endpoint = openAIImagesEditsEndpoint }},
		{"multipart", "image", func(r *OpenAIImagesRequest) { r.Multipart = true }},
		{"input", "image", func(r *OpenAIImagesRequest) { r.InputImageURLs = []string{"https://example.invalid/image.png"} }},
		{"mask", "image", func(r *OpenAIImagesRequest) { r.HasMask = true }},
		{"multiple", "n", func(r *OpenAIImagesRequest) { r.N = 2 }},
		{"stream", "stream", func(r *OpenAIImagesRequest) { r.Stream = true }},
		{"size", "size", func(r *OpenAIImagesRequest) { r.Size = "1024xinvalid" }},
		{"quality", "quality", func(r *OpenAIImagesRequest) { r.Quality = "high" }},
		{"transparent", "background", func(r *OpenAIImagesRequest) { r.Background = "transparent" }},
		{"url_response", "response_format", func(r *OpenAIImagesRequest) { r.ResponseFormat = "url" }},
		{"jpeg", "output_format", func(r *OpenAIImagesRequest) { r.OutputFormat = "jpeg" }},
		{"moderation", "moderation", func(r *OpenAIImagesRequest) { r.Moderation = "low" }},
		{"fidelity", "input_fidelity", func(r *OpenAIImagesRequest) { r.InputFidelity = "high" }},
		{"style", "style", func(r *OpenAIImagesRequest) { r.Style = "vivid" }},
		{"compression", "output_compression", func(r *OpenAIImagesRequest) { n := 100; r.OutputCompression = &n }},
		{"partial", "partial_images", func(r *OpenAIImagesRequest) { n := 1; r.PartialImages = &n }},
		{"store_false", "store", func(r *OpenAIImagesRequest) { r.Body = []byte(`{"store":false}`) }},
		{"temporary", "temporary", func(r *OpenAIImagesRequest) { r.Body = []byte(`{"temporary":true}`) }},
		{"no_history", "history_and_training_disabled", func(r *OpenAIImagesRequest) { r.Body = []byte(`{"history_and_training_disabled":true}`) }},
		{"unparsed_input", "images", func(r *OpenAIImagesRequest) { r.Body = []byte(`{"images":[{"file_id":"file-example"}]}`) }},
		{"empty_prompt", "prompt", func(r *OpenAIImagesRequest) { r.Prompt = " " }},
		{"long_prompt", "prompt", func(r *OpenAIImagesRequest) { r.Prompt = strings.Repeat("a", 1+(64<<10)) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := webImageTestRequest()
			tc.edit(req)
			err := validateChatGPTWebImagesRequest(req)
			require.NotNil(t, err)
			require.Equal(t, http.StatusBadRequest, err.StatusCode)
			require.Equal(t, tc.param, err.Param)
		})
	}
}

func TestForwardImagesChatGPTWebDisabledDoesNotFallBackToCodex(t *testing.T) {
	c, rec := webImageTestContext()
	account := webImageTestAccount()
	account.Extra[AccountExtraChatGPTWebImageEnabled] = false
	_, err := (&OpenAIGatewayService{}).ForwardImages(context.Background(), c, account, nil, webImageTestRequest(), "")
	var typed *OpenAIImagesUpstreamError
	require.ErrorAs(t, err, &typed)
	require.Equal(t, "chatgpt_web_image_disabled", typed.Code)
	require.Equal(t, "chatgpt_web", rec.Header().Get("X-Sub2API-Image-Provider"))
	require.True(t, IsChatGPTWebImageRequest(c))
}

func TestForwardImagesUnconfiguredFreeRejectsWithoutNativeFallback(t *testing.T) {
	for _, extra := range []map[string]any{nil, {}, {AccountExtraChatGPTWebImageEnabled: nil}, {AccountExtraChatGPTWebImageEnabled: "true"}} {
		c, rec := webImageTestContext()
		account := webImageTestAccount()
		account.Extra = extra
		// Reject locally before any credential fetch or upstream I/O.
		_, err := (&OpenAIGatewayService{}).ForwardImages(context.Background(), c, account, nil, webImageTestRequest(), "")
		var typed *OpenAIImagesUpstreamError
		require.ErrorAs(t, err, &typed)
		require.Equal(t, "chatgpt_web_image_disabled", typed.Code)
		require.Equal(t, http.StatusBadRequest, rec.Code)
		require.Equal(t, "chatgpt_web", rec.Header().Get("X-Sub2API-Image-Provider"))
		require.True(t, IsChatGPTWebImageRequest(c))
	}
}

func TestForwardImagesChatGPTWebChecksChannelMappedAlias(t *testing.T) {
	c, _ := webImageTestContext()
	a := webImageTestAccount()
	a.Credentials["model_mapping"] = map[string]any{"poster": "gpt-image-1.5"}
	r := webImageTestRequest()
	r.Model = "public-picture"
	// Validation of the resolved channel alias reaches the authenticated-job
	// boundary. The fixture deliberately has no API key and performs no I/O.
	_, err := (&OpenAIGatewayService{}).ForwardImages(context.Background(), c, a, nil, r, "poster")
	var typed *OpenAIImagesUpstreamError
	require.ErrorAs(t, err, &typed)
	require.Equal(t, "image_job_unavailable", typed.Code)
	c, _ = webImageTestContext()
	_, err = (&OpenAIGatewayService{}).ForwardImages(context.Background(), c, a, nil, r, "not-allowed")
	require.ErrorAs(t, err, &typed)
	require.Equal(t, "chatgpt_web_image_model_not_allowed", typed.Code)
}

func TestChatGPTWebImageRequestMarkerCannotBeSpoofedByHeaders(t *testing.T) {
	require.False(t, IsChatGPTWebImageRequest(nil))
	c, _ := webImageTestContext()
	c.Request.Header.Set("X-Sub2API-Image-Provider", "chatgpt_web")
	c.Header("X-Sub2API-Image-Provider", "chatgpt_web")
	require.False(t, IsChatGPTWebImageRequest(c))
	req := webImageTestRequest()
	req.N = 2 // Reject before credentials or network access.
	_, err := (&OpenAIGatewayService{}).ForwardImages(context.Background(), c, webImageTestAccount(), nil, req, "")
	require.Error(t, err)
	require.True(t, IsChatGPTWebImageRequest(c))
}

func TestForwardImagesChatGPTWebRejectsBeforeCredentialsAndPreservesAccountState(t *testing.T) {
	c, rec := webImageTestContext()
	account := webImageTestAccount()
	now := time.Now()
	account.Status, account.ErrorMessage = "error", "existing error"
	account.Schedulable, account.RateLimitResetAt = false, &now
	req := webImageTestRequest()
	req.Background = "transparent"
	_, err := (&OpenAIGatewayService{}).ForwardImages(context.Background(), c, account, nil, req, "")
	var typed *OpenAIImagesUpstreamError
	require.ErrorAs(t, err, &typed)
	require.Equal(t, "background", typed.Param)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, "chatgpt_web", rec.Header().Get("X-Sub2API-Image-Provider"))
	require.True(t, IsChatGPTWebImageRequest(c))
	require.Equal(t, "error", account.Status)
	require.Equal(t, "existing error", account.ErrorMessage)
	require.False(t, account.Schedulable)
	require.Equal(t, &now, account.RateLimitResetAt)
}

func TestChatGPTWebImageErrorNeverRequestsFailoverOrLeaksDetails(t *testing.T) {
	for _, accepted := range []bool{false, true} {
		providerErr := &chatgptwebimage.Error{Kind: "rate_limited", Stage: "secret-stage", StatusCode: 429, Accepted: accepted, ConversationID: "secret-conversation"}
		err := chatGPTWebImagePublicError(providerErr)
		var failover *UpstreamFailoverError
		require.False(t, errors.As(err, &failover))
		serialized, marshalErr := json.Marshal(err)
		require.NoError(t, marshalErr)
		require.NotContains(t, string(serialized), "secret")
		if accepted {
			require.Equal(t, http.StatusBadRequest, err.StatusCode)
			require.Equal(t, "image_generation_outcome_unknown", err.Code)
			require.False(t, IsOpenAIImagesRetryableUpstreamError(err))
		} else {
			require.Equal(t, 429, err.StatusCode)
			require.Equal(t, "chatgpt_web_image_rate_limited", err.Code)
		}
	}
	err := chatGPTWebImagePublicError(errors.New("signed URL or credential must not escape"))
	require.NotContains(t, err.Error(), "credential")
}

func TestChatGPTWebImagesResultReportsActualImageWithoutInventedModelOrUsage(t *testing.T) {
	var imageBytes bytes.Buffer
	require.NoError(t, png.Encode(&imageBytes, image.NewRGBA(image.Rect(0, 0, 8, 6))))
	c, rec := webImageTestContext()
	result, err := writeChatGPTWebImagesResult(c, webImageTestRequest(), chatgptwebimage.Result{
		Images: []chatgptwebimage.Image{{Bytes: imageBytes.Bytes(), Format: "png", Width: 999, Height: 999}}, WebDriver: "auto",
	}, time.Now())
	require.NoError(t, err)
	require.Equal(t, 200, rec.Code)
	var payload map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
	require.NotContains(t, payload, "model")
	require.NotContains(t, payload, "usage")
	var data []struct {
		B64JSON string `json:"b64_json"`
	}
	require.NoError(t, json.Unmarshal(payload["data"], &data))
	require.Len(t, data, 1)
	decoded, err := base64.StdEncoding.DecodeString(data[0].B64JSON)
	require.NoError(t, err)
	require.Equal(t, imageBytes.Bytes(), decoded)
	require.Equal(t, "8x6", result.ImageOutputSize)
	require.Equal(t, 1, result.ImageCount)
	require.Equal(t, "gpt-image-2", result.Model)
	require.Equal(t, "auto", result.UpstreamModel)
	require.Empty(t, result.UpstreamResponseModel)
	require.Equal(t, OpenAIUsage{}, result.Usage)
}

func TestChatGPTWebImagesResultRejectsInvalidImages(t *testing.T) {
	for _, images := range [][]chatgptwebimage.Image{
		nil,
		{{Bytes: []byte("not a PNG")}},
		{{Bytes: []byte("one")}, {Bytes: []byte("two")}},
	} {
		c, rec := webImageTestContext()
		result, err := writeChatGPTWebImagesResult(c, webImageTestRequest(), chatgptwebimage.Result{Images: images}, time.Now())
		require.Error(t, err)
		require.Nil(t, result)
		require.Equal(t, http.StatusBadRequest, rec.Code)
		require.Contains(t, rec.Body.String(), "image_generation_outcome_unknown")
		require.NotContains(t, rec.Body.String(), "b64_json")
	}
}
