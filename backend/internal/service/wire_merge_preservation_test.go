//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProvideAccountUsageServicePreservesKiroAndAgentIdentityDependencies(t *testing.T) {
	kiro := &KiroTokenProvider{}
	gateway := &OpenAIGatewayService{}

	svc := ProvideAccountUsageService(
		nil, nil, nil, nil, nil, nil, nil, nil,
		NewUsageCache(), nil, nil, gateway, kiro,
	)

	require.Equal(t, kiro, svc.kiroTokenProvider)
	require.Equal(t, gateway, svc.agentIdentityWS)
}

func TestProvideAccountTestServicePreservesKiroAndAgentIdentityDependencies(t *testing.T) {
	kiro := &KiroTokenProvider{}
	gateway := &OpenAIGatewayService{}

	svc := ProvideAccountTestService(
		nil, nil, nil, kiro, nil, nil, nil, nil, nil, gateway, nil,
	)

	require.Equal(t, kiro, svc.kiroTokenProvider)
	require.Equal(t, gateway, svc.agentIdentityWS)
}
