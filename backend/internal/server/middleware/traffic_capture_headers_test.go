package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/pkg/traffic"
	"github.com/gin-gonic/gin"
)

func TestTrafficMultipleContentEncodingFieldsAreNotIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, fields := range [][]string{{"identity", "gzip"}, {"gzip", "br"}, {"", "gzip"}} {
		encoded := encodeTrafficBody(t, "gzip", []byte("compressed entity"))
		engine := gin.New()
		engine.Use(TrafficCapture(), ResponseCompress())
		var usage *traffic.Usage
		engine.POST("/", func(c *gin.Context) {
			traffic.AfterFinalize(c.Request.Context(), func(u *traffic.Usage) { usage = u })
			if _, err := pkghttputil.ReadRequestBodyWithPrealloc(c.Request); err == nil {
				t.Fatal("stacked fields were silently decoded as first coding")
			}
			for _, encoding := range fields {
				c.Writer.Header().Add("Content-Encoding", encoding)
			}
			_, _ = c.Writer.Write(encoded)
		})
		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(encoded))
		for _, encoding := range fields {
			req.Header.Add("Content-Encoding", encoding)
		}
		req.Header.Set("Accept-Encoding", "gzip")
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, req)
		if !reflect.DeepEqual(fields, recorder.Header().Values("Content-Encoding")) {
			t.Fatal("pre-encoded response was re-encoded")
		}
		if !bytes.Equal(encoded, recorder.Body.Bytes()) {
			t.Fatal("opaque encoded bytes changed")
		}
		for _, measurement := range []*traffic.Measurement{usage.Downstream.Request, usage.Downstream.Response} {
			if !measurement.Compressed || measurement.OriginalBytes != nil {
				t.Fatal("stacked coding was reported as identity")
			}
			assertTrafficCount(t, measurement.ActualBytes, len(encoded))
		}
	}
}
