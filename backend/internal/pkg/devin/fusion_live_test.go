//go:build live

package devin

import (
	"context"
	"strings"
	"testing"
	"time"
)

// Read-only entitlement/catalog diagnostic. Field numbers are from oh-my-pi
// 5d72f5d3f49a802e51d04c810002eb1830440172, discovery/devin-proto.ts.
// No user/email/org IDs, token values, balances, or raw responses are logged.
func TestDevinLiveFusionAvailability(t *testing.T) {
	client := newLiveDevinClient(t)
	token, err := liveDevinToken()
	if err != nil {
		t.Fatal(err)
	}
	if token == "" {
		t.Skip("no explicit Devin credential; Fusion availability NOT checked")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	status, err := client.unary(ctx, DefaultBaseURL, "/exa.seat_management_pb.SeatManagementService/GetUserStatus", pb(nil, 1, metadata(token, "", false)))
	liveDevinCheckError(t, err, "account capability lookup")
	plan := mustProto(t, status.data(2))
	if len(plan) == 0 {
		user := mustProto(t, status.data(1))
		period := mustProto(t, user.data(13))
		plan = mustProto(t, period.data(1))
	}
	if len(plan) > 0 {
		info := mustProto(t, plan.data(33))
		t.Logf("plan_name=%q tier=%d paid_features=%t CLI_access=%t", plan.text(2), plan.num(1), plan.num(32) != 0, info.num(2) != 0)
	} else {
		t.Log("server omitted plan capabilities; entitlement remains unknown")
	}
	catalog, err := client.unary(ctx, DefaultBaseURL, modelsPath, pb(nil, 1, metadata(token, "", true)))
	liveDevinCheckError(t, err, "unfiltered catalog lookup")
	matches, enabled := 0, 0
	for _, field := range catalog[1] {
		cfg := mustProto(t, field.b)
		id := strings.TrimSpace(cfg.text(22))
		label := strings.ToLower(cfg.text(1))
		if !strings.Contains(strings.ToLower(id), "fusion") && !strings.Contains(label, "fusion") {
			continue
		}
		matches++
		info := mustProto(t, cfg.data(23))
		display := info.num(22)
		disabled := cfg.num(4) != 0
		if !disabled && display != 4 && display != 6 {
			enabled++
		}
		if matches <= 5 {
			t.Logf("fusion_uid=%q disabled=%t display=%d harness_count=%d", id, disabled, display, len(info[20]))
		}
	}
	if matches > 5 {
		t.Logf("%d additional Fusion entries omitted from diagnostic output", matches-5)
	}
	t.Logf("unfiltered_catalog_entries=%d fusion_entries=%d enabled_visible_fusion_entries=%d", len(catalog[1]), matches, enabled)
	if enabled == 0 {
		t.Skip("account catalog exposes no enabled Fusion entry; Fusion inference/orchestration NOT validated")
	}
	t.Log("Fusion catalog entries exist; availability alone does not prove lead+sidekick orchestration")
}
