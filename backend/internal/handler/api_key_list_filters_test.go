//go:build unit

package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestParseAPIKeyListStatusFilter(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		statusQuery    string
		wantStatus     string
		wantHasUsage   *bool
	}{
		{
			name:        "empty status",
			statusQuery: "",
			wantStatus:  "",
		},
		{
			name:        "real status active",
			statusQuery: "active",
			wantStatus:  "active",
		},
		{
			name:         "has_usage pseudo status",
			statusQuery:  service.APIKeyFilterStatusHasUsage,
			wantStatus:   "",
			wantHasUsage: boolPtr(true),
		},
		{
			name:         "no_usage pseudo status",
			statusQuery:  service.APIKeyFilterStatusNoUsage,
			wantStatus:   "",
			wantHasUsage: boolPtr(false),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			req := httptest.NewRequest(http.MethodGet, "/api/v1/api-keys?status="+tt.statusQuery, nil)
			c.Request = req

			filters := parseAPIKeyListFilters(c)
			require.Equal(t, tt.wantStatus, filters.Status)
			if tt.wantHasUsage == nil {
				require.Nil(t, filters.HasUsage)
			} else {
				require.NotNil(t, filters.HasUsage)
				require.Equal(t, *tt.wantHasUsage, *filters.HasUsage)
			}
		})
	}
}

func boolPtr(v bool) *bool {
	return &v
}
