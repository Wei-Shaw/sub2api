package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/pkg/packguard"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
)

type packGuardUpstreamFixture struct {
	calls             int
	redirectsDisabled bool
}

func (u *packGuardUpstreamFixture) Do(r *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	u.calls++
	u.redirectsDisabled = HTTPUpstreamRedirectsDisabled(r.Context())
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"data":[]}`))}, nil
}
func (u *packGuardUpstreamFixture) DoWithTLS(r *http.Request, p string, id int64, n int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(r, p, id, n)
}

func TestPackGuardSharedTransportCoversBothMethodsAndPreservesOtherProviders(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	mock.ExpectQuery("SELECT b.account_id").WillReturnRows(sqlmock.NewRows([]string{"account_id"}).AddRow(12))
	next := &packGuardUpstreamFixture{}
	u := mustPackGuardHTTP(t, next, packguard.NewStore(db))
	for _, tls := range []bool{false, true} {
		expectFrozenPack(mock)
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://ark.ap-southeast.bytepluses.com/api/v3/chat/completions", strings.NewReader(`{"model":"dola-seed-2-1-turbo-260628","messages":[{"role":"user","content":"Hi"}],"max_tokens":16}`))
		req.Header.Set("Authorization", "Bearer test-only-key")
		var resp *http.Response
		if tls {
			resp, err = u.DoWithTLS(req, "", 12, 1, nil)
		} else {
			resp, err = u.Do(req, "", 12, 1)
		}
		if err != nil || resp.StatusCode != 402 || next.calls != 0 {
			t.Fatalf("unfunded request escaped: %v %v %d", resp, err, next.calls)
		}
		_ = resp.Body.Close()
	}
	mock.ExpectQuery(regexp.QuoteMeta(packLookupPrefix)).WillReturnRows(sqlmock.NewRows([]string{"id", "hash", "base", "frozen", "matches", "binding"}))
	req, _ := http.NewRequest(http.MethodGet, "https://api.openai.com/v1/models", nil)
	resp, err := u.Do(req, "", 12, 1)
	if err != nil || next.calls != 1 || next.redirectsDisabled {
		t.Fatal("unrelated provider behavior changed")
	}
	_ = resp.Body.Close()
	key := "test-only-key"
	mock.ExpectQuery(regexp.QuoteMeta(packLookupPrefix)).WillReturnRows(sqlmock.NewRows([]string{"id", "hash", "base", "frozen", "matches", "binding"}).AddRow("scope", packguard.HashCredential(key), "https://ark.ap-southeast.bytepluses.com/api/v3", false, true, "scope"))
	req, _ = http.NewRequest(http.MethodGet, "https://ark.ap-southeast.bytepluses.com/api/v3/models", nil)
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err = u.Do(req, "", 12, 1)
	if err != nil || next.calls != 2 || !next.redirectsDisabled {
		t.Fatal("guarded requests must not follow redirects")
	}
	_ = resp.Body.Close()
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func mustPackGuardHTTP(t *testing.T, upstream HTTPUpstream, ledger packguard.Ledger) HTTPUpstream {
	t.Helper()
	wrapped, err := NewBytePlusPackHTTP(upstream, ledger)
	if err != nil {
		t.Fatal(err)
	}
	return wrapped
}

func TestPackGuardDenialSurvivesChatResponsesAndPassthroughWithoutFailover(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	mock.ExpectQuery("SELECT b.account_id").WillReturnRows(sqlmock.NewRows([]string{"account_id"}).AddRow(12))
	next := &packGuardUpstreamFixture{}
	u := mustPackGuardHTTP(t, next, packguard.NewStore(db))
	for _, status := range []int{402, 503} {
		if status == 402 {
			expectFrozenPack(mock)
		} else {
			mock.ExpectQuery(regexp.QuoteMeta(packLookupPrefix)).WillReturnError(errors.New("isolated storage failure"))
		}
		req, _ := http.NewRequest(http.MethodPost, "https://ark.ap-southeast.bytepluses.com/api/v3/chat/completions", strings.NewReader(`{"model":"dola-seed-2-1-turbo-260628","messages":[{"role":"user","content":"Hi"}],"max_tokens":16}`))
		req.Header.Set("Authorization", "Bearer test-only-key")
		resp, e := u.Do(req, "", 12, 1)
		if e != nil || resp.StatusCode != status || !isBytePlusPackDenial(resp) {
			t.Fatalf("missing local denial: %v", e)
		}
		body, e := io.ReadAll(resp.Body)
		if e != nil {
			t.Fatal(e)
		}
		_ = resp.Body.Close()
		for _, path := range []string{"chat", "responses", "passthrough", "passthrough-failover"} {
			t.Run(fmt.Sprintf("%s-%d", path, status), func(t *testing.T) {
				copyResp := *resp
				copyResp.Body = io.NopCloser(strings.NewReader(string(body)))
				recorder := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(recorder)
				c.Request = req
				svc := &OpenAIGatewayService{}
				if fo := svc.failoverOpenAIUpstreamHTTPError(context.Background(), c, nil, &copyResp, body, "", ""); fo != nil {
					t.Fatal("local denial must never fail over")
				}
				var result error
				switch path {
				case "chat":
					_, result = svc.handleChatCompletionsErrorResponse(&copyResp, c, nil)
				case "responses":
					_, result = svc.handleErrorResponse(context.Background(), &copyResp, c, nil, nil)
				case "passthrough":
					result = svc.handleErrorResponsePassthrough(context.Background(), &copyResp, c, nil, nil, body)
				case "passthrough-failover":
					result = svc.handleFailoverErrorResponsePassthrough(context.Background(), &copyResp, c, nil, nil, body)
				}
				if result == nil || recorder.Code != status || recorder.Body.String() != string(body) || !IsResponseCommitted(c) {
					t.Fatalf("denial was hidden: %d %s", recorder.Code, recorder.Body.String())
				}
			})
		}
	}
	if next.calls != 0 {
		t.Fatal("denial dispatched upstream")
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

const packLookupPrefix = "SELECT s.id,$2,s.api_base,s.frozen"

func expectFrozenPack(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(regexp.QuoteMeta(packLookupPrefix)).WillReturnRows(sqlmock.NewRows([]string{"id", "hash", "base", "frozen", "matches", "binding"}).AddRow("scope", packguard.HashCredential("test-only-key"), "https://ark.ap-southeast.bytepluses.com/api/v3", false, true, "scope"))
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT frozen,enforced,clock_timestamp()")).WillReturnRows(sqlmock.NewRows([]string{"frozen", "enforced", "now"}).AddRow(true, true, time.Now()))
	mock.ExpectRollback()
}
func TestPackGuardMissingStoreFailsStartup(t *testing.T) {
	if u, err := NewBytePlusPackHTTP(&packGuardUpstreamFixture{}, nil); err == nil || u != nil {
		t.Fatal("missing store returned an unguarded transport")
	}
}
