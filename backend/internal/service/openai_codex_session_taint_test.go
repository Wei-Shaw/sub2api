package service

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newSessionTaintTestContext(t *testing.T, apiKeyID int64, sessionID string) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	if sessionID != "" {
		c.Request.Header.Set("session-id", sessionID)
	}
	if apiKeyID > 0 {
		c.Set("api_key", &APIKey{ID: apiKeyID})
	}
	return c
}

// TestRekeyCodexTaintID covers the deterministic id remapping: same input →
// same output, prefix inherited, UUIDv5-shaped suffix, legacy ids (no
// underscore or empty suffix) mapped to "" (caller deletes), account change →
// new value.
func TestRekeyCodexTaintID(t *testing.T) {
	a := rekeyCodexTaintID(101, "msg_0a1b2c3d-4e5f-6789-abcd-ef0123456789")
	b := rekeyCodexTaintID(101, "msg_0a1b2c3d-4e5f-6789-abcd-ef0123456789")
	require.Equal(t, a, b, "deterministic for same account + old id")
	require.True(t, strings.HasPrefix(a, "msg_"), "prefix inherited")
	suffix := strings.TrimPrefix(a, "msg_")
	require.Len(t, suffix, 36, "UUID-shaped suffix")
	require.Equal(t, "5", string(suffix[14]), "UUIDv5 version nibble")
	require.NotEqual(t, suffix, "0a1b2c3d-4e5f-6789-abcd-ef0123456789", "suffix must differ from old id")
}

// TestRekeyCodexTaintID_PrefixMatrix covers prefix inheritance across id
// families and the legacy-deletion rule.
func TestRekeyCodexTaintID_PrefixMatrix(t *testing.T) {
	for _, id := range []string{
		"msg_abc", "fc_abc", "fco_abc", "rs_abc", "cmp_abc", "lsh_abc", "tsc_abc",
	} {
		mapped := rekeyCodexTaintID(7, id)
		prefix := id[:strings.Index(id, "_")]
		require.True(t, strings.HasPrefix(mapped, prefix+"_"), "%s keeps prefix", id)
		require.NotEqual(t, mapped, id)
	}

	// Legacy ids without prefix_suffix shape are deleted (official client strips
	// them too before sending, client.rs:944-950).
	require.Empty(t, rekeyCodexTaintID(7, "legacy-uuid-no-underscore"))
	require.Empty(t, rekeyCodexTaintID(7, "msg_"), "empty suffix is not prefixed")
	require.Empty(t, rekeyCodexTaintID(7, "_abc"), "empty prefix is not prefixed")
	require.Empty(t, rekeyCodexTaintID(7, ""))
}

// TestRekeyCodexTaintID_AccountSensitivity verifies switching accounts changes
// every mapped id while call/output pairing survives (same function on both).
func TestRekeyCodexTaintID_AccountSensitivity(t *testing.T) {
	call := rekeyCodexTaintID(202, "fc_old-call")
	callOut := rekeyCodexTaintID(202, "fc_old-call") // output side uses the same call_id input
	require.Equal(t, call, callOut, "call_id pairing stable within one account")

	callNewAccount := rekeyCodexTaintID(303, "fc_old-call")
	require.NotEqual(t, call, callNewAccount, "account switch rekeys the id")
	require.True(t, strings.HasPrefix(callNewAccount, "fc_"))
}

// TestDeriveCodexTaintUUID covers the header/body session identifier derivation.
func TestDeriveCodexTaintUUID(t *testing.T) {
	v1 := deriveCodexTaintUUID("pck", 11, 22, "seed-1")
	require.Len(t, v1, 36, "UUID shape")
	require.Equal(t, v1, deriveCodexTaintUUID("pck", 11, 22, "seed-1"), "deterministic")
	require.NotEqual(t, v1, deriveCodexTaintUUID("pck", 11, 23, "seed-1"), "account-sensitive")
	require.NotEqual(t, v1, deriveCodexTaintUUID("pck", 12, 22, "seed-1"), "apiKey-sensitive")
	require.NotEqual(t, v1, deriveCodexTaintUUID("thread", 11, 22, "seed-1"), "domain-separated")
	require.Empty(t, deriveCodexTaintUUID("pck", 11, 22, ""), "empty seed → empty")
}

// TestOpenAICodexSessionTaintStateMachine covers the provenance table:
// first account recorded without taint, same account keeps it untainted,
// a different account marks it switched forever (sticky), and TTL expiry
// clears the record.
func TestOpenAICodexSessionTaintStateMachine(t *testing.T) {
	svc := &OpenAIGatewayService{}
	c := newSessionTaintTestContext(t, 7, "sess-taint")

	require.False(t, svc.isOpenAICodexSessionTainted(c), "unknown session is untainted")

	svc.noteOpenAICodexSessionAttempt(c, &Account{ID: 1})
	require.False(t, svc.isOpenAICodexSessionTainted(c), "single account never taints")

	svc.noteOpenAICodexSessionAttempt(c, &Account{ID: 1})
	require.False(t, svc.isOpenAICodexSessionTainted(c), "same account retry never taints")

	svc.noteOpenAICodexSessionAttempt(c, &Account{ID: 2})
	require.True(t, svc.isOpenAICodexSessionTainted(c), "account switch taints")

	// Sticky: switching back to the first account keeps the taint (rollout
	// still replays ids minted by account 2).
	svc.noteOpenAICodexSessionAttempt(c, &Account{ID: 1})
	require.True(t, svc.isOpenAICodexSessionTainted(c))

	// TTL expiry clears the record.
	seed := openAICodexTurnStateSeed(c)
	svc.openaiCodexSessionTaints.Store(seed, openAICodexSessionTaint{
		firstAccountID: 1,
		everSwitched:   true,
		expiresAt:      time.Now().Add(-time.Second),
	})
	require.False(t, svc.isOpenAICodexSessionTainted(c), "expired record is ignored and deleted")
	_, ok := svc.openaiCodexSessionTaints.Load(seed)
	require.False(t, ok)
}

// TestOpenAICodexSessionTaint_NoSessionSeed verifies requests without a client
// session header are not tracked (taint detection impossible → passthrough).
func TestOpenAICodexSessionTaint_NoSessionSeed(t *testing.T) {
	svc := &OpenAIGatewayService{}
	c := newSessionTaintTestContext(t, 7, "")
	svc.noteOpenAICodexSessionAttempt(c, &Account{ID: 1})
	svc.noteOpenAICodexSessionAttempt(c, &Account{ID: 2})
	require.False(t, svc.isOpenAICodexSessionTainted(c))
	require.False(t, svc.isOpenAICodexSessionTainted(nil), "nil context is safe")
}

// TestSweepOpenAICodexSessionTaints verifies the opportunistic sweep removes
// expired entries once the write counter hits its stride.
func TestSweepOpenAICodexSessionTaints(t *testing.T) {
	svc := &OpenAIGatewayService{}
	svc.openaiCodexSessionTaints.Store("expired", openAICodexSessionTaint{
		firstAccountID: 1,
		everSwitched:   true,
		expiresAt:      time.Now().Add(-time.Second),
	})
	svc.openaiCodexSessionTaints.Store("fresh", openAICodexSessionTaint{
		firstAccountID: 1,
		everSwitched:   true,
		expiresAt:      time.Now().Add(time.Hour),
	})
	// Sweep triggers when the internal counter (incremented inside the sweep
	// call itself) hits a multiple of 256; pre-drive it to 255.
	for i := uint64(0); i < 255; i++ {
		svc.openaiCodexSessionTaintWrites.Add(1)
	}
	svc.sweepOpenAICodexSessionTaints()
	_, ok := svc.openaiCodexSessionTaints.Load("expired")
	require.False(t, ok, "expired entry swept")
	_, ok = svc.openaiCodexSessionTaints.Load("fresh")
	require.True(t, ok, "fresh entry kept")
}

// TestOpenAICodexTaintSanitizeContextRoundTrip covers the gin-context marker
// used to carry "rekey with account N" from the handler failover loop into the
// service transform/header layers.
func TestOpenAICodexTaintSanitizeContextRoundTrip(t *testing.T) {
	c := newSessionTaintTestContext(t, 7, "sess-ctx")
	require.Zero(t, openAICodexTaintSanitizeAccountID(c), "default: disabled")

	setOpenAICodexTaintSanitize(c, 42)
	require.Equal(t, int64(42), openAICodexTaintSanitizeAccountID(c))

	// A later attempt against another account overrides the marker.
	setOpenAICodexTaintSanitize(c, 43)
	require.Equal(t, int64(43), openAICodexTaintSanitizeAccountID(c))

	require.Zero(t, openAICodexTaintSanitizeAccountID(nil), "nil context is safe")
}
