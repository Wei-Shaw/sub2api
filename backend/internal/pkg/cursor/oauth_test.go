package cursor

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGeneratePKCEUsesS256(t *testing.T) {
	verifier, challenge, err := GeneratePKCE()
	require.NoError(t, err)
	require.NotEmpty(t, verifier)
	require.NotEmpty(t, challenge)
	sum := sha256.Sum256([]byte(verifier))
	require.Equal(t, base64.RawURLEncoding.EncodeToString(sum[:]), challenge)
}

func TestBuildLoginURL(t *testing.T) {
	url := BuildLoginURL("challenge-token", "login-uuid")
	require.True(t, strings.HasPrefix(url, LoginURL+"?"))
	require.Contains(t, url, "challenge=challenge-token")
	require.Contains(t, url, "uuid=login-uuid")
	require.Contains(t, url, "mode=login")
	require.Contains(t, url, "redirectTarget=cli")
}

func TestExtractUserIDFromAuth0Subject(t *testing.T) {
	token := fakeJWT(t, map[string]any{"sub": "auth0|user-123", "exp": float64(time.Now().Add(time.Hour).Unix())})
	require.Equal(t, "user-123", ExtractUserID(token))
	require.False(t, TokenExpiry(token).IsZero())
}

func TestSessionTryConsumeOnce(t *testing.T) {
	session := &OAuthSession{CreatedAt: time.Now()}
	require.True(t, session.TryConsume())
	require.False(t, session.TryConsume())
}

func fakeJWT(t *testing.T, payload map[string]any) string {
	t.Helper()
	raw, err := json.Marshal(payload)
	require.NoError(t, err)
	return "header." + base64.RawURLEncoding.EncodeToString(raw) + ".sig"
}
