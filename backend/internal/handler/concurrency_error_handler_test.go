package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestConcurrencyAcquireFailureDetails_ReturnsServiceUnavailable(t *testing.T) {
	status, errType, message := concurrencyAcquireFailureDetails(errors.New("MISCONF redis write disabled"))
	require.Equal(t, http.StatusServiceUnavailable, status)
	require.Equal(t, "service_unavailable", errType)
	require.Equal(t, "Concurrency backend unavailable, please retry later", message)
}

func TestOpenAIGatewayHandler_HandleConcurrencyError_ReturnsServiceUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	h := &OpenAIGatewayHandler{}
	h.handleConcurrencyError(c, errors.New("MISCONF redis write disabled"), "user", false)

	require.Equal(t, http.StatusServiceUnavailable, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	errorObj := body["error"].(map[string]any)
	require.Equal(t, "service_unavailable", errorObj["type"])
	require.Equal(t, "Concurrency backend unavailable, please retry later", errorObj["message"])
}

func TestGatewayHandler_HandleConcurrencyError_ReturnsServiceUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	h := &GatewayHandler{}
	h.handleConcurrencyError(c, errors.New("MISCONF redis write disabled"), "user", false)

	require.Equal(t, http.StatusServiceUnavailable, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	errorObj := body["error"].(map[string]any)
	require.Equal(t, "service_unavailable", errorObj["type"])
	require.Equal(t, "Concurrency backend unavailable, please retry later", errorObj["message"])
}
