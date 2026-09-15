// devin_gateway_service_test.go 覆盖 Devin 网关服务的账号适配层：
// adapter 按账号指纹缓存复用（catalog/AssignModel 缓存存活的前提）、
// devin "model:level" 语法在 model_mapping/分组白名单中的归一化语义。
package service

import (
	"testing"
	"time"

	devinadapter "github.com/Wei-Shaw/sub2api/internal/pkg/devin/adapter"
)

// newTestDevinAccount 构造一个最小可用的 Devin OAuth 账号。
func newTestDevinAccount(token string) *Account {
	return &Account{
		ID:          1,
		Platform:    PlatformDevin,
		Type:        AccountTypeOAuth,
		Concurrency: 5,
		Credentials: map[string]any{
			"access_token":   token,
			"api_server_url": "https://server.codeium.example",
			"client_version": "v0.1.0",
		},
	}
}

func newTestDevinGatewayService() *DevinGatewayService {
	return NewDevinGatewayService(nil, nil)
}

// TestDevinAdapterForAccountValidation 校验非 devin 账号与缺失凭据的拒绝路径。
func TestDevinAdapterForAccountValidation(t *testing.T) {
	svc := newTestDevinGatewayService()

	if _, err := svc.adapterForAccount(nil); err == nil {
		t.Fatal("nil account should fail")
	}
	if _, err := svc.adapterForAccount(&Account{Platform: PlatformOpenAI}); err == nil {
		t.Fatal("non-devin account should fail")
	}
	noToken := newTestDevinAccount("")
	if _, err := svc.adapterForAccount(noToken); err == nil {
		t.Fatal("account without access_token should fail")
	}
}

// TestDevinAdapterCacheReuse 同账号同凭据第二次取回同一 adapter 实例——
// 这是 catalog(5min TTL) 与 AssignModel 缓存能够命中的前提。
func TestDevinAdapterCacheReuse(t *testing.T) {
	svc := newTestDevinGatewayService()
	account := newTestDevinAccount("devin-session-token$abc")

	first, err := svc.adapterForAccount(account)
	if err != nil {
		t.Fatalf("adapterForAccount: %v", err)
	}
	second, err := svc.adapterForAccount(account)
	if err != nil {
		t.Fatalf("adapterForAccount again: %v", err)
	}
	if first != second {
		t.Fatal("same fingerprint should reuse the cached adapter")
	}
	if len(svc.adapters) != 1 {
		t.Fatalf("expected 1 cached adapter, got %d", len(svc.adapters))
	}
}

// TestDevinAdapterCacheFingerprintRotation 凭据/地址/并发漂移必须换指纹。
func TestDevinAdapterCacheFingerprintRotation(t *testing.T) {
	svc := newTestDevinGatewayService()
	account := newTestDevinAccount("devin-session-token$abc")

	first, err := svc.adapterForAccount(account)
	if err != nil {
		t.Fatalf("adapterForAccount: %v", err)
	}

	// token 轮换 → 新 adapter。
	account.Credentials["access_token"] = "devin-session-token$rotated"
	second, err := svc.adapterForAccount(account)
	if err != nil {
		t.Fatalf("adapterForAccount rotated: %v", err)
	}
	if second == first {
		t.Fatal("token rotation must produce a new adapter")
	}

	// base_url 变化 → 新 adapter。
	account.Credentials["api_server_url"] = "https://other.codeium.example"
	third, err := svc.adapterForAccount(account)
	if err != nil {
		t.Fatalf("adapterForAccount base url change: %v", err)
	}
	if third == second {
		t.Fatal("base_url change must produce a new adapter")
	}
}

// TestDevinAdapterCacheIdleExpiry 空闲超 TTL 的条目重建。
func TestDevinAdapterCacheIdleExpiry(t *testing.T) {
	svc := newTestDevinGatewayService()
	account := newTestDevinAccount("devin-session-token$abc")

	first, err := svc.adapterForAccount(account)
	if err != nil {
		t.Fatalf("adapterForAccount: %v", err)
	}
	// 直接把条目标记为超龄。
	for k, e := range svc.adapters {
		e.lastUsedAt = time.Now().Add(-devinAdapterIdleTTL - time.Minute)
		svc.adapters[k] = e
	}
	second, err := svc.adapterForAccount(account)
	if err != nil {
		t.Fatalf("adapterForAccount after expiry: %v", err)
	}
	if second == first {
		t.Fatal("expired adapter should have been rebuilt")
	}
	if len(svc.adapters) != 1 {
		t.Fatalf("stale entry should be evicted, got %d adapters", len(svc.adapters))
	}
}

// TestDevinAdapterCacheCap 容量上限触发最久未用逐出。
func TestDevinAdapterCacheCap(t *testing.T) {
	svc := newTestDevinGatewayService()

	// 灌满缓存。
	filler := newTestDevinAccount("devin-session-token$old")
	oldAdapter, err := svc.adapterForAccount(filler)
	if err != nil {
		t.Fatalf("adapterForAccount: %v", err)
	}
	for i := 0; i < devinAdapterCacheCap; i++ {
		key := string(rune('a'+i%26)) + string(rune('A'+i/26)) + "|fake"
		svc.adapters[key] = cachedDevinAdapter{adapter: oldAdapter, lastUsedAt: time.Now()}
	}
	// 老条目标为最久未用。
	for k, e := range svc.adapters {
		e.lastUsedAt = time.Now().Add(-time.Minute)
		svc.adapters[k] = e
		break // 只标第一个真实 key——其实全部同刻也行，逐出任意一个即可
	}

	fresh := newTestDevinAccount("devin-session-token$new")
	fresh.ID = 999
	if _, err := svc.adapterForAccount(fresh); err != nil {
		t.Fatalf("adapterForAccount fresh: %v", err)
	}
	if len(svc.adapters) > devinAdapterCacheCap {
		t.Fatalf("cache exceeded cap: %d", len(svc.adapters))
	}
}

// 防止 unused import。
var _ = devinadapter.New

// --- model:level 语法与 model_mapping / 分组白名单 ---

// TestDevinIsModelSupportedLevelSuffix devin 请求模型带 :level 后缀时按
// 基础模型名查 model_mapping。
func TestDevinIsModelSupportedLevelSuffix(t *testing.T) {
	account := newTestDevinAccount("devin-session-token$abc")
	account.Credentials["model_mapping"] = map[string]any{"swe-2": "swe-2"}

	cases := []struct {
		model string
		want  bool
	}{
		{"swe-2", true},
		{"swe-2:max", true}, // 档位后缀不参与白名单键匹配
		{"swe-2:low", true},
		{"swe-2:garbage", false}, // 非档位后缀不剥，整名查不到
		{"gpt-6-astra", false},
		{"gpt-6-astra:xhigh", false},
		{"swe-2:", false},
	}
	for _, c := range cases {
		if got := account.IsModelSupported(c.model); got != c.want {
			t.Errorf("IsModelSupported(%q) = %v, want %v", c.model, got, c.want)
		}
	}
}

// TestDevinResolveMappedModelLevelPreserved 归一化命中映射后 :level 必须
// 保留回映射结果（除非映射值自带 level 覆盖）。
func TestDevinResolveMappedModelLevelPreserved(t *testing.T) {
	account := newTestDevinAccount("devin-session-token$abc")
	account.Credentials["model_mapping"] = map[string]any{
		"swe-2":      "swe-2-renamed",
		"swe-2-lite": "swe-2:high",
	}

	cases := []struct {
		model, want string
	}{
		{"swe-2", "swe-2-renamed"},
		{"swe-2:max", "swe-2-renamed:max"}, // 档位保留
		{"swe-2-lite:low", "swe-2:high"},   // 映射值自带 level → 覆盖请求档位
	}
	for _, c := range cases {
		got, matched := account.ResolveMappedModel(c.model)
		if !matched {
			t.Errorf("ResolveMappedModel(%q) should match", c.model)
			continue
		}
		if got != c.want {
			t.Errorf("ResolveMappedModel(%q) = %q, want %q", c.model, got, c.want)
		}
	}
}

// TestDevinGroupAllowlistCandidates 分组白名单应把 :level 基础名列为候选。
func TestDevinGroupAllowlistCandidates(t *testing.T) {
	candidates := groupModelAllowlistCandidates("swe-2:max")
	found := false
	for _, c := range candidates {
		if c == "swe-2" {
			found = true
		}
	}
	if !found {
		t.Fatalf("candidates %v missing level-stripped base 'swe-2'", candidates)
	}
	// 含后缀的原始名也应保留（白名单若显式写了带档位条目）。
	found = false
	for _, c := range candidates {
		if c == "swe-2:max" {
			found = true
		}
	}
	if !found {
		t.Fatalf("candidates %v missing original 'swe-2:max'", candidates)
	}
}
