//go:build unit

package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// TestWriteModelsListCapabilityFields 验证 /v1/models 列表项携带能力字段，
// 客户端（opencode 插件等）据此生成 variants / 附件能力，不做本地推断。
func TestWriteModelsListCapabilityFields(t *testing.T) {
	gin.SetMode(gin.TestMode)

	type listItem struct {
		ID                      string `json:"id"`
		SupportsReasoningEffort bool   `json:"supportsReasoningEffort"`
		ReasoningEfforts        []struct {
			Value   string `json:"value"`
			Label   string `json:"label"`
			Default bool   `json:"default"`
		} `json:"reasoningEfforts"`
		MaxReasoningEffort  string `json:"maxReasoningEffort"`
		SupportsAttachments bool   `json:"supportsAttachments"`
	}
	type modelsResponse struct {
		Object string     `json:"object"`
		Data   []listItem `json:"data"`
	}

	cases := []struct {
		name       string
		modelIDs   []string
		group      *service.Group
		wantID     string
		wantEffort bool
		wantValues []string
		wantAttach bool
	}{
		{
			name:       "deepseek 带四档且禁附件",
			modelIDs:   []string{"deepseek-v4-flash"},
			wantID:     "deepseek-v4-flash",
			wantEffort: true,
			wantValues: []string{"low", "medium", "high", "max"},
			wantAttach: false,
		},
		{
			name:       "长尾模型不支持 effort 且禁附件",
			modelIDs:   []string{"longcat-2.0-free"},
			wantID:     "longcat-2.0-free",
			wantEffort: false,
			wantAttach: false,
		},
		{
			name:       "group high 上限截掉 max",
			modelIDs:   []string{"deepseek-v4-flash"},
			group:      &service.Group{MaxReasoningEffort: "high"},
			wantID:     "deepseek-v4-flash",
			wantEffort: true,
			wantValues: []string{"low", "medium", "high"},
			wantAttach: false,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.GET("/v1/models", func(c *gin.Context) {
				writeModelsList(c, service.PlatformComposite, tt.modelIDs, tt.group)
			})
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
			router.ServeHTTP(w, req)

			require.Equal(t, http.StatusOK, w.Code)
			var resp modelsResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Len(t, resp.Data, 1)

			item := resp.Data[0]
			require.Equal(t, tt.wantID, item.ID)
			require.Equal(t, tt.wantEffort, item.SupportsReasoningEffort)
			require.Equal(t, tt.wantAttach, item.SupportsAttachments)
			if tt.wantEffort {
				values := make([]string, 0, len(item.ReasoningEfforts))
				for _, opt := range item.ReasoningEfforts {
					values = append(values, opt.Value)
				}
				require.Equal(t, tt.wantValues, values)
				require.Equal(t, tt.wantValues[len(tt.wantValues)-1], item.MaxReasoningEffort)
			} else {
				require.Empty(t, item.ReasoningEfforts)
				require.Empty(t, item.MaxReasoningEffort)
			}
		})
	}
}
