package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/require"
)

type emailBroadcastRepoStub struct {
	created *EmailBroadcast
	listErr error
	listOut *EmailBroadcastListResult
	getErr  error
	getOut  *EmailBroadcast
	patches []EmailBroadcastStatusUpdate
}

func (s *emailBroadcastRepoStub) Create(_ context.Context, b *EmailBroadcast) error {
	cp := *b
	cp.ID = 1
	s.created = &cp
	b.ID = 1
	return nil
}

func (s *emailBroadcastRepoStub) GetByID(_ context.Context, _ int64) (*EmailBroadcast, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	if s.getOut != nil {
		return s.getOut, nil
	}
	if s.created == nil {
		return nil, ErrEmailBroadcastNotFound
	}
	cp := *s.created
	return &cp, nil
}

func (s *emailBroadcastRepoStub) UpdateStatus(_ context.Context, _ int64, patch EmailBroadcastStatusUpdate) error {
	s.patches = append(s.patches, patch)
	return nil
}

func (s *emailBroadcastRepoStub) List(_ context.Context, _ EmailBroadcastListParams) (*EmailBroadcastListResult, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	if s.listOut == nil {
		return &EmailBroadcastListResult{Items: []EmailBroadcast{}}, nil
	}
	return s.listOut, nil
}

func (s *emailBroadcastRepoStub) Delete(_ context.Context, _ int64) error {
	s.created = nil
	s.getOut = nil
	return nil
}

// settingRepoStubNoSMTP returns no SMTP host so GetSMTPConfig fails with ErrEmailNotConfigured.
type settingRepoStubNoSMTP struct{}

func (settingRepoStubNoSMTP) Get(context.Context, string) (*Setting, error) {
	return nil, errors.New("not configured")
}
func (settingRepoStubNoSMTP) GetValue(context.Context, string) (string, error) {
	return "", errors.New("not configured")
}
func (settingRepoStubNoSMTP) Set(context.Context, string, string) error { return nil }
func (settingRepoStubNoSMTP) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, k := range keys {
		out[k] = ""
	}
	return out, nil
}
func (settingRepoStubNoSMTP) SetMultiple(context.Context, map[string]string) error { return nil }
func (settingRepoStubNoSMTP) GetAll(context.Context) (map[string]string, error) {
	return map[string]string{}, nil
}
func (settingRepoStubNoSMTP) Delete(context.Context, string) error { return nil }

func newTestEmailBroadcastService() (*EmailBroadcastService, *emailBroadcastRepoStub) {
	repo := &emailBroadcastRepoStub{}
	emailSvc := NewEmailService(settingRepoStubNoSMTP{}, nil)
	svc := NewEmailBroadcastService(repo, nil, emailSvc, settingRepoStubNoSMTP{})
	return svc, repo
}

func TestEmailBroadcastSend_RejectsEmptySubject(t *testing.T) {
	svc, _ := newTestEmailBroadcastService()
	_, err := svc.Send(context.Background(), EmailBroadcastSendInput{
		Subject:        "  ",
		Body:           "hello",
		RecipientsMode: EmailBroadcastRecipientsModeSelected,
		RecipientUserIDs: []int64{1},
	})
	require.ErrorIs(t, err, ErrEmailBroadcastSubjectRequired)
}

func TestEmailBroadcastSend_RejectsEmptyBody(t *testing.T) {
	svc, _ := newTestEmailBroadcastService()
	_, err := svc.Send(context.Background(), EmailBroadcastSendInput{
		Subject:          "subject",
		Body:             "",
		RecipientsMode:   EmailBroadcastRecipientsModeSelected,
		RecipientUserIDs: []int64{1},
	})
	require.ErrorIs(t, err, ErrEmailBroadcastBodyRequired)
}

func TestEmailBroadcastSend_RejectsSubjectTooLong(t *testing.T) {
	svc, _ := newTestEmailBroadcastService()
	_, err := svc.Send(context.Background(), EmailBroadcastSendInput{
		Subject:          strings.Repeat("a", domain.EmailBroadcastSubjectMaxLen+1),
		Body:             "hello",
		RecipientsMode:   EmailBroadcastRecipientsModeSelected,
		RecipientUserIDs: []int64{1},
	})
	require.ErrorIs(t, err, ErrEmailBroadcastSubjectTooLong)
}

func TestEmailBroadcastSend_RejectsBodyTooLong(t *testing.T) {
	svc, _ := newTestEmailBroadcastService()
	_, err := svc.Send(context.Background(), EmailBroadcastSendInput{
		Subject:          "subject",
		Body:             strings.Repeat("a", domain.EmailBroadcastBodyMaxLen+1),
		RecipientsMode:   EmailBroadcastRecipientsModeSelected,
		RecipientUserIDs: []int64{1},
	})
	require.ErrorIs(t, err, ErrEmailBroadcastBodyTooLong)
}

func TestEmailBroadcastSend_RejectsBadFormat(t *testing.T) {
	svc, _ := newTestEmailBroadcastService()
	_, err := svc.Send(context.Background(), EmailBroadcastSendInput{
		Subject:          "s",
		Body:             "b",
		BodyFormat:       "markdown",
		RecipientsMode:   EmailBroadcastRecipientsModeSelected,
		RecipientUserIDs: []int64{1},
	})
	require.ErrorIs(t, err, ErrEmailBroadcastInvalidBodyFormat)
}

func TestEmailBroadcastSend_RejectsBadMode(t *testing.T) {
	svc, _ := newTestEmailBroadcastService()
	_, err := svc.Send(context.Background(), EmailBroadcastSendInput{
		Subject:        "s",
		Body:           "b",
		RecipientsMode: "everyone",
	})
	require.ErrorIs(t, err, ErrEmailBroadcastInvalidMode)
}

func TestEmailBroadcastSend_RejectsSelectedWithoutRecipients(t *testing.T) {
	svc, _ := newTestEmailBroadcastService()
	_, err := svc.Send(context.Background(), EmailBroadcastSendInput{
		Subject:        "s",
		Body:           "b",
		RecipientsMode: EmailBroadcastRecipientsModeSelected,
	})
	require.ErrorIs(t, err, ErrEmailBroadcastNoRecipients)
}

func TestEmailBroadcastSend_RejectsTooManyRecipients(t *testing.T) {
	svc, _ := newTestEmailBroadcastService()
	ids := make([]int64, 0, domain.EmailBroadcastMaxSelectedRecipients+1)
	for i := 0; i < domain.EmailBroadcastMaxSelectedRecipients+1; i++ {
		ids = append(ids, int64(i+1))
	}
	_, err := svc.Send(context.Background(), EmailBroadcastSendInput{
		Subject:          "s",
		Body:             "b",
		RecipientsMode:   EmailBroadcastRecipientsModeSelected,
		RecipientUserIDs: ids,
	})
	require.ErrorIs(t, err, ErrEmailBroadcastTooManyRecipients)
}

func TestEmailBroadcastSend_RejectsWhenSMTPNotConfigured(t *testing.T) {
	svc, _ := newTestEmailBroadcastService()
	_, err := svc.Send(context.Background(), EmailBroadcastSendInput{
		Subject:          "s",
		Body:             "b",
		RecipientsMode:   EmailBroadcastRecipientsModeSelected,
		RecipientUserIDs: []int64{1, 2, 3},
	})
	require.ErrorIs(t, err, ErrEmailBroadcastEmailNotConfigured)
}

func TestDedupePositiveInt64s(t *testing.T) {
	in := []int64{0, 1, 1, 2, -3, 2, 4}
	out := dedupePositiveInt64s(in)
	require.Equal(t, []int64{1, 2, 4}, out)
}

func TestComposeHTMLBody_PlainTextEscapesAndPreservesNewlines(t *testing.T) {
	svc, _ := newTestEmailBroadcastService()
	got := svc.composeHTMLBody("subj", "<script>alert(1)</script>\nline2", EmailBroadcastBodyFormatText, "MySite")
	require.Contains(t, got, "&lt;script&gt;")
	require.Contains(t, got, "<br>")
	require.Contains(t, got, "MySite")
	require.Contains(t, got, "subj")
}

func TestComposeHTMLBody_PlainTextSplitsParagraphsOnBlankLine(t *testing.T) {
	svc, _ := newTestEmailBroadcastService()
	got := svc.composeHTMLBody("s", "first paragraph\n\nsecond paragraph", EmailBroadcastBodyFormatText, "Site")
	require.Contains(t, got, "first paragraph")
	require.Contains(t, got, "second paragraph")
	// Each paragraph wrapped in its own <p>
	require.GreaterOrEqual(t, strings.Count(got, "<p"), 2)
}

func TestComposeHTMLBody_HTMLStripsDangerousTags(t *testing.T) {
	svc, _ := newTestEmailBroadcastService()
	got := svc.composeHTMLBody("s", `<p>safe</p><script>alert(1)</script><a href="https://example.com">link</a>`, EmailBroadcastBodyFormatHTML, "Site")
	require.Contains(t, got, "<p>safe</p>")
	require.NotContains(t, got, "<script>")
	require.Contains(t, got, "example.com")
}

func TestComposeHTMLBody_IncludesSubjectAndSiteName(t *testing.T) {
	svc, _ := newTestEmailBroadcastService()
	got := svc.composeHTMLBody("Welcome onboard!", "<p>hi</p>", EmailBroadcastBodyFormatHTML, "Acme Lab")
	require.Contains(t, got, "Welcome onboard!")
	require.Contains(t, got, "Acme Lab")
}

func TestPreviewHTML_UsesSiteNameWhenAvailable(t *testing.T) {
	svc, _ := newTestEmailBroadcastService()
	got := svc.PreviewHTML(context.Background(), "subj", "hi", EmailBroadcastBodyFormatText)
	require.Contains(t, got, "subj")
	// Stub settingRepo returns empty values → fallback to "Sub2API".
	require.Contains(t, got, "Sub2API")
}

// settingRepoStubWithValues lets a test pre-seed setting values.
type settingRepoStubWithValues struct {
	values map[string]string
}

func (s *settingRepoStubWithValues) Get(_ context.Context, key string) (*Setting, error) {
	if v, ok := s.values[key]; ok {
		return &Setting{Key: key, Value: v}, nil
	}
	return nil, errors.New("not set")
}
func (s *settingRepoStubWithValues) GetValue(_ context.Context, key string) (string, error) {
	if v, ok := s.values[key]; ok {
		return v, nil
	}
	return "", errors.New("not set")
}
func (s *settingRepoStubWithValues) Set(context.Context, string, string) error { return nil }
func (s *settingRepoStubWithValues) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, k := range keys {
		out[k] = s.values[k]
	}
	return out, nil
}
func (s *settingRepoStubWithValues) SetMultiple(context.Context, map[string]string) error { return nil }
func (s *settingRepoStubWithValues) GetAll(context.Context) (map[string]string, error) {
	return s.values, nil
}
func (s *settingRepoStubWithValues) Delete(context.Context, string) error { return nil }

func TestResolveSenderName_PrefersSMTPFromName(t *testing.T) {
	repo := &settingRepoStubWithValues{
		values: map[string]string{
			SettingKeySMTPFromName: "TurboAPI",
			SettingKeySiteName:     "Sub2API instance",
		},
	}
	emailSvc := NewEmailService(repo, nil)
	svc := NewEmailBroadcastService(&emailBroadcastRepoStub{}, nil, emailSvc, repo)
	got := svc.resolveSenderName(context.Background())
	require.Equal(t, "TurboAPI", got)
}

func TestResolveSenderName_FallsBackToSiteName(t *testing.T) {
	repo := &settingRepoStubWithValues{
		values: map[string]string{
			SettingKeySMTPFromName: "  ",
			SettingKeySiteName:     "My Site",
		},
	}
	emailSvc := NewEmailService(repo, nil)
	svc := NewEmailBroadcastService(&emailBroadcastRepoStub{}, nil, emailSvc, repo)
	got := svc.resolveSenderName(context.Background())
	require.Equal(t, "My Site", got)
}

func TestDelete_RejectsPending(t *testing.T) {
	svc, repo := newTestEmailBroadcastService()
	repo.getOut = &EmailBroadcast{ID: 7, Status: EmailBroadcastStatusPending}
	err := svc.Delete(context.Background(), 7)
	require.ErrorIs(t, err, ErrEmailBroadcastDeleteInFlight)
}

func TestDelete_RejectsSending(t *testing.T) {
	svc, repo := newTestEmailBroadcastService()
	repo.getOut = &EmailBroadcast{ID: 8, Status: EmailBroadcastStatusSending}
	err := svc.Delete(context.Background(), 8)
	require.ErrorIs(t, err, ErrEmailBroadcastDeleteInFlight)
}

func TestDelete_AllowsCompleted(t *testing.T) {
	svc, repo := newTestEmailBroadcastService()
	repo.getOut = &EmailBroadcast{ID: 9, Status: EmailBroadcastStatusCompleted}
	err := svc.Delete(context.Background(), 9)
	require.NoError(t, err)
}

func TestDelete_AllowsFailed(t *testing.T) {
	svc, repo := newTestEmailBroadcastService()
	repo.getOut = &EmailBroadcast{ID: 10, Status: EmailBroadcastStatusFailed}
	err := svc.Delete(context.Background(), 10)
	require.NoError(t, err)
}

func TestDelete_RejectsZeroID(t *testing.T) {
	svc, _ := newTestEmailBroadcastService()
	err := svc.Delete(context.Background(), 0)
	require.ErrorIs(t, err, ErrEmailBroadcastNotFound)
}

func TestPreviewHTML_RendersSMTPFromNameInTemplate(t *testing.T) {
	repo := &settingRepoStubWithValues{
		values: map[string]string{
			SettingKeySMTPFromName: "TurboAPI",
		},
	}
	emailSvc := NewEmailService(repo, nil)
	svc := NewEmailBroadcastService(&emailBroadcastRepoStub{}, nil, emailSvc, repo)
	got := svc.PreviewHTML(context.Background(), "subj", "hi", EmailBroadcastBodyFormatText)
	// Sender name should appear in header banner + footer (both zh + en strings).
	require.GreaterOrEqual(t, strings.Count(got, "TurboAPI"), 3)
}
