//go:build unit

package xai

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseGrokAccountStateExtractsRSCFields(t *testing.T) {
	t.Parallel()

	html := `self.__next_f.push([1,"{\"botFlagSource\":2,\"botFlagDetails\":\"risk=0.82,policy=deny,event=$registration,eapi_ip_bot_farm=1\"}"])`
	state := ParseGrokAccountState(html)
	require.True(t, state.Found)
	require.NotNil(t, state.BotFlagSource)
	require.EqualValues(t, 2, *state.BotFlagSource)
	require.Equal(t, "deny", state.Policy)
	require.Equal(t, "$registration", state.Event)
	require.True(t, state.Denied)
	require.NotNil(t, state.Risk)
	require.InDelta(t, 0.82, *state.Risk, 0.001)
	require.Equal(t, GrokRiskFlagged, ClassifyGrokAccountState(state))
	require.Equal(t, GrokRiskKindIP, GrokFlagKind(state.BotFlagDetails))
}

func TestParseGrokAccountStateCleanZero(t *testing.T) {
	t.Parallel()

	state := ParseGrokAccountState(`{"botFlagSource":0,"botFlagDetails":null}`)
	require.True(t, state.Found)
	require.NotNil(t, state.BotFlagSource)
	require.EqualValues(t, 0, *state.BotFlagSource)
	require.False(t, state.Denied)
	require.Equal(t, GrokRiskClean, ClassifyGrokAccountState(state))
}

func TestParseGrokAccountStateMissingIsUnknown(t *testing.T) {
	t.Parallel()

	state := ParseGrokAccountState(`<html>no flags</html>`)
	require.False(t, state.Found)
	require.Equal(t, GrokRiskUnknown, ClassifyGrokAccountState(state))

	state.StatusCode = http.StatusForbidden
	state.Error = "grok.com HTTP 403"
	require.Equal(t, GrokRiskError, ClassifyGrokAccountState(state))
}

func TestClassifyGrokAccountStatePolicyDeny(t *testing.T) {
	t.Parallel()

	source := int64(0)
	state := GrokAccountState{Found: true, BotFlagSource: &source, Policy: "deny", Denied: true}
	require.Equal(t, GrokRiskFlagged, ClassifyGrokAccountState(state))
	require.Equal(t, GrokRiskKindAccount, GrokFlagKind("policy=deny,event=$login"))
}

func TestExtractSSOTokenFromLine(t *testing.T) {
	t.Parallel()

	jwt := testJWTWithClaims(t, `{"sub":"u1"}`)
	require.Equal(t, jwt, ExtractSSOTokenFromLine(jwt))
	require.Equal(t, jwt, ExtractSSOTokenFromLine("sso="+jwt))
	require.Equal(t, jwt, ExtractSSOTokenFromLine("user@x.com----secret----"+jwt))
	require.Equal(t, jwt, ExtractSSOTokenFromLine("user@x.com----"+jwt))
	require.Empty(t, ExtractSSOTokenFromLine("# comment"))
	require.Empty(t, ExtractSSOTokenFromLine("   "))
}

func TestInspectJWTRiskDetectsBFSPresence(t *testing.T) {
	t.Parallel()

	flagged := testJWTWithClaims(t, `{"bfs":2,"sub":"user-flagged","tier":"1"}`)
	info := InspectJWTRisk(flagged)
	require.True(t, info.OK)
	require.True(t, info.HasBFS)
	require.EqualValues(t, 2, info.BFS)
	require.True(t, HasBFSClaim(flagged))
	require.True(t, IsRiskFlaggedToken(flagged))

	clean := testJWTWithClaims(t, `{"sub":"user-clean","bot_flag_source":0}`)
	info = InspectJWTRisk(clean)
	require.True(t, info.OK)
	require.False(t, info.HasBFS)
	require.False(t, HasBFSClaim(clean))
	require.False(t, IsRiskFlaggedToken(clean))

	media := testJWTWithClaims(t, `{"bot_flag_source":1,"sub":"user-media"}`)
	require.True(t, IsRiskFlaggedToken(media))
	require.False(t, HasBFSClaim(media))
}

func TestInspectSSOAccountStateParsesHomepage(t *testing.T) {
	t.Parallel()

	jwt := testJWTWithClaims(t, `{"sub":"u1"}`)
	client := &accountStateRoundTripClient{t: t, handler: func(req *http.Request) *http.Response {
		require.Equal(t, GrokHomeURL, req.URL.String())
		require.Contains(t, req.Header.Get("Cookie"), "sso=")
		body := `{"botFlagSource":1,"botFlagDetails":"risk=0.4,policy=allow,event=$login"}`
		return ssoDeviceResponse(http.StatusOK, nil, body)
	}}
	state := InspectSSOAccountState(context.Background(), jwt, client)
	require.True(t, state.Found)
	require.NotNil(t, state.BotFlagSource)
	require.EqualValues(t, 1, *state.BotFlagSource)
	require.Equal(t, GrokRiskFlagged, ClassifyGrokAccountState(state))
}

func TestInspectSSOAccountStateHTTPError(t *testing.T) {
	t.Parallel()

	jwt := testJWTWithClaims(t, `{"sub":"u1"}`)
	client := &accountStateRoundTripClient{t: t, handler: func(*http.Request) *http.Response {
		return ssoDeviceResponse(http.StatusForbidden, nil, "cf")
	}}
	state := InspectSSOAccountState(context.Background(), jwt, client)
	require.Equal(t, http.StatusForbidden, state.StatusCode)
	require.Contains(t, state.Error, "403")
	require.Equal(t, GrokRiskError, ClassifyGrokAccountState(state))
}

func TestInspectSSOAccountStateEmptySSO(t *testing.T) {
	t.Parallel()

	state := InspectSSOAccountState(context.Background(), "   ", nil)
	require.Equal(t, "sso is empty", state.Error)
	require.Equal(t, GrokRiskError, ClassifyGrokAccountState(state))
}

func testJWTWithClaims(t *testing.T, payloadJSON string) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(payloadJSON))
	return header + "." + payload + ".sig"
}

type accountStateRoundTripClient struct {
	t       *testing.T
	handler func(*http.Request) *http.Response
}

func (c *accountStateRoundTripClient) Do(req *http.Request) (*http.Response, error) {
	c.t.Helper()
	resp := c.handler(req)
	if resp.Body == nil {
		resp.Body = io.NopCloser(strings.NewReader(""))
	}
	resp.Request = req
	return resp, nil
}
