package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func runUsageMetadata(t *testing.T, header string) map[string]any {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	if header != "" {
		c.Request.Header.Set(usageMetadataHeader, header)
	}
	UsageMetadata()(c)
	md, _ := c.Request.Context().Value(ctxkey.UsageMetadata).(map[string]any)
	return md
}

func TestUsageMetadataParsesObject(t *testing.T) {
	md := runUsageMetadata(t, `{"source":"agent","uid":"u_123","feature":"digest"}`)
	require.Equal(t, "agent", md["source"])
	require.Equal(t, "u_123", md["uid"])
	require.Equal(t, "digest", md["feature"])
}

func TestUsageMetadataIgnoresInvalid(t *testing.T) {
	require.Nil(t, runUsageMetadata(t, ""))            // missing
	require.Nil(t, runUsageMetadata(t, "not-json"))    // malformed
	require.Nil(t, runUsageMetadata(t, `["a","b"]`))   // not an object
	require.Nil(t, runUsageMetadata(t, `{}`))          // empty object
	require.Nil(t, runUsageMetadata(t, `"just-text"`)) // not an object
}

func TestUsageMetadataRejectsOversizedHeader(t *testing.T) {
	big := `{"k":"` + strings.Repeat("x", usageMetadataMaxBytes) + `"}`
	require.Nil(t, runUsageMetadata(t, big))
}

func TestUsageMetadataRejectsTooManyKeys(t *testing.T) {
	require.Nil(t, runUsageMetadata(t, manyKeysJSON(usageMetadataMaxKeys+1)))
	require.NotNil(t, runUsageMetadata(t, manyKeysJSON(usageMetadataMaxKeys)))
}

func manyKeysJSON(n int) string {
	var b strings.Builder
	b.WriteString("{")
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(`"key`)
		b.WriteString(itoa(i))
		b.WriteString(`":"v"`)
	}
	b.WriteString("}")
	return b.String()
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var digits []byte
	for i > 0 {
		digits = append([]byte{byte('0' + i%10)}, digits...)
		i /= 10
	}
	return string(digits)
}
