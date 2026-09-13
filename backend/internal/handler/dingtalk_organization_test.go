package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestDingTalkApplicationStateAndSyntheticIdentity(t *testing.T) {
	require.Equal(t, "engineering", dingTalkAppFromState("random-state.engineering"))
	require.Empty(t, dingTalkAppFromState("legacy-state"))
	require.Equal(t, buildDingTalkSyntheticEmail("user"), buildDingTalkAppSyntheticEmail("default", "user"))
	a := buildDingTalkAppSyntheticEmail("a", "user")
	b := buildDingTalkAppSyntheticEmail("b", "user")
	require.NotEqual(t, a, b)
	require.NotContains(t, a, ":")
	h := &AuthHandler{}
	_, err := h.getDingTalkOAuthConfigForApp(context.Background(), "unknown")
	require.Error(t, err)
}
func TestDingTalkOrganizationAdminGuard(t *testing.T) {
	h := &DingTalkOrganizationHandler{}
	for _, fn := range []gin.HandlerFunc{h.SaveApps, h.SaveManager, h.Sync} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("PUT", "/", nil)
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 2})
		c.Set(string(middleware.ContextKeyUserRole), "user")
		fn(c)
		require.Equal(t, http.StatusForbidden, w.Code)
	}
}
func TestDingTalkOrganizationPaginationAndFailure(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "complete", true: "api-error"}[fail], func(t *testing.T) {
			memberPages := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "app-token", r.URL.Query().Get("access_token"))
				var body struct {
					DepartmentID int64 `json:"dept_id"`
					Cursor       int64 `json:"cursor"`
				}
				require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/topapi/v2/department/get":
					_, _ = w.Write([]byte(`{"errcode":0,"result":{"dept_id":1,"name":"Corp","parent_id":0}}`))
				case "/topapi/v2/department/listsub":
					if body.DepartmentID == 1 {
						_, _ = w.Write([]byte(`{"errcode":0,"result":[{"dept_id":2,"parent_id":1,"name":"Team"}]}`))
					} else {
						_, _ = w.Write([]byte(`{"errcode":0,"result":[]}`))
					}
				case "/topapi/v2/user/list":
					memberPages++
					if body.DepartmentID == 1 {
						_, _ = w.Write([]byte(`{"errcode":0,"result":{"has_more":false,"list":[]}}`))
						return
					}
					if fail {
						_, _ = w.Write([]byte(`{"errcode":60011,"errmsg":"directory access denied"}`))
						return
					}
					if body.Cursor == 0 {
						_, _ = w.Write([]byte(`{"errcode":0,"result":{"has_more":true,"next_cursor":100,"list":[{"unionid":"u1","userid":"s1","name":"A"}]}}`))
					} else {
						_, _ = w.Write([]byte(`{"errcode":0,"result":{"has_more":false,"list":[{"unionid":"u2","userid":"s2","name":"B"}]}}`))
					}
				default:
					t.Fatalf("unexpected path %s", r.URL.Path)
				}
			}))
			defer srv.Close()
			client := &DingTalkClient{cfg: dingTalkClientConfig{UserInfoURL: srv.URL + "/v1.0/contact/users/me"}, httpClient: srv.Client(), appToken: "app-token", appTokenExp: time.Now().Add(time.Hour)}
			ds, ms, err := client.ReadOrganization(context.Background())
			if fail {
				require.Error(t, err)
				require.Nil(t, ds)
				require.Nil(t, ms)
			} else {
				require.NoError(t, err)
				require.Len(t, ds, 2)
				require.Len(t, ms, 2)
				require.Equal(t, 3, memberPages)
				require.EqualValues(t, 2, ms[1].DepartmentID)
			}
		})
	}
}
