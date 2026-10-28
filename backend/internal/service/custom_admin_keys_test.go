//go:build unit

package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/adminaccess"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

func TestScopedAdminKeyValidation(t *testing.T) {
	input := ScopedAdminKeyInput{Name: "测试", Enabled: true, AllowedIPs: []string{"192.0.2.0/24"}, Policy: adminaccess.Policy{}}
	if err := validateScopedAdminKeyInput(&input); err != nil {
		t.Fatal(err)
	}
	input.AllowedIPs = []string{"invalid"}
	if validateScopedAdminKeyInput(&input) == nil {
		t.Fatal("应拒绝无效 IP")
	}
	input.AllowedIPs = nil
	past := time.Now().Add(-time.Hour)
	input.ExpiresAt = &past
	if validateScopedAdminKeyInput(&input) == nil {
		t.Fatal("应拒绝过去的有效期")
	}
}
func TestScopedAdminKeySecret(t *testing.T) {
	key, hash, err := generateScopedAdminKey("abc")
	if err != nil {
		t.Fatal(err)
	}
	if key == hash || !verifyScopedAdminKey(key, hash) || verifyScopedAdminKey(key+"x", hash) {
		t.Fatal("密钥摘要校验错误")
	}
	if !scopedAdminIPAllowed([]string{"192.0.2.0/24"}, "192.0.2.5") || scopedAdminIPAllowed([]string{"192.0.2.0/24"}, "198.51.100.1") {
		t.Fatal("IP 范围错误")
	}
}

type scopedKeyMemoryRepo struct {
	item    *ScopedAdminKey
	deleted bool
}

func (r *scopedKeyMemoryRepo) ListScopedAdminKeys(context.Context) ([]*ScopedAdminKey, error) {
	if r.deleted {
		return []*ScopedAdminKey{}, nil
	}
	return []*ScopedAdminKey{r.item}, nil
}
func (r *scopedKeyMemoryRepo) GetScopedAdminKey(_ context.Context, id string) (*ScopedAdminKey, error) {
	if r.item == nil || r.item.ID != id {
		return nil, ErrScopedAdminKeyNotFound
	}
	copy := *r.item
	return &copy, nil
}
func (r *scopedKeyMemoryRepo) InsertScopedAdminKey(_ context.Context, k *ScopedAdminKey) error {
	copy := *k
	r.item = &copy
	return nil
}
func (r *scopedKeyMemoryRepo) UpdateScopedAdminKey(_ context.Context, k *ScopedAdminKey) error {
	copy := *k
	r.item = &copy
	return nil
}
func (r *scopedKeyMemoryRepo) TouchScopedAdminKey(context.Context, string, string, time.Time, string, time.Time) error {
	return nil
}
func (r *scopedKeyMemoryRepo) RevokeScopedAdminKey(_ context.Context, _ string, at time.Time) error {
	r.item.RevokedAt = &at
	return nil
}

func (r *scopedKeyMemoryRepo) DeleteScopedAdminKeyRecord(_ context.Context, id string, _ time.Time) error {
	if r.item == nil || r.item.ID != id || r.item.RevokedAt == nil || r.deleted {
		return ErrScopedAdminKeyNotFound
	}
	r.deleted = true
	return nil
}
func TestScopedAdminKeyLifecycle(t *testing.T) {
	repo := &scopedKeyMemoryRepo{}
	s := &SettingService{scopedAdminKeyRepo: repo}
	ctx := context.Background()
	key, item, err := s.CreateScopedAdminKey(ctx, ScopedAdminKeyInput{Name: "集成", Enabled: true, AllowedIPs: []string{"192.0.2.0/24"}}, 42)
	if err != nil {
		t.Fatal(err)
	}
	if item.CreatedBy != 42 || strings.Contains(item.Hash, key) {
		t.Fatal("存储不应包含原始密钥")
	}
	if _, err = s.AuthenticateScopedAdminKey(ctx, key, "192.0.2.1"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.AuthenticateScopedAdminKey(ctx, key, "198.51.100.1"); err == nil {
		t.Fatal("必须验证 IP")
	}
	next, _, err := s.RotateScopedAdminKey(ctx, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.AuthenticateScopedAdminKey(ctx, key, "192.0.2.1"); err == nil {
		t.Fatal("轮换必须即时撤销旧密钥")
	}
	if _, err = s.AuthenticateScopedAdminKey(ctx, next, "192.0.2.1"); err != nil {
		t.Fatal(err)
	}
	repo.item.Enabled = false
	if _, err = s.AuthenticateScopedAdminKey(ctx, next, "192.0.2.1"); err == nil {
		t.Fatal("停用必须生效")
	}
	repo.item.Enabled = true
	if err = s.RevokeScopedAdminKey(ctx, item.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.AuthenticateScopedAdminKey(ctx, next, "192.0.2.1"); err == nil {
		t.Fatal("撤销必须生效")
	}
	if _, _, err = s.RotateScopedAdminKey(ctx, item.ID); err == nil {
		t.Fatal("撤销不可轮换恢复")
	}
}

func (r *scopedKeyMemoryRepo) RotateScopedAdminKeyHash(_ context.Context, _ string, expectedHash, hash string, at time.Time) error {
	if r.item.Hash != expectedHash || r.item.RevokedAt != nil {
		return ErrScopedAdminKeyConflict
	}
	r.item.Hash = hash
	r.item.UpdatedAt = at
	return nil
}

func TestScopedAdminKeyPolicyValidationIsBadRequest(t *testing.T) {
	in := ScopedAdminKeyInput{Name: "测试", Policy: adminaccess.Policy{GlobalPermissions: []string{"*"}}}
	if err := validateScopedAdminKeyInput(&in); !infraerrors.IsBadRequest(err) {
		t.Fatalf("策略错误应为400，实际 %v", err)
	}
}
func TestScopedAdminKeyExpiredCanBeDisabledOrExtended(t *testing.T) {
	expired := time.Now().Add(-time.Hour)
	repo := &scopedKeyMemoryRepo{item: &ScopedAdminKey{ID: "expired", ScopedAdminKeyInput: ScopedAdminKeyInput{Name: "已过期", Enabled: true, ExpiresAt: &expired}}}
	s := &SettingService{scopedAdminKeyRepo: repo}
	in := repo.item.ScopedAdminKeyInput
	in.Enabled = false
	in.Description = "停用过期密钥"
	if _, err := s.UpdateScopedAdminKey(context.Background(), "expired", in); err != nil {
		t.Fatal(err)
	}
	in.ExpiresAt = ptrScopedTime(time.Now().Add(time.Hour))
	if _, err := s.UpdateScopedAdminKey(context.Background(), "expired", in); err != nil {
		t.Fatal(err)
	}
	in.ExpiresAt = ptrScopedTime(time.Now().Add(-2 * time.Hour))
	if _, err := s.UpdateScopedAdminKey(context.Background(), "expired", in); err == nil {
		t.Fatal("不允许改为另一过去时间")
	}
}

type scopedKeyCASRepo struct {
	scopedKeyMemoryRepo
	rotateOnTouch bool
}

func (r *scopedKeyCASRepo) TouchScopedAdminKey(_ context.Context, id, hash string, updated time.Time, ip string, now time.Time) error {
	if r.rotateOnTouch {
		r.item.Hash = "new-hash"
		r.item.UpdatedAt = now
		return ErrScopedAdminKeyNotFound
	}
	if r.item.ID != id || r.item.Hash != hash || !r.item.UpdatedAt.Equal(updated) || !r.item.Enabled || r.item.RevokedAt != nil || (r.item.ExpiresAt != nil && !r.item.ExpiresAt.After(now)) {
		return ErrScopedAdminKeyNotFound
	}
	return nil
}
func TestScopedAdminKeyConcurrentRotationRejectsStaleAuthentication(t *testing.T) {
	repo := &scopedKeyCASRepo{rotateOnTouch: true}
	s := &SettingService{scopedAdminKeyRepo: repo}
	key, _, err := s.CreateScopedAdminKey(context.Background(), ScopedAdminKeyInput{Name: "CAS", Enabled: true}, 2)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.AuthenticateScopedAdminKey(context.Background(), key, "192.0.2.1")
	if !errors.Is(err, ErrScopedAdminKeyInvalid) {
		t.Fatalf("并发轮换后的旧读取必须拒绝，实际 %v", err)
	}
}
func ptrScopedTime(t time.Time) *time.Time { return &t }

func TestScopedAdminKeyMappedCIDRAuthentication(t *testing.T) {
	for _, tc := range []struct {
		prefix, ip string
		allowed    bool
	}{
		{"::ffff:192.0.2.0/120", "192.0.2.9", true},
		{"::ffff:192.0.2.0/120", "::ffff:192.0.2.9", true},
		{"::ffff:192.0.2.0/120", "192.0.3.9", false},
		{"::ffff:192.0.2.0/120", "2001:db8::9", false},
		{"::ffff:192.0.2.9/128", "192.0.2.9", true},
		{"::ffff:192.0.2.9/128", "192.0.2.8", false},
		{"::ffff:0.0.0.0/96", "203.0.113.9", true},
		{"192.0.2.0/24", "::ffff:192.0.2.9", true},
		{"2001:db8::/32", "2001:db8::9", true},
		{"2001:db8::/32", "192.0.2.9", false},
		{"::ffff:192.0.2.9", "192.0.2.9", true},
	} {
		t.Run(tc.prefix+"_"+tc.ip, func(t *testing.T) {
			repo := &scopedKeyMemoryRepo{}
			settings := &SettingService{scopedAdminKeyRepo: repo}
			key, _, err := settings.CreateScopedAdminKey(context.Background(), ScopedAdminKeyInput{Name: "CIDR验收", Enabled: true, AllowedIPs: []string{tc.prefix}}, 9)
			if err != nil {
				t.Fatal(err)
			}
			_, err = settings.AuthenticateScopedAdminKey(context.Background(), key, tc.ip)
			if tc.allowed && err != nil {
				t.Fatalf("合法CIDR被接受但范围内地址认证失败: %v", err)
			}
			if !tc.allowed && !errors.Is(err, ErrScopedAdminKeyInvalid) {
				t.Fatalf("范围外地址必须拒绝: %v", err)
			}
		})
	}
}
