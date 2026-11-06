//go:build unit

package admin

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/adminaccess"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type scopedProxyReadTestService struct {
	service.AdminService
	page, size, calls int
	ids               []int64
}

func (s *scopedProxyReadTestService) ListProxyOptions(_ context.Context, page, size int, _ string) ([]service.Proxy, int64, error) {
	s.page = page
	s.size = size
	s.calls++
	items := make([]service.Proxy, size)
	for i := range items {
		items[i] = service.Proxy{ID: int64((page-1)*size + i + 1), Host: "192.0.2.1", Username: "secret-user", Password: "secret-password", Status: service.StatusActive}
	}
	return items, 20001, nil
}
func (s *scopedProxyReadTestService) GetProxiesByIDs(_ context.Context, ids []int64) ([]service.Proxy, error) {
	s.ids = ids
	s.calls++
	items := []service.Proxy{}
	for _, id := range ids {
		if id == 999 {
			continue
		}
		items = append(items, service.Proxy{ID: id, Username: "secret-user", Password: "secret-password"})
	}
	return items, nil
}
func (s *scopedProxyReadTestService) GetProxy(_ context.Context, id int64) (*service.Proxy, error) {
	s.calls++
	return &service.Proxy{ID: id, Username: "secret-user", Password: "secret-password"}, nil
}

func TestScopedProxyReadBoundedPaginationAndProjection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		query                     string
		status, page, size, count int
	}{
		{"", 200, 1, 50, 50}, {"?page=2&page_size=50000", 200, 2, 100, 100}, {"?page=201&page_size=100", 200, 201, 100, 100},
		{"?page=-1", 400, 0, 0, 0}, {"?page_size=0", 400, 0, 0, 0}, {"?page_size=no", 400, 0, 0, 0}, {"?page=999999999999999999999", 400, 0, 0, 0},
		{"?ids=1,2,3&page=2&page_size=2", 200, 2, 2, 1}, {"?ids=", 200, 1, 50, 0},
		{"?ids=999,1&page_size=1", 200, 1, 1, 1}, {"?ids=1,2,2&page=4&page_size=1", 200, 4, 1, 0},
	} {
		t.Run(tc.query, func(t *testing.T) {
			svc := &scopedProxyReadTestService{}
			h := NewProxyHandler(svc)
			r := gin.New()
			r.Use(func(c *gin.Context) {
				c.Request = c.Request.WithContext(adminaccess.WithPrincipal(c.Request.Context(), &adminaccess.Principal{Policy: adminaccess.Policy{GlobalPermissions: []string{"proxies.read"}}}))
			})
			r.GET("/proxies", h.List)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest("GET", "/proxies"+tc.query, nil))
			require.Equal(t, tc.status, w.Code, w.Body.String())
			require.NotContains(t, w.Body.String(), "secret-")
			require.NotContains(t, w.Body.String(), "username")
			require.NotContains(t, w.Body.String(), "password")
			if tc.status != 200 {
				require.Zero(t, svc.calls)
				return
			}
			var body struct {
				Data struct {
					Items    []map[string]any `json:"items"`
					Page     int              `json:"page"`
					PageSize int              `json:"page_size"`
					Total    int64            `json:"total"`
				}
			}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			require.Equal(t, tc.page, body.Data.Page)
			require.Equal(t, tc.size, body.Data.PageSize)
			require.Len(t, body.Data.Items, tc.count)
			if strings.Contains(tc.query, "ids=999,1") {
				require.EqualValues(t, 1, body.Data.Total)
				require.EqualValues(t, 1, body.Data.Items[0]["id"])
			}
			if strings.Contains(tc.query, "ids=1,2,3") {
				require.Equal(t, []int64{1, 2, 3}, svc.ids)
			}
		})
	}
}

func TestScopedProxyReadRejectsLargeIDBatchAndRequiresPermission(t *testing.T) {
	for _, allowed := range []bool{false, true} {
		svc := &scopedProxyReadTestService{}
		h := NewProxyHandler(svc)
		r := gin.New()
		p := &adminaccess.Principal{}
		if allowed {
			p.Policy.GlobalPermissions = []string{"proxies.read"}
		}
		r.Use(func(c *gin.Context) {
			c.Request = c.Request.WithContext(adminaccess.WithPrincipal(context.Background(), p))
		})
		r.GET("/proxies", h.GetAll)
		r.GET("/proxies/:id", h.GetByID)
		ids := []string{}
		for i := 1; i <= 101; i++ {
			ids = append(ids, strconv.Itoa(i))
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", "/proxies?ids="+strings.Join(ids, ","), nil))
		if allowed {
			require.Equal(t, 400, w.Code)
		} else {
			require.Equal(t, 403, w.Code)
		}
		require.Zero(t, svc.calls)
		w = httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", "/proxies/1", nil))
		if allowed {
			require.Equal(t, 200, w.Code)
			require.NotContains(t, w.Body.String(), "secret-")
			require.NotContains(t, w.Body.String(), "username")
		} else {
			require.Equal(t, 403, w.Code)
		}
	}
}

type scopedProxySampleTestService struct {
	scopedProxyReadTestService
	samples int
	search  string
}

func (s *scopedProxySampleTestService) SampleProxyOptions(_ context.Context, search string) ([]service.Proxy, int64, error) {
	s.samples++
	s.search = search
	return []service.Proxy{{ID: 12345, Host: "192.0.2.1", Username: "secret-user", Password: "secret-password"}}, 20000, nil
}
func TestScopedProxyAllSamplesOnlyWithoutPagination(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		path, query string
		sample      bool
		status      int
	}{
		{"/api/v1/admin/proxies/all", "", true, 200},
		{"/api/v1/admin/proxies/all", "?search=region", true, 200},
		{"/api/v1/admin/proxies/all", "?page=1", false, 200},
		{"/api/v1/admin/proxies/all", "?page_size=100", false, 200},
		{"/api/v1/admin/proxies/all", "?page=", false, 400},
		{"/api/v1/admin/proxies/all", "?ids=1,2", false, 200},
		{"/api/v1/admin/proxies", "", false, 200},
	} {
		t.Run(tc.path+tc.query, func(t *testing.T) {
			svc := &scopedProxySampleTestService{}
			h := NewProxyHandler(svc)
			r := gin.New()
			r.Use(func(c *gin.Context) {
				c.Request = c.Request.WithContext(adminaccess.WithPrincipal(c.Request.Context(), &adminaccess.Principal{Policy: adminaccess.Policy{GlobalPermissions: []string{"proxies.read"}}}))
			})
			r.GET("/api/v1/admin/proxies/all", h.GetAll)
			r.GET("/api/v1/admin/proxies", h.List)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest("GET", tc.path+tc.query, nil))
			require.Equal(t, tc.status, w.Code, w.Body.String())
			if tc.sample {
				require.Equal(t, 1, svc.samples)
				require.Zero(t, svc.calls)
				require.Contains(t, w.Body.String(), `"data":[`)
				require.Contains(t, w.Body.String(), `"id":12345`)
				require.Equal(t, "20000", w.Header().Get("X-Total-Count"))
				require.Equal(t, "50", w.Header().Get("X-Page-Size"))
				require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
				if tc.query != "" {
					require.Equal(t, "region", svc.search)
				}
			} else {
				require.Zero(t, svc.samples)
			}
			require.NotContains(t, w.Body.String(), "secret-")
			require.NotContains(t, w.Body.String(), "password")
		})
	}
}
