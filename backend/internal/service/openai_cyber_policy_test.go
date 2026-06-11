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
		Body:           `{"error":{"code":"cyber_policy"REDACTEDREDACTED`,
		UpstreamStatus: 400,
REDACTED)

	got := GetOpsCyberPolicy(c)
	require.NotNil(t, got)
	require.Equal(t, "cyber_policy", got.Code)
	require.Equal(t, 400, got.UpstreamStatus)
REDACTED

func TestMarkOpsCyberPolicyFirstWins(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	MarkOpsCyberPolicy(c, CyberPolicyMark{Code: "cyber_policy", Message: "first"REDACTED)
	MarkOpsCyberPolicy(c, CyberPolicyMark{Code: "cyber_policy", Message: "second"REDACTED)
	require.Equal(t, "first", GetOpsCyberPolicy(c).Message, "first mark wins, later marks ignored")
REDACTED

func TestMarkOpsCyberPolicyNilContext(t *testing.T) {
	MarkOpsCyberPolicy(nil, CyberPolicyMark{Code: "cyber_policy"REDACTED)
	require.Nil(t, GetOpsCyberPolicy(nil))
REDACTED

// TestClearOpsCyberPolicy_AllowsRemark verifies F1: after Clear, Get returns nil
// and a subsequent Mark takes effect (per-turn lifecycle in WS connections).
func TestClearOpsCyberPolicy_AllowsRemark(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	MarkOpsCyberPolicy(c, CyberPolicyMark{Message: "first", UpstreamStatus: 200REDACTED)
	require.NotNil(t, GetOpsCyberPolicy(c))

	ClearOpsCyberPolicy(c)
	require.Nil(t, GetOpsCyberPolicy(c), "mark must be invisible after Clear")

	MarkOpsCyberPolicy(c, CyberPolicyMark{Message: "second", UpstreamStatus: 400REDACTED)
	got := GetOpsCyberPolicy(c)
	require.NotNil(t, got, "re-mark after Clear must take effect")
	require.Equal(t, "second", got.Message)
REDACTED

func TestDetectOpenAICyberPolicy(t *testing.T) {
	cases := []struct {
		name    string
		payload string
		hit     bool
		msg     string
REDACTED{
		{"top-level error", `{"error":{"code":"cyber_policy","message":"flagged"REDACTEDREDACTED`, true, "flagged"REDACTED,
		{"response-wrapped", `{"response":{"error":{"code":"cyber_policy","message":"  bad  "REDACTEDREDACTEDREDACTED`, true, "bad"REDACTED,
		{"case-insensitive", `{"error":{"code":"Cyber_Policy"REDACTEDREDACTED`, true, ""REDACTED,
		{"content_policy not cyber", `{"error":{"code":"content_policy","message":"x"REDACTEDREDACTED`, false, ""REDACTED,
		{"safety message not cyber", `{"error":{"type":"safety_error","message":"high-risk cyber activity"REDACTEDREDACTED`, false, ""REDACTED,
		{"empty", ``, false, ""REDACTED,
		{"upstream_error", `{"error":{"code":"upstream_error"REDACTEDREDACTED`, false, ""REDACTED,
REDACTED
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hit, code, msg := detectOpenAICyberPolicy([]byte(tc.payload))
			require.Equal(t, tc.hit, hit)
			if tc.hit {
				require.Equal(t, "cyber_policy", code)
				require.Equal(t, tc.msg, msg)
		REDACTED
	REDACTED)
REDACTED
REDACTED
