package service

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestMarkAndGetOpsCyberPolicy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	require.Nil(t, GetOpsCyberPolicy(c), "no mark initially")

	MarkOpsCyberPolicy(c, CyberPolicyMark{
		Code:           "cyber_policy",
		Message:        "This request was flagged for cyber policy.",
		Body:           `{"error":{"code":"cyber_policy"}}`,
		UpstreamStatus: 400,
	})

	got := GetOpsCyberPolicy(c)
	require.NotNil(t, got)
	require.Equal(t, "cyber_policy", got.Code)
	require.Equal(t, 400, got.UpstreamStatus)
}

func TestMarkOpsCyberPolicyFirstWins(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	MarkOpsCyberPolicy(c, CyberPolicyMark{Code: "cyber_policy", Message: "first"})
	MarkOpsCyberPolicy(c, CyberPolicyMark{Code: "cyber_policy", Message: "second"})
	require.Equal(t, "first", GetOpsCyberPolicy(c).Message, "first mark wins, later marks ignored")
}

func TestMarkOpsCyberPolicyNilContext(t *testing.T) {
	MarkOpsCyberPolicy(nil, CyberPolicyMark{Code: "cyber_policy"})
	require.Nil(t, GetOpsCyberPolicy(nil))
}

// TestClearOpsCyberPolicy_AllowsRemark verifies F1: after Clear, Get returns nil
// and a subsequent Mark takes effect (per-turn lifecycle in WS connections).
func TestClearOpsCyberPolicy_AllowsRemark(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	MarkOpsCyberPolicy(c, CyberPolicyMark{Message: "first", UpstreamStatus: 200})
	require.NotNil(t, GetOpsCyberPolicy(c))

	ClearOpsCyberPolicy(c)
	require.Nil(t, GetOpsCyberPolicy(c), "mark must be invisible after Clear")

	MarkOpsCyberPolicy(c, CyberPolicyMark{Message: "second", UpstreamStatus: 400})
	got := GetOpsCyberPolicy(c)
	require.NotNil(t, got, "re-mark after Clear must take effect")
	require.Equal(t, "second", got.Message)
}

func TestMarkAndGetOpsCyberPolicyPreservesUpstreamCode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	MarkOpsCyberPolicy(c, CyberPolicyMark{
		Code:           "invalid_prompt",
		Message:        "Invalid prompt: your prompt was flagged as potentially violating our usage policy.",
		UpstreamStatus: 400,
	})

	got := GetOpsCyberPolicy(c)
	require.NotNil(t, got)
	require.Equal(t, "invalid_prompt", got.Code, "upstream policy code must survive the mark")
	require.Equal(t, 400, got.UpstreamStatus)
}

func TestDetectOpenAIPolicyBlock(t *testing.T) {
	cases := []struct {
		name     string
		payload  string
		hit      bool
		wantCode string
		msg      string
	}{
		{"cyber top-level error", `{"error":{"code":"cyber_policy","message":"flagged"}}`, true, "cyber_policy", "flagged"},
		{"cyber response-wrapped", `{"response":{"error":{"code":"cyber_policy","message":"  bad  "}}}`, true, "cyber_policy", "bad"},
		{"cyber case-insensitive", `{"error":{"code":"Cyber_Policy"}}`, true, "cyber_policy", ""},
		{"content_policy not policy block", `{"error":{"code":"content_policy","message":"x"}}`, false, "", ""},
		{"safety message not policy block", `{"error":{"type":"safety_error","message":"high-risk cyber activity"}}`, false, "", ""},
		{"empty", ``, false, "", ""},
		{"upstream_error", `{"error":{"code":"upstream_error"}}`, false, "", ""},
		{
			"invalid_prompt usage policy (issue 8551)",
			`{"error":{"code":"invalid_prompt","message":"Invalid prompt: your prompt was flagged as potentially violating our usage policy. Please try again with a different prompt.","param":null,"type":"invalid_request_error"}}`,
			true, "invalid_prompt",
			"Invalid prompt: your prompt was flagged as potentially violating our usage policy. Please try again with a different prompt.",
		},
		{
			"invalid_prompt response-wrapped",
			`{"response":{"error":{"code":"invalid_prompt","message":"Your request was flagged by our safety system."}}}`,
			true, "invalid_prompt", "Your request was flagged by our safety system.",
		},
		{
			"invalid_prompt without policy wording stays out",
			`{"error":{"code":"invalid_prompt","message":"Request blocked."}}`,
			false, "", "",
		},
		{
			"invalid_prompt malformed prompt stays out",
			`{"error":{"code":"invalid_prompt","message":"Invalid prompt: unsupported message role"}}`,
			false, "", "",
		},
		{
			"invalid_request_error policy wording without code",
			`{"error":{"type":"invalid_request_error","message":"Your prompt was flagged as potentially violating our usage policy."}}`,
			true, "invalid_prompt", "Your prompt was flagged as potentially violating our usage policy.",
		},
		{
			"invalid_request_error parameter error stays out",
			`{"error":{"type":"invalid_request_error","message":"Invalid schema for function 'automation_update'","param":"tools[0].parameters"}}`,
			false, "", "",
		},
		{
			"invalid_prompt type fallback",
			`{"error":{"type":"invalid_prompt","message":"The prompt violates our usage policy."}}`,
			true, "invalid_prompt", "The prompt violates our usage policy.",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hit, code, msg := detectOpenAIPolicyBlock([]byte(tc.payload))
			require.Equal(t, tc.hit, hit)
			if tc.hit {
				require.Equal(t, tc.wantCode, code)
				require.Equal(t, tc.msg, msg)
			}
		})
	}
}
