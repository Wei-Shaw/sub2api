package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type emptyStreamCooldownRepo struct {
	AccountRepository
	account *Account
	cleared []int64
}

func (r *emptyStreamCooldownRepo) GetByID(_ context.Context, _ int64) (*Account, error) {
	return r.account, nil
}

func (r *emptyStreamCooldownRepo) ClearTempUnschedulable(_ context.Context, id int64) error {
	r.cleared = append(r.cleared, id)
	return nil
}

func TestReleaseEmptyStreamTempUnschedule(t *testing.T) {
	future := time.Now().Add(time.Minute)
	past := time.Now().Add(-time.Minute)

	for _, tc := range []struct {
		name    string
		account *Account
		want    bool
	}{
		{"active empty-stream cooldown is cleared", &Account{ID: 13, TempUnschedulableUntil: &future, TempUnschedulableReason: emptyResponseCooldownReason}, true},
		{"another reason is kept", &Account{ID: 13, TempUnschedulableUntil: &future, TempUnschedulableReason: "google config error"}, false},
		{"expired cooldown is left alone", &Account{ID: 13, TempUnschedulableUntil: &past, TempUnschedulableReason: emptyResponseCooldownReason}, false},
		{"no cooldown", &Account{ID: 13}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &emptyStreamCooldownRepo{account: tc.account}
			svc := &GatewayService{accountRepo: repo}
			require.Equal(t, tc.want, svc.ReleaseEmptyStreamTempUnschedule(context.Background(), 13))
			if tc.want {
				require.Equal(t, []int64{13}, repo.cleared)
			} else {
				require.Empty(t, repo.cleared)
			}
		})
	}
}

func TestAntigravityCompatEmptyStreamErrorIsMarkedEmptyStream(t *testing.T) {
	err, ok := antigravityCompatEmptyStreamError().(*UpstreamFailoverError)
	require.True(t, ok)
	require.True(t, err.EmptyStream)
	require.False(t, antigravityMalformedFunctionCallFailoverError().EmptyStream, "malformed calls are already request-scoped")
}

func TestAntigravityCompatSessionKeepsUpstreamTail(t *testing.T) {
	s := &antigravityCompatStreamSession{}
	s.recordUpstreamLine(": ping")
	s.recordUpstreamLine("")
	s.recordUpstreamLine(`data: {"a":1}`)
	s.recordUpstreamLine(`data: {"a":2}`)
	s.recordUpstreamLine(`data: {"a":3}`)
	s.recordUpstreamLine("data: " + strings.Repeat("x", 2000))

	require.Equal(t, 4, s.upstreamDataLines)
	require.Len(t, s.upstreamTail, antigravityCompatTailLines)
	require.Equal(t, `data: {"a":2}`, s.upstreamTail[0])
	require.LessOrEqual(t, len(s.upstreamTail[2]), antigravityCompatTailLineBytes+3)
	require.True(t, strings.HasSuffix(s.upstreamTail[2], "..."))
}
