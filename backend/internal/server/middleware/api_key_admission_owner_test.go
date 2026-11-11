package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type missingAdmissionLeaseCache struct{ service.ConcurrencyCache }

func (*missingAdmissionLeaseCache) TrackAPIKeySlot(context.Context, int64, string) error { return nil }
func (*missingAdmissionLeaseCache) AcquireAPIKeySlot(context.Context, int64, int, string) (bool, error) {
	return true, nil
}
func (*missingAdmissionLeaseCache) ReleaseAPIKeySlot(context.Context, int64, string) error {
	return nil
}
func (*missingAdmissionLeaseCache) GetAPIKeyConcurrencyBatch(context.Context, []int64) (map[int64]int, error) {
	return nil, nil
}
func (*missingAdmissionLeaseCache) RefreshAPIKeySlot(context.Context, int64, string) (bool, error) {
	return false, nil
}
func (*missingAdmissionLeaseCache) APIKeySlotRefreshInterval() time.Duration { return time.Millisecond }
func (*missingAdmissionLeaseCache) APIKeySlotTTL() time.Duration             { return 3 * time.Second }

func TestAPIKeyAdmissionOwnerChangesUncommittedFailureTo503(t *testing.T) {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		nextWithAPIKeyAdmissionOwner(c, nil, "", "", &service.APIKey{ID: 1, ConcurrencyLimit: 1}, false)
	})
	router.GET("/forward", func(c *gin.Context) {
		lease, err := service.NewConcurrencyService(&missingAdmissionLeaseCache{}).AcquireAPIKeySlot(c.Request.Context(), 1, 1)
		require.NoError(t, err)
		defer lease.ReleaseFunc()
		select {
		case <-c.Request.Context().Done():
		case <-time.After(time.Second):
			t.Fatal("lease loss did not cancel request")
		}
		c.JSON(http.StatusBadGateway, gin.H{"error": "upstream interrupted"})
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/forward", nil))
	require.Equal(t, http.StatusServiceUnavailable, response.Code)
}

func TestAPIKeyAdmissionOwnerGinFinalizationAndImplicitWrites(t *testing.T) {
	for _, googleStyle := range []bool{false, true} {
		for _, variant := range []string{"blank", "write", "write_string", "flush", "header_now", "json", "queued_204", "head"} {
			t.Run(strconv.FormatBool(googleStyle)+"/"+variant, func(t *testing.T) {
				router := gin.New()
				router.Use(func(c *gin.Context) {
					nextWithAPIKeyAdmissionOwner(c, nil, "", "", &service.APIKey{ID: 1, ConcurrencyLimit: 1}, googleStyle)
				})
				method := http.MethodGet
				if variant == "head" {
					method = http.MethodHead
				}
				router.Handle(method, "/forward", func(c *gin.Context) {
					c.Header("Content-Type", "application/octet-stream")
					c.Header("Content-Length", "99999")
					c.Header("Content-Encoding", "gzip")
					c.Header("ETag", "upstream-etag")
					if variant == "queued_204" {
						c.Status(http.StatusNoContent)
					}
					lease, err := service.NewConcurrencyService(&missingAdmissionLeaseCache{}).AcquireAPIKeySlot(c.Request.Context(), 1, 1)
					require.NoError(t, err)
					defer lease.ReleaseFunc()
					select {
					case <-c.Request.Context().Done():
					case <-time.After(time.Second):
						t.Fatal("lease did not fail")
					}
					switch variant {
					case "write":
						n, err := c.Writer.Write([]byte("upstream bytes"))
						require.NoError(t, err)
						require.Equal(t, len("upstream bytes"), n)
					case "write_string", "head":
						n, err := c.Writer.WriteString("upstream bytes")
						require.NoError(t, err)
						require.Equal(t, len("upstream bytes"), n)
					case "flush":
						c.Writer.Flush()
					case "header_now":
						c.Writer.WriteHeaderNow()
					case "json":
						c.JSON(http.StatusOK, gin.H{"upstream": "bytes"})
					}
					if variant != "blank" && variant != "queued_204" {
						_, _ = c.Writer.WriteString("more upstream bytes")
					}
				})
				response := httptest.NewRecorder()
				router.ServeHTTP(response, httptest.NewRequest(method, "/forward", nil))
				result := response.Result()
				defer func() { _ = result.Body.Close() }()
				require.Equal(t, http.StatusServiceUnavailable, result.StatusCode)
				require.Equal(t, "application/json; charset=utf-8", result.Header.Get("Content-Type"))
				require.Empty(t, result.Header.Get("Content-Encoding"))
				require.Empty(t, result.Header.Get("ETag"))
				require.Equal(t, "no-store", result.Header.Get("Cache-Control"))
				require.Positive(t, result.ContentLength)
				if method == http.MethodHead {
					require.Empty(t, response.Body.String())
				} else {
					require.EqualValues(t, response.Body.Len(), result.ContentLength)
					var body struct {
						Error struct {
							Type, Message, Status string
							Code                  int
						}
					}
					require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
					require.Contains(t, body.Error.Message, "API key concurrency lease lost")
					if googleStyle {
						require.Equal(t, 503, body.Error.Code)
						require.Equal(t, "UNAVAILABLE", body.Error.Status)
					} else {
						require.Equal(t, "api_error", body.Error.Type)
					}
				}
			})
		}
	}
}

func TestAPIKeyAdmissionOwnerPreservesNormalAndCommittedResponses(t *testing.T) {
	for _, variant := range []string{"normal", "blank", "unlimited", "streaming"} {
		t.Run(variant, func(t *testing.T) {
			router := gin.New()
			limit := 1
			if variant == "unlimited" {
				limit = 0
			}
			router.Use(func(c *gin.Context) {
				nextWithAPIKeyAdmissionOwner(c, nil, "", "", &service.APIKey{ID: 1, ConcurrencyLimit: limit}, false)
			})
			router.GET("/forward", func(c *gin.Context) {
				require.Implements(t, (*http.Hijacker)(nil), c.Writer)
				require.Implements(t, (*http.Flusher)(nil), c.Writer)
				require.NotPanics(t, func() { c.Writer.Pusher() })
				if variant == "blank" {
					return
				}
				if variant != "streaming" {
					c.String(http.StatusOK, "ordinary")
					return
				}
				_, _ = c.Writer.WriteString("first")
				c.Writer.Flush()
				lease, err := service.NewConcurrencyService(&missingAdmissionLeaseCache{}).AcquireAPIKeySlot(c.Request.Context(), 1, 1)
				require.NoError(t, err)
				defer lease.ReleaseFunc()
				select {
				case <-c.Request.Context().Done():
				case <-time.After(time.Second):
					t.Fatal("lease did not fail")
				}
				_, _ = c.Writer.Write([]byte("last"))
				c.Writer.Flush()
			})
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/forward", nil))
			require.Equal(t, http.StatusOK, response.Code)
			switch variant {
			case "blank":
				require.Empty(t, response.Body.String())
			case "streaming":
				require.Equal(t, "firstlast", response.Body.String())
			default:
				require.Equal(t, "ordinary", response.Body.String())
			}
		})
	}
}
