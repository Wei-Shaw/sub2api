package dto

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type PlaygroundChatSession struct {
	ID            int64          `json:"id"`
	UserID        int64          `json:"user_id"`
	Title         string         `json:"title"`
	Model         string         `json:"model"`
	APIKeyID      *int64         `json:"api_key_id"`
	SystemPrompt  string         `json:"system_prompt"`
	UseContext    bool           `json:"use_context"`
	Metadata      map[string]any `json:"metadata"`
	LastMessageAt time.Time      `json:"last_message_at"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
}

type PlaygroundChatMessage struct {
	ID          int64            `json:"id"`
	SessionID   int64            `json:"session_id"`
	UserID      int64            `json:"user_id"`
	APIKeyID    *int64           `json:"api_key_id"`
	Role        string           `json:"role"`
	Model       string           `json:"model"`
	Content     string           `json:"content"`
	ContentJSON map[string]any   `json:"content_json"`
	Images      []map[string]any `json:"images"`
	Usage       map[string]any   `json:"usage"`
	Status      string           `json:"status"`
	Error       string           `json:"error"`
	DurationMS  *int             `json:"duration_ms"`
	Metadata    map[string]any   `json:"metadata"`
	CreatedAt   time.Time        `json:"created_at"`
	UpdatedAt   time.Time        `json:"updated_at"`
}

type PlaygroundImageTask struct {
	ID              int64            `json:"id"`
	UserID          int64            `json:"user_id"`
	APIKeyID        *int64           `json:"api_key_id"`
	Model           string           `json:"model"`
	Prompt          string           `json:"prompt"`
	Quality         string           `json:"quality"`
	Size            string           `json:"size"`
	N               int              `json:"n"`
	Endpoint        string           `json:"endpoint"`
	Status          string           `json:"status"`
	Request         map[string]any   `json:"request"`
	ReferenceImages []map[string]any `json:"reference_images"`
	ResultImages    []map[string]any `json:"result_images"`
	Response        map[string]any   `json:"response"`
	Error           string           `json:"error"`
	Usage           map[string]any   `json:"usage"`
	Cost            float64          `json:"cost"`
	DurationMS      *int             `json:"duration_ms"`
	Metadata        map[string]any   `json:"metadata"`
	CreatedAt       time.Time        `json:"created_at"`
	UpdatedAt       time.Time        `json:"updated_at"`
}

func PlaygroundChatSessionFromService(v *service.PlaygroundChatSession) *PlaygroundChatSession {
	if v == nil {
		return nil
	}
	return &PlaygroundChatSession{ID: v.ID, UserID: v.UserID, Title: v.Title, Model: v.Model, APIKeyID: v.APIKeyID, SystemPrompt: v.SystemPrompt, UseContext: v.UseContext, Metadata: v.Metadata, LastMessageAt: v.LastMessageAt, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt}
}

func PlaygroundChatSessionsFromService(items []service.PlaygroundChatSession) []PlaygroundChatSession {
	out := make([]PlaygroundChatSession, 0, len(items))
	for i := range items {
		out = append(out, *PlaygroundChatSessionFromService(&items[i]))
	}
	return out
}

func PlaygroundChatMessageFromService(v *service.PlaygroundChatMessage) *PlaygroundChatMessage {
	if v == nil {
		return nil
	}
	return &PlaygroundChatMessage{ID: v.ID, SessionID: v.SessionID, UserID: v.UserID, APIKeyID: v.APIKeyID, Role: v.Role, Model: v.Model, Content: v.Content, ContentJSON: v.ContentJSON, Images: v.Images, Usage: v.Usage, Status: v.Status, Error: v.Error, DurationMS: v.DurationMS, Metadata: v.Metadata, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt}
}

func PlaygroundChatMessagesFromService(items []service.PlaygroundChatMessage) []PlaygroundChatMessage {
	out := make([]PlaygroundChatMessage, 0, len(items))
	for i := range items {
		out = append(out, *PlaygroundChatMessageFromService(&items[i]))
	}
	return out
}

func PlaygroundImageTaskFromService(v *service.PlaygroundImageTask) *PlaygroundImageTask {
	if v == nil {
		return nil
	}
	return &PlaygroundImageTask{ID: v.ID, UserID: v.UserID, APIKeyID: v.APIKeyID, Model: v.Model, Prompt: v.Prompt, Quality: v.Quality, Size: v.Size, N: v.N, Endpoint: v.Endpoint, Status: v.Status, Request: v.Request, ReferenceImages: v.ReferenceImages, ResultImages: v.ResultImages, Response: v.Response, Error: v.Error, Usage: v.Usage, Cost: v.Cost, DurationMS: v.DurationMS, Metadata: v.Metadata, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt}
}

func PlaygroundImageTasksFromService(items []service.PlaygroundImageTask) []PlaygroundImageTask {
	out := make([]PlaygroundImageTask, 0, len(items))
	for i := range items {
		out = append(out, *PlaygroundImageTaskFromService(&items[i]))
	}
	return out
}
