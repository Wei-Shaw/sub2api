//go:build unit

package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// raceSafeUserRepo 是仅覆盖注册路径的并发安全用户仓储桩。
// 未用到的方法走嵌入接口（调用即 panic，注册路径不会触发）。
type raceSafeUserRepo struct {
	UserRepository

	mu      sync.Mutex
	nextID  int64
	byEmail map[string]*User
	byID    map[int64]*User
}

func newRaceSafeUserRepo() *raceSafeUserRepo {
	return &raceSafeUserRepo{nextID: 1, byEmail: map[string]*User{}, byID: map[int64]*User{}}
}

func (s *raceSafeUserRepo) ExistsByEmail(_ context.Context, email string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.byEmail[email]
	return ok, nil
}

func (s *raceSafeUserRepo) ExistsByEmailAlias(ctx context.Context, email string) (bool, error) {
	return s.ExistsByEmail(ctx, email)
}

func (s *raceSafeUserRepo) CreateWithEmailAliasGuard(_ context.Context, user *User) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.byEmail[user.Email]; ok {
		return ErrEmailExists
	}
	user.ID = s.nextID
	s.nextID++
	clone := *user
	s.byEmail[user.Email] = &clone
	s.byID[user.ID] = &clone
	return nil
}

func (s *raceSafeUserRepo) GetByEmail(_ context.Context, email string) (*User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.byEmail[email]
	if !ok {
		return nil, ErrUserNotFound
	}
	clone := *u
	return &clone, nil
}

func (s *raceSafeUserRepo) GetByID(_ context.Context, id int64) (*User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.byID[id]
	if !ok {
		return nil, ErrUserNotFound
	}
	clone := *u
	return &clone, nil
}

func (s *raceSafeUserRepo) Update(context.Context, *User, UserUpdateFields) error {
	return nil
}

// raceSafeRedeemRepo 是并发安全的兑换码仓储桩：Use 以互斥锁 + 状态条件
// 模拟数据库的条件更新（WHERE status='unused'），语义与线上实现一致。
type raceSafeRedeemRepo struct {
	RedeemCodeRepository

	mu    sync.Mutex
	codes map[string]*RedeemCode
}

func (s *raceSafeRedeemRepo) GetByCode(_ context.Context, code string) (*RedeemCode, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.codes[code]
	if !ok {
		return nil, ErrRedeemCodeNotFound
	}
	clone := *c
	return &clone, nil
}

func (s *raceSafeRedeemRepo) Use(_ context.Context, id, userID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.codes {
		if c.ID != id {
			continue
		}
		if c.Status != StatusUnused {
			return ErrRedeemCodeUsed
		}
		now := time.Now().UTC()
		c.Status = StatusUsed
		c.UsedBy = &userID
		c.UsedAt = &now
		return nil
	}
	return ErrRedeemCodeNotFound
}

// TestAuthService_Register_InvitationCodeSingleUseUnderConcurrency 回归测试：
// 同一邀请码并发注册必须恰好成功 1 次，其余请求以 INVITATION_CODE_INVALID 拒绝。
//
// 修复前：邀请码“检查(CanUse) 与 标记已用(Use)”分离且不在同一事务，Use 失败被吞，
// 并发请求全部注册成功（一个邀请码可创建任意数量账号）。此测试在该实现下必然失败。
// 修复后：用户创建与邀请码占用在同一事务内原子完成（或退化路径下由 Use 条件更新
// 兜底），并发下仅最先占码的注册成功。
func TestAuthService_Register_InvitationCodeSingleUseUnderConcurrency(t *testing.T) {
	const code = "INV-RACE-001"
	userRepo := newRaceSafeUserRepo()
	redeemRepo := &raceSafeRedeemRepo{codes: map[string]*RedeemCode{
		code: {ID: 1, Code: code, Type: RedeemTypeInvitation, Status: StatusUnused},
	}}
	settings := map[string]string{
		"registration_enabled":    "true",
		"invitation_code_enabled": "true",
	}
	svc := newOAuthEmailFlowAuthService(
		userRepo,
		redeemRepo,
		&refreshTokenCacheStub{},
		settings,
		nil, // emailCache：注册不要求邮箱验证，保持关闭
		&userPlatformQuotaRepoStub{},
	)

	const n = 8
	ctx := context.Background()
	start := make(chan struct{})
	results := make(chan error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			email := fmt.Sprintf("race-%d@example.com", i)
			_, _, err := svc.RegisterWithVerification(ctx, email, "Password123!", "", "", code, "", "")
			results <- err
		}(i)
	}
	close(start)
	wg.Wait()
	close(results)

	successes := 0
	rejected := 0
	for err := range results {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrInvitationCodeInvalid):
			rejected++
		default:
			t.Fatalf("unexpected registration error: %v", err)
		}
	}
	require.Equal(t, 1, successes, "同一邀请码并发注册必须恰好成功 1 次")
	require.Equal(t, n-1, rejected, "其余并发请求必须以 INVITATION_CODE_INVALID 拒绝")

	claimed, err := redeemRepo.GetByCode(ctx, code)
	require.NoError(t, err)
	require.Equal(t, StatusUsed, claimed.Status, "邀请码最终必须处于 used 状态")
	require.NotNil(t, claimed.UsedBy, "used_by 必须记录实际注册用户")
}

// TestAuthService_Register_InvitationCodeRejectedWhenAlreadyUsed 顺序路径回归：
// 已使用过的邀请码再次注册（即便换邮箱）必须被拒绝。
func TestAuthService_Register_InvitationCodeRejectedWhenAlreadyUsed(t *testing.T) {
	const code = "INV-RACE-002"
	userRepo := newRaceSafeUserRepo()
	redeemRepo := &raceSafeRedeemRepo{codes: map[string]*RedeemCode{
		code: {ID: 2, Code: code, Type: RedeemTypeInvitation, Status: StatusUsed},
	}}
	settings := map[string]string{
		"registration_enabled":    "true",
		"invitation_code_enabled": "true",
	}
	svc := newOAuthEmailFlowAuthService(
		userRepo,
		redeemRepo,
		&refreshTokenCacheStub{},
		settings,
		nil,
		&userPlatformQuotaRepoStub{},
	)

	_, _, err := svc.RegisterWithVerification(context.Background(), "later@example.com", "Password123!", "", "", code, "", "")
	require.ErrorIs(t, err, ErrInvitationCodeInvalid)
}

// TestAuthService_Register_InvitationCodeMissingWhenEnabled 门控回归：
// 邀请码开启时，不带邀请码的注册必须被拒绝（不产生用户）。
func TestAuthService_Register_InvitationCodeMissingWhenEnabled(t *testing.T) {
	userRepo := newRaceSafeUserRepo()
	redeemRepo := &raceSafeRedeemRepo{codes: map[string]*RedeemCode{}}
	settings := map[string]string{
		"registration_enabled":    "true",
		"invitation_code_enabled": "true",
	}
	svc := newOAuthEmailFlowAuthService(
		userRepo,
		redeemRepo,
		&refreshTokenCacheStub{},
		settings,
		nil,
		&userPlatformQuotaRepoStub{},
	)

	_, _, err := svc.RegisterWithVerification(context.Background(), "no-invite@example.com", "Password123!", "", "", "", "", "")
	require.ErrorIs(t, err, ErrInvitationCodeRequired)

	ok, err := userRepo.ExistsByEmail(context.Background(), "no-invite@example.com")
	require.NoError(t, err)
	require.False(t, ok, "被拒绝的注册不应产生用户")
}
