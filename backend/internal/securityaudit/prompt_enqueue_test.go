package securityaudit

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
)

func TestEnqueuerLatestTurnOnlyPayloadAndMetadata(t *testing.T) {
	const conversation = `{"messages":[{"role":"system","content":"system instruction"},{"role":"user","content":"older user input"},{"role":"assistant","content":"previous output"},{"role":"user","content":"最新 user input"},{"role":"tool","content":"later tool output"}]}`
	const noUser = `{"messages":[{"role":"system","content":"system instruction"},{"role":"assistant","content":"previous output"},{"role":"tool","content":"tool output"}]}`
	for _, tt := range []struct {
		name       string
		latestOnly bool
		body       string
		segments   []string
	}{
		{"latest turn", true, conversation, []string{"最新 user input", "previous output"}},
		{"full transcript", false, conversation, []string{"最新 user input", "system instruction", "older user input", "previous output", "later tool output"}},
		{"no user falls back to full transcript", true, noUser, []string{"tool output", "system instruction", "previous output"}},
		{"no user full transcript", false, noUser, []string{"tool output", "system instruction", "previous output"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := asyncConfig()
			cfg.BlockingLatestTurnOnly = tt.latestOnly
			trace := []string{}
			repo := &fakeJobRepository{trace: &trace, createJob: &Job{ID: 41}}
			payload := &fakePayloadStore{trace: &trace}
			req := asyncRequest()
			req.Body = []byte(tt.body)
			req.Model = "test-model"
			require.NoError(t, NewEnqueuer(&fakeConfigStore{cfg: cfg, active: true}, repo, payload).Enqueue(context.Background(), req))

			wantText := strings.Join(tt.segments, "\n\n")
			wantPayload := tt.segments[0] + promptAuditPrioritySeparator + strings.Join(tt.segments[1:], "\n\n")
			require.Equal(t, wantPayload, payload.values[41])
			require.Equal(t, []string{"create_staging", "payload_set", "publish_queued"}, trace)
			require.Equal(t, DefaultPayloadTTL, payload.setTTL)
			snapshot := repo.createdSnapshot
			require.Empty(t, snapshot.ScanText)
			require.Equal(t, fmt.Sprintf("%x", sha256.Sum256([]byte(wantText))), snapshot.PromptHash)
			require.Equal(t, utf8.RuneCountInString(wantText), snapshot.PromptLength)
			require.Equal(t, len(tt.segments), snapshot.MessageCount)
			require.Equal(t, BuildPromptPreview(wantText, DefaultPromptPreviewMaxRunes), snapshot.RedactedPreview)
			require.Equal(t, wantText, snapshot.FullPrompt)
			require.Equal(t, wantText, FullPromptFromScanText(payload.values[41]))
			require.Equal(t, req.RequestID, snapshot.RequestID)
			require.Equal(t, req.Protocol, snapshot.Protocol)
			require.Equal(t, req.Model, snapshot.Model)
			require.Equal(t, "http", snapshot.Stage)
		})
	}
}
