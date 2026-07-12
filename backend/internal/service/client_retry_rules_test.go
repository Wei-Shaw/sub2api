package service

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestClientRetryManifestValidationAndOverride(t *testing.T) {
	manifest := ClientRetryRulesManifest{
		SchemaVersion: 1,
		Revision:      2,
		GeneratedAt:   time.Now().Add(-time.Minute),
		ExpiresAt:     time.Now().Add(time.Hour),
		Rules:         []ClientRetryRule{{ID: "remote", Originators: []string{"client"}, UAContainsAll: []string{"marker"}, RetryCapable: true, GatewayAttemptLimit: 2}},
	}
	raw, err := json.Marshal(manifest)
	require.NoError(t, err)
	decoded, err := decodeClientRetryManifest(raw)
	require.NoError(t, err)
	require.Equal(t, int64(2), decoded.Revision)

	merged := mergeClientRetryRules(manifest.Rules, []ClientRetryRule{{ID: "remote", Originators: []string{"client"}, UAContainsAll: []string{"marker"}, RetryCapable: false, GatewayAttemptLimit: 3}})
	var overridden ClientRetryRule
	for _, rule := range merged {
		if rule.ID == "remote" {
			overridden = rule
		}
	}
	require.False(t, overridden.RetryCapable)
}
