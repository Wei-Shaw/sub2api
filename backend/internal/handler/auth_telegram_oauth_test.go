//go:build unit

package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestVerifyTelegramAuthResult(t *testing.T) {
	const token = "123456:test-bot-token"
	now := time.Unix(1_700_000_600, 0)

	// data_check_string written by hand: sorted keys, "\n"-joined, hash excluded.
	sign := func(dataCheck string) string {
		secret := sha256.Sum256([]byte(token))
		mac := hmac.New(sha256.New, secret[:])
		_, _ = mac.Write([]byte(dataCheck))
		return hex.EncodeToString(mac.Sum(nil))
	}
	hash := sign("auth_date=1700000000\nfirst_name=Ann\nid=987654321\nusername=ann")
	encode := func(json string) string { return base64.StdEncoding.EncodeToString([]byte(json)) }
	valid := fmt.Sprintf(`{"id":987654321,"first_name":"Ann","username":"ann","auth_date":1700000000,"hash":"%s"}`, hash)

	user, err := verifyTelegramAuthResult(encode(valid), token, now)
	require.NoError(t, err)
	require.Equal(t, "987654321", user.ID)
	require.Equal(t, "ann", user.Username)
	require.Equal(t, "Ann", user.FirstName)

	// base64url without padding, as Telegram puts it in #tgAuthResult.
	_, err = verifyTelegramAuthResult(base64.RawURLEncoding.EncodeToString([]byte(valid)), token, now)
	require.NoError(t, err)

	_, err = verifyTelegramAuthResult(encode(valid), "123456:other-token", now)
	require.ErrorContains(t, err, "signature")

	tampered := fmt.Sprintf(`{"id":1,"first_name":"Ann","username":"ann","auth_date":1700000000,"hash":"%s"}`, hash)
	_, err = verifyTelegramAuthResult(encode(tampered), token, now)
	require.ErrorContains(t, err, "signature")

	_, err = verifyTelegramAuthResult(encode(valid), token, now.Add(2*time.Hour))
	require.ErrorContains(t, err, "expired")

	_, err = verifyTelegramAuthResult("", token, now)
	require.Error(t, err)
	_, err = verifyTelegramAuthResult("false", token, now)
	require.Error(t, err)
}
