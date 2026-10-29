package conversationlog

import (
	"encoding/json"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
)

func TestPreviewTextUnicodeAndPartialCodePoints(t *testing.T) {
	for _, text := range []string{"short", strings.Repeat("a", 1000), strings.Repeat("中文", 400), strings.Repeat("😀", 400), strings.Repeat("a中😀", 300)} {
		prefix := text
		if len(prefix) > 960 {
			prefix = prefix[:960]
		}
		got := previewText(prefix)
		want := []rune(text)
		if len(want) > 240 {
			want = want[:240]
		}
		if got != string(want) || !utf8.ValidString(got) {
			t.Fatalf("invalid Unicode preview: runes=%d bytes=%d", utf8.RuneCountInString(got), len(got))
		}
	}
	if got := previewText("hello\xe4\xb8"); got != "hello" {
		t.Fatalf("partial suffix was not removed: %q", got)
	}
}

func TestListReadsBinaryPreviewsAndOmitsBodies(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	r := fixture(false, 1<<20)
	r.db = db
	mock.ExpectQuery(regexp.QuoteMeta("SELECT count(*) FROM conversation_logs WHERE TRUE")).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	listColumns := strings.NewReplacer("request_body", "''::text", "response_body", "''::text", "tool_calls", "'[]'::jsonb",
		"prompt,answer", "substring(convert_to(prompt,'UTF8') from 1 for 960),substring(convert_to(answer,'UTF8') from 1 for 960)").Replace(columns)
	query := "SELECT " + listColumns + " FROM conversation_logs WHERE TRUE ORDER BY created_at DESC,id DESC LIMIT $1 OFFSET $2"
	prompt := strings.Repeat("中😀a", 200)
	answer := strings.Repeat("文🌏b", 200)
	rows := sqlmock.NewRows(strings.Split(strings.Join(strings.Fields(columns), ""), ",")).AddRow(
		1, "unicode-regression", time.Now(), 1, 1, "fixture", "fixture", "/v1/responses", "test", false,
		200, "completed", 10, []byte(prompt[:960]), []byte(answer[:960]), "", "", []byte("[]"), false, false, 1000, 1000, true)
	mock.ExpectQuery(regexp.QuoteMeta(query)).WithArgs(20, 0).WillReturnRows(rows)
	engine := gin.New()
	r.RegisterAdmin(engine.Group("/api/v1/admin"))
	out := httptest.NewRecorder()
	engine.ServeHTTP(out, httptest.NewRequest("GET", "/api/v1/admin/conversation-logs", nil))
	if out.Code != 200 {
		t.Fatalf("list failed: %d %s", out.Code, out.Body.String())
	}
	var result struct {
		Data struct {
			Items []Record `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Data.Items) != 1 {
		t.Fatal("expected one preview")
	}
	item := result.Data.Items[0]
	if item.Prompt != string([]rune(prompt)[:240]) || item.Answer != string([]rune(answer)[:240]) {
		t.Fatal("list did not preserve the first 240 Unicode characters")
	}
	if item.RequestBody != "" || item.ResponseBody != "" || string(item.ToolCalls) != "[]" {
		t.Fatal("list leaked full captured content")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
