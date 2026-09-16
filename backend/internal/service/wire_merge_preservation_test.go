//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProvideAccountUsageServicePreservesKiroAndAgentIdentityDependencies(t *testing.T) {
	kiro := &KiroTokenProvider{}
	gateway := &OpenAIGatewayService{}
	adobeProvider := NewAdobeTokenProvider(nil, NewOAuthRefreshAPI(nil, nil))

	svc := ProvideAccountUsageService(
		nil, nil, nil, nil, nil, nil, nil, nil,
		NewUsageCache(), nil, nil, gateway, kiro, adobeProvider,
	)

	require.Equal(t, kiro, svc.kiroTokenProvider)
	require.Equal(t, gateway, svc.agentIdentityWS)
	require.Same(t, adobeProvider, svc.adobeTokenProvider)
}

func TestProvideAccountTestServicePreservesKiroAndAgentIdentityDependencies(t *testing.T) {
	kiro := &KiroTokenProvider{}
	gateway := &OpenAIGatewayService{}

	adobeProvider := &AdobeTokenProvider{}

	svc := ProvideAccountTestService(
		nil, nil, nil, kiro, nil, nil, nil, nil, nil, gateway, nil, nil, adobeProvider,
	)

	require.Equal(t, kiro, svc.kiroTokenProvider)
	require.Equal(t, gateway, svc.agentIdentityWS)
	require.Same(t, adobeProvider, svc.adobeTokenProvider)
}
