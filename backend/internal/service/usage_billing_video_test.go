package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
)

type videoUsageAtomicBillingRepoStub struct {
	openAIRecordUsageBillingRepoStub
}

func (r *videoUsageAtomicBillingRepoStub) ApplyVideo(ctx context.Context, cmd *UsageBillingCommand, usageLog *UsageLog) (*UsageBillingApplyResult, error) {
	cmd.VideoUsageLog = usageLog
	return r.Apply(ctx, cmd)
}

func TestVideoRecordUsage_AtomicPath(t *testing.T) {
	insertErr := errors.New("video log insert rejected")
	for _, tc := range []struct {
		name         string
		applied      bool
		err          error
		noRepo       bool
		ordinaryRepo bool
	}{
		{name: "committed", applied: true},
		{name: "duplicate"},
		{name: "insert_failure", err: insertErr},
		{name: "repository_unavailable", noRepo: true},
		{name: "repository_without_atomic_support", ordinaryRepo: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
			userRepo := &openAIRecordUsageUserRepoStub{}
			billingRepo := &videoUsageAtomicBillingRepoStub{
				openAIRecordUsageBillingRepoStub: openAIRecordUsageBillingRepoStub{
					result: &UsageBillingApplyResult{Applied: tc.applied}, err: tc.err,
				},
			}
			svc := newOpenAIRecordUsageServiceWithBillingRepoForTest(usageRepo, billingRepo, userRepo, &openAIRecordUsageSubRepoStub{}, nil)
			if tc.noRepo {
				svc.usageBillingRepo = nil
			}
			if tc.ordinaryRepo {
				svc.usageBillingRepo = &billingRepo.openAIRecordUsageBillingRepoStub
			}
			ctx, cancel := context.WithCancel(context.WithValue(context.Background(), ctxkey.ClientRequestID, "poll-id"))
			cancel()
			input := &OpenAIRecordUsageInput{
				RequireAtomicVideo: true,
				Result: &OpenAIForwardResult{
					RequestID: "grok-video:task-1", Model: "grok-imagine-video-1.5",
					VideoCount: 1, VideoResolution: VideoBillingResolution720P, VideoDurationSeconds: 5,
				},
				APIKey: &APIKey{ID: 2}, User: &User{ID: 1}, Account: &Account{ID: 3, Platform: PlatformGrok},
			}
			err := svc.RecordUsage(ctx, input)
			if tc.noRepo {
				require.ErrorContains(t, err, "repository is required")
				require.Zero(t, billingRepo.calls)
			} else if tc.ordinaryRepo {
				require.ErrorContains(t, err, "does not support atomic video")
				require.Zero(t, billingRepo.calls)
			} else {
				if tc.err != nil {
					require.ErrorIs(t, err, tc.err)
				} else {
					require.NoError(t, err)
				}
				require.Equal(t, 1, billingRepo.calls)
				require.NoError(t, billingRepo.lastCtxErr)
				cmd := billingRepo.lastCmd
				require.Equal(t, "grok-video:task-1", cmd.RequestID)
				require.Empty(t, cmd.RequestPayloadHash, "poll context IDs must not become task payload fingerprints")
				require.NotNil(t, cmd.VideoUsageLog)
				require.Equal(t, cmd.RequestID, cmd.VideoUsageLog.RequestID)
				require.Equal(t, 1, cmd.VideoUsageLog.VideoCount)
				require.Greater(t, cmd.VideoUsageLog.ActualCost, 0.0, "failed transactions must not replace the row with zero cost")
				require.Equal(t, cmd.BalanceCost, QuantizeUsageBillingAmount(cmd.VideoUsageLog.ActualCost))
				fingerprint := cmd.RequestFingerprint
				cmd.VideoUsageLog = nil
				require.Equal(t, fingerprint, buildUsageBillingFingerprint(cmd), "attaching the row must preserve existing dedup fingerprints")
			}
			require.Zero(t, usageRepo.calls, "video logs must not escape the billing transaction, including errors and duplicates")
			require.Zero(t, userRepo.deductCalls, "video billing must never fall back to standalone deductions")
		})
	}
}

func TestVideoRecordUsage_RequiresVideoResult(t *testing.T) {
	svc := &OpenAIGatewayService{}
	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		RequireAtomicVideo: true,
		Result:             &OpenAIForwardResult{Model: "gpt-5.1"},
	})
	require.ErrorContains(t, err, "requires a completed video result")
}

func TestVideoRecordUsage_PollContextDoesNotChangeFingerprint(t *testing.T) {
	for _, payloadHash := range []string{"", "saved-task-payload-hash"} {
		usageRepo := &openAIRecordUsageLogRepoStub{}
		billingRepo := &videoUsageAtomicBillingRepoStub{
			openAIRecordUsageBillingRepoStub: openAIRecordUsageBillingRepoStub{
				result: &UsageBillingApplyResult{Applied: false},
			},
		}
		svc := newOpenAIRecordUsageServiceWithBillingRepoForTest(usageRepo, billingRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)
		var fingerprint string
		for _, pollID := range []string{"status-poll", "content-poll"} {
			ctx := context.WithValue(context.Background(), ctxkey.ClientRequestID, pollID)
			err := svc.RecordUsage(ctx, &OpenAIRecordUsageInput{
				RequireAtomicVideo: true, RequestPayloadHash: payloadHash,
				Result: &OpenAIForwardResult{
					RequestID: "grok-video:stable-task", Model: "grok-imagine-video-1.5", VideoCount: 1,
				},
				APIKey: &APIKey{ID: 2}, User: &User{ID: 1}, Account: &Account{ID: 3, Platform: PlatformGrok},
			})
			require.NoError(t, err)
			require.Equal(t, "grok-video:stable-task", billingRepo.lastCmd.RequestID)
			require.Equal(t, payloadHash, billingRepo.lastCmd.RequestPayloadHash)
			if fingerprint != "" {
				require.Equal(t, fingerprint, billingRepo.lastCmd.RequestFingerprint)
			}
			fingerprint = billingRepo.lastCmd.RequestFingerprint
		}
		require.Zero(t, usageRepo.calls)
	}
}

func TestVideoRecordUsage_LegacyVideoDoesNotRequireAtomicRepository(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	userRepo := &openAIRecordUsageUserRepoStub{}
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, &openAIRecordUsageSubRepoStub{}, nil)
	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{RequestID: "legacy-video", Model: "grok-imagine-video-1.5", VideoCount: 1},
		APIKey: &APIKey{ID: 2}, User: &User{ID: 1}, Account: &Account{ID: 3, Platform: PlatformGrok},
	})
	require.NoError(t, err)
	require.Equal(t, 1, usageRepo.calls)
	require.Equal(t, 1, userRepo.deductCalls)
}

func TestVideoRecordUsage_TextAndImageKeepBestEffortLogging(t *testing.T) {
	for _, image := range []bool{false, true} {
		name := "text"
		if image {
			name = "image"
		}
		t.Run(name, func(t *testing.T) {
			for _, failBilling := range []bool{false, true} {
				usageRepo := &openAIRecordUsageLogRepoStub{err: errors.New("log write failed")}
				billingRepo := &openAIRecordUsageBillingRepoStub{}
				if failBilling {
					billingRepo.err = errors.New("billing failed")
				}
				svc := newOpenAIRecordUsageServiceWithBillingRepoForTest(usageRepo, billingRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)
				result := &OpenAIForwardResult{RequestID: "ordinary", Model: "gpt-5.1", Usage: OpenAIUsage{InputTokens: 100, OutputTokens: 50}}
				if image {
					result.Model = "gpt-image-2"
					result.ImageCount = 1
					result.ImageSize = "1K"
				}
				err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
					Result: result, APIKey: &APIKey{ID: 2}, User: &User{ID: 1}, Account: &Account{ID: 3},
				})
				if failBilling {
					require.ErrorIs(t, err, billingRepo.err)
					require.Zero(t, usageRepo.lastLog.ActualCost)
				} else {
					require.NoError(t, err, "non-video log errors retain their existing best-effort semantics")
				}
				require.Equal(t, 1, billingRepo.calls)
				require.Nil(t, billingRepo.lastCmd.VideoUsageLog)
				require.Equal(t, 1, usageRepo.calls)
			}
		})
	}
}
