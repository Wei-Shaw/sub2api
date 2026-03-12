//go:build unit

package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestCreateGeminiTestPayload_ImageModel(t *testing.T) {
	t.Parallel()

	payload := createGeminiTestPayload("gemini-2.5-flash-image", "draw a tiny robot")

	var parsed struct {
		Contents []struct {
			Parts []struct {
				Text string `json:"text"`
		REDACTED `json:"parts"`
	REDACTED `json:"contents"`
		GenerationConfig struct {
			ResponseModalities []string `json:"responseModalities"`
			ImageConfig        struct {
				AspectRatio string `json:"aspectRatio"`
		REDACTED `json:"imageConfig"`
	REDACTED `json:"generationConfig"`
REDACTED

	require.NoError(t, json.Unmarshal(payload, &parsed))
	require.Len(t, parsed.Contents, 1)
	require.Len(t, parsed.Contents[0].Parts, 1)
	require.Equal(t, "draw a tiny robot", parsed.Contents[0].Parts[0].Text)
	require.Equal(t, []string{"TEXT", "IMAGE"REDACTED, parsed.GenerationConfig.ResponseModalities)
	require.Equal(t, "1:1", parsed.GenerationConfig.ImageConfig.AspectRatio)
REDACTED

func TestProcessGeminiStream_EmitsImageEvent(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	ctx, recorder := newSoraTestContext()
	svc := &AccountTestService{REDACTED

	stream := strings.NewReader("data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"ok\"REDACTED,{\"inlineData\":{\"mimeType\":\"image/png\",\"data\":\"QUJD\"REDACTEDREDACTED]REDACTEDREDACTED]REDACTED\n\ndata: [DONE]\n\n")

	err := svc.processGeminiStream(ctx, stream)
REDACTED

	body := recorder.Body.String()
	require.Contains(t, body, "\"type\":\"content\"")
	require.Contains(t, body, "\"text\":\"ok\"")
	require.Contains(t, body, "\"type\":\"image\"")
	require.Contains(t, body, "\"image_url\":\"data:image/png;base64,QUJD\"")
	require.Contains(t, body, "\"mime_type\":\"image/png\"")
REDACTED
