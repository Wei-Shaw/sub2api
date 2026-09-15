package handler

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAPIKeyConcurrencyLimitBinding(t *testing.T) {
	for _, tc := range []struct {
		name, value string
		invalid     bool
		want        int
		omitted     bool
	}{
		{"omitted", "", false, 0, true},
		{"zero", `,"concurrency_limit":0`, false, 0, false},
		{"positive", `,"concurrency_limit":7`, false, 7, false},
		{"negative", `,"concurrency_limit":-1`, true, 0, false},
		{"fraction", `,"concurrency_limit":1.5`, true, 0, false},
		{"overflow", `,"concurrency_limit":9223372036854775808`, true, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, update := range []bool{false, true} {
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest("POST", "/", strings.NewReader(`{"name":"test"`+tc.value+`}`))
				var createReq CreateAPIKeyRequest
				var updateReq UpdateAPIKeyRequest
				var err error
				if update {
					err = c.ShouldBindJSON(&updateReq)
				} else {
					err = c.ShouldBindJSON(&createReq)
				}
				if tc.invalid {
					require.Error(t, err)
					continue
				}
				require.NoError(t, err)
				if update {
					if tc.omitted {
						require.Nil(t, updateReq.ConcurrencyLimit)
					} else {
						require.NotNil(t, updateReq.ConcurrencyLimit)
						require.Equal(t, tc.want, *updateReq.ConcurrencyLimit)
					}
				} else {
					require.Equal(t, tc.want, createReq.ConcurrencyLimit)
				}
			}
		})
	}
}
