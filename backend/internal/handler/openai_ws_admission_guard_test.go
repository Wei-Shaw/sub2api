package handler

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestOpenAIWSFirstAdmissionNonMigratable(t *testing.T) {
	bound := func(id int64) func() int64 { return func() int64 { return id } }
	noHit := service.OpenAIAccountScheduleDecision{}
	cases := []struct {
		name        string
		switchCount int
		cont        bool
		mode        string
		decision    service.OpenAIAccountScheduleDecision
		selected    int64
		lookup      func() int64
		want        bool
	}{
		{"passthrough_continuation_bound_elsewhere", 0, true, service.OpenAIWSIngressModePassthrough, noHit, 802, bound(801), true},
		{"failover_reselection_is_exempt", 1, true, service.OpenAIWSIngressModePassthrough, noHit, 802, bound(801), false},
		{"first_frame_not_continuation", 0, false, service.OpenAIWSIngressModePassthrough, noHit, 802, bound(801), false},
		{"ctx_pool_can_rebuild_context", 0, true, service.OpenAIWSIngressModeCtxPool, noHit, 802, bound(801), false},
		{"http_bridge_can_rebuild_context", 0, true, service.OpenAIWSIngressModeHTTPBridge, noHit, 802, bound(801), false},
		{"sticky_session_hit_is_own_account", 0, true, service.OpenAIWSIngressModePassthrough, service.OpenAIAccountScheduleDecision{StickySessionHit: true}, 801, bound(801), false},
		{"previous_response_hit_is_own_account", 0, true, service.OpenAIWSIngressModePassthrough, service.OpenAIAccountScheduleDecision{StickyPreviousHit: true}, 801, bound(801), false},
		{"no_live_binding_is_new_session", 0, true, service.OpenAIWSIngressModePassthrough, noHit, 802, bound(0), false},
		{"bound_to_selected_account", 0, true, service.OpenAIWSIngressModePassthrough, noHit, 801, bound(801), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, openAIWSFirstAdmissionNonMigratable(tc.switchCount, tc.cont, tc.mode, tc.decision, tc.selected, tc.lookup))
		})
	}
}

func TestOpenAIWSStickyBindPolicyFor(t *testing.T) {
	preserved := service.OpenAIAccountScheduleDecision{StickyBindingPreserved: true}
	plain := service.OpenAIAccountScheduleDecision{}
	cases := []struct {
		name          string
		platform      string
		afterFailover bool
		profitVeto    bool
		decision      service.OpenAIAccountScheduleDecision
		want          service.StickyBindPolicy
	}{
		{"failover_migrates", service.PlatformOpenAI, true, false, plain, service.StickyBindPolicyMigrate},
		{"failover_wins_over_preserved", service.PlatformOpenAI, true, true, preserved, service.StickyBindPolicyMigrate},
		{"scheduler_preserved_binding", service.PlatformOpenAI, false, false, preserved, service.StickyBindPolicyPreserve},
		{"profit_veto_reselect", service.PlatformOpenAI, false, true, plain, service.StickyBindPolicyPreserve},
		{"plain_first_admission_is_legacy", service.PlatformOpenAI, false, false, plain, service.StickyBindPolicyLegacy},
		{"non_openai_failover_is_legacy", service.PlatformGrok, true, false, plain, service.StickyBindPolicyLegacy},
		{"non_openai_preserved_is_legacy", service.PlatformGrok, false, true, preserved, service.StickyBindPolicyLegacy},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, openAIWSStickyBindPolicyFor(tc.platform, tc.afterFailover, tc.profitVeto, tc.decision))
		})
	}
}

func TestOpenAIWSFirstAdmissionNonMigratable_SkipsLookupWhenPreconditionsFail(t *testing.T) {
	called := false
	lookup := func() int64 { called = true; return 801 }
	require.False(t, openAIWSFirstAdmissionNonMigratable(0, true, service.OpenAIWSIngressModeCtxPool, service.OpenAIAccountScheduleDecision{}, 802, lookup))
	require.False(t, called, "前置条件不满足时不读绑定")
}
