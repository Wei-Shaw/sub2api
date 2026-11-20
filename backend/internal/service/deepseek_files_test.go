//go:build unit

package service

import (
	"bytes"
	"context"
	"errors"
	"mime/multipart"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestDeepseekFilesListPagination(t *testing.T) {
	ctx := context.Background()
	cache := &testDeepseekFileCache{}
	svc := &OpenAIGatewayService{cache: cache}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i, id := range []string{"a", "b", "c", "d", "e"} {
		f := DeepseekFile{ID: id, AccountID: 11, Filename: id + ".png", Bytes: 15, CreatedAt: base.Add(time.Duration(i) * time.Second), MIMEType: "image/png", Purpose: "user_data"}
		if i == 1 {
			f.Purpose = "text_recognition"
		}
		if i == 4 {
			f.ExpiresAt = time.Now().Unix() - 1
		}
		require.NoError(t, cache.SaveDeepseekFile(ctx, 1, f))
	}
	require.NoError(t, cache.SaveDeepseekFile(ctx, 2, DeepseekFile{ID: "foreign", AccountID: 11, CreatedAt: base}))
	for _, tt := range []struct {
		query     string
		anthropic bool
		ids       []string
		more      bool
	}{
		{"", false, []string{"a", "b", "c", "d"}, false},
		{"after=a&limit=2", false, []string{"b", "c"}, true},
		{"after=d&order=desc&purpose=user_data", false, []string{"c", "a"}, false},
		{"purpose=text_recognition", false, []string{"b"}, false},
		{"", true, []string{"d", "c", "b", "a"}, false},
		{"after_id=c&limit=1", true, []string{"b"}, true},
		{"before_id=a&limit=2", true, []string{"c", "b"}, true},
		{"after_id=a", true, []string{}, false},
	} {
		page, err := svc.ListDeepseekFiles(ctx, 1, tt.query, tt.anthropic)
		require.NoError(t, err)
		require.Equal(t, tt.more, page["has_more"])
		rows := page["data"].([]map[string]any)
		ids := make([]string, 0, len(rows))
		for _, row := range rows {
			ids = append(ids, row["id"].(string))
			require.NotContains(t, row, "AccountID")
			if tt.anthropic {
				require.Equal(t, "file", row["type"])
				require.Equal(t, int64(15), row["size_bytes"])
				require.Equal(t, "image/png", row["mime_type"])
				_, err := time.Parse(time.RFC3339Nano, row["created_at"].(string))
				require.NoError(t, err)
				require.NotContains(t, page, "object")
				require.NotContains(t, row, "purpose")
			} else {
				require.Equal(t, "list", page["object"])
				require.Equal(t, "file", row["object"])
				require.Equal(t, int64(15), row["bytes"])
				require.IsType(t, int64(0), row["created_at"])
			}
		}
		require.Equal(t, tt.ids, ids)
		if len(ids) > 0 {
			require.Equal(t, ids[0], page["first_id"])
			require.Equal(t, ids[len(ids)-1], page["last_id"])
		} else {
			require.Nil(t, page["first_id"])
			require.Nil(t, page["last_id"])
		}
	}
	for _, q := range []string{"after_id=foreign", "before_id=missing", "after_id=b&before_id=c", "limit=0", "limit=1001", "purpose=user_data"} {
		_, err := svc.ListDeepseekFiles(ctx, 1, q, true)
		require.Equal(t, 400, infraerrors.Code(err), q)
	}
}

func deepseekUploadForm(t *testing.T, purposes ...string) ([]byte, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for _, purpose := range purposes {
		require.NoError(t, w.WriteField("purpose", purpose))
	}
	part, err := w.CreateFormFile("file", "report.txt")
	require.NoError(t, err)
	_, err = part.Write([]byte("report"))
	require.NoError(t, err)
	require.NoError(t, w.Close())
	return buf.Bytes(), w.FormDataContentType()
}

func TestDeepseekFileUploadPurposePolicy(t *testing.T) {
	ctx := context.Background()
	for _, anthropic := range []bool{false, true} {
		for _, enabled := range []bool{false, true} {
			svc := &OpenAIGatewayService{cache: &testDeepseekFileCache{}, cfg: &config.Config{},
				httpUpstream: &deepseekToolsHTTPUpstream{handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					_, _ = w.Write([]byte(`{"id":"file-1","object":"file","type":"file","page_count":3}`))
				})},
			}
			svc.cfg.Gateway.AllowUnbilledDeepseekDocumentParsing = enabled
			for _, purpose := range []string{"user_data", "text_recognition", "rich_layout_recognition"} {
				body, ct := deepseekUploadForm(t, purpose)
				c, _ := newDeepseekToolsTestContext("POST", "/v1/files", body)
				if anthropic {
					c.Request.Header.Set("anthropic-beta", deepseekFilesBetaHeader)
				}
				r, err := svc.PrepareDeepseekToolsRequest(ctx, c, 7, "files_create", body, ct)
				if enabled || purpose == "user_data" {
					require.NoError(t, err)
					require.Equal(t, purpose, r.upload.purpose)
					result, err := svc.ForwardDeepseekToolsRequest(ctx, c, newDeepseekToolsTestAccount("https://relay.test"), r)
					require.NoError(t, err)
					require.Equal(t, http.StatusOK, result.StatusCode)
					require.Zero(t, result.Usage, "files upload must stay unbilled, purpose=%s", purpose)
				} else {
					require.Equal(t, 400, infraerrors.Code(err))
					require.Contains(t, err.Error(), "/v1/tools/ocr")
					require.Contains(t, err.Error(), "purpose=user_data")
				}
			}
		}
	}
	body, ct := deepseekUploadForm(t, "user_data", "text_recognition")
	_, err := (&OpenAIGatewayService{}).deepseekUploadMetadata(body, ct)
	require.Equal(t, 400, infraerrors.Code(err))
}

func TestDeepseekFileUploadStoreFailureBeforeReply(t *testing.T) {
	cache := &testDeepseekFileCache{writeErr: errors.New("redis unavailable")}
	svc := &OpenAIGatewayService{cache: cache, httpUpstream: &deepseekToolsHTTPUpstream{handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.True(t, HTTPUpstreamRedirectsDisabled(r.Context()))
		_, _ = w.Write([]byte(`{"id":"file1","object":"file","created_at":12345}`))
	})}}
	body, ct := deepseekUploadForm(t, "user_data")
	c, rec := newDeepseekToolsTestContext("POST", "/v1/files", body)
	_, err := forwardDeepseekToolsTest(svc, c, newDeepseekToolsTestAccount("https://relay.test"), 7, "files_create", body, ct)
	require.Equal(t, 503, infraerrors.Code(err))
	require.Empty(t, rec.Body.String())
}

func TestDeepseekFileDeleteRetainsRecordUntilConfirmed(t *testing.T) {
	ctx := context.Background()
	cache := &testDeepseekFileCache{}
	account := newDeepseekToolsTestAccount("https://relay.test")
	require.NoError(t, cache.SaveDeepseekFile(ctx, 1, DeepseekFile{ID: "f", AccountID: account.ID}))
	for _, tc := range []struct {
		status    int
		body      string
		confirmed bool
	}{
		{404, `{"error":{"type":"not_found_error"}}`, false},
		{500, `{"error":"upstream failure"}`, false},
		{200, `{"id":"f","deleted":false}`, false},
		{200, `{"id":"other","deleted":true}`, false},
		{200, `{"id":"f","error":"denied"}`, false},
		{204, "", true},
	} {
		svc := &OpenAIGatewayService{cache: cache, httpUpstream: &deepseekToolsHTTPUpstream{handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(tc.status)
			_, _ = w.Write([]byte(tc.body))
		})}}
		c, _ := newDeepseekToolsTestContext("DELETE", "/v1/files/f", nil)
		result, err := forwardDeepseekToolsTest(svc, c, account, 1, "files_delete/f", nil, "")
		switch tc.status {
		case http.StatusNotFound:
			require.Equal(t, tc.status, infraerrors.Code(err))
		case http.StatusInternalServerError:
			var retry *UpstreamFailoverError
			require.ErrorAs(t, err, &retry)
			require.Equal(t, tc.status, retry.StatusCode)
		case http.StatusOK:
			require.Equal(t, 502, infraerrors.Code(err))
		default:
			require.NoError(t, err)
			require.Equal(t, tc.status, result.StatusCode)
		}
		_, bound := cache.files[1]["f"]
		require.Equal(t, !tc.confirmed, bound)
	}
}

func TestDeepseekToolsQueryAndAllowlistBeforeSending(t *testing.T) {
	var received string
	upstream := &deepseekToolsHTTPUpstream{handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = r.URL.String()
		_, _ = w.Write([]byte(`{"results":[]}`))
	})}
	account := newDeepseekToolsTestAccount("https://stale.test")
	account.Credentials["api_protocol"] = APIProtocolAdaptive
	account.Credentials["api_base_urls"] = map[string]any{
		APIProtocolChatCompletions: "https://relay.test/custom/v1?configured=1",
		APIProtocolAnthropic:       "https://messages.test/custom",
	}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, cache: &testDeepseekFileCache{}, httpUpstream: upstream}
	svc.cfg.Security.URLAllowlist.Enabled = true
	svc.cfg.Security.URLAllowlist.UpstreamHosts = []string{"stale.test"}
	c, _ := newDeepseekToolsTestContext("POST", "/v1/tools/search?k=x%2By&k=z", nil)
	_, err := forwardDeepseekToolsTest(svc, c, account, 7, "search", []byte("{}"), "application/json")
	require.Equal(t, 502, infraerrors.Code(err))
	require.Empty(t, received)
	svc.cfg.Security.URLAllowlist.UpstreamHosts = []string{"relay.test"}
	body, ct := deepseekUploadForm(t, "user_data")
	f, _ := newDeepseekToolsTestContext("POST", "/v1/files", body)
	f.Request.Header.Set("anthropic-beta", deepseekFilesBetaHeader)
	_, err = forwardDeepseekToolsTest(svc, f, account, 7, "files_create", body, ct)
	require.Equal(t, 502, infraerrors.Code(err))
	require.Empty(t, received)
	_, err = forwardDeepseekToolsTest(svc, c, account, 7, "search", []byte("{}"), "application/json")
	require.NoError(t, err)
	require.Equal(t, "https://relay.test/custom/v1/tools/search?configured=1&k=x%2By&k=z", received)
}

func TestDeepseekFilesListDefaultLimits(t *testing.T) {
	cache := &testDeepseekFileCache{}
	svc := &OpenAIGatewayService{cache: cache}
	for i := 0; i < 1001; i++ {
		id := strconv.Itoa(i)
		require.NoError(t, cache.SaveDeepseekFile(context.Background(), 1, DeepseekFile{ID: id, AccountID: 1}))
	}
	for _, tc := range []struct {
		anthropic bool
		limit     int
	}{{false, 1000}, {true, 20}} {
		page, err := svc.ListDeepseekFiles(context.Background(), 1, "", tc.anthropic)
		require.NoError(t, err)
		require.Len(t, page["data"], tc.limit)
		require.Equal(t, true, page["has_more"])
	}
}
