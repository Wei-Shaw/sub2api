package repository

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/plugin-service/internal/model"
)

var ErrNotFound = errors.New("record not found")

type HistoryRepository struct {
	mu      sync.RWMutex
	records map[string]model.HistoryRecord
	now     func() time.Time
	newID   func() string
}

func NewHistoryRepository() *HistoryRepository {
	return &HistoryRepository{
		records: make(map[string]model.HistoryRecord),
		now:     time.Now,
		newID:   randomID,
	}
}

func (r *HistoryRepository) Create(_ context.Context, principal model.CurrentPrincipal, prompt string, request map[string]any) (*model.HistoryRecord, error) {
	now := r.now().UTC()
	record := model.HistoryRecord{
		ID:             r.newID(),
		ConversationID: conversationIDFromRequest(request),
		UserID:         principal.UserID,
		UserEmail:      principal.Email,
		PluginKey:      principal.Plugin,
		Prompt:         strings.TrimSpace(prompt),
		Status:         model.HistoryStatusPending,
		Request:        copyMap(request),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if record.Request == nil {
		record.Request = map[string]any{}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.records[record.ID] = record
	return &record, nil
}

func (r *HistoryRepository) Update(_ context.Context, record *model.HistoryRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.records[record.ID]; !ok {
		return ErrNotFound
	}
	record.UpdatedAt = r.now().UTC()
	r.records[record.ID] = *record
	return nil
}

func (r *HistoryRepository) Get(_ context.Context, id string) (*model.HistoryRecord, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	record, ok := r.records[id]
	if !ok {
		return nil, false, nil
	}
	return &record, true, nil
}

func (r *HistoryRepository) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.records[id]; !ok {
		return ErrNotFound
	}
	delete(r.records, id)
	return nil
}

func (r *HistoryRepository) ListAll(_ context.Context, query model.HistoryQuery) ([]model.HistoryRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	records := make([]model.HistoryRecord, 0, len(r.records))
	for _, record := range r.records {
		records = append(records, record)
	}
	return paginate(sortRecords(records), query), nil
}

func (r *HistoryRepository) ListByUser(_ context.Context, userID int64, query model.HistoryQuery) ([]model.HistoryRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	records := make([]model.HistoryRecord, 0)
	for _, record := range r.records {
		if record.UserID == userID {
			records = append(records, record)
		}
	}
	return paginate(sortRecords(records), query), nil
}

func (r *HistoryRepository) ListConversations(_ context.Context, userID *int64, query model.CursorQuery) ([]model.ConversationSummary, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	latest := make(map[string]model.HistoryRecord)
	for _, record := range r.records {
		if record.ConversationID == "" || (userID != nil && record.UserID != *userID) {
			continue
		}
		current, ok := latest[record.ConversationID]
		if !ok || record.UpdatedAt.After(current.UpdatedAt) || (record.UpdatedAt.Equal(current.UpdatedAt) && record.ID > current.ID) {
			latest[record.ConversationID] = record
		}
	}
	items := make([]model.ConversationSummary, 0, len(latest))
	for id, record := range latest {
		if !query.BeforeTime.IsZero() && (record.UpdatedAt.After(query.BeforeTime) || (record.UpdatedAt.Equal(query.BeforeTime) && id >= query.BeforeID)) {
			continue
		}
		preview := record.Prompt
		if record.ErrorMessage != "" {
			preview = record.ErrorMessage
		}
		items = append(items, model.ConversationSummary{ID: id, Title: record.Prompt, Preview: preview, Status: record.Status, UpdatedAt: record.UpdatedAt})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].UpdatedAt.Equal(items[j].UpdatedAt) {
			return items[i].ID > items[j].ID
		}
		return items[i].UpdatedAt.After(items[j].UpdatedAt)
	})
	limit := normalizeCursorLimit(query.Limit)
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func (r *HistoryRepository) ListConversationMessages(_ context.Context, userID *int64, conversationID string, query model.CursorQuery) ([]model.HistoryRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := make([]model.HistoryRecord, 0)
	for _, record := range r.records {
		if record.ConversationID != conversationID || (userID != nil && record.UserID != *userID) {
			continue
		}
		if !query.BeforeTime.IsZero() && (record.CreatedAt.After(query.BeforeTime) || (record.CreatedAt.Equal(query.BeforeTime) && record.ID >= query.BeforeID)) {
			continue
		}
		items = append(items, record)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].ID > items[j].ID
		}
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})
	limit := normalizeCursorLimit(query.Limit)
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func normalizeCursorLimit(limit int) int {
	if limit <= 0 || limit > 100 {
		return 20
	}
	return limit
}

func sortRecords(records []model.HistoryRecord) []model.HistoryRecord {
	sort.Slice(records, func(i, j int) bool {
		return records[i].CreatedAt.After(records[j].CreatedAt)
	})
	return records
}

func paginate(records []model.HistoryRecord, query model.HistoryQuery) []model.HistoryRecord {
	if query.Page <= 0 {
		query.Page = 1
	}
	if query.PageSize <= 0 || query.PageSize > 100 {
		query.PageSize = 20
	}
	start := (query.Page - 1) * query.PageSize
	if start >= len(records) {
		return []model.HistoryRecord{}
	}
	end := start + query.PageSize
	if end > len(records) {
		end = len(records)
	}
	return records[start:end]
}

func randomID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return time.Now().UTC().Format("20060102150405.000000000")
	}
	return hex.EncodeToString(buf)
}

func copyMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func conversationIDFromRequest(request map[string]any) string {
	if request == nil {
		return ""
	}
	value, ok := request["conversation_id"].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(value)
}
