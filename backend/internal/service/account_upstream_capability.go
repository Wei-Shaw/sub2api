package service

import (
	"sort"
	"strings"
	"time"
)

// UpstreamModelSnapshotExtraKey stores the last successful upstream model list.
// This is scheduling/coverage reference only and never becomes the user shelf.
const UpstreamModelSnapshotExtraKey = "upstream_model_snapshot"

// UpstreamModelSnapshot is the last successful account capability probe.
type UpstreamModelSnapshot struct {
	Models   []string `json:"models"`
	SyncedAt string   `json:"synced_at,omitempty"`
}

func ParseUpstreamModelSnapshot(extra map[string]any) *UpstreamModelSnapshot {
	if extra == nil {
		return nil
	}
	raw, ok := extra[UpstreamModelSnapshotExtraKey]
	if !ok || raw == nil {
		return nil
	}
	switch typed := raw.(type) {
	case *UpstreamModelSnapshot:
		return cloneUpstreamModelSnapshot(typed)
	case UpstreamModelSnapshot:
		return cloneUpstreamModelSnapshot(&typed)
	case map[string]any:
		return upstreamModelSnapshotFromMap(typed)
	default:
		return nil
	}
}

func cloneUpstreamModelSnapshot(snapshot *UpstreamModelSnapshot) *UpstreamModelSnapshot {
	if snapshot == nil {
		return nil
	}
	out := &UpstreamModelSnapshot{SyncedAt: strings.TrimSpace(snapshot.SyncedAt)}
	if len(snapshot.Models) > 0 {
		out.Models = append([]string(nil), snapshot.Models...)
	}
	return out
}

func upstreamModelSnapshotFromMap(raw map[string]any) *UpstreamModelSnapshot {
	if len(raw) == 0 {
		return nil
	}
	snapshot := &UpstreamModelSnapshot{}
	if syncedAt, ok := raw["synced_at"].(string); ok {
		snapshot.SyncedAt = strings.TrimSpace(syncedAt)
	}
	switch models := raw["models"].(type) {
	case []string:
		snapshot.Models = append([]string(nil), models...)
	case []any:
		snapshot.Models = make([]string, 0, len(models))
		for _, item := range models {
			model, ok := item.(string)
			model = strings.TrimSpace(model)
			if !ok || model == "" {
				continue
			}
			snapshot.Models = append(snapshot.Models, model)
		}
	}
	snapshot.Models = uniqueSortedModelIDs(snapshot.Models)
	if len(snapshot.Models) == 0 && snapshot.SyncedAt == "" {
		return nil
	}
	return snapshot
}

func BuildUpstreamModelSnapshot(models []string, now time.Time) *UpstreamModelSnapshot {
	cleaned := uniqueSortedModelIDs(models)
	if len(cleaned) == 0 {
		return nil
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return &UpstreamModelSnapshot{
		Models:   cleaned,
		SyncedAt: now.UTC().Format(time.RFC3339),
	}
}

func ApplyUpstreamModelSnapshot(extra map[string]any, models []string, now time.Time) map[string]any {
	snapshot := BuildUpstreamModelSnapshot(models, now)
	if snapshot == nil {
		return extra
	}
	if extra == nil {
		extra = make(map[string]any, 1)
	}
	extra[UpstreamModelSnapshotExtraKey] = snapshot
	return extra
}

func uniqueSortedModelIDs(models []string) []string {
	if len(models) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(models))
	out := make([]string, 0, len(models))
	for _, model := range models {
		model = strings.TrimSpace(model)
		if model == "" {
			continue
		}
		if _, ok := seen[model]; ok {
			continue
		}
		seen[model] = struct{}{}
		out = append(out, model)
	}
	sort.Strings(out)
	return out
}

func (a *Account) UpstreamModelSnapshot() *UpstreamModelSnapshot {
	if a == nil {
		return nil
	}
	return ParseUpstreamModelSnapshot(a.Extra)
}

// HasSyncedUpstreamModel reports whether the last successful probe included
// this model. Missing or empty snapshots do not restrict scheduling, so a
// failed probe cannot shrink the candidate set to empty.
func (a *Account) HasSyncedUpstreamModel(requestedModel string) bool {
	if a == nil {
		return true
	}
	snapshot := a.UpstreamModelSnapshot()
	if snapshot == nil || len(snapshot.Models) == 0 {
		return true
	}
	requestedModel = strings.TrimSpace(strings.TrimPrefix(requestedModel, "models/"))
	if requestedModel == "" {
		return true
	}
	return snapshotCoversRequestedModel(a, snapshot.Models, requestedModel)
}

// SnapshotCoversModel reports whether the last successful snapshot includes
// this model or its mapped upstream name. Missing or empty snapshots return
// false so coverage never pretends an unsynced account has the model.
func (a *Account) SnapshotCoversModel(requestedModel string) bool {
	if a == nil {
		return false
	}
	snapshot := a.UpstreamModelSnapshot()
	if snapshot == nil || len(snapshot.Models) == 0 {
		return false
	}
	return snapshotCoversRequestedModel(a, snapshot.Models, requestedModel)
}

func snapshotCoversRequestedModel(account *Account, models []string, requestedModel string) bool {
	requestedModel = strings.TrimSpace(strings.TrimPrefix(requestedModel, "models/"))
	if requestedModel == "" || len(models) == 0 {
		return false
	}
	allowed := make(map[string]struct{}, len(models))
	for _, model := range models {
		allowed[model] = struct{}{}
	}
	if _, ok := allowed[requestedModel]; ok {
		return true
	}
	if account == nil {
		return false
	}
	mapped := strings.TrimSpace(account.GetMappedModel(requestedModel))
	if mapped != "" {
		if _, ok := allowed[mapped]; ok {
			return true
		}
	}
	return false
}

func snapshotSyncedAt(snapshot *UpstreamModelSnapshot) time.Time {
	if snapshot == nil {
		return time.Time{}
	}
	raw := strings.TrimSpace(snapshot.SyncedAt)
	if raw == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

func supportsUpstreamModelSync(account *Account) bool {
	if account == nil || account.IsShadow() {
		return false
	}
	switch {
	case account.IsAnthropic(), account.IsOpenAI(), account.IsGemini(), account.IsGrok(), account.IsCNProvider():
		return true
	case account.Platform == PlatformAntigravity:
		return true
	default:
		return false
	}
}
