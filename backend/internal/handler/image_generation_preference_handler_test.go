//go:build unit

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type imageGenerationPreferenceRepoStub struct {
	*userHandlerRepoStub
	lastAPIKey *int64
	ownedKeys  map[int64]bool
}

func (s *imageGenerationPreferenceRepoStub) GetLastImageAPIKeyID(context.Context, int64) (*int64, error) {
	return s.lastAPIKey, nil
}

func (s *imageGenerationPreferenceRepoStub) SetLastImageAPIKeyID(_ context.Context, _ int64, apiKeyID *int64) (*int64, error) {
	if apiKeyID != nil && !s.ownedKeys[*apiKeyID] {
		return nil, service.ErrAPIKeyNotFound
	}
	s.lastAPIKey = apiKeyID
	return s.lastAPIKey, nil
}

func newImageGenerationPreferenceHandler(lastAPIKey *int64, ownedKeys map[int64]bool) *UserHandler {
	repo := &imageGenerationPreferenceRepoStub{
		userHandlerRepoStub: &userHandlerRepoStub{user: &service.User{ID: 11}},
		lastAPIKey:          lastAPIKey,
		ownedKeys:           ownedKeys,
	}
	return NewUserHandler(service.NewUserService(repo, nil, nil, nil), nil, nil, nil, nil, nil)
}

func newImageGenerationPreferenceContext(method, body string) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(method, "/api/v1/user/preferences/image-generation", bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 11})
	return c, recorder
}

func TestUserHandlerGetImageGenerationPreferenceReturnsNull(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := newImageGenerationPreferenceHandler(nil, nil)
	c, recorder := newImageGenerationPreferenceContext(http.MethodGet, "")

	handler.GetImageGenerationPreference(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Code int `json:"code"`
		Data struct {
			LastAPIKeyID *int64 `json:"last_api_key_id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, 0, response.Code)
	require.Nil(t, response.Data.LastAPIKeyID)
}

func TestUserHandlerGetImageGenerationPreferenceReturnsSavedKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	keyID := int64(41)
	handler := newImageGenerationPreferenceHandler(&keyID, nil)
	c, recorder := newImageGenerationPreferenceContext(http.MethodGet, "")

	handler.GetImageGenerationPreference(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Data struct {
			LastAPIKeyID *int64 `json:"last_api_key_id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.NotNil(t, response.Data.LastAPIKeyID)
	require.Equal(t, keyID, *response.Data.LastAPIKeyID)
}

func TestUserHandlerUpdateImageGenerationPreferenceSavesOwnedKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := newImageGenerationPreferenceHandler(nil, map[int64]bool{41: true})
	c, recorder := newImageGenerationPreferenceContext(http.MethodPut, `{"last_api_key_id":41}`)

	handler.UpdateImageGenerationPreference(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Code int `json:"code"`
		Data struct {
			LastAPIKeyID *int64 `json:"last_api_key_id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, 0, response.Code)
	require.NotNil(t, response.Data.LastAPIKeyID)
	require.Equal(t, int64(41), *response.Data.LastAPIKeyID)
}

func TestUserHandlerUpdateImageGenerationPreferenceRejectsUnknownKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := newImageGenerationPreferenceHandler(nil, map[int64]bool{41: true})
	c, recorder := newImageGenerationPreferenceContext(http.MethodPut, `{"last_api_key_id":99}`)

	handler.UpdateImageGenerationPreference(c)

	require.Equal(t, http.StatusNotFound, recorder.Code)
}

func TestUserHandlerUpdateImageGenerationPreferenceRejectsForeignKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := newImageGenerationPreferenceHandler(nil, map[int64]bool{41: true})
	c, recorder := newImageGenerationPreferenceContext(http.MethodPut, `{"last_api_key_id":42}`)

	handler.UpdateImageGenerationPreference(c)

	require.Equal(t, http.StatusNotFound, recorder.Code)
}

func TestUserHandlerUpdateImageGenerationPreferenceClearsKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	keyID := int64(41)
	handler := newImageGenerationPreferenceHandler(&keyID, map[int64]bool{41: true})
	c, recorder := newImageGenerationPreferenceContext(http.MethodPut, `{"last_api_key_id":null}`)

	handler.UpdateImageGenerationPreference(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Data struct {
			LastAPIKeyID *int64 `json:"last_api_key_id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.Nil(t, response.Data.LastAPIKeyID)
}

func TestUserHandlerUpdateImageGenerationPreferenceRejectsMissingKeyFieldWithoutClearing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	keyID := int64(41)
	handler := newImageGenerationPreferenceHandler(&keyID, map[int64]bool{41: true})
	c, recorder := newImageGenerationPreferenceContext(http.MethodPut, `{}`)

	handler.UpdateImageGenerationPreference(c)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	getContext, getRecorder := newImageGenerationPreferenceContext(http.MethodGet, "")
	handler.GetImageGenerationPreference(getContext)
	require.Equal(t, http.StatusOK, getRecorder.Code)
	var response struct {
		Data struct {
			LastAPIKeyID *int64 `json:"last_api_key_id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(getRecorder.Body.Bytes(), &response))
	require.NotNil(t, response.Data.LastAPIKeyID)
	require.Equal(t, keyID, *response.Data.LastAPIKeyID)
}
