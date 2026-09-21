//go:build unit

package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func qualityEvents(text string, complete bool) string {
	content, _ := json.Marshal(TestEvent{Type: "content", Text: text})
	result := "data: " + string(content) + "\n\n"
	if complete {
		result += "data: {\"type\":\"test_complete\",\"success\":true}\n\n"
	}
	return result
}
func TestQualityPromptReachesActualAccountRequest(t *testing.T) {
	for _, prompt := range []string{QualityGoosePrompt, QualityCandyPrompt, ""} {
		c, _ := newTestContext()
		upstream := &queuedHTTPUpstream{responses: []*http.Response{newJSONResponse(200, "data: {\"type\":\"response.completed\"}\n\n")}}
		svc := &AccountTestService{httpUpstream: upstream}
		account := &Account{ID: 987, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1, Credentials: map[string]any{"access_token": "mock-token"}}
		require.NoError(t, svc.testOpenAIAccountConnection(c, account, "gpt-6-astra", prompt, ""))
		require.Len(t, upstream.requests, 1)
		body, err := io.ReadAll(upstream.requests[0].Body)
		require.NoError(t, err)
		expected := prompt
		if expected == "" {
			expected = "hi"
		}
		require.Equal(t, expected, gjson.GetBytes(body, "input.0.content.0.text").String())
		require.Empty(t, gjson.GetBytes(body, "tools").Array())
	}
}
func TestQualityRequiresCompletedOutputAndFinalAnswer(t *testing.T) {
	r := parseQualityOutput(qualityEvents("答案：21颗", false), "candy")
	require.Equal(t, "error", r.Status)
	r = parseQualityOutput(qualityEvents("答案：21颗", true)+"data: {\"type\":\"error\",\"error\":\"upstream failed\"}\n\n", "candy")
	require.Equal(t, "error", r.Status)
	r = parseQualityOutput(qualityEvents("可能有人给答案21，但我的最终答案：29颗", true), "candy")
	require.Equal(t, "mismatch", r.Status)
	require.Equal(t, 29, *r.Answer)
	r = parseQualityOutput(qualityEvents("9圆形与12五角星。\n答案：21颗", true), "candy")
	require.Equal(t, "pass", r.Status)
	r = parseQualityOutput(qualityEvents("无法确定", true), "candy")
	require.Equal(t, "review", r.Status)
	r = parseQualityOutput(qualityEvents("", true), "candy")
	require.Equal(t, "error", r.Status)
}
func TestQualitySVGDoesNotClaimVisualCorrectness(t *testing.T) {
	valid := "<svg xmlns=\"http://www.w3.org/2000/svg\"><circle r=\"5\"/><circle r=\"6\"/><path d=\"M0 0L1 1\"/></svg>"
	r := parseQualityOutput(qualityEvents(valid, true), "goose_svg")
	require.Equal(t, "review", r.Status)
	for _, text := range []string{"<svg><circle/></svg>", "<svg><circle></svg>", "<svg><circle/>", "hello"} {
		require.Equal(t, "mismatch", parseQualityOutput(qualityEvents(text, true), "goose_svg").Status)
	}
}
func TestQualityCandyReferenceByExhaustiveCounts(t *testing.T) {
	bad := map[[2]int]bool{}
	largest := 0
	for ar := 0; ar <= 7; ar++ {
		for pr := 0; pr <= 9; pr++ {
			for wr := 0; wr <= 8; wr++ {
				for as := 0; as <= 7; as++ {
					for ps := 0; ps <= 6; ps++ {
						for ws := 0; ws <= 4; ws++ {
							if ar*ps == 0 && pr*as == 0 {
								r, s := ar+pr+wr, as+ps+ws
								bad[[2]int{r, s}] = true
								if r+s > largest {
									largest = r + s
								}
							}
						}
					}
				}
			}
		}
	}
	minDraw := 42
	for r := 0; r <= 24; r++ {
		for s := 0; s <= 17; s++ {
			if !bad[[2]int{r, s}] && r+s < minDraw {
				minDraw = r + s
			}
		}
	}
	require.Equal(t, 21, minDraw)
	require.False(t, bad[[2]int{9, 12}])
	require.Equal(t, 29, largest+1)
}
func TestQualitySchedulerPauseAndLostLeaseDoNotCallUpstream(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	svc := NewQualityCheckService(db, nil)
	defer svc.Stop()
	mock.ExpectQuery("SELECT enabled,model").WillReturnRows(sqlmock.NewRows([]string{"enabled", "model"}).AddRow(false, "gpt-6-astra"))
	n, err := svc.Dispatch(context.Background(), false, 0)
	require.NoError(t, err)
	require.Zero(t, n)
	mock.ExpectQuery("SELECT enabled,model").WillReturnRows(sqlmock.NewRows([]string{"enabled", "model"}).AddRow(true, "gpt-6-astra"))
	mock.ExpectExec("INSERT INTO account_quality_schedule").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("UPDATE account_quality_runs").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT a.id FROM accounts").WithArgs(int64(3), true).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(3))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE account_quality_schedule").WithArgs(int64(3), sqlmock.AnyArg(), true).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()
	n, err = svc.Dispatch(context.Background(), true, 3)
	require.NoError(t, err)
	require.Zero(t, n)
	require.Empty(t, svc.slots)
	require.NoError(t, mock.ExpectationsWereMet())
}
func TestQualityRecorderCancelsOversizedOutput(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := qualityRecorder{header: make(http.Header), cancel: cancel}
	_, err := w.Write([]byte(strings.Repeat("x", (2<<20)+1)))
	require.Error(t, err)
	require.Error(t, ctx.Err())
	require.True(t, w.overflow)
}
