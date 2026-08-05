//go:build unit

package repository

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	capcha "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/captcha/v20190722"
)

type tencentCaptchaAPIStub struct {
	request  *capcha.DescribeCaptchaResultRequest
	response *capcha.DescribeCaptchaResultResponse
	err      error
}

func (s *tencentCaptchaAPIStub) DescribeCaptchaResultWithContext(_ context.Context, request *capcha.DescribeCaptchaResultRequest) (*capcha.DescribeCaptchaResultResponse, error) {
	s.request = request
	return s.response, s.err
}

func TestTencentCaptchaVerifierMapsCredentialsRequestAndResponse(t *testing.T) {
	code := int64(1)
	message := "OK"
	requestID := "request-id"
	client := &tencentCaptchaAPIStub{response: &capcha.DescribeCaptchaResultResponse{
		Response: &capcha.DescribeCaptchaResultResponseParams{
			CaptchaCode: &code,
			CaptchaMsg:  &message,
			RequestId:   &requestID,
		},
	}}
	var gotSecretID, gotSecretKey string
	verifier := &tencentCaptchaVerifier{
		newClient: func(secretID, secretKey string) (tencentCaptchaAPI, error) {
			gotSecretID, gotSecretKey = secretID, secretKey
			return client, nil
		},
	}

	result, err := verifier.VerifyTicket(context.Background(), service.TencentCaptchaCredentials{
		AppID:          123456789,
		AppSecretKey:   "app-secret",
		CloudSecretID:  "cloud-secret-id",
		CloudSecretKey: "cloud-secret-key",
	}, service.TencentCaptchaProof{Ticket: "ticket", Randstr: "@rand"}, "203.0.113.10")

	require.NoError(t, err)
	require.Equal(t, "cloud-secret-id", gotSecretID)
	require.Equal(t, "cloud-secret-key", gotSecretKey)
	require.NotNil(t, client.request)
	require.Equal(t, uint64(9), *client.request.CaptchaType)
	require.Equal(t, uint64(123456789), *client.request.CaptchaAppId)
	require.Equal(t, "app-secret", *client.request.AppSecretKey)
	require.Equal(t, "ticket", *client.request.Ticket)
	require.Equal(t, "@rand", *client.request.Randstr)
	require.Equal(t, "203.0.113.10", *client.request.UserIp)
	require.Equal(t, &service.TencentCaptchaVerifyResponse{
		CaptchaCode: 1,
		CaptchaMsg:  "OK",
		RequestID:   "request-id",
	}, result)
}
