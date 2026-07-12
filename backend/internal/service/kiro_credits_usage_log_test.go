//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	kiropkg "github.com/Wei-Shaw/sub2api/internal/pkg/kiro"
	"github.com/stretchr/testify/require"
)

func TestKiroUsageToClaudeMapsKiroCredits(t *testing.T) {
	usage := kiroUsageToClaude(kiropkg.Usage{
		InputTokens:  12,
		OutputTokens: 7,
		KiroCredits:  0.17,
	}, 0)

	require.Equal(t, 12, usage.InputTokens)
	require.Equal(t, 7, usage.OutputTokens)
	require.InDelta(t, 0.17, usage.KiroCredits, 0.000001)
}

func TestBuildRecordUsageLogPersistsPositiveKiroCredits(t *testing.T) {
	svc := &GatewayService{}
	log := svc.buildRecordUsageLog(
		context.Background(),
		&recordUsageCoreInput{},
		&ForwardResult{
			RequestID: "req_kiro_credits",
			Model:     "claude-sonnet-4-5",
			Usage: ClaudeUsage{
				InputTokens:  12,
				OutputTokens: 7,
				KiroCredits:  0.17,
			},
			Duration: time.Second,
		},
		&APIKey{ID: 10},
		&User{ID: 20},
		&Account{ID: 30},
		nil,
		"claude-sonnet-4-5",
		1,
		1,
		1,
		1,
		false,
		&CostBreakdown{},
		nil,
	)

	require.NotNil(t, log.KiroCredits)
	require.InDelta(t, 0.17, *log.KiroCredits, 0.000001)
}

func TestBuildRecordUsageLogOmitsZeroKiroCredits(t *testing.T) {
	svc := &GatewayService{}
	log := svc.buildRecordUsageLog(
		context.Background(),
		&recordUsageCoreInput{},
		&ForwardResult{
			RequestID: "req_kiro_zero_credits",
			Model:     "claude-sonnet-4-5",
			Usage: ClaudeUsage{
				InputTokens:  12,
				OutputTokens: 7,
			},
			Duration: time.Second,
		},
		&APIKey{ID: 10},
		&User{ID: 20},
		&Account{ID: 30},
		nil,
		"claude-sonnet-4-5",
		1,
		1,
		1,
		1,
		false,
		&CostBreakdown{},
		nil,
	)

	require.Nil(t, log.KiroCredits)
}
