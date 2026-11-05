package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/traffic"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type userTrafficRepo struct {
	userUsageRepoCapture
	record *service.UsageLog
}

func (r *userTrafficRepo) GetByID(context.Context, int64) (*service.UsageLog, error) {
	return r.record, nil
}

func TestUserUsageTrafficListAndDetailRedaction(t *testing.T) {
	zero, secret := int64(0), int64(987654321)
	observation := &traffic.Usage{
		Upstream:   &traffic.Link{Request: &traffic.Measurement{OriginalBytes: &secret, ActualBytes: &secret}},
		Downstream: &traffic.Link{Request: &traffic.Measurement{ActualBytes: &zero}},
	}
	for _, endpoint := range []string{"/usage", "/usage/7"} {
		for _, unknown := range []bool{false, true} {
			t.Run(endpoint+map[bool]string{false: "/measured", true: "/unknown"}[unknown], func(t *testing.T) {
				value := observation
				if unknown {
					value = nil
				}
				record := service.UsageLog{ID: 7, UserID: 42, Traffic: value}
				repo := &userTrafficRepo{record: &record, userUsageRepoCapture: userUsageRepoCapture{listRows: []service.UsageLog{record}}}
				h := NewUsageHandler(service.NewUsageService(repo, nil, nil, nil), nil, nil, nil)
				gin.SetMode(gin.TestMode)
				router := gin.New()
				router.Use(func(c *gin.Context) {
					c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 42})
					c.Next()
				})
				router.GET("/usage", h.List)
				router.GET("/usage/:id", h.GetByID)
				recorder := httptest.NewRecorder()
				router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, endpoint, nil))
				require.Equal(t, http.StatusOK, recorder.Code)
				require.NotContains(t, recorder.Body.String(), `"upstream"`)
				require.NotContains(t, recorder.Body.String(), "987654321")
				var result struct {
					Data json.RawMessage `json:"data"`
				}
				require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &result))
				var row map[string]json.RawMessage
				if endpoint == "/usage" {
					var list struct {
						Items []map[string]json.RawMessage `json:"items"`
					}
					require.NoError(t, json.Unmarshal(result.Data, &list))
					require.Len(t, list.Items, 1)
					row = list.Items[0]
				} else {
					require.NoError(t, json.Unmarshal(result.Data, &row))
				}
				if unknown {
					require.JSONEq(t, "null", string(row["traffic"]))
				} else {
					require.JSONEq(t, `{"downstream":{"request":{"original_bytes":null,"actual_bytes":0,"compressed":false},"response":null}}`, string(row["traffic"]))
				}
			})
		}
	}
}
