package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"net/netip"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/adminaccess"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

var ErrScopedAdminKeyInvalid = infraerrors.Unauthorized("INVALID_ADMIN_KEY", "Invalid admin API key")
var ErrScopedAdminKeyConflict = infraerrors.Conflict("ADMIN_KEY_CONFLICT", "密钥已变化，请刷新后重试")
var ErrScopedAdminKeyNotFound = infraerrors.NotFound("ADMIN_KEY_NOT_FOUND", "Admin key not found")

type ScopedAdminKeyInput struct {
	Name        string             `json:"name"`
	Description string             `json:"description"`
	Enabled     bool               `json:"enabled"`
	ExpiresAt   *time.Time         `json:"expires_at"`
	AllowedIPs  []string           `json:"allowed_ips"`
	Policy      adminaccess.Policy `json:"policy"`
}
type ScopedAdminKey struct {
	ScopedAdminKeyInput
	ID         string     `json:"id"`
	KeyPrefix  string     `json:"key_prefix"`
	Hash       string     `json:"-"`
	CreatedBy  int64      `json:"created_by"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
	LastUsedIP string     `json:"last_used_ip"`
	RevokedAt  *time.Time `json:"revoked_at"`
}
type ScopedAdminKeyRepository interface {
	ListScopedAdminKeys(context.Context) ([]*ScopedAdminKey, error)
	GetScopedAdminKey(context.Context, string) (*ScopedAdminKey, error)
	InsertScopedAdminKey(context.Context, *ScopedAdminKey) error
	UpdateScopedAdminKey(context.Context, *ScopedAdminKey) error
	RotateScopedAdminKeyHash(context.Context, string, string, string, time.Time) error
	TouchScopedAdminKey(context.Context, string, string, time.Time, string, time.Time) error
	RevokeScopedAdminKey(context.Context, string, time.Time) error
	DeleteScopedAdminKeyRecord(context.Context, string, time.Time) error
}

func validateScopedAdminKeyInput(in *ScopedAdminKeyInput) error {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 100 || len(in.Description) > 1000 || len(in.AllowedIPs) > 100 {
		return infraerrors.BadRequest("INVALID_ADMIN_KEY_INPUT", "名称、备注或 IP 列表无效")
	}
	if in.ExpiresAt != nil && !in.ExpiresAt.After(time.Now()) {
		return infraerrors.BadRequest("INVALID_ADMIN_KEY_EXPIRY", "有效期必须在未来")
	}
	for _, ip := range in.AllowedIPs {
		if _, e := netip.ParseAddr(ip); e != nil {
			if _, e = netip.ParsePrefix(ip); e != nil {
				return infraerrors.BadRequest("INVALID_ADMIN_KEY_IP", "IP 或 CIDR 无效")
			}
		}
	}
	if in.AllowedIPs == nil {
		in.AllowedIPs = []string{}
	}
	if err := adminaccess.ValidatePolicy(in.Policy); err != nil {
		return infraerrors.BadRequest("INVALID_ADMIN_KEY_POLICY", "权限策略无效")
	}
	return nil
}
func generateScopedAdminKey(id string) (string, string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	key := "adm_sc_" + id + "_" + hex.EncodeToString(b)
	hash := sha256.Sum256([]byte(key))
	return key, hex.EncodeToString(hash[:]), nil
}
func verifyScopedAdminKey(key, hash string) bool {
	h := sha256.Sum256([]byte(key))
	return subtle.ConstantTimeCompare([]byte(hex.EncodeToString(h[:])), []byte(hash)) == 1
}
func scopedAdminIPAllowed(allowed []string, ip string) bool {
	if len(allowed) == 0 {
		return true
	}
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return false
	}
	addr = addr.Unmap()
	for _, s := range allowed {
		if a, e := netip.ParseAddr(s); e == nil && a.Unmap() == addr {
			return true
		}
		if p, e := netip.ParsePrefix(s); e == nil {
			candidate := addr
			if p.Addr().Is4In6() && addr.Is4() {
				// 使用前缀的映射地址表示，保留原始位数和网段边界。
				candidate = netip.AddrFrom16(addr.As16())
			}
			if p.Contains(candidate) {
				return true
			}
		}
	}
	return false
}
func (s *SettingService) scopedAdminKeys() (ScopedAdminKeyRepository, error) {
	if s.scopedAdminKeyRepo == nil {
		return nil, errors.New("管理密钥存储未配置")
	}
	return s.scopedAdminKeyRepo, nil
}
func (s *SettingService) ListScopedAdminKeys(ctx context.Context) ([]*ScopedAdminKey, error) {
	r, e := s.scopedAdminKeys()
	if e != nil {
		return nil, e
	}
	return r.ListScopedAdminKeys(ctx)
}
func (s *SettingService) CreateScopedAdminKey(ctx context.Context, in ScopedAdminKeyInput, creator int64) (string, *ScopedAdminKey, error) {
	if e := validateScopedAdminKeyInput(&in); e != nil {
		return "", nil, e
	}
	r, e := s.scopedAdminKeys()
	if e != nil {
		return "", nil, e
	}
	idBytes := make([]byte, 12)
	if _, e = rand.Read(idBytes); e != nil {
		return "", nil, e
	}
	id := hex.EncodeToString(idBytes)
	key, hash, e := generateScopedAdminKey(id)
	if e != nil {
		return "", nil, e
	}
	now := time.Now().UTC()
	item := &ScopedAdminKey{ScopedAdminKeyInput: in, ID: id, Hash: hash, KeyPrefix: "adm_sc_" + id, CreatedBy: creator, CreatedAt: now, UpdatedAt: now}
	if e = r.InsertScopedAdminKey(ctx, item); e != nil {
		return "", nil, e
	}
	return key, item, nil
}
func (s *SettingService) UpdateScopedAdminKey(ctx context.Context, id string, in ScopedAdminKeyInput) (*ScopedAdminKey, error) {
	r, e := s.scopedAdminKeys()
	if e != nil {
		return nil, e
	}
	item, e := r.GetScopedAdminKey(ctx, id)
	if e != nil {
		return nil, e
	}
	if item.RevokedAt != nil {
		return nil, ErrScopedAdminKeyNotFound
	}
	if e := validateScopedAdminKeyInputForUpdate(&in, item.ExpiresAt); e != nil {
		return nil, e
	}
	item.ScopedAdminKeyInput = in
	item.UpdatedAt = time.Now().UTC()
	if e = r.UpdateScopedAdminKey(ctx, item); e != nil {
		return nil, e
	}
	return item, nil
}

func validateScopedAdminKeyInputForUpdate(in *ScopedAdminKeyInput, existingExpiry *time.Time) error {
	if in.ExpiresAt != nil && !in.ExpiresAt.After(time.Now()) && (existingExpiry == nil || !in.ExpiresAt.Equal(*existingExpiry)) {
		return infraerrors.BadRequest("INVALID_ADMIN_KEY_EXPIRY", "新的有效期必须在未来")
	}
	expiry := in.ExpiresAt
	in.ExpiresAt = nil
	err := validateScopedAdminKeyInput(in)
	in.ExpiresAt = expiry
	return err
}
func (s *SettingService) RotateScopedAdminKey(ctx context.Context, id string) (string, *ScopedAdminKey, error) {
	r, e := s.scopedAdminKeys()
	if e != nil {
		return "", nil, e
	}
	item, e := r.GetScopedAdminKey(ctx, id)
	if e != nil {
		return "", nil, e
	}
	if item.RevokedAt != nil {
		return "", nil, ErrScopedAdminKeyNotFound
	}
	key, hash, e := generateScopedAdminKey(id)
	if e != nil {
		return "", nil, e
	}
	previousHash := item.Hash
	item.Hash = hash
	item.UpdatedAt = time.Now().UTC()
	if e = r.RotateScopedAdminKeyHash(ctx, item.ID, previousHash, hash, item.UpdatedAt); e != nil {
		return "", nil, e
	}
	return key, item, nil
}
func (s *SettingService) RevokeScopedAdminKey(ctx context.Context, id string) error {
	r, e := s.scopedAdminKeys()
	if e != nil {
		return e
	}
	return r.RevokeScopedAdminKey(ctx, id, time.Now().UTC())
}

func (s *SettingService) DeleteScopedAdminKeyRecord(ctx context.Context, id string) error {
	r, e := s.scopedAdminKeys()
	if e != nil {
		return e
	}
	return r.DeleteScopedAdminKeyRecord(ctx, id, time.Now().UTC())
}

func (s *SettingService) AuthenticateScopedAdminKey(ctx context.Context, key, ip string) (*ScopedAdminKey, error) {
	parts := strings.Split(key, "_")
	if len(parts) != 4 || parts[0] != "adm" || parts[1] != "sc" || len(parts[2]) != 24 || len(parts[3]) != 64 {
		return nil, ErrScopedAdminKeyInvalid
	}
	r, e := s.scopedAdminKeys()
	if e != nil {
		return nil, e
	}
	item, e := r.GetScopedAdminKey(ctx, parts[2])
	if e != nil {
		return nil, ErrScopedAdminKeyInvalid
	}
	if !verifyScopedAdminKey(key, item.Hash) {
		return nil, ErrScopedAdminKeyInvalid
	}
	if !item.Enabled || item.RevokedAt != nil || (item.ExpiresAt != nil && !item.ExpiresAt.After(time.Now())) || !scopedAdminIPAllowed(item.AllowedIPs, ip) {
		return item, ErrScopedAdminKeyInvalid
	}
	if e = r.TouchScopedAdminKey(ctx, item.ID, item.Hash, item.UpdatedAt, ip, time.Now().UTC()); e != nil {
		return nil, ErrScopedAdminKeyInvalid
	}
	return item, nil
}

// SetScopedAdminKeyRepository 注入独立密钥存储。
func (s *SettingService) SetScopedAdminKeyRepository(repo ScopedAdminKeyRepository) {
	s.scopedAdminKeyRepo = repo
}
