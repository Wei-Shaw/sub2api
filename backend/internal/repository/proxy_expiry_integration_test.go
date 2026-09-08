//go:build integration

package repository

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/proxy"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/suite"
)

type ProxyExpirySuite struct {
	suite.Suite
	ctx  context.Context
	tx   *dbent.Tx
	repo *proxyRepository
}

func (s *ProxyExpirySuite) SetupTest() {
	s.ctx = context.Background()
	s.tx = testEntTx(s.T())
	s.repo = newProxyRepositoryWithSQL(s.tx.Client(), s.tx)
}
func TestProxyExpirySuite(t *testing.T) { suite.Run(t, new(ProxyExpirySuite)) }

func (s *ProxyExpirySuite) mkProxy(name, mode string, expiresAt *time.Time, backupID *int64) int64 {
	p := &service.Proxy{Name: name, Protocol: "http", Host: "127.0.0.1", Port: 8080,
		Status: service.StatusActive, FallbackMode: mode, ExpiryWarnDays: 7,
		ExpiresAt: expiresAt, BackupProxyID: backupID}
	s.Require().NoError(s.repo.Create(s.ctx, p))
	return p.ID
}

func (s *ProxyExpirySuite) mkAccountWithProxy(proxyID int64) int64 {
	var id int64
	err := scanSingleRow(s.ctx, s.tx, `
		INSERT INTO accounts (name, platform, type, credentials, extra, status, proxy_id, created_at, updated_at)
		VALUES ($1,'claude','api','{}','{}','active',$2,NOW(),NOW()) RETURNING id`,
		[]any{"acc-" + time.Now().Format("150405.000000"), proxyID}, &id)
	s.Require().NoError(err)
	return id
}

func (s *ProxyExpirySuite) accountProxyID(id int64) *int64 {
	var pid *int64
	err := scanSingleRow(s.ctx, s.tx, `SELECT proxy_id FROM accounts WHERE id=$1`, []any{id}, &pid)
	s.Require().NoError(err)
	return pid
}

func (s *ProxyExpirySuite) TestSweep_DirectMode() {
	past := time.Now().Add(-time.Hour)
	pid := s.mkProxy("p-direct", service.FallbackModeDirect, &past, nil)
	aid := s.mkAccountWithProxy(pid)

	changed, err := s.repo.SweepExpiredProxies(s.ctx, time.Now())
	s.Require().NoError(err)
	s.Require().GreaterOrEqual(changed, int64(1))

	got, _ := s.repo.GetByID(s.ctx, pid)
	s.Require().Equal(service.StatusExpired, got.Status)
	s.Require().Nil(s.accountProxyID(aid))
	var origin *int64
	err = scanSingleRow(s.ctx, s.tx, `SELECT proxy_fallback_origin_id FROM accounts WHERE id=$1`, []any{aid}, &origin)
	s.Require().NoError(err)
	s.Require().NotNil(origin)
	s.Require().Equal(pid, *origin)
}

func (s *ProxyExpirySuite) TestSweep_EnqueuesChangedAccountIDsWithoutFullRebuild() {
	past := time.Now().Add(-time.Hour)
	firstProxyID := s.mkProxy("p-bulk-first", service.FallbackModeDirect, &past, nil)
	secondProxyID := s.mkProxy("p-bulk-second", service.FallbackModeDirect, &past, nil)
	firstAccountID := s.mkAccountWithProxy(firstProxyID)
	secondAccountID := s.mkAccountWithProxy(secondProxyID)

	changed, err := s.repo.SweepExpiredProxies(s.ctx, time.Now())
	s.Require().NoError(err)
	s.Require().EqualValues(2, changed)

	var payloadRaw []byte
	err = scanSingleRow(s.ctx, s.tx, `
		SELECT payload
		FROM scheduler_outbox
		WHERE event_type=$1
		ORDER BY id DESC
		LIMIT 1`, []any{service.SchedulerOutboxEventAccountBulkChanged}, &payloadRaw)
	s.Require().NoError(err)

	var payload struct {
		AccountIDs []int64 `json:"account_ids"`
	}
	s.Require().NoError(json.Unmarshal(payloadRaw, &payload))
	s.Require().Equal([]int64{firstAccountID, secondAccountID}, payload.AccountIDs)

	var fullRebuildCount int
	err = scanSingleRow(s.ctx, s.tx, `
		SELECT COUNT(*)
		FROM scheduler_outbox
		WHERE event_type=$1`, []any{service.SchedulerOutboxEventFullRebuild}, &fullRebuildCount)
	s.Require().NoError(err)
	s.Require().Zero(fullRebuildCount)
}

func (s *ProxyExpirySuite) TestSweep_ProxyMode_Healthy() {
	future := time.Now().Add(24 * time.Hour)
	past := time.Now().Add(-time.Hour)
	backup := s.mkProxy("p-backup", service.FallbackModeNone, &future, nil)
	pid := s.mkProxy("p-main", service.FallbackModeProxy, &past, &backup)
	aid := s.mkAccountWithProxy(pid)

	_, err := s.repo.SweepExpiredProxies(s.ctx, time.Now())
	s.Require().NoError(err)
	s.Require().Equal(backup, *s.accountProxyID(aid))
	var origin *int64
	err = scanSingleRow(s.ctx, s.tx, `SELECT proxy_fallback_origin_id FROM accounts WHERE id=$1`, []any{aid}, &origin)
	s.Require().NoError(err)
	s.Require().NotNil(origin)
	s.Require().Equal(pid, *origin)
}

func (s *ProxyExpirySuite) TestSweep_NoneMode_KeepsAccount() {
	past := time.Now().Add(-time.Hour)
	pid := s.mkProxy("p-none", service.FallbackModeNone, &past, nil)
	aid := s.mkAccountWithProxy(pid)

	_, err := s.repo.SweepExpiredProxies(s.ctx, time.Now())
	s.Require().NoError(err)
	got, _ := s.repo.GetByID(s.ctx, pid)
	s.Require().Equal(service.StatusExpired, got.Status)
	s.Require().Equal(pid, *s.accountProxyID(aid))
	var origin *int64
	err = scanSingleRow(s.ctx, s.tx, `SELECT proxy_fallback_origin_id FROM accounts WHERE id=$1`, []any{aid}, &origin)
	s.Require().NoError(err)
	s.Require().Nil(origin)
}

func (s *ProxyExpirySuite) TestSweep_RotatesInheritedCodexProfileAndImmediatelyRebindsFallback() {
	future := time.Now().Add(24 * time.Hour)
	backup := s.mkProxy("p-codex-backup", service.FallbackModeNone, &future, nil)
	main := s.mkProxy("p-codex-main", service.FallbackModeProxy, &future, &backup)
	accountRepo := newAccountRepositoryWithSQL(s.tx.Client(), s.tx, nil)
	user := mustCreateUser(s.T(), s.tx.Client(), &service.User{Email: "proxy-expiry-codex@example.com"})
	apiKey := mustCreateApiKey(s.T(), s.tx.Client(), &service.APIKey{UserID: user.ID, Key: "sk-proxy-expiry-codex"})
	policy := service.CodexIdentityPolicySpec{
		Mode: service.CodexIdentityPolicyOSProfileDevicePool,
		Profiles: []service.CodexOSProfilePolicy{{
			OSClass: service.CodexOSLinux, CanonicalSurface: service.CodexSurfaceCLI,
			Architecture: service.CodexArchX8664, SlotCount: 1,
		}},
	}
	account := &service.Account{
		Name: "proxy-expiry-codex", Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth,
		Credentials: map[string]any{"access_token": "test-token"}, Extra: map[string]any{}, ProxyID: &main,
		Status: service.StatusActive, Schedulable: true, Concurrency: 3, Priority: 50,
	}
	s.Require().NoError(accountRepo.ProvisionAccount(s.ctx, &service.AccountProvisioningSpec{
		Account: account, Identity: &policy, FinalStatus: service.StatusActive,
		Schedulable: true, ProvisioningState: service.AccountProvisioningActive,
	}))
	oldBinding, err := accountRepo.ResolveCodexDeviceBinding(s.ctx, account.ID, apiKey.ID, service.CodexOSLinux, service.CodexSurfaceCLI)
	s.Require().NoError(err)
	past := time.Now().Add(-time.Hour)
	_, err = s.tx.Proxy.UpdateOneID(main).SetExpiresAt(past).Save(s.ctx)
	s.Require().NoError(err)

	changed, err := s.repo.SweepExpiredProxies(s.ctx, time.Now())
	s.Require().NoError(err)
	s.Require().GreaterOrEqual(changed, int64(1))
	updated, err := accountRepo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().NotNil(updated.ProxyID)
	s.Require().Equal(backup, *updated.ProxyID)
	s.Require().EqualValues(2, updated.CodexIdentityPolicy.Version)
	s.Require().EqualValues(2, updated.CodexIdentityPolicy.Profiles[0].Epoch)
	rebound, err := accountRepo.ResolveCodexDeviceBinding(s.ctx, account.ID, apiKey.ID, service.CodexOSLinux, service.CodexSurfaceCLI)
	s.Require().NoError(err)
	s.Require().NotEqual(oldBinding.SlotID, rebound.SlotID)
	s.Require().Equal("active", rebound.State)
	s.Require().NotNil(rebound.ProxyID)
	s.Require().Equal(backup, *rebound.ProxyID)

	_, err = s.tx.Proxy.UpdateOneID(main).SetStatus(service.StatusActive).SetExpiresAt(future).Save(s.ctx)
	s.Require().NoError(err)
	s.Require().NoError(accountRepo.RevertProxyFallback(s.ctx, account.ID))
	reverted, err := accountRepo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().NotNil(reverted.ProxyID)
	s.Require().Equal(main, *reverted.ProxyID)
	s.Require().Nil(reverted.ProxyFallbackOriginID)
	s.Require().EqualValues(3, reverted.CodexIdentityPolicy.Version)
	s.Require().EqualValues(3, reverted.CodexIdentityPolicy.Profiles[0].Epoch)
	drainingBackup, err := accountRepo.ResolveCodexDeviceBinding(s.ctx, account.ID, apiKey.ID, service.CodexOSLinux, service.CodexSurfaceCLI)
	s.Require().NoError(err)
	s.Require().Equal(rebound.SlotID, drainingBackup.SlotID)
	s.Require().Equal("draining", drainingBackup.State)
	s.Require().NotNil(drainingBackup.ProxyID)
	s.Require().Equal(backup, *drainingBackup.ProxyID)
	_, err = s.tx.ExecContext(s.ctx, "UPDATE account_codex_device_bindings SET updated_at=NOW()-INTERVAL '2 hours' WHERE id=$1", drainingBackup.BindingID)
	s.Require().NoError(err)
	reboundOrigin, err := accountRepo.ResolveCodexDeviceBinding(s.ctx, account.ID, apiKey.ID, service.CodexOSLinux, service.CodexSurfaceCLI)
	s.Require().NoError(err)
	s.Require().NotEqual(drainingBackup.SlotID, reboundOrigin.SlotID)
	s.Require().NotNil(reboundOrigin.ProxyID)
	s.Require().Equal(main, *reboundOrigin.ProxyID)
}

func (s *ProxyExpirySuite) TestSweep_RotatesCodexProfileProxyOverride() {
	s.runCodexOverrideExpiry("profile")
}

func (s *ProxyExpirySuite) TestSweep_RotatesCodexSlotProxyOverride() {
	s.runCodexOverrideExpiry("slot")
}

func (s *ProxyExpirySuite) runCodexOverrideExpiry(scope string) {
	future := time.Now().Add(24 * time.Hour)
	backup := s.mkProxy("p-codex-"+scope+"-backup", service.FallbackModeNone, &future, nil)
	main := s.mkProxy("p-codex-"+scope+"-main", service.FallbackModeProxy, &future, &backup)
	accountRepo := newAccountRepositoryWithSQL(s.tx.Client(), s.tx, nil)
	user := mustCreateUser(s.T(), s.tx.Client(), &service.User{Email: "proxy-expiry-codex-" + scope + "@example.com"})
	apiKey := mustCreateApiKey(s.T(), s.tx.Client(), &service.APIKey{UserID: user.ID, Key: "sk-proxy-expiry-codex-" + scope})
	profile := service.CodexOSProfilePolicy{
		OSClass: service.CodexOSLinux, CanonicalSurface: service.CodexSurfaceCLI,
		Architecture: service.CodexArchX8664, SlotCount: 1,
	}
	if scope == "profile" {
		profile.ProxyID = &main
	} else {
		profile.Slots = []service.CodexDeviceSlotPolicy{{Index: 0, ProxyID: &main}}
	}
	policy := service.CodexIdentityPolicySpec{
		Mode: service.CodexIdentityPolicyOSProfileDevicePool, Profiles: []service.CodexOSProfilePolicy{profile},
	}
	account := &service.Account{
		Name: "proxy-expiry-codex-" + scope, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth,
		Credentials: map[string]any{"access_token": "test-token"}, Extra: map[string]any{},
		Status: service.StatusActive, Schedulable: true, Concurrency: 3, Priority: 50,
	}
	s.Require().NoError(accountRepo.ProvisionAccount(s.ctx, &service.AccountProvisioningSpec{
		Account: account, Identity: &policy, FinalStatus: service.StatusActive,
		Schedulable: true, ProvisioningState: service.AccountProvisioningActive,
	}))
	oldBinding, err := accountRepo.ResolveCodexDeviceBinding(s.ctx, account.ID, apiKey.ID, service.CodexOSLinux, service.CodexSurfaceCLI)
	s.Require().NoError(err)
	past := time.Now().Add(-time.Hour)
	_, err = s.tx.Proxy.UpdateOneID(main).SetExpiresAt(past).Save(s.ctx)
	s.Require().NoError(err)

	changed, err := s.repo.SweepExpiredProxies(s.ctx, time.Now())
	s.Require().NoError(err)
	s.Require().GreaterOrEqual(changed, int64(1))
	updated, err := accountRepo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().EqualValues(2, updated.CodexIdentityPolicy.Version)
	s.Require().EqualValues(2, updated.CodexIdentityPolicy.Profiles[0].Epoch)
	if scope == "profile" {
		s.Require().NotNil(updated.CodexIdentityPolicy.Profiles[0].ProxyID)
		s.Require().Equal(backup, *updated.CodexIdentityPolicy.Profiles[0].ProxyID)
	} else {
		s.Require().NotNil(updated.CodexIdentityPolicy.Profiles[0].Slots[0].ProxyID)
		s.Require().Equal(backup, *updated.CodexIdentityPolicy.Profiles[0].Slots[0].ProxyID)
	}
	rebound, err := accountRepo.ResolveCodexDeviceBinding(s.ctx, account.ID, apiKey.ID, service.CodexOSLinux, service.CodexSurfaceCLI)
	s.Require().NoError(err)
	s.Require().NotEqual(oldBinding.SlotID, rebound.SlotID)
	s.Require().NotNil(rebound.ProxyID)
	s.Require().Equal(backup, *rebound.ProxyID)
}

func (s *ProxyExpirySuite) TestSweep_CodexProfileDirectFallbackDoesNotInheritAccountProxy() {
	s.runCodexDirectFallback("profile")
}

func (s *ProxyExpirySuite) TestSweep_CodexSlotDirectFallbackDoesNotInheritAccountProxy() {
	s.runCodexDirectFallback("slot")
}

func (s *ProxyExpirySuite) runCodexDirectFallback(scope string) {
	future := time.Now().Add(24 * time.Hour)
	accountProxy := s.mkProxy("p-codex-direct-account-"+scope, service.FallbackModeNone, &future, nil)
	overrideProxy := s.mkProxy("p-codex-direct-override-"+scope, service.FallbackModeDirect, &future, nil)
	accountRepo := newAccountRepositoryWithSQL(s.tx.Client(), s.tx, nil)
	user := mustCreateUser(s.T(), s.tx.Client(), &service.User{Email: "proxy-expiry-direct-" + scope + "@example.com"})
	apiKey := mustCreateApiKey(s.T(), s.tx.Client(), &service.APIKey{UserID: user.ID, Key: "sk-proxy-expiry-direct-" + scope})
	profile := service.CodexOSProfilePolicy{
		OSClass: service.CodexOSLinux, CanonicalSurface: service.CodexSurfaceCLI,
		Architecture: service.CodexArchX8664, SlotCount: 1,
	}
	if scope == "profile" {
		profile.ProxyID = &overrideProxy
	} else {
		profile.Slots = []service.CodexDeviceSlotPolicy{{Index: 0, ProxyID: &overrideProxy}}
	}
	policy := service.CodexIdentityPolicySpec{
		Mode:     service.CodexIdentityPolicyOSProfileDevicePool,
		Profiles: []service.CodexOSProfilePolicy{profile},
	}
	account := &service.Account{
		Name: "proxy-expiry-direct-" + scope, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth,
		Credentials: map[string]any{"access_token": "test-token"}, Extra: map[string]any{}, ProxyID: &accountProxy,
		Status: service.StatusActive, Schedulable: true, Concurrency: 3, Priority: 50,
	}
	s.Require().NoError(accountRepo.ProvisionAccount(s.ctx, &service.AccountProvisioningSpec{
		Account: account, Identity: &policy, FinalStatus: service.StatusActive,
		Schedulable: true, ProvisioningState: service.AccountProvisioningActive,
	}))
	past := time.Now().Add(-time.Hour)
	_, err := s.tx.Proxy.UpdateOneID(overrideProxy).SetExpiresAt(past).Save(s.ctx)
	s.Require().NoError(err)

	_, err = s.repo.SweepExpiredProxies(s.ctx, time.Now())
	s.Require().NoError(err)
	updated, err := accountRepo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	if scope == "profile" {
		s.Require().Equal(service.CodexProxyDirect, updated.CodexIdentityPolicy.Profiles[0].ProxyMode)
		s.Require().Nil(updated.CodexIdentityPolicy.Profiles[0].ProxyID)
	} else {
		s.Require().Equal(service.CodexProxyDirect, updated.CodexIdentityPolicy.Profiles[0].Slots[0].ProxyMode)
		s.Require().Nil(updated.CodexIdentityPolicy.Profiles[0].Slots[0].ProxyID)
	}
	resolved, err := accountRepo.ResolveCodexDeviceBinding(s.ctx, account.ID, apiKey.ID, service.CodexOSLinux, service.CodexSurfaceCLI)
	s.Require().NoError(err)
	s.Require().Nil(resolved.ProxyID, "profile direct fallback must not inherit the account proxy")
}

func (s *ProxyExpirySuite) TestSweep_MultipleExpiredCodexOverridesDoNotBlockEachOther() {
	future := time.Now().Add(24 * time.Hour)
	linuxBackup := s.mkProxy("p-codex-multi-linux-backup", service.FallbackModeNone, &future, nil)
	windowsBackup := s.mkProxy("p-codex-multi-windows-backup", service.FallbackModeNone, &future, nil)
	linuxProxy := s.mkProxy("p-codex-multi-linux", service.FallbackModeProxy, &future, &linuxBackup)
	windowsProxy := s.mkProxy("p-codex-multi-windows", service.FallbackModeProxy, &future, &windowsBackup)
	accountRepo := newAccountRepositoryWithSQL(s.tx.Client(), s.tx, nil)
	policy := service.CodexIdentityPolicySpec{
		Mode: service.CodexIdentityPolicyOSProfileDevicePool,
		Profiles: []service.CodexOSProfilePolicy{
			{OSClass: service.CodexOSLinux, CanonicalSurface: service.CodexSurfaceCLI, Architecture: service.CodexArchX8664, SlotCount: 1, ProxyID: &linuxProxy},
			{OSClass: service.CodexOSWindows, CanonicalSurface: service.CodexSurfaceCLI, Architecture: service.CodexArchX8664, SlotCount: 1, ProxyID: &windowsProxy},
		},
	}
	account := &service.Account{
		Name: "proxy-expiry-multiple", Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth,
		Credentials: map[string]any{"access_token": "test-token"}, Extra: map[string]any{},
		Status: service.StatusActive, Schedulable: true, Concurrency: 3, Priority: 50,
	}
	s.Require().NoError(accountRepo.ProvisionAccount(s.ctx, &service.AccountProvisioningSpec{
		Account: account, Identity: &policy, FinalStatus: service.StatusActive,
		Schedulable: true, ProvisioningState: service.AccountProvisioningActive,
	}))
	past := time.Now().Add(-time.Hour)
	_, err := s.tx.Proxy.Update().Where(proxy.IDIn(linuxProxy, windowsProxy)).SetExpiresAt(past).Save(s.ctx)
	s.Require().NoError(err)

	_, err = s.repo.SweepExpiredProxies(s.ctx, time.Now())
	s.Require().NoError(err)
	updated, err := accountRepo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	resolvedByOS := make(map[service.CodexOSClass]int64)
	for _, profile := range updated.CodexIdentityPolicy.Profiles {
		s.Require().Equal(service.CodexProxyExplicit, profile.ProxyMode)
		s.Require().NotNil(profile.ProxyID)
		resolvedByOS[profile.OSClass] = *profile.ProxyID
	}
	s.Require().Equal(linuxBackup, resolvedByOS[service.CodexOSLinux])
	s.Require().Equal(windowsBackup, resolvedByOS[service.CodexOSWindows])
}

func (s *ProxyExpirySuite) TestProxyUsageIncludesCodexProfileAndSlotOverrides() {
	future := time.Now().Add(24 * time.Hour)
	profileProxy := s.mkProxy("p-codex-count-profile", service.FallbackModeNone, &future, nil)
	slotProxy := s.mkProxy("p-codex-count-slot", service.FallbackModeNone, &future, nil)
	accountRepo := newAccountRepositoryWithSQL(s.tx.Client(), s.tx, nil)
	policy := service.CodexIdentityPolicySpec{
		Mode: service.CodexIdentityPolicyOSProfileDevicePool,
		Profiles: []service.CodexOSProfilePolicy{
			{OSClass: service.CodexOSLinux, CanonicalSurface: service.CodexSurfaceCLI, Architecture: service.CodexArchX8664, SlotCount: 1, ProxyID: &profileProxy},
			{OSClass: service.CodexOSWindows, CanonicalSurface: service.CodexSurfaceCLI, Architecture: service.CodexArchX8664, SlotCount: 1, Slots: []service.CodexDeviceSlotPolicy{{Index: 0, ProxyID: &slotProxy}}},
		},
	}
	account := &service.Account{
		Name: "proxy-codex-count", Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth,
		Credentials: map[string]any{"access_token": "test-token"}, Extra: map[string]any{},
		Status: service.StatusActive, Schedulable: true, Concurrency: 3, Priority: 50,
	}
	s.Require().NoError(accountRepo.ProvisionAccount(s.ctx, &service.AccountProvisioningSpec{
		Account: account, Identity: &policy, FinalStatus: service.StatusActive,
		Schedulable: true, ProvisioningState: service.AccountProvisioningActive,
	}))
	for _, proxyID := range []int64{profileProxy, slotProxy} {
		count, err := s.repo.CountAccountsByProxyID(s.ctx, proxyID)
		s.Require().NoError(err)
		s.Require().EqualValues(1, count)
		summaries, err := s.repo.ListAccountSummariesByProxyID(s.ctx, proxyID)
		s.Require().NoError(err)
		s.Require().Len(summaries, 1)
		s.Require().Equal(account.ID, summaries[0].ID)
	}
}

func (s *ProxyExpirySuite) TestProvisionAccountOffModePreservesLegacyInactiveProxyCompatibility() {
	past := time.Now().Add(-time.Hour)
	proxyID := s.mkProxy("p-off-mode-expired", service.FallbackModeNone, &past, nil)
	accountRepo := newAccountRepositoryWithSQL(s.tx.Client(), s.tx, nil)
	policy := service.DefaultCodexIdentityPolicySpec()
	account := &service.Account{
		Name: "off-mode-expired-proxy", Platform: service.PlatformAnthropic, Type: service.AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "test-key"}, Extra: map[string]any{}, ProxyID: &proxyID,
		Status: service.StatusActive, Schedulable: true, Concurrency: 3, Priority: 50,
	}
	s.Require().NoError(accountRepo.ProvisionAccount(s.ctx, &service.AccountProvisioningSpec{
		Account: account, Identity: &policy, FinalStatus: service.StatusActive,
		Schedulable: true, ProvisioningState: service.AccountProvisioningActive,
	}))
	s.Require().NotZero(account.ID)
}
