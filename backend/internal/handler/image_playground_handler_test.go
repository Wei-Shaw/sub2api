package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type imagePlaygroundApplicationStub struct {
	options  *service.ImagePlaygroundOptions
	key      *service.APIKey
	err      error
	disabled bool
}

func (s *imagePlaygroundApplicationStub) Enabled(context.Context) bool { return !s.disabled }

func (s *imagePlaygroundApplicationStub) Options(context.Context, int64) (*service.ImagePlaygroundOptions, error) {
	return s.options, s.err
}

func (s *imagePlaygroundApplicationStub) ResolveAPIKey(context.Context, int64, int64) (*service.APIKey, error) {
	return s.key, s.err
}

func (s *imagePlaygroundApplicationStub) ValidateGenerationOptions(context.Context, *service.Group, string, string, string, string, string, int) error {
	return s.err
}

func imagePlaygroundTestContext(method, path string, body []byte) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(method, path, bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 7})
	return c, recorder
}

func TestImagePlaygroundOptionsUsesDashboardEnvelope(t *testing.T) {
	c, recorder := imagePlaygroundTestContext(http.MethodGet, "/api/v1/image-playground/options", nil)
	h := &ImagePlaygroundHandler{playground: &imagePlaygroundApplicationStub{
		options: &service.ImagePlaygroundOptions{Enabled: true, Groups: []service.ImagePlaygroundGroupOption{}},
	}}

	h.Options(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	var envelope map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &envelope))
	require.Equal(t, float64(0), envelope["code"])
	data, ok := envelope["data"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, true, data["enabled"])
}

func TestImagePlaygroundResolveAPIKeyReplacesJWTWithoutWritingKey(t *testing.T) {
	c, recorder := imagePlaygroundTestContext(http.MethodPost, "/api/v1/image-playground/tasks", []byte(`{"group_id":3}`))
	c.Request.Header.Set("Authorization", "Bearer dashboard-jwt")
	c.Request.Header.Set("x-api-key", "caller-key")
	h := &ImagePlaygroundHandler{playground: &imagePlaygroundApplicationStub{
		key: &service.APIKey{ID: 9, UserID: 7, Key: "sk-selected"},
	}}

	h.ResolveAPIKey(c)

	require.False(t, c.IsAborted())
	require.Equal(t, "Bearer sk-selected", c.Request.Header.Get("Authorization"))
	require.Empty(t, c.Request.Header.Get("x-api-key"))
	require.NotContains(t, recorder.Body.String(), "sk-selected")
}

func TestImagePlaygroundSubmitCreatesDashboardTask(t *testing.T) {
	groupID := int64(3)
	group := &service.Group{ID: groupID, Platform: service.PlatformOpenAI, Status: service.StatusActive, AllowImageGeneration: true}
	apiKey := &service.APIKey{ID: 9, UserID: 7, GroupID: &groupID, Group: group, Status: service.StatusActive}
	store := &imageTaskMemoryStoreForPlayground{}
	tasks := service.NewImageTaskServiceWithUploader(store, nil, time.Hour, time.Minute)
	async := NewAsyncImageHandler(tasks, nil)
	release := make(chan struct{})
	async.execute = func(_ string, c *gin.Context) {
		<-release
		c.JSON(http.StatusOK, gin.H{"data": []gin.H{{"url": "https://cdn.example/image.png"}}})
	}
	h := &ImagePlaygroundHandler{
		playground: &imagePlaygroundApplicationStub{},
		tasks:      tasks,
		async:      async,
	}
	c, recorder := imagePlaygroundTestContext(http.MethodPost, "/api/v1/image-playground/tasks", []byte(`{"group_id":3,"model":"gpt-image-2","prompt":"draw a lighthouse"}`))
	c.Set(string(middleware2.ContextKeyAPIKey), apiKey)

	h.Submit(c)

	require.Equal(t, http.StatusAccepted, recorder.Code)
	require.Equal(t, "3", recorder.Header().Get("Retry-After"))
	var envelope struct {
		Code int `json:"code"`
		Data struct {
			ID      string `json:"id"`
			Status  string `json:"status"`
			PollURL string `json:"poll_url"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &envelope))
	require.Zero(t, envelope.Code)
	require.NotEmpty(t, envelope.Data.ID)
	require.Equal(t, service.ImageTaskStatusProcessing, envelope.Data.Status)
	require.Equal(t, "/api/v1/image-playground/tasks/"+envelope.Data.ID, envelope.Data.PollURL)
	close(release)
	require.Eventually(t, func() bool {
		task, err := tasks.GetForUser(context.Background(), 7, envelope.Data.ID)
		return err == nil && task.Status == service.ImageTaskStatusCompleted
	}, time.Second, 10*time.Millisecond)
}

func TestImagePlaygroundSubmitWithReferenceImageUsesAsyncEdits(t *testing.T) {
	groupID := int64(3)
	group := &service.Group{ID: groupID, Platform: service.PlatformOpenAI, Status: service.StatusActive, AllowImageGeneration: true}
	apiKey := &service.APIKey{ID: 9, UserID: 7, GroupID: &groupID, Group: group, Status: service.StatusActive}
	store := &imageTaskMemoryStoreForPlayground{}
	tasks := service.NewImageTaskServiceWithUploader(store, nil, time.Hour, time.Minute)
	async := NewAsyncImageHandler(tasks, nil)
	forwarded := make(chan struct {
		path            string
		inboundEndpoint string
		userID          int64
		apiKeyID        int64
		groupID         int64
		prompt          string
		imageCount      int
		imageName       string
		imageType       string
		imageData       []byte
	}, 1)
	async.execute = func(_ string, c *gin.Context) {
		result := struct {
			path            string
			inboundEndpoint string
			userID          int64
			apiKeyID        int64
			groupID         int64
			prompt          string
			imageCount      int
			imageName       string
			imageType       string
			imageData       []byte
		}{path: c.Request.URL.Path, inboundEndpoint: GetInboundEndpoint(c)}
		subject, ok := middleware2.GetAuthSubjectFromContext(c)
		require.True(t, ok)
		result.userID = subject.UserID
		forwardedKey, ok := middleware2.GetAPIKeyFromContext(c)
		require.True(t, ok)
		result.apiKeyID = forwardedKey.ID
		require.NotNil(t, forwardedKey.GroupID)
		result.groupID = *forwardedKey.GroupID
		reader, err := c.Request.MultipartReader()
		require.NoError(t, err)
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}
			require.NoError(t, err)
			data, err := io.ReadAll(part)
			require.NoError(t, err)
			if part.FormName() == "prompt" {
				result.prompt = string(data)
			}
			if part.FormName() == "image[]" {
				result.imageCount++
				result.imageName = part.FileName()
				result.imageType = part.Header.Get("Content-Type")
				result.imageData = append([]byte(nil), data...)
			}
		}
		forwarded <- result
		c.JSON(http.StatusOK, gin.H{"data": []gin.H{{"url": "https://cdn.example/edited.png"}}})
	}
	h := &ImagePlaygroundHandler{playground: &imagePlaygroundApplicationStub{}, tasks: tasks, async: async}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("group_id", "3"))
	require.NoError(t, writer.WriteField("model", "gpt-image-2"))
	image, err := writer.CreateFormFile("images", "reference.png")
	require.NoError(t, err)
	_, err = image.Write([]byte("\x89PNG\r\n\x1a\n"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	c, recorder := imagePlaygroundTestContext(http.MethodPost, "/api/v1/image-playground/tasks", body.Bytes())
	c.Request.Header.Set("Content-Type", writer.FormDataContentType())
	c.Set(string(middleware2.ContextKeyAPIKey), apiKey)
	h.Submit(c)

	require.Equal(t, http.StatusAccepted, recorder.Code)
	select {
	case got := <-forwarded:
		require.Equal(t, "/v1/images/edits", got.path)
		require.Equal(t, EndpointImagesEdits, got.inboundEndpoint)
		require.Equal(t, int64(7), got.userID)
		require.Equal(t, int64(9), got.apiKeyID)
		require.Equal(t, int64(3), got.groupID)
		require.Equal(t, imagePlaygroundDefaultEditPrompt, got.prompt)
		require.Equal(t, 1, got.imageCount)
		require.Equal(t, "reference.png", got.imageName)
		require.Equal(t, "image/png", got.imageType)
		require.Equal(t, []byte("\x89PNG\r\n\x1a\n"), got.imageData)
	case <-time.After(time.Second):
		t.Fatal("async edit request was not executed")
	}
}

func TestReadImagePlaygroundUploadsValidatesFiles(t *testing.T) {
	t.Run("rejects empty files", func(t *testing.T) {
		headers := imagePlaygroundMultipartHeaders(t, []imagePlaygroundMultipartFile{{name: "empty.png"}})
		_, err := readImagePlaygroundUploads(headers)
		require.Error(t, err)
		require.Contains(t, err.Error(), "cannot be empty")
	})

	t.Run("rejects content that is not an image even with an image filename", func(t *testing.T) {
		headers := imagePlaygroundMultipartHeaders(t, []imagePlaygroundMultipartFile{{name: "fake.png", data: []byte("plain text")}})
		_, err := readImagePlaygroundUploads(headers)
		require.Error(t, err)
		require.Contains(t, err.Error(), "PNG, JPEG, or WebP")
	})

	t.Run("rejects a file larger than ten megabytes", func(t *testing.T) {
		data := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0}, int(imagePlaygroundMaxInputImageBytes))...)
		headers := imagePlaygroundMultipartHeaders(t, []imagePlaygroundMultipartFile{{name: "large.png", data: data}})
		_, err := readImagePlaygroundUploads(headers)
		require.Error(t, err)
		require.Contains(t, err.Error(), "10 MB or smaller")
	})

	t.Run("rejects more than four files", func(t *testing.T) {
		files := make([]imagePlaygroundMultipartFile, imagePlaygroundMaxInputImages+1)
		for i := range files {
			files[i] = imagePlaygroundMultipartFile{name: "reference.png", data: []byte("\x89PNG\r\n\x1a\n")}
		}
		headers := imagePlaygroundMultipartHeaders(t, files)
		_, err := readImagePlaygroundUploads(headers)
		require.Error(t, err)
		require.Contains(t, err.Error(), "between 1 and 4")
	})
}

type imagePlaygroundMultipartFile struct {
	name string
	data []byte
}

func imagePlaygroundMultipartHeaders(t *testing.T, files []imagePlaygroundMultipartFile) []*multipart.FileHeader {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, file := range files {
		part, err := writer.CreateFormFile("images", file.name)
		require.NoError(t, err)
		_, err = part.Write(file.data)
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())
	request := httptest.NewRequest(http.MethodPost, "/", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	require.NoError(t, request.ParseMultipartForm(imagePlaygroundMultipartMemory))
	t.Cleanup(func() {
		if request.MultipartForm != nil {
			_ = request.MultipartForm.RemoveAll()
		}
	})
	return request.MultipartForm.File["images"]
}

func TestImagePlaygroundSubmitValidatesInput(t *testing.T) {
	c, recorder := imagePlaygroundTestContext(http.MethodPost, "/api/v1/image-playground/tasks", []byte(`{"group_id":3,"model":"gpt-image-2","prompt":"draw","n":11}`))
	h := &ImagePlaygroundHandler{playground: &imagePlaygroundApplicationStub{}}

	h.Submit(c)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "IMAGE_PLAYGROUND_INVALID_COUNT")
}

func TestImagePlaygroundGetTaskBuildsPreviewAndDownloadURLs(t *testing.T) {
	tasks := &imagePlaygroundTasksStub{task: &service.ImageTask{
		ID:         "imgtask_123",
		Status:     service.ImageTaskStatusCompleted,
		GroupID:    3,
		Platform:   service.PlatformOpenAI,
		Model:      "gpt-image-2",
		ImageCount: 1,
		CreatedAt:  1,
		ExpiresAt:  2,
	}}
	h := &ImagePlaygroundHandler{playground: &imagePlaygroundApplicationStub{}, tasks: tasks}
	c, recorder := imagePlaygroundTestContext(http.MethodGet, "/api/v1/image-playground/tasks/imgtask_123", nil)
	c.Params = gin.Params{{Key: "task_id", Value: "imgtask_123"}}

	h.GetTask(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	var envelope struct {
		Data struct {
			Images []struct {
				URL         string `json:"url"`
				DownloadURL string `json:"download_url"`
			} `json:"images"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &envelope))
	require.Len(t, envelope.Data.Images, 1)
	require.Equal(t, int64(7), tasks.gotUserID)
	require.Equal(t, "/api/v1/image-playground/tasks/imgtask_123/images/0", envelope.Data.Images[0].URL)
	require.Equal(t, "/api/v1/image-playground/tasks/imgtask_123/images/0/download", envelope.Data.Images[0].DownloadURL)
}

func TestImagePlaygroundDownloadReturnsAttachment(t *testing.T) {
	tasks := &imagePlaygroundTasksStub{download: &service.ImageTaskDownload{
		Data:        []byte("png-data"),
		ContentType: "image/png",
		Filename:    "imgtask_123-1.png",
	}}
	h := &ImagePlaygroundHandler{playground: &imagePlaygroundApplicationStub{}, tasks: tasks}
	c, recorder := imagePlaygroundTestContext(http.MethodGet, "/api/v1/image-playground/tasks/imgtask_123/images/0/download", nil)
	c.Params = gin.Params{{Key: "task_id", Value: "imgtask_123"}, {Key: "image_index", Value: "0"}}

	h.Download(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "private, no-store", recorder.Header().Get("Cache-Control"))
	require.Equal(t, "image/png", recorder.Header().Get("Content-Type"))
	_, params, err := mime.ParseMediaType(recorder.Header().Get("Content-Disposition"))
	require.NoError(t, err)
	require.Equal(t, "imgtask_123-1.png", params["filename"])
	require.Equal(t, "png-data", recorder.Body.String())
	require.Equal(t, int64(7), tasks.downloadedUserID)
}

func TestImagePlaygroundPreviewReturnsInlineImageForAuthenticatedOwner(t *testing.T) {
	tasks := &imagePlaygroundTasksStub{download: &service.ImageTaskDownload{
		Data: []byte("image-data"), ContentType: "image/png", Filename: "image.png",
	}}
	h := &ImagePlaygroundHandler{playground: &imagePlaygroundApplicationStub{}, tasks: tasks}
	c, recorder := imagePlaygroundTestContext(http.MethodGet, "/api/v1/image-playground/tasks/imgtask_123/images/0", nil)
	c.Params = gin.Params{{Key: "task_id", Value: "imgtask_123"}, {Key: "image_index", Value: "0"}}

	h.Preview(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "image/png", recorder.Header().Get("Content-Type"))
	require.Empty(t, recorder.Header().Get("Content-Disposition"))
	require.Equal(t, "private, no-store", recorder.Header().Get("Cache-Control"))
	require.Equal(t, int64(7), tasks.downloadedUserID)
}

func TestImagePlaygroundDownloadRejectsInvalidIndex(t *testing.T) {
	h := &ImagePlaygroundHandler{playground: &imagePlaygroundApplicationStub{}, tasks: &imagePlaygroundTasksStub{}}
	c, recorder := imagePlaygroundTestContext(http.MethodGet, "/api/v1/image-playground/tasks/imgtask_123/images/nope/download", nil)
	c.Params = gin.Params{{Key: "task_id", Value: "imgtask_123"}, {Key: "image_index", Value: "nope"}}

	h.Download(c)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, strings.ToLower(recorder.Body.String()), "invalid image index")
}

type imagePlaygroundTasksStub struct {
	tasks              []*service.ImageTask
	task               *service.ImageTask
	download           *service.ImageTaskDownload
	err                error
	listedUserID       int64
	gotUserID          int64
	downloadedUserID   int64
	deletedUserID      int64
	deletedTaskID      string
	deletedImageUserID int64
	deletedImageTaskID string
	deletedImageRef    string
}

func (s *imagePlaygroundTasksStub) ListForUser(_ context.Context, userID int64, _ int) ([]*service.ImageTask, error) {
	s.listedUserID = userID
	return s.tasks, s.err
}

func (s *imagePlaygroundTasksStub) ListForAdmin(context.Context, int, int) (*service.AdminImageTaskPage, error) {
	return &service.AdminImageTaskPage{}, s.err
}

func (s *imagePlaygroundTasksStub) GetForUser(_ context.Context, userID int64, _ string) (*service.ImageTask, error) {
	s.gotUserID = userID
	return s.task, s.err
}

func (s *imagePlaygroundTasksStub) DownloadForUser(_ context.Context, userID int64, _, _ string) (*service.ImageTaskDownload, error) {
	s.downloadedUserID = userID
	return s.download, s.err
}

func (s *imagePlaygroundTasksStub) DownloadForAdmin(context.Context, string, string) (*service.ImageTaskDownload, error) {
	return s.download, s.err
}

func (s *imagePlaygroundTasksStub) DeleteForUser(_ context.Context, userID int64, taskID string) error {
	s.deletedUserID = userID
	s.deletedTaskID = taskID
	return s.err
}

func (s *imagePlaygroundTasksStub) DeleteForAdmin(context.Context, string) error { return s.err }

func (s *imagePlaygroundTasksStub) DeleteImageForUser(_ context.Context, userID int64, taskID, imageRef string) (*service.ImageTask, error) {
	s.deletedImageUserID = userID
	s.deletedImageTaskID = taskID
	s.deletedImageRef = imageRef
	return s.task, s.err
}

func (s *imagePlaygroundTasksStub) DeleteImageForAdmin(context.Context, string, string) (*service.ImageTask, error) {
	return s.task, s.err
}

func TestImagePlaygroundDeleteTaskUsesAuthenticatedOwner(t *testing.T) {
	tasks := &imagePlaygroundTasksStub{}
	h := &ImagePlaygroundHandler{playground: &imagePlaygroundApplicationStub{}, tasks: tasks}
	c, _ := imagePlaygroundTestContext(http.MethodDelete, "/api/v1/image-playground/tasks/imgtask_123", nil)
	c.Params = gin.Params{{Key: "task_id", Value: "imgtask_123"}}

	h.DeleteTask(c)

	require.Equal(t, http.StatusNoContent, c.Writer.Status())
	require.Equal(t, int64(7), tasks.deletedUserID)
	require.Equal(t, "imgtask_123", tasks.deletedTaskID)
}

func TestImagePlaygroundDeleteImageUsesAuthenticatedOwner(t *testing.T) {
	tasks := &imagePlaygroundTasksStub{}
	h := &ImagePlaygroundHandler{playground: &imagePlaygroundApplicationStub{}, tasks: tasks}
	c, _ := imagePlaygroundTestContext(http.MethodDelete, "/api/v1/image-playground/tasks/imgtask_123/images/2", nil)
	c.Params = gin.Params{{Key: "task_id", Value: "imgtask_123"}, {Key: "image_index", Value: "2"}}

	h.DeleteImage(c)

	require.Equal(t, http.StatusNoContent, c.Writer.Status())
	require.Equal(t, int64(7), tasks.deletedImageUserID)
	require.Equal(t, "imgtask_123", tasks.deletedImageTaskID)
	require.Equal(t, "2", tasks.deletedImageRef)
}

func TestImagePlaygroundListTasksUsesAuthenticatedUser(t *testing.T) {
	tasks := &imagePlaygroundTasksStub{tasks: []*service.ImageTask{{
		ID: "imgtask_123", Status: service.ImageTaskStatusCompleted, Model: "gpt-image-2",
		ImageCount: 1,
	}}}
	h := &ImagePlaygroundHandler{playground: &imagePlaygroundApplicationStub{}, tasks: tasks}
	c, recorder := imagePlaygroundTestContext(http.MethodGet, "/api/v1/image-playground/tasks", nil)

	h.ListTasks(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))
	require.Equal(t, int64(7), tasks.listedUserID)
	var envelope struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &envelope))
	require.Len(t, envelope.Data, 1)
	require.Equal(t, "imgtask_123", envelope.Data[0].ID)
}

type imageTaskMemoryStoreForPlayground struct {
	mu   sync.Mutex
	task *service.ImageTaskRecord
}

func (s *imageTaskMemoryStoreForPlayground) Save(_ context.Context, task *service.ImageTaskRecord, _ time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	copy := *task
	s.task = &copy
	return nil
}

func (s *imageTaskMemoryStoreForPlayground) Get(_ context.Context, id string) (*service.ImageTaskRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.task == nil || s.task.ID != id {
		return nil, service.ErrImageTaskNotFound
	}
	copy := *s.task
	return &copy, nil
}

func (s *imageTaskMemoryStoreForPlayground) ListByUser(_ context.Context, userID int64, _ int) ([]*service.ImageTaskRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.task == nil || s.task.UserID != userID {
		return []*service.ImageTaskRecord{}, nil
	}
	copy := *s.task
	return []*service.ImageTaskRecord{&copy}, nil
}

func (s *imageTaskMemoryStoreForPlayground) ListForAdmin(_ context.Context, _ int64, _ int) ([]*service.ImageTaskRecord, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.task == nil {
		return []*service.ImageTaskRecord{}, 0, nil
	}
	copy := *s.task
	return []*service.ImageTaskRecord{&copy}, 1, nil
}

func (s *imageTaskMemoryStoreForPlayground) AdminStorageStats(context.Context) (int, int64, error) {
	return 0, 0, nil
}

func (s *imageTaskMemoryStoreForPlayground) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.task != nil && s.task.ID == id {
		s.task = nil
	}
	return nil
}

func (s *imageTaskMemoryStoreForPlayground) ScheduleCleanup(context.Context, service.ImageTaskCleanup) error {
	return nil
}

func (s *imageTaskMemoryStoreForPlayground) GetCleanup(context.Context, string) (*service.ImageTaskCleanup, error) {
	return nil, service.ErrImageTaskNotFound
}

func (s *imageTaskMemoryStoreForPlayground) ListDueCleanup(context.Context, time.Time, int) ([]service.ImageTaskCleanup, error) {
	return nil, nil
}

func (s *imageTaskMemoryStoreForPlayground) DeleteCleanup(context.Context, string) error { return nil }

func (s *imageTaskMemoryStoreForPlayground) TryLock(context.Context, string, string, time.Duration) (bool, error) {
	return true, nil
}

func (s *imageTaskMemoryStoreForPlayground) Unlock(context.Context, string, string) error { return nil }
