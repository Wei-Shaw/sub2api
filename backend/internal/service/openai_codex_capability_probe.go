package service

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

const (
	// codexCapabilityProbeTimeout 单次探测的上游超时。
	codexCapabilityProbeTimeout = 15 * time.Second
	// codexCapabilityReprobeCooldown 流内拒绝触发重探测的按账号冷却窗口。
	codexCapabilityReprobeCooldown = 10 * time.Minute
	// codexCapabilityBackfillInterval 启动后补齐探测的节奏（每秒 2 个）。
	codexCapabilityBackfillInterval = 500 * time.Millisecond

	codexCapabilityProbeReasonAccountCreated     = "account_created"
	codexCapabilityProbeReasonCredentialsUpdated = "credentials_updated"
	codexCapabilityProbeReasonAccessProgramDeny  = "access_program_rejected"
)

var errCodexCapabilityManifestEmpty = errors.New("codex models manifest has no models")

// CodexCapabilityProber 是探测触发点（账号创建 / 重新授权 / token 刷新 / 账号测试）
// 依赖的窄接口；nil 表示未接入，调用方直接跳过。
type CodexCapabilityProber interface {
	Probe(ctx context.Context, account *Account) error
	ProbeAsync(accountID int64, reason string)
	RecordFromManifest(ctx context.Context, account *Account, clientVersion string, body []byte)
}

// isCodexCapabilityProbeAccount 是探测与能力谓词共用的账号类型范围（D2/D3）：
// 使用 ChatGPT/Codex 协议的 OpenAI OAuth-like 账号（oauth 与 setup_token）。
// 若 setup_token 凭据被验证无法调用目录接口，这里收窄为 IsOpenAIOAuth() 即可。
func isCodexCapabilityProbeAccount(account *Account) bool {
	return account != nil && account.IsOpenAIOAuthLike()
}

// CodexCapabilityProbe 直连上游拉取 Codex 模型目录并把推导出的能力记录写入账号。
// 它绕过 FetchCodexModelsManifest 的账号级缓存（60 秒新鲜 / 5 分钟乐观返回旧值），
// 否则权益刚变化时探测会读到旧目录；成功后回填该账号的缓存条目。
type CodexCapabilityProbe struct {
	gateway *OpenAIGatewayService

	mu sync.Mutex
	// lastReprobeAt 只记录流内拒绝触发的重探测时间，用于冷却；常规探测（创建 / 刷新 / 测试 /
	// 补齐）不参与，否则一次常规探测或其失败会把随后十分钟的拒绝重探测全部压掉。
	lastReprobeAt map[int64]time.Time
	inflight      singleflight.Group

	stopCh   chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup
	started  bool
}

func newCodexCapabilityProbe(gateway *OpenAIGatewayService) *CodexCapabilityProbe {
	return &CodexCapabilityProbe{
		gateway:       gateway,
		lastReprobeAt: make(map[int64]time.Time),
		stopCh:        make(chan struct{}),
	}
}

// getCodexCapabilityProbe 返回网关持有的唯一探测器实例（惰性创建）。
func (s *OpenAIGatewayService) getCodexCapabilityProbe() *CodexCapabilityProbe {
	s.codexCapabilityProbeOnce.Do(func() {
		s.codexCapabilityProbe = newCodexCapabilityProbe(s)
	})
	return s.codexCapabilityProbe
}

// CodexCapabilityProbe 暴露探测器供触发点注入。
func (s *OpenAIGatewayService) CodexCapabilityProbe() *CodexCapabilityProbe {
	return s.getCodexCapabilityProbe()
}

// Probe 用规范客户端版本直连上游拉取目录，推导能力并按「仅变化时写入」规则持久化。
// 非 OAuth-like 账号直接返回 nil。
func (p *CodexCapabilityProbe) Probe(ctx context.Context, account *Account) error {
	if p == nil || p.gateway == nil || !isCodexCapabilityProbeAccount(account) {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	probeCtx, cancel := context.WithTimeout(ctx, codexCapabilityProbeTimeout)
	defer cancel()

	request, credAccount, err := p.gateway.buildCodexModelsManifestRequest(probeCtx, account, CodexCanonicalClientVersion())
	if err != nil {
		return err
	}
	manifest, err := p.gateway.fetchCodexModelsManifestOAuthUpstream(probeCtx, account, credAccount, request, "")
	if err != nil {
		return err
	}
	if manifest == nil || manifest.NotModified || len(manifest.Body) == 0 {
		return errCodexCapabilityManifestEmpty
	}
	p.gateway.openAIModelsCache.set(buildOpenAIModelsCacheKey(request), manifest, time.Now())
	return p.record(probeCtx, account, manifest.Body)
}

// ProbeAsync 在后台按账号单飞执行一次探测，供账号创建、重新授权等不可阻塞路径使用。
func (p *CodexCapabilityProbe) ProbeAsync(accountID int64, reason string) {
	if p == nil || p.gateway == nil || p.gateway.accountRepo == nil || accountID <= 0 {
		return
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("codex_capability_probe_panic", "account_id", accountID, "reason", reason, "recover", r)
			}
		}()
		_, err, _ := p.inflight.Do(singleflightKeyForAccount(accountID), func() (any, error) {
			ctx, cancel := context.WithTimeout(context.Background(), codexCapabilityProbeTimeout)
			defer cancel()
			account, err := p.gateway.accountRepo.GetByID(ctx, accountID)
			if err != nil {
				return nil, err
			}
			return nil, p.Probe(ctx, account)
		})
		if err != nil {
			slog.Warn("codex_capability_probe_failed", "account_id", accountID, "reason", reason, "error", err)
		}
	}()
}

// ReprobeAsync 在流内访问项目拒绝后触发重探测；同一账号在冷却窗口内最多一次。
// 冷却只约束拒绝触发的重探测，与常规探测互不影响；探测失败也不回滚冷却：
// 上游不可达时拒绝会持续出现，逐请求重试只会放大对上游的请求量。返回是否真正发起了探测。
func (p *CodexCapabilityProbe) ReprobeAsync(accountID int64) bool {
	if p == nil || accountID <= 0 {
		return false
	}
	p.mu.Lock()
	last, ok := p.lastReprobeAt[accountID]
	if ok && time.Since(last) < codexCapabilityReprobeCooldown {
		p.mu.Unlock()
		return false
	}
	p.lastReprobeAt[accountID] = time.Now()
	p.mu.Unlock()
	p.ProbeAsync(accountID, codexCapabilityProbeReasonAccessProgramDeny)
	return true
}

// RecordFromManifest 用已拿到的目录 body 顺手回写能力记录，避免重复请求。
// 目录按 minimal_client_version 过滤模型，旧版本客户端拉到的目录是子集，
// 只在 clientVersion 不低于规范版本时生效。
func (p *CodexCapabilityProbe) RecordFromManifest(ctx context.Context, account *Account, clientVersion string, body []byte) {
	if p == nil || !isCodexCapabilityProbeAccount(account) || len(body) == 0 {
		return
	}
	version := NormalizeCodexClientVersion(clientVersion)
	if version == "" || CompareVersions(version, CodexCanonicalClientVersion()) < 0 {
		return
	}
	// 顺手回写运行在 manifest 拉取的共享单飞协程里，属于尽力而为的副作用：
	// 任何异常都只记日志，不能拖垮正在服务客户端的 manifest 请求。
	defer func() {
		if r := recover(); r != nil {
			slog.Error("codex_capability_record_panic", "account_id", account.ID, "recover", r)
		}
	}()
	if err := p.record(ctx, account, body); err != nil {
		slog.Warn("codex_capability_record_failed", "account_id", account.ID, "client_version", clientVersion, "error", err)
	}
}

// record 推导并按「仅变化时写入」持久化；空目录视为失败且不覆盖已有记录。
func (p *CodexCapabilityProbe) record(ctx context.Context, account *Account, body []byte) error {
	caps, ok := DeriveCodexAccountCapabilities(body)
	if !ok {
		return errCodexCapabilityManifestEmpty
	}
	if existing := account.CodexCapabilities(); existing != nil && existing.EquivalentTo(&caps) {
		return nil
	}
	if p.gateway == nil || p.gateway.accountRepo == nil {
		return errors.New("codex capability probe has no account repository")
	}
	updates := map[string]any{CodexCapabilitiesExtraKey: caps}
	if err := p.gateway.accountRepo.UpdateExtra(ctx, account.ID, updates); err != nil {
		return err
	}
	mergeAccountExtra(account, updates)
	slog.Info("codex_capabilities_updated",
		"account_id", account.ID,
		"service_tiers", caps.ServiceTiers,
		"cyber_programs", caps.CyberPrograms,
	)
	return nil
}

// Start 启动一次性的部署后补齐：部署后若等 token 刷新周期，期间所有 Daybreak / Ultrafast
// 请求都会报无可用账号。
func (p *CodexCapabilityProbe) Start() {
	if p == nil || p.gateway == nil || p.gateway.accountRepo == nil {
		return
	}
	p.mu.Lock()
	if p.started {
		p.mu.Unlock()
		return
	}
	p.started = true
	p.mu.Unlock()
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		p.BackfillMissing(context.Background())
	}()
}

// Stop 停止补齐任务并等待其退出。
func (p *CodexCapabilityProbe) Stop() {
	if p == nil {
		return
	}
	p.stopOnce.Do(func() {
		close(p.stopCh)
	})
	p.wg.Wait()
}

// BackfillMissing 串行、限速地探测所有没有能力记录的活跃 OpenAI OAuth-like 账号；
// 失败只记日志。返回实际探测的账号数。
func (p *CodexCapabilityProbe) BackfillMissing(ctx context.Context) int {
	if p == nil || p.gateway == nil || p.gateway.accountRepo == nil {
		return 0
	}
	listCtx, cancel := context.WithTimeout(ctx, codexCapabilityProbeTimeout)
	accounts, err := p.gateway.accountRepo.ListByPlatform(listCtx, PlatformOpenAI)
	cancel()
	if err != nil {
		slog.Warn("codex_capability_backfill_list_failed", "error", err)
		return 0
	}
	probed := 0
	for i := range accounts {
		account := &accounts[i]
		if !isCodexCapabilityProbeAccount(account) || !account.IsActive() || account.CodexCapabilities() != nil {
			continue
		}
		if probed > 0 {
			select {
			case <-p.stopCh:
				return probed
			case <-ctx.Done():
				return probed
			case <-time.After(codexCapabilityBackfillInterval):
			}
		}
		select {
		case <-p.stopCh:
			return probed
		default:
		}
		probed++
		if err := p.Probe(ctx, account); err != nil {
			slog.Warn("codex_capability_backfill_probe_failed", "account_id", account.ID, "error", err)
		}
	}
	if probed > 0 {
		slog.Info("codex_capability_backfill_completed", "probed", probed)
	}
	return probed
}

func singleflightKeyForAccount(accountID int64) string {
	return "codex-capability-probe:" + strconv.FormatInt(accountID, 10)
}
