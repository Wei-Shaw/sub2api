package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	agentConversationStatusIdle     = "idle"
	agentConversationStatusRunning  = "running"
	agentConversationStatusStopping = "stopping"
	agentConversationStatusStopped  = "stopped"
	agentConversationStatusError    = "error"
	agentMaxConversations           = 30
	agentMaxProcessEvents           = 120
)

type aiAgentPersistentStore struct {
	ActiveID      string                          `json:"active_id,omitempty"`
	Conversations []aiAgentPersistentConversation `json:"conversations"`
}

type aiAgentPersistentConversation struct {
	ID           string                  `json:"id"`
	Title        string                  `json:"title"`
	Status       string                  `json:"status"`
	ErrorMessage string                  `json:"error_message,omitempty"`
	CreatedAt    time.Time               `json:"created_at"`
	UpdatedAt    time.Time               `json:"updated_at"`
	Model        []agentModelMessage     `json:"model"`
	Public       []AIAgentMessage        `json:"public"`
	Events       []AIAgentProcessEvent   `json:"events"`
	Pending      *AIAgentPendingAction   `json:"pending,omitempty"`
	PendingQueue []*AIAgentPendingAction `json:"pending_queue,omitempty"`
	Rollbacks    []AIAgentRollback       `json:"rollbacks"`
	Observed     map[string]bool         `json:"observed"`
}

func agentConversationSettingKey(userID int64) string {
	return fmt.Sprintf("ai_agent_conversations_%d_encrypted", userID)
}

func newAIAgentSession(title string) *aiAgentSession {
	now := time.Now()
	return &aiAgentSession{
		id:           uuid.NewString(),
		title:        title,
		status:       agentConversationStatusIdle,
		createdAt:    now,
		updatedAt:    now,
		lastActivity: now,
		observed:     make(map[string]bool),
	}
}

func (s *AIAgentService) ensureConversationsLoaded(ctx context.Context, userID int64) error {
	s.sessionsMu.Lock()
	recoveredRollbacks := false
	defer func() {
		s.sessionsMu.Unlock()
		if recoveredRollbacks {
			go s.persistConversationsDetached(userID)
		}
	}()
	if s.loaded[userID] {
		return nil
	}
	conversations := make(map[string]*aiAgentSession)
	encrypted, err := s.settings.GetValue(ctx, agentConversationSettingKey(userID))
	if err != nil && !errors.Is(err, ErrSettingNotFound) {
		return err
	}
	if encrypted != "" {
		plaintext, decryptErr := s.encryptor.Decrypt(encrypted)
		if decryptErr != nil {
			return errors.New("stored Agent conversations cannot be decrypted")
		}
		var stored aiAgentPersistentStore
		if json.Unmarshal([]byte(plaintext), &stored) != nil {
			return errors.New("stored Agent conversations are invalid")
		}
		for _, item := range stored.Conversations {
			recoverInterruptedAgentRollbacks(item.Rollbacks)
			status := item.Status
			errorMessage := item.ErrorMessage
			if status == agentConversationStatusRunning || status == agentConversationStatusStopping {
				status = agentConversationStatusStopped
				errorMessage = "The server restarted before this response completed."
				for index := range item.Public {
					item.Public[index].Streaming = false
				}
				if item.Pending != nil && item.Pending.Plan != nil && item.Pending.Plan.Status == "running" {
					item.Pending.Plan.Status = "stopped"
					item.Pending.Plan.UpdatedAt = time.Now()
				}
			}
			observed := item.Observed
			if observed == nil {
				observed = make(map[string]bool)
			}
			conversation := &aiAgentSession{
				id: item.ID, title: item.Title, status: status, errorMessage: errorMessage,
				createdAt: item.CreatedAt, updatedAt: item.UpdatedAt, lastActivity: time.Now(),
				model: item.Model, public: item.Public, events: item.Events, pending: item.Pending,
				pendingQueue: item.PendingQueue, rollbacks: item.Rollbacks, observed: observed,
			}
			recoveredRollbacks = s.recoverMissingAgentPlanRollbacks(conversation) || recoveredRollbacks
			if conversation.id != "" {
				conversations[conversation.id] = conversation
			}
		}
		s.active[userID] = stored.ActiveID
	}
	s.sessions[userID] = conversations
	s.loaded[userID] = true
	return nil
}

func (s *AIAgentService) conversation(ctx context.Context, userID int64, conversationID string, create bool) (*aiAgentSession, error) {
	if err := s.ensureConversationsLoaded(ctx, userID); err != nil {
		return nil, err
	}
	s.sessionsMu.Lock()
	defer s.sessionsMu.Unlock()
	if conversationID == "" {
		conversationID = s.active[userID]
	}
	if conversation := s.sessions[userID][conversationID]; conversation != nil {
		s.active[userID] = conversationID
		return conversation, nil
	}
	if !create {
		return nil, errors.New("agent conversation not found")
	}
	conversation := newAIAgentSession("New conversation")
	s.sessions[userID][conversation.id] = conversation
	s.active[userID] = conversation.id
	return conversation, nil
}

func (s *AIAgentService) CreateConversation(ctx context.Context, userID int64) (AIAgentSessionSnapshot, error) {
	if _, err := s.requireEnabled(ctx); err != nil {
		return AIAgentSessionSnapshot{}, err
	}
	conversation, err := s.conversation(ctx, userID, uuid.NewString(), true)
	if err != nil {
		return AIAgentSessionSnapshot{}, err
	}
	if err := s.persistConversations(ctx, userID); err != nil {
		return AIAgentSessionSnapshot{}, err
	}
	return snapshotAIAgentSession(conversation), nil
}

func (s *AIAgentService) Conversations(ctx context.Context, userID int64) (AIAgentConversationList, error) {
	if err := s.ensureConversationsLoaded(ctx, userID); err != nil {
		return AIAgentConversationList{}, err
	}
	s.sessionsMu.Lock()
	activeID := s.active[userID]
	items := make([]*aiAgentSession, 0, len(s.sessions[userID]))
	for _, conversation := range s.sessions[userID] {
		items = append(items, conversation)
	}
	s.sessionsMu.Unlock()
	summaries := make([]AIAgentConversationSummary, 0, len(items))
	for _, conversation := range items {
		conversation.mu.Lock()
		summaries = append(summaries, conversationSummary(conversation))
		conversation.mu.Unlock()
	}
	sort.Slice(summaries, func(i, j int) bool { return summaries[i].UpdatedAt.After(summaries[j].UpdatedAt) })
	return AIAgentConversationList{ActiveID: activeID, Conversations: summaries}, nil
}

func (s *AIAgentService) Session(ctx context.Context, userID int64, conversationID string) (AIAgentSessionSnapshot, error) {
	conversation, err := s.conversation(ctx, userID, conversationID, true)
	if err != nil {
		return AIAgentSessionSnapshot{}, err
	}
	return snapshotAIAgentSession(conversation), nil
}

func (s *AIAgentService) DeleteConversation(ctx context.Context, userID int64, conversationID string) error {
	if err := s.ensureConversationsLoaded(ctx, userID); err != nil {
		return err
	}
	s.Stop(userID, conversationID)
	s.sessionsMu.Lock()
	if s.sessions[userID][conversationID] == nil {
		s.sessionsMu.Unlock()
		return errors.New("agent conversation not found")
	}
	delete(s.sessions[userID], conversationID)
	if s.active[userID] == conversationID {
		s.active[userID] = ""
		for id := range s.sessions[userID] {
			s.active[userID] = id
			break
		}
	}
	s.sessionsMu.Unlock()
	return s.persistConversations(ctx, userID)
}

func conversationSummary(conversation *aiAgentSession) AIAgentConversationSummary {
	return AIAgentConversationSummary{ID: conversation.id, Title: redactAgentTextSecrets(conversation.title), Status: conversation.status, CreatedAt: conversation.createdAt, UpdatedAt: conversation.updatedAt}
}

func snapshotAIAgentSession(conversation *aiAgentSession) AIAgentSessionSnapshot {
	conversation.mu.Lock()
	defer conversation.mu.Unlock()
	messages := append([]AIAgentMessage(nil), conversation.public...)
	for index := range messages {
		messages[index].Content = redactAgentTextSecrets(messages[index].Content)
	}
	return AIAgentSessionSnapshot{
		Conversation: conversationSummary(conversation),
		Messages:     messages,
		Events:       append([]AIAgentProcessEvent(nil), conversation.events...),
		Pending:      publicAgentPending(conversation.pending),
		Rollbacks:    publicAgentRollbacks(conversation.rollbacks),
		Error:        conversation.errorMessage,
	}
}

func (s *AIAgentService) persistConversations(ctx context.Context, userID int64) error {
	s.persistMu.Lock()
	defer s.persistMu.Unlock()
	s.sessionsMu.Lock()
	activeID := s.active[userID]
	items := make([]*aiAgentSession, 0, len(s.sessions[userID]))
	for _, conversation := range s.sessions[userID] {
		items = append(items, conversation)
	}
	s.sessionsMu.Unlock()
	type persistedItem struct {
		conversation *aiAgentSession
		updatedAt    time.Time
	}
	ordered := make([]persistedItem, 0, len(items))
	for _, conversation := range items {
		conversation.mu.Lock()
		ordered = append(ordered, persistedItem{conversation: conversation, updatedAt: conversation.updatedAt})
		conversation.mu.Unlock()
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].updatedAt.After(ordered[j].updatedAt) })
	if len(ordered) > agentMaxConversations {
		ordered = ordered[:agentMaxConversations]
	}
	items = items[:0]
	for _, item := range ordered {
		items = append(items, item.conversation)
	}
	stored := aiAgentPersistentStore{ActiveID: activeID, Conversations: make([]aiAgentPersistentConversation, 0, len(items))}
	for _, conversation := range items {
		conversation.mu.Lock()
		stored.Conversations = append(stored.Conversations, aiAgentPersistentConversation{
			ID: conversation.id, Title: conversation.title, Status: conversation.status, ErrorMessage: conversation.errorMessage,
			CreatedAt: conversation.createdAt, UpdatedAt: conversation.updatedAt, Model: conversation.model,
			Public: conversation.public, Events: conversation.events, Pending: clonePending(conversation.pending),
			PendingQueue: clonePendingQueue(conversation.pendingQueue), Rollbacks: conversation.rollbacks, Observed: conversation.observed,
		})
		conversation.mu.Unlock()
	}
	encoded, err := json.Marshal(stored)
	if err != nil {
		return err
	}
	encrypted, err := s.encryptor.Encrypt(string(encoded))
	if err != nil {
		return err
	}
	return s.settings.Set(ctx, agentConversationSettingKey(userID), encrypted)
}

func setConversationTitle(conversation *aiAgentSession, prompt string) {
	if conversation.title != "New conversation" {
		return
	}
	title := redactAgentTextSecrets(strings.Join(strings.Fields(prompt), " "))
	if len([]rune(title)) > 42 {
		title = string([]rune(title)[:42]) + "..."
	}
	if title != "" {
		conversation.title = title
	}
}

func appendAgentEvent(conversation *aiAgentSession, mode, kind, summary string, detail any, metadata ...map[string]any) {
	if mode == "off" {
		return
	}
	event := AIAgentProcessEvent{ID: uuid.NewString(), RunID: conversation.activeRunID, Kind: kind, Summary: summary, CreatedAt: time.Now()}
	if len(metadata) > 0 {
		event.Metadata, _ = redactAgentValue(metadata[0]).(map[string]any)
	}
	if mode == "full" && detail != nil {
		encoded, _ := json.Marshal(redactAgentValue(detail))
		event.Detail = string(encoded)
		if len(event.Detail) > 6000 {
			event.Detail = event.Detail[:6000] + "...(truncated)"
		}
	}
	conversation.events = append(conversation.events, event)
	if len(conversation.events) > agentMaxProcessEvents {
		conversation.events = append([]AIAgentProcessEvent(nil), conversation.events[len(conversation.events)-agentMaxProcessEvents:]...)
	}
}
