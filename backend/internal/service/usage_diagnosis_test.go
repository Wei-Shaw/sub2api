package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type stubUsageRepoForDiagnosis struct {
	UsageLogRepository
	log *UsageLog
}

func (s *stubUsageRepoForDiagnosis) GetByID(ctx context.Context, id int64) (*UsageLog, error) {
	if s.log == nil || s.log.ID != id {
		return nil, ErrUsageLogNotFound
	}
	cp := *s.log
	return &cp, nil
}

func TestGetUsageDiagnosis_MergesDumpAndMasksAlreadyStored(t *testing.T) {
	dir := t.TempDir()
	store := NewUsageRequestDumpStore(dir)
	require.NoError(t, store.Put(&UsageRequestDump{
		RequestID:  "rid-1",
		ReqBody:    `{"model":"x","messages":[{"role":"user","content":"hi"}]}`,
		ResBody:    `{"choices":[{"message":{"content":"ok"}}]}`,
		ReqHeaders: map[string]string{"Authorization": "Bearer z", "X-Test": "1"},
		StatusCode: 200,
		Path:       "/v1/chat/completions",
	}))

	ip := "1.2.3.4"
	inbound := "/v1/chat/completions"
	log := &UsageLog{
		ID:              42,
		RequestID:       "rid-1",
		Model:           "x",
		RequestedModel:  "x",
		CreatedAt:       time.Now().UTC(),
		IPAddress:       &ip,
		InboundEndpoint: &inbound,
		InputTokens:     1,
		OutputTokens:    2,
	}
	svc := &UsageService{
		usageRepo: &stubUsageRepoForDiagnosis{log: log},
		dumpStore: store,
	}
	detail, err := svc.GetUsageDiagnosis(context.Background(), 42)
	require.NoError(t, err)
	require.True(t, detail.HasDetail)
	require.Equal(t, 200, detail.StatusCode)
	require.Equal(t, "***", detail.ReqHeaders["Authorization"])
	require.Equal(t, "1", detail.ReqHeaders["X-Test"])
	require.Contains(t, detail.ReqBody, "messages")
	require.Equal(t, "/v1/chat/completions", detail.Path)
}
