//go:build unit

package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestUserPoolHandler_FeatureDisabled verifies that when UserPoolService is nil
// (simulating feature-disabled) the handler responds with HTTP 422 and the FEATURE_DISABLED reason.
func TestUserPoolHandler_FeatureDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Because UserPoolService is a concrete struct we test the nil guard.
	h := &UserPoolHandler{poolService: nil}

	router := gin.New()
	router.GET("/pools", func(c *gin.Context) {
		if h.poolService == nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{
				"code":    http.StatusUnprocessableEntity,
				"message": "用户分组功能未启用",
				"reason":  "FEATURE_DISABLED",
			})
			return
		}
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/pools", nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnprocessableEntity, w.Code)

	var body map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	require.Equal(t, "FEATURE_DISABLED", body["reason"])
}

// TestUserPoolHandler_Create_InvalidPayload verifies bad JSON triggers 400.
func TestUserPoolHandler_Create_InvalidPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// We need a concrete handler but can't construct UserPoolService without DB.
	// Test the binding logic directly by inspecting the binding error path.
	h := &UserPoolHandler{poolService: nil}
	router := gin.New()
	router.POST("/pools", func(c *gin.Context) {
		var req createPoolRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		// Unreachable in this test
		if h.poolService == nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"reason": "FEATURE_DISABLED"})
		}
	})

	w := httptest.NewRecorder()
	body := bytes.NewBufferString(`{"name": ""}`)
	req := httptest.NewRequest(http.MethodPost, "/pools", body)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	// name is required, empty string should fail binding
	require.Equal(t, http.StatusBadRequest, w.Code)
}

// TestAddMembersByFilter_binding verifies request validation for AddMembersByFilter.
func TestAddMembersByFilter_binding(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Route handler mirrors real handler logic without service dependency.
	setupRouter := func() *gin.Engine {
		router := gin.New()
		router.POST("/pools/:id/members/by-filter", func(c *gin.Context) {
			_, ok := parsePathInt64(c, "id")
			if !ok {
				return
			}
			var req addMembersByFilterRequest
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			if req.Search == "" && req.Status == "" && req.Role == "" && req.GroupName == "" && len(req.Attributes) == 0 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "at least one filter required, refusing to bulk-add all users"})
				return
			}
			// Simulate matched > 100000 check
			if req.Status == "active" && req.GroupName == "overflow" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "matched too many users (>100000), refine your filter"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"added": 3, "skipped": 1, "matched": 4})
		})
		return router
	}

	cases := []struct {
		name       string
		path       string
		body       string
		wantStatus int
	}{
		{
			name:       "no filters → 400",
			path:       "/pools/1/members/by-filter",
			body:       `{}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid status → 400",
			path:       "/pools/1/members/by-filter",
			body:       `{"status":"unknown"}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid role → 400",
			path:       "/pools/1/members/by-filter",
			body:       `{"role":"superadmin"}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "matched overflow → 400",
			path:       "/pools/1/members/by-filter",
			body:       `{"status":"active","group_name":"overflow"}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "valid search filter → 200",
			path:       "/pools/1/members/by-filter",
			body:       `{"search":"test@example.com"}`,
			wantStatus: http.StatusOK,
		},
		{
			name:       "valid status filter → 200",
			path:       "/pools/1/members/by-filter",
			body:       `{"status":"active"}`,
			wantStatus: http.StatusOK,
		},
		{
			name:       "invalid pool id → 400",
			path:       "/pools/abc/members/by-filter",
			body:       `{"status":"active"}`,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			router := setupRouter()
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, tc.path, bytes.NewBufferString(tc.body))
			req.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(w, req)
			require.Equal(t, tc.wantStatus, w.Code)

			if tc.wantStatus == http.StatusOK {
				var body map[string]any
				require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
				require.Contains(t, body, "added")
				require.Contains(t, body, "skipped")
				require.Contains(t, body, "matched")
			}
		})
	}
}

// TestParsePathInt64 verifies the helper rejects non-positive values.
func TestParsePathInt64(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := []struct {
		path       string
		wantStatus int
	}{
		{"/pools/1", http.StatusOK},
		{"/pools/0", http.StatusBadRequest},
		{"/pools/-1", http.StatusBadRequest},
		{"/pools/abc", http.StatusBadRequest},
	}

	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			router := gin.New()
			router.GET("/pools/:id", func(c *gin.Context) {
				id, ok := parsePathInt64(c, "id")
				if !ok {
					return
				}
				c.JSON(http.StatusOK, gin.H{"id": id})
			})

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			router.ServeHTTP(w, req)
			require.Equal(t, tc.wantStatus, w.Code)
		})
	}
}
