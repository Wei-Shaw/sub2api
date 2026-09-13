package securityaudit

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
)

func stopRecordService(t *testing.T, s *PromptRecordService) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, s.Shutdown(ctx))
	require.Zero(t, s.QueueStats().InFlightBytes)
	require.Zero(t, s.QueueStats().PendingResponses)
}

func TestRecordReadyResponseBypassesPendingAndLateRequestStillCompletes(t *testing.T) {
	repo := &blockingPromptRecordRepository{responseUpdated: make(chan PromptRecordKey, 2)}
	s := newPromptRecordService(repo, 2, 2, 1)
	_, pending := newPromptRecordingRequestPair(Request{RequestID: "slow"})
	_, ready := newPromptRecordingRequestPair(Request{RequestID: "ready"})
	ready.recordingCorrelation.complete(PromptRecordKey{ID: 2}, true)
	s.RecordResponse(context.Background(), pending, PromptResponse{Text: "slow result"})
	s.RecordResponse(context.Background(), ready, PromptResponse{Text: "ready result"})
	select {
	case key := <-repo.responseUpdated:
		require.EqualValues(t, 2, key.ID)
	case <-time.After(time.Second):
		t.Fatal("ready response blocked by pending request")
	}
	// Exceed the removed five-second identity timeout.
	time.Sleep(5100 * time.Millisecond)
	pending.recordingCorrelation.complete(PromptRecordKey{ID: 1}, true)
	select {
	case key := <-repo.responseUpdated:
		require.EqualValues(t, 1, key.ID)
	case <-time.After(time.Second):
		t.Fatal("late request lost its response")
	}
	stopRecordService(t, s)
	require.Zero(t, s.QueueStats().ResponseFailed)
}

func TestRecordShutdownDrainsRequestsAndResponses(t *testing.T) {
	repo := &blockingPromptRecordRepository{started: make(chan struct{}, 1), release: make(chan struct{}), responseUpdated: make(chan PromptRecordKey, 1)}
	s := newPromptRecordService(repo, 1, 1, 1)
	req, ref := newPromptRecordingRequestPair(Request{RequestID: "drain", Body: []byte(`{"input":"hello"}`)})
	s.RecordPrompt(context.Background(), req)
	<-repo.started
	s.RecordResponse(context.Background(), ref, PromptResponse{Text: "answer"})
	finished := make(chan error, 1)
	go func() { finished <- (&PromptService{records: s}).Shutdown(context.Background()) }()
	select {
	case <-finished:
		t.Fatal("shutdown returned before the pending insert")
	case <-time.After(20 * time.Millisecond):
	}
	close(repo.release)
	select {
	case err := <-finished:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("shutdown did not drain")
	}
	require.Len(t, repo.responseUpdated, 1)
	require.Zero(t, s.QueueStats().InFlightBytes)
	s.RecordPrompt(context.Background(), req)
	require.EqualValues(t, 1, s.QueueStats().RequestDropped)
}

type cancellableRecordRepository struct{ blockingPromptRecordRepository }

func (r *cancellableRecordRepository) InsertPromptRecord(ctx context.Context, _ *PromptRecord) (int64, error) {
	r.started <- struct{}{}
	<-ctx.Done()
	return 0, ctx.Err()
}

func TestRecordShutdownDeadlineCancelsStorage(t *testing.T) {
	repo := &cancellableRecordRepository{blockingPromptRecordRepository: blockingPromptRecordRepository{started: make(chan struct{}, 1)}}
	s := newPromptRecordService(repo, 1, 1, 1)
	s.RecordPrompt(context.Background(), Request{Body: []byte(`{"input":"hello"}`)})
	<-repo.started
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, s.Shutdown(ctx), context.DeadlineExceeded)
	stopRecordService(t, s)
	require.EqualValues(t, 1, s.QueueStats().RequestFailed)
}

func TestRecordByteBudgetIncludesPendingResponsesAndSkipsDisabledBody(t *testing.T) {
	repo := &blockingPromptRecordRepository{}
	s := newPromptRecordService(repo, 10, 10, 1)
	s.byteLimit, s.itemByteLimit = 4096, 3000
	_, ref := newPromptRecordingRequestPair(Request{})
	s.RecordResponse(context.Background(), ref, PromptResponse{Text: strings.Repeat("a", 2000)})
	require.EqualValues(t, 1, s.QueueStats().ResponseDropped) // single-item budget
	s.RecordResponse(context.Background(), ref, PromptResponse{Text: strings.Repeat("a", 1500)})
	require.EqualValues(t, 2524, s.QueueStats().InFlightBytes)
	_, other := newPromptRecordingRequestPair(Request{})
	s.RecordResponse(context.Background(), other, PromptResponse{Text: strings.Repeat("a", 1500)})
	require.EqualValues(t, 2, s.QueueStats().ResponseDropped) // shared byte budget
	stopRecordService(t, s)                                   // unresolved references are released at shutdown

	captured := &capturedRequestRepository{records: make(chan *PromptRecord, 1)}
	s = newPromptRecordService(captured, 1, 1, 1)
	s.itemByteLimit = 2048
	s.RecordPrompt(context.Background(), Request{Body: []byte(strings.Repeat("not parsed", 1024)), recordingSkipPrompt: true, recordingSkipHeaders: true})
	stopRecordService(t, s)
	record := <-captured.records
	require.Empty(t, record.RequestBody)
	require.NotEmpty(t, record.PromptHash)
	require.Zero(t, s.QueueStats().RequestDropped)
}

func TestRecordConcurrentSubmissionAndShutdown(t *testing.T) {
	repo := &blockingPromptRecordRepository{release: make(chan struct{})}
	close(repo.release)
	s := newPromptRecordService(repo, 32, 32, 2)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 25 {
				req, ref := newPromptRecordingRequestPair(Request{Body: []byte(`{"input":"hello"}`)})
				s.RecordPrompt(context.Background(), req)
				s.RecordResponse(context.Background(), ref, PromptResponse{Text: "answer"})
			}
		}()
	}
	stopRecordService(t, s)
	wg.Wait()
	require.Zero(t, s.QueueStats().InFlightBytes)
}

func TestRecordRetentionConfigurationAndCapturedExpiry(t *testing.T) {
	settings := &promptRecordingSettingRepository{}
	manager := NewConfigManager(nil, settings, nil, prefixEncryptor{}, testTotpKeyConfig())
	require.Zero(t, manager.PromptRecordingRetentionDays())
	for _, days := range []int{-1, 3651} {
		require.Error(t, manager.SavePromptRecordingSettings(context.Background(), PromptRecordingSettingsUpdate{RetentionDays: &days}))
	}
	days := 7
	require.NoError(t, manager.SavePromptRecordingSettings(context.Background(), PromptRecordingSettingsUpdate{RetentionDays: &days}))
	require.NoError(t, manager.Reload(context.Background()))
	require.Equal(t, 7, manager.PromptRecordingRetentionDays())
	settings.writeError = errors.New("write failed")
	days = 30
	require.Error(t, manager.SavePromptRecordingSettings(context.Background(), PromptRecordingSettingsUpdate{RetentionDays: &days}))
	require.Equal(t, 7, manager.PromptRecordingRetentionDays())
	repo := &capturedRequestRepository{records: make(chan *PromptRecord, 1)}
	s := &PromptService{config: manager, records: newPromptRecordService(repo, 1, 1, 1)}
	s.RecordPrompt(context.Background(), Request{Body: []byte(`{"input":"retained"}`)})
	stopRecordService(t, s.records)
	record := <-repo.records
	require.NotNil(t, record.ExpiresAt)
	require.Equal(t, record.CreatedAt.AddDate(0, 0, 7), *record.ExpiresAt)
}

func TestRecordAgentPresetPathAndBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name, identity, text string
		removed              bool
	}{
		{"windows", "You are Codex", "# AGENTS.md instructions for D:\\sample\\project\n\n<INSTRUCTIONS>private preset</INSTRUCTIONS>\nkeep", true},
		{"unix crlf", "You are Codex", "# AGENTS.md instructions for /tmp/project\r\n<INSTRUCTIONS>private preset</INSTRUCTIONS>\r\nkeep", true},
		{"no identity", "assistant", "# AGENTS.md instructions for /tmp/project\n<INSTRUCTIONS>private preset</INSTRUCTIONS>", false},
		{"quoted", "You are Codex", "Explain this: # AGENTS.md instructions for /tmp/project\n<INSTRUCTIONS>private preset</INSTRUCTIONS>", false},
		{"unclosed", "You are Codex", "# AGENTS.md instructions for /tmp/project\n<INSTRUCTIONS>private preset", false},
		{"wrong heading", "You are Codex", "# AGENTS.md instructions for \n<INSTRUCTIONS>private preset</INSTRUCTIONS>", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, err := json.Marshal(map[string]any{"instructions": tc.identity, "input": []any{map[string]any{"role": "user", "content": tc.text}}})
			require.NoError(t, err)
			filtered := string(filterPresetRequestBody("responses", body, allPromptRecordFilters))
			require.Equal(t, !tc.removed, strings.Contains(filtered, "private preset"), filtered)
			if tc.removed {
				require.Contains(t, filtered, "keep")
			}
		})
	}
}

func TestRecordResponseCaptureRecoversTextAndMarksIncomplete(t *testing.T) {
	body := []byte(`{"choices":[{"message":{"content":"` + strings.Repeat("a", 3*1024*1024) + `"}}]}`)
	result := ExtractResponseText(body[:2*1024*1024], true)
	require.True(t, result.Recognized)
	require.True(t, result.Truncated)
	require.Len(t, result.Text, PromptResponseTextLimit)
	for _, suffix := range []string{`hello\`, `hello\u12`, "hello你"[:7]} {
		result := ExtractResponseText([]byte(`{"output_text":"`+suffix), true)
		require.True(t, result.Truncated)
		require.True(t, utf8.ValidString(result.Text))
		require.True(t, strings.HasPrefix(result.Text, "hello"))
	}
	sse := "data: {\"choices\":[{\"delta\":{\"content\":\"first\"}}]}\n\ndata: " + string(body) + "\n\ndata: [DONE]\n"
	result = ExtractResponseText([]byte(sse), false)
	require.True(t, result.Recognized)
	// A large valid event is read without the old one-MiB scanner failure.
	require.Equal(t, "first", result.Text)
	longDelta := "data: {\"choices\":[{\"delta\":{\"content\":\"" + strings.Repeat("b", 1100*1024) + "\"}}]}\n\ndata: [DONE]\n"
	result = ExtractResponseText([]byte(longDelta), false)
	require.True(t, result.Truncated)
	require.Len(t, result.Text, PromptResponseTextLimit)
	require.Equal(t, 1100*1024, result.Length)
	broken := ExtractResponseText([]byte("data: {\"output_text\":\"first\"}\n\ndata: {broken\n"), false)
	require.True(t, broken.Truncated)
	require.Equal(t, "first", broken.Text)
}

func BenchmarkRecordRejectedBeforeClone(b *testing.B) {
	s := newPromptRecordService(&blockingPromptRecordRepository{}, 1, 1, 1)
	s.once.Do(func() {})
	s.requestSlots <- struct{}{}
	s.requestSlots <- struct{}{}
	req := Request{Body: []byte(strings.Repeat("a", 1024*1024))}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		s.RecordPrompt(context.Background(), req)
	}
}

type expiryBatchRepository struct {
	blockingPromptRecordRepository
	calls int
	fail  bool
}

func (r *expiryBatchRepository) DeleteExpiredPromptRecords(context.Context, int) (int64, error) {
	r.calls++
	if r.fail {
		return 0, errors.New("cleanup failed")
	}
	return 1000, nil
}

func TestRecordCleanupBoundsEachPassAndReportsBacklog(t *testing.T) {
	repo := &expiryBatchRepository{}
	s := newPromptRecordService(repo, 1, 1, 1)
	s.cleanupExpired()
	require.Equal(t, 10, repo.calls)
	require.EqualValues(t, 10000, s.QueueStats().ExpiredDeleted)
	require.True(t, s.QueueStats().CleanupBacklog)
	repo.fail = true
	s.cleanupExpired()
	require.EqualValues(t, 1, s.QueueStats().CleanupFailed)
	stopRecordService(t, s)
}
