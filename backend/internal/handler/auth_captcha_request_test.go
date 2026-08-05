//go:build unit

package handler

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAuthRequestsBindTencentCaptchaProof(t *testing.T) {
	const payload = `{"email":"user@example.com","password":"secret-123","tencent_captcha_ticket":"ticket-value","tencent_captcha_randstr":"@rand-value"}`

	var login LoginRequest
	require.NoError(t, json.Unmarshal([]byte(payload), &login))
	proof := captchaProof(login.TurnstileToken, login.TencentCaptchaTicket, login.TencentCaptchaRandstr)
	require.Equal(t, "ticket-value", proof.TencentTicket)
	require.Equal(t, "@rand-value", proof.TencentRandstr)

	var pending createPendingOAuthAccountRequest
	require.NoError(t, json.Unmarshal([]byte(payload), &pending))
	proof = captchaProof(pending.TurnstileToken, pending.TencentCaptchaTicket, pending.TencentCaptchaRandstr)
	require.Equal(t, "ticket-value", proof.TencentTicket)
	require.Equal(t, "@rand-value", proof.TencentRandstr)
}
