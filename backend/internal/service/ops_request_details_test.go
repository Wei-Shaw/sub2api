package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type opsRequestDetailsRepoStub struct {
	opsRepoMock
	items []*OpsRequestDetail
	total int64
	err   error
}

func (s *opsRequestDetailsRepoStub) ListRequestDetails(ctx context.Context, filter *OpsRequestDetailFilter) ([]*OpsRequestDetail, int64, error) {
	return s.items, s.total, s.err
}

func TestOpsServiceListRequestDetails_PreservesRoutingFields(t *testing.T) {
	t.Parallel()

	repo := &opsRequestDetailsRepoStub{
		items: []*OpsRequestDetail{{
			Kind:                         OpsRequestKindSuccess,
			CreatedAt:                    time.Date(2026, 4, 4, 12, 0, 0, 0, time.UTC),
			RequestID:                    "req-routing-1",
			Platform:                     PlatformOpenAI,
			Model:                        "gpt-5.4-Sys",
			RoutingTargetGroup:           "exhausted",
			RoutingSelectedGroup:         "reserve",
			RoutingScheduleLayer:         "load_balance",
			RoutingSelectedAccountID:     i64p(66),
			RoutingSelectedAccountName:   strPtr("acc-66"),
			RoutingEffectiveModel:        "gpt-5.4",
			RoutingFailoverCount:         intValuePtr(1),
			RoutingFailoverFinalReason:   "upstream_502",
			StickySessionSource:          strPtr("header_x_session_affinity"),
			StickySessionHashPresent:     boolPtr(true),
			StickyEvalResult:             strPtr("hit"),
			StickySelectedAccountChanged: boolPtr(false),
			StickyParentSessionPresent:   boolPtr(true),
			StickyParentSessionKey:       strPtr("parent_abc"),
		}},
		total: 1,
	}
	svc := NewOpsService(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	out, err := svc.ListRequestDetails(context.Background(), &OpsRequestDetailFilter{Page: 1, PageSize: 10})
	require.NoError(t, err)
	require.Len(t, out.Items, 1)
	require.Equal(t, "exhausted", out.Items[0].RoutingTargetGroup)
	require.Equal(t, "reserve", out.Items[0].RoutingSelectedGroup)
	require.Equal(t, "load_balance", out.Items[0].RoutingScheduleLayer)
	require.NotNil(t, out.Items[0].RoutingSelectedAccountID)
	require.Equal(t, int64(66), *out.Items[0].RoutingSelectedAccountID)
	require.NotNil(t, out.Items[0].RoutingSelectedAccountName)
	require.Equal(t, "acc-66", *out.Items[0].RoutingSelectedAccountName)
	require.Equal(t, "gpt-5.4", out.Items[0].RoutingEffectiveModel)
	require.NotNil(t, out.Items[0].RoutingFailoverCount)
	require.Equal(t, 1, *out.Items[0].RoutingFailoverCount)
	require.Equal(t, "upstream_502", out.Items[0].RoutingFailoverFinalReason)
	require.NotNil(t, out.Items[0].StickySessionSource)
	require.Equal(t, "header_x_session_affinity", *out.Items[0].StickySessionSource)
	require.NotNil(t, out.Items[0].StickySessionHashPresent)
	require.True(t, *out.Items[0].StickySessionHashPresent)
	require.NotNil(t, out.Items[0].StickyEvalResult)
	require.Equal(t, "hit", *out.Items[0].StickyEvalResult)
	require.NotNil(t, out.Items[0].StickySelectedAccountChanged)
	require.False(t, *out.Items[0].StickySelectedAccountChanged)
	require.NotNil(t, out.Items[0].StickyParentSessionPresent)
	require.True(t, *out.Items[0].StickyParentSessionPresent)
	require.NotNil(t, out.Items[0].StickyParentSessionKey)
	require.Equal(t, "parent_abc", *out.Items[0].StickyParentSessionKey)
}

func intValuePtr(v int) *int {
	return &v
}
