package service

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const usageDiagnosisDraftKey = "usage_diagnosis_draft"

// UsageDiagnosisDraft accumulates request/response bodies for later dump persistence.
type UsageDiagnosisDraft struct {
	RequestID       string
	Method          string
	Path            string
	StatusCode      int
	UpstreamURL     string
	UpstreamStatus  int
	ReqHeaders      map[string]string
	ResHeaders      map[string]string
	ReqBody         string
	ResBody         string
	UpstreamReqBody string
}

func headerMapFromHTTP(h http.Header) map[string]string {
	if len(h) == 0 {
		return nil
	}
	out := make(map[string]string, len(h))
	for k, vals := range h {
		if len(vals) == 0 {
			continue
		}
		out[k] = strings.Join(vals, ", ")
	}
	return MaskSensitiveHeaders(out)
}

// CaptureUsageDiagnosisRequest stores inbound request headers/body on the gin context.
func CaptureUsageDiagnosisRequest(c *gin.Context, reqBody []byte) {
	if c == nil {
		return
	}
	draft := getOrCreateDiagnosisDraft(c)
	draft.Method = c.Request.Method
	draft.Path = c.FullPath()
	if draft.Path == "" && c.Request.URL != nil {
		draft.Path = c.Request.URL.Path
	}
	draft.ReqHeaders = headerMapFromHTTP(c.Request.Header)
	if len(reqBody) > 0 {
		draft.ReqBody = string(reqBody)
	}
}

// CaptureUsageDiagnosisUpstream records upstream request/response snapshot fields.
func CaptureUsageDiagnosisUpstream(c *gin.Context, upstreamURL string, status int, upstreamReqBody, resBody []byte, resHeaders http.Header) {
	if c == nil {
		return
	}
	draft := getOrCreateDiagnosisDraft(c)
	if u := strings.TrimSpace(upstreamURL); u != "" {
		draft.UpstreamURL = u
	}
	if status > 0 {
		draft.UpstreamStatus = status
		draft.StatusCode = status
	}
	if len(upstreamReqBody) > 0 {
		draft.UpstreamReqBody = string(upstreamReqBody)
	}
	if len(resBody) > 0 {
		draft.ResBody = string(resBody)
	}
	if resHeaders != nil {
		draft.ResHeaders = headerMapFromHTTP(resHeaders)
	}
}

// CaptureUsageDiagnosisResponseBody sets/overrides the response body on the draft.
func CaptureUsageDiagnosisResponseBody(c *gin.Context, resBody []byte, status int) {
	if c == nil || len(resBody) == 0 {
		return
	}
	draft := getOrCreateDiagnosisDraft(c)
	draft.ResBody = string(resBody)
	if status > 0 {
		draft.StatusCode = status
	}
}

func getOrCreateDiagnosisDraft(c *gin.Context) *UsageDiagnosisDraft {
	if v, ok := c.Get(usageDiagnosisDraftKey); ok {
		if d, ok := v.(*UsageDiagnosisDraft); ok && d != nil {
			return d
		}
	}
	d := &UsageDiagnosisDraft{}
	c.Set(usageDiagnosisDraftKey, d)
	return d
}

// GetUsageDiagnosisDraft returns the draft if present.
func GetUsageDiagnosisDraft(c *gin.Context) *UsageDiagnosisDraft {
	if c == nil {
		return nil
	}
	if v, ok := c.Get(usageDiagnosisDraftKey); ok {
		if d, ok := v.(*UsageDiagnosisDraft); ok {
			return d
		}
	}
	return nil
}

// BuildDumpFromDraft converts a draft into a persistable dump.
func BuildDumpFromDraft(requestID string, draft *UsageDiagnosisDraft) *UsageRequestDump {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" || draft == nil {
		return nil
	}
	return &UsageRequestDump{
		RequestID:       requestID,
		Method:          draft.Method,
		Path:            draft.Path,
		StatusCode:      draft.StatusCode,
		UpstreamURL:     draft.UpstreamURL,
		UpstreamStatus:  draft.UpstreamStatus,
		ReqHeaders:      draft.ReqHeaders,
		ResHeaders:      draft.ResHeaders,
		ReqBody:         draft.ReqBody,
		ResBody:         draft.ResBody,
		UpstreamReqBody: firstNonEmptyStr(draft.UpstreamReqBody, draft.ReqBody),
	}
}
