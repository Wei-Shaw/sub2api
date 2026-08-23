//go:build unit

package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type userMaterialHandlerRenameRepo struct {
	service.UserMaterialRepository
	userID   int64
	id       int64
	fileName string
	result   *service.UserMaterial
}

func (r *userMaterialHandlerRenameRepo) UpdateFileNameByID(_ context.Context, userID, id int64, fileName string) (*service.UserMaterial, error) {
	r.userID = userID
	r.id = id
	r.fileName = fileName
	return r.result, nil
}

func TestUserMaterialHandlerRename(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &userMaterialHandlerRenameRepo{result: &service.UserMaterial{
		ID:          42,
		UserID:      7,
		FileName:    "renamed.png",
		CosURL:      "https://cdn.example.com/material.png",
		ContentType: "image/png",
		SizeBytes:   123,
		Kind:        "image",
		Source:      "upload",
		CreatedAt:   time.Date(2026, 8, 23, 3, 0, 0, 0, time.UTC),
	}}
	handler := NewUserMaterialHandler(service.NewUserMaterialService(repo, nil, nil, nil))
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPatch, "/api/v1/user/materials/42", strings.NewReader(`{"file_name":"  renamed.png  "}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: "42"}}
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 7})

	handler.Rename(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, int64(7), repo.userID)
	require.Equal(t, int64(42), repo.id)
	require.Equal(t, "renamed.png", repo.fileName)
	var body struct {
		Code int              `json:"code"`
		Data userMaterialItem `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	require.Zero(t, body.Code)
	require.Equal(t, "renamed.png", body.Data.FileName)
	require.Equal(t, "https://cdn.example.com/material.png", body.Data.URL)
}

func TestUserMaterialHandlerRenameDoesNotExposeForeignMaterial(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &userMaterialHandlerRenameRepo{}
	handler := NewUserMaterialHandler(service.NewUserMaterialService(repo, nil, nil, nil))
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPatch, "/api/v1/user/materials/42", strings.NewReader(`{"file_name":"renamed.png"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: "42"}}
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 7})

	handler.Rename(c)

	require.Equal(t, http.StatusNotFound, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"reason":"MATERIAL_NOT_FOUND"`)
}
