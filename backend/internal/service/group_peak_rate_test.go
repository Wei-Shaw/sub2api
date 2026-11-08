package service

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

func init() {
	// 测试固定全局时区为 UTC，确保判定可复现。
	_ = timezone.Init("UTC")
}

func newPeakGroup(enabled bool, start, end string, mult float64) *Group {
	return &Group{
		SubscriptionType:   "subscription",
		PeakRateEnabled:    enabled,
		PeakStart:          start,
		PeakEnd:            end,
		PeakRateMultiplier: mult,
	}
}

func at(hour, min int) time.Time {
	return time.Date(2026, 6, 29, hour, min, 0, 0, time.UTC)
}

func TestPeakMultiplierAt_DisabledOrUnconfigured(t *testing.T) {
	cases := []struct {
		name string
		g    *Group
	}{
		{"disabled", newPeakGroup(false, "14:00", "18:00", 3.0)},
		{"empty start", newPeakGroup(true, "", "18:00", 3.0)},
		{"empty end", newPeakGroup(true, "14:00", "", 3.0)},
		{"invalid start>=end", newPeakGroup(true, "18:00", "14:00", 3.0)},
		{"equal start==end", newPeakGroup(true, "14:00", "14:00", 3.0)},
		{"malformed start", newPeakGroup(true, "99:99", "18:00", 3.0)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.g.PeakMultiplierAt(at(15, 0)); got != 1.0 {
				t.Fatalf("expect 1.0, got %v", got)
			}
		})
	}
}

func TestPeakMultiplierAt_NilReceiver(t *testing.T) {
	var g *Group
	if got := g.PeakMultiplierAt(at(15, 0)); got != 1.0 {
		t.Fatalf("expect 1.0, got %v", got)
	}
}

func TestPeakMultiplierAt_Boundaries(t *testing.T) {
	g := newPeakGroup(true, "14:00", "18:00", 3.0)
	cases := []struct {
		t    time.Time
		want float64
	}{
		{at(13, 59), 1.0},
		{at(14, 0), 3.0},
		{at(15, 30), 3.0},
		{at(17, 59), 3.0},
		{at(18, 0), 1.0},
		{at(23, 0), 1.0},
	}
	for _, c := range cases {
		t.Run(c.t.Format("15:04"), func(t *testing.T) {
			if got := g.PeakMultiplierAt(c.t); got != c.want {
				t.Fatalf("at %s: expect %v, got %v", c.t.Format("15:04"), c.want, got)
			}
		})
	}
}

func TestPeakMultiplierAt_RespectsTimezoneLocation(t *testing.T) {
	// 全局时区为 UTC。北京 15:00 = UTC 07:00，不在 [14:00,18:00)。
	nonUTC := time.Date(2026, 6, 29, 15, 0, 0, 0, mustLoad("Asia/Shanghai"))
	g := newPeakGroup(true, "14:00", "18:00", 3.0)
	if got := g.PeakMultiplierAt(nonUTC); got != 1.0 {
		t.Fatalf("expect 1.0 (converted to UTC 07:00), got %v", got)
	}
}

func mustLoad(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return loc
}

func TestValidatePeakRateConfig(t *testing.T) {
	cases := []struct {
		name    string
		subType string
		enabled bool
		start   string
		end     string
		mult    float64
		wantErr bool
	}{
		{"disabled passes through", "subscription", false, "", "", 0, false},
		{"subscription enabled valid", "subscription", true, "14:00", "18:00", 3.0, false},
		{"standard enabled rejected", "standard", true, "14:00", "18:00", 3.0, true},
		{"empty type treated as standard", "", true, "14:00", "18:00", 3.0, true},
		{"standard disabled passes", "standard", false, "", "", 0, false},
		{"enabled empty start", "subscription", true, "", "18:00", 1.0, true},
		{"enabled empty end", "subscription", true, "14:00", "", 1.0, true},
		{"enabled malformed start", "subscription", true, "99:99", "18:00", 1.0, true},
		{"enabled malformed end", "subscription", true, "14:00", "25:00", 1.0, true},
		{"enabled equal start==end", "subscription", true, "14:00", "14:00", 1.0, true},
		{"enabled cross-day rejected", "subscription", true, "22:00", "02:00", 1.0, true},
		{"enabled negative multiplier", "subscription", true, "14:00", "18:00", -0.5, true},
		{"enabled zero multiplier allowed", "subscription", true, "14:00", "18:00", 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ValidatePeakRateConfig(c.subType, c.enabled, c.start, c.end, c.mult)
			if c.wantErr && err == nil {
				t.Fatalf("expect error, got nil")
			}
			if !c.wantErr && err != nil {
				t.Fatalf("expect no error, got %v", err)
			}
		})
	}
}

func TestPeakMultiplierAt_StandardTypeDegradesToOne(t *testing.T) {
	g := newPeakGroup(true, "14:00", "18:00", 3.0)
	g.SubscriptionType = "standard"
	if got := g.PeakMultiplierAt(at(15, 30)); got != 1.0 {
		t.Fatalf("standard group must degrade to 1.0, got %v", got)
	}

	sub := newPeakGroup(true, "14:00", "18:00", 3.0)
	sub.SubscriptionType = "subscription"
	if got := sub.PeakMultiplierAt(at(15, 30)); got != 3.0 {
		t.Fatalf("subscription group peak multiplier: got %v, want 3.0", got)
	}
}

// TestPeakMultiplier_GatewayBillingSequence 调用 gateway_service.recordUsageCore 与
// openai_gateway_service.RecordUsage 共用的 computePeakAwareMultipliers，验证计费叠加顺序：
// 图片按次倍率基于基础倍率算出且不受高峰影响，高峰因子只乘入 token 倍率。
// 若有人调换叠加顺序或把高峰并入 imageMultiplier，此测试会失败。
func TestPeakMultiplier_GatewayBillingSequence(t *testing.T) {
	const baseMultiplier = 0.8
	apiKey := &APIKey{Group: newPeakGroup(true, "14:00", "18:00", 3.0)}
	approxEq := func(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

	t.Run("peak hour amplifies token multiplier only", func(t *testing.T) {
		now := at(15, 30) // 处于 [14:00, 18:00)
		tokenMultiplier, imageMultiplier := computePeakAwareMultipliers(apiKey, baseMultiplier, now)
		if !approxEq(imageMultiplier, baseMultiplier) {
			t.Fatalf("image multiplier must not be affected by peak: got %v, want %v", imageMultiplier, baseMultiplier)
		}
		if want := baseMultiplier * 3.0; !approxEq(tokenMultiplier, want) {
			t.Fatalf("token multiplier should include peak factor: got %v, want %v", tokenMultiplier, want)
		}
	})

	t.Run("off-peak leaves both multipliers at base", func(t *testing.T) {
		now := at(20, 0)
		tokenMultiplier, imageMultiplier := computePeakAwareMultipliers(apiKey, baseMultiplier, now)
		if !approxEq(imageMultiplier, baseMultiplier) {
			t.Fatalf("image multiplier: got %v, want %v", imageMultiplier, baseMultiplier)
		}
		if !approxEq(tokenMultiplier, baseMultiplier) {
			t.Fatalf("token multiplier should equal base off-peak: got %v, want %v", tokenMultiplier, baseMultiplier)
		}
	})

	t.Run("image independent mode decoupled from peak", func(t *testing.T) {
		indGroup := newPeakGroup(true, "14:00", "18:00", 3.0)
		indGroup.ImageRateIndependent = true
		indGroup.ImageRateMultiplier = 0.5
		indKey := &APIKey{Group: indGroup}
		now := at(15, 30)
		tokenMultiplier, imageMultiplier := computePeakAwareMultipliers(indKey, baseMultiplier, now)
		if !approxEq(imageMultiplier, 0.5) {
			t.Fatalf("independent image multiplier: got %v, want 0.5", imageMultiplier)
		}
		if want := baseMultiplier * 3.0; !approxEq(tokenMultiplier, want) {
			t.Fatalf("token multiplier should include peak factor: got %v, want %v", tokenMultiplier, want)
		}
	})

	t.Run("nil api key degrades to base multipliers", func(t *testing.T) {
		now := at(15, 30)
		tokenMultiplier, imageMultiplier := computePeakAwareMultipliers(nil, baseMultiplier, now)
		if !approxEq(tokenMultiplier, baseMultiplier) {
			t.Fatalf("nil group token multiplier: got %v, want %v", tokenMultiplier, baseMultiplier)
		}
		if !approxEq(imageMultiplier, baseMultiplier) {
			t.Fatalf("nil group image multiplier: got %v, want %v", imageMultiplier, baseMultiplier)
		}
	})
}

// TestPeakMultiplier_SnapshotRoundTrip 防回归：认证缓存快照（APIKeyAuthGroupSnapshot）
// 必须携带高峰倍率 4 字段，否则扣费路径拿到的 apiKey.Group 会缺字段、PeakMultiplierAt 恒降级为 1.0。
// 调用真实链路 snapshotFromAPIKey → snapshotToAPIKey，验证 peak 配置经快照往返后仍生效。
func TestPeakMultiplier_SnapshotRoundTrip(t *testing.T) {
	apiKey := &APIKey{
		User:  &User{ID: 1, Status: StatusActive, Role: RoleUser},
		Group: newPeakGroup(true, "14:00", "18:00", 3.0),
	}
	svc := &APIKeyService{}

	snapshot := svc.snapshotFromAPIKey(context.Background(), apiKey)
	if snapshot == nil || snapshot.Group == nil {
		t.Fatalf("snapshot or snapshot.Group must not be nil")
	}
	restored := svc.snapshotToAPIKey("k", snapshot)
	if restored.Group == nil {
		t.Fatalf("restored.Group must not be nil")
	}

	if !restored.Group.PeakRateEnabled ||
		restored.Group.PeakStart != "14:00" ||
		restored.Group.PeakEnd != "18:00" ||
		restored.Group.PeakRateMultiplier != 3.0 {
		t.Fatalf("peak fields lost in snapshot round-trip: %+v", restored.Group)
	}
	if got := restored.Group.PeakMultiplierAt(at(15, 30)); got != 3.0 {
		t.Fatalf("peak hour multiplier after round-trip: got %v, want 3.0", got)
	}
	if got := restored.Group.PeakMultiplierAt(at(20, 0)); got != 1.0 {
		t.Fatalf("off-peak multiplier after round-trip: got %v, want 1.0", got)
	}
}
