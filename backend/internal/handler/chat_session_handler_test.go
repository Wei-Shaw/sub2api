package handler

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/chatsession"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func newChatSessionHandlerTestClient(t *testing.T) *ent.Client {
	t.Helper()

	dbName := "file:chat_session_handler_" + strconv.FormatInt(time.Now().UnixNano(), 10) + "?mode=memory&cache=shared&_fk=1"
	db, err := sql.Open("sqlite", dbName)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	return enttest.NewClient(t, enttest.WithOptions(ent.Driver(drv)))
}

func chatSessionTestContext(method, target string, body []byte, userID int64) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(method, target, bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: userID})
	return c, recorder
}

func seedChatSessionUserAndKey(t *testing.T, client *ent.Client, email string) (int64, *ent.APIKey) {
	t.Helper()

	ctx := context.Background()
	user := client.User.Create().
		SetEmail(email).
		SetPasswordHash("hash").
		SaveX(ctx)
	key := client.APIKey.Create().
		SetUserID(user.ID).
		SetKey("sk-test-" + email).
		SetName("Key").
		SaveX(ctx)
	return user.ID, key
}

func TestChatSessionHandlerCreatesAndListsOnlyCurrentUsersActiveSessions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := newChatSessionHandlerTestClient(t)
	userID, key := seedChatSessionUserAndKey(t, client, "chat-user@example.com")
	otherUserID, otherKey := seedChatSessionUserAndKey(t, client, "chat-other@example.com")

	expired := time.Now().Add(-time.Hour)
	client.ChatSession.Create().
		SetUserID(userID).
		SetAPIKeyID(key.ID).
		SetTitle("Expired").
		SetModel("gpt-5.4").
		SetExpiresAt(expired).
		SaveX(context.Background())
	client.ChatSession.Create().
		SetUserID(otherUserID).
		SetAPIKeyID(otherKey.ID).
		SetTitle("Other").
		SetModel("gpt-5.4").
		SetExpiresAt(time.Now().Add(24 * time.Hour)).
		SaveX(context.Background())

	h := NewChatSessionHandler(client)
	c, recorder := chatSessionTestContext(http.MethodPost, "/api/v1/chat/sessions", []byte(`{"api_key_id":1,"title":"Hello world","model":"gpt-5.4"}`), userID)
	h.CreateSession(c)
	require.Equal(t, http.StatusOK, recorder.Code)

	var createResp struct {
		Code int `json:"code"`
		Data struct {
			ID        int64  `json:"id"`
			Title     string `json:"title"`
			Model     string `json:"model"`
			ExpiresAt string `json:"expires_at"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &createResp))
	require.Equal(t, 0, createResp.Code)
	require.Equal(t, "Hello world", createResp.Data.Title)
	require.Equal(t, "gpt-5.4", createResp.Data.Model)
	require.NotEmpty(t, createResp.Data.ExpiresAt)

	c, recorder = chatSessionTestContext(http.MethodGet, "/api/v1/chat/sessions", nil, userID)
	h.ListSessions(c)
	require.Equal(t, http.StatusOK, recorder.Code)

	var listResp struct {
		Code int `json:"code"`
		Data []struct {
			ID    int64  `json:"id"`
			Title string `json:"title"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &listResp))
	require.Len(t, listResp.Data, 1)
	require.Equal(t, createResp.Data.ID, listResp.Data[0].ID)
}

func formatTestID(id int64) string {
	return strconv.FormatInt(id, 10)
}

func TestChatSessionHandlerRejectsCrossUserMessagesAndSavesOwnMessages(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := newChatSessionHandlerTestClient(t)
	userID, key := seedChatSessionUserAndKey(t, client, "chat-owner@example.com")
	intruderID, _ := seedChatSessionUserAndKey(t, client, "chat-intruder@example.com")
	session := client.ChatSession.Create().
		SetUserID(userID).
		SetAPIKeyID(key.ID).
		SetTitle("Owned").
		SetModel("gpt-5.4").
		SetExpiresAt(time.Now().Add(24 * time.Hour)).
		SaveX(context.Background())

	h := NewChatSessionHandler(client)
	c, recorder := chatSessionTestContext(http.MethodGet, "/api/v1/chat/sessions/1/messages", nil, intruderID)
	c.Params = gin.Params{{Key: "id", Value: formatTestID(session.ID)}}
	h.ListMessages(c)
	require.Equal(t, http.StatusForbidden, recorder.Code)

	c, recorder = chatSessionTestContext(http.MethodPost, "/api/v1/chat/sessions/1/messages", []byte(`{"role":"user","content":"hello","status":"completed"}`), userID)
	c.Params = gin.Params{{Key: "id", Value: formatTestID(session.ID)}}
	h.CreateMessage(c)
	require.Equal(t, http.StatusOK, recorder.Code)

	c, recorder = chatSessionTestContext(http.MethodGet, "/api/v1/chat/sessions/1/messages", nil, userID)
	c.Params = gin.Params{{Key: "id", Value: formatTestID(session.ID)}}
	h.ListMessages(c)
	require.Equal(t, http.StatusOK, recorder.Code)

	var resp struct {
		Data []struct {
			SessionID int64  `json:"session_id"`
			Role      string `json:"role"`
			Content   string `json:"content"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.Len(t, resp.Data, 1)
	require.Equal(t, session.ID, resp.Data[0].SessionID)
	require.Equal(t, "user", resp.Data[0].Role)
	require.Equal(t, "hello", resp.Data[0].Content)
}

func TestChatSessionHandlerPersistsImageAttachments(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := newChatSessionHandlerTestClient(t)
	userID, key := seedChatSessionUserAndKey(t, client, "chat-attachments@example.com")
	session := client.ChatSession.Create().
		SetUserID(userID).
		SetAPIKeyID(key.ID).
		SetTitle("Attachments").
		SetModel("gpt-5.5").
		SetExpiresAt(time.Now().Add(24 * time.Hour)).
		SaveX(context.Background())

	h := NewChatSessionHandler(client)
	body := []byte(`{
		"role":"user",
		"content":"look",
		"status":"completed",
		"attachments":[{
			"type":"image",
			"image_url":"data:image/png;base64,ZmFrZQ==",
			"mime_type":"image/png",
			"name":"shot.png",
			"size":4
		}]
	}`)
	c, recorder := chatSessionTestContext(http.MethodPost, "/api/v1/chat/sessions/1/messages", body, userID)
	c.Params = gin.Params{{Key: "id", Value: formatTestID(session.ID)}}
	h.CreateMessage(c)
	require.Equal(t, http.StatusOK, recorder.Code)

	c, recorder = chatSessionTestContext(http.MethodGet, "/api/v1/chat/sessions/1/messages", nil, userID)
	c.Params = gin.Params{{Key: "id", Value: formatTestID(session.ID)}}
	h.ListMessages(c)
	require.Equal(t, http.StatusOK, recorder.Code)

	var resp struct {
		Data []struct {
			Content     string `json:"content"`
			Attachments []struct {
				Type     string `json:"type"`
				ImageURL string `json:"image_url"`
				MimeType string `json:"mime_type"`
				Name     string `json:"name"`
				Size     int64  `json:"size"`
			} `json:"attachments"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.Len(t, resp.Data, 1)
	require.Equal(t, "look", resp.Data[0].Content)
	require.Len(t, resp.Data[0].Attachments, 1)
	require.Equal(t, "image", resp.Data[0].Attachments[0].Type)
	require.Equal(t, "data:image/png;base64,ZmFrZQ==", resp.Data[0].Attachments[0].ImageURL)
	require.Equal(t, "image/png", resp.Data[0].Attachments[0].MimeType)
	require.Equal(t, "shot.png", resp.Data[0].Attachments[0].Name)
	require.EqualValues(t, 4, resp.Data[0].Attachments[0].Size)
}

func TestChatSessionHandlerUpdatesAssistantMessageAndDeletesSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := newChatSessionHandlerTestClient(t)
	userID, key := seedChatSessionUserAndKey(t, client, "chat-update@example.com")
	session := client.ChatSession.Create().
		SetUserID(userID).
		SetAPIKeyID(key.ID).
		SetTitle("Update").
		SetModel("gpt-5.4").
		SetExpiresAt(time.Now().Add(24 * time.Hour)).
		SaveX(context.Background())
	message := client.ChatMessage.Create().
		SetSessionID(session.ID).
		SetUserID(userID).
		SetRole("assistant").
		SetStatus("streaming").
		SetContent("").
		SaveX(context.Background())

	h := NewChatSessionHandler(client)
	c, recorder := chatSessionTestContext(http.MethodPatch, "/api/v1/chat/sessions/1/messages/1", []byte(`{"content":"done","status":"completed","duration_ms":1200,"actual_cost":0.0012}`), userID)
	c.Params = gin.Params{{Key: "id", Value: formatTestID(session.ID)}, {Key: "message_id", Value: formatTestID(message.ID)}}
	h.UpdateMessage(c)
	require.Equal(t, http.StatusOK, recorder.Code)

	updated := client.ChatMessage.GetX(context.Background(), message.ID)
	require.Equal(t, "completed", updated.Status)
	require.Equal(t, "done", updated.Content)
	require.NotNil(t, updated.DurationMs)
	require.Equal(t, 1200, *updated.DurationMs)
	require.NotNil(t, updated.ActualCost)

	c, recorder = chatSessionTestContext(http.MethodDelete, "/api/v1/chat/sessions/1", nil, userID)
	c.Params = gin.Params{{Key: "id", Value: formatTestID(session.ID)}}
	h.DeleteSession(c)
	require.Equal(t, http.StatusOK, recorder.Code)

	count := client.ChatSession.Query().
		Where(chatsession.IDEQ(session.ID), chatsession.DeletedAtIsNil()).
		CountX(context.Background())
	require.Equal(t, 0, count)
}
