package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

// 智力检测默认参数。这些值后续会被全局设置覆盖（设置接入后以设置为准）。
const (
	IntelligenceCheckDefaultMaxTokens       = 32000
	IntelligenceCheckDefaultMaxArtifactByte = 2 << 20 // 2 MiB
	IntelligenceCheckDefaultTimeout         = IntelligenceCheckDefaultTimeoutSeconds * time.Second
)

// IntelligenceCheckDefaultPrompt 是第一版题面：要求模型产出一段描述「鹈鹕骑自行车」的 SVG 2D 动画。
// 题面本身可被全局设置覆盖，变体标识落在 prompt_variant 列上。
const IntelligenceCheckDefaultPrompt = "请直接输出一个完整、可独立运行的单文件 HTML 页面：用内联 SVG 与 CSS 动画，" +
	"表现一只鹈鹕骑着自行车向前骑行的 2D 动画。\n" +
	"要求：\n" +
	"1. 只输出 HTML 本体，不要任何解释文字，不要 markdown 代码块围栏。\n" +
	"2. 不使用任何外部资源（不得引用外部图片、字体、脚本或 CDN），全部内联。\n" +
	"3. 页面自适应视口，配色自行设计，动画持续循环播放。\n" +
	"4. 必须包含 <!DOCTYPE html> 与完整的 <html> 结构。"

// IntelligenceCheckRequest 描述一次跑测的输入。
type IntelligenceCheckRequest struct {
	AccountID        int64
	ModelID          string
	Prompt           string
	PromptVariant    string
	ReasoningEffort  string
	MaxTokens        int
	MaxArtifactBytes int
	Timeout          time.Duration
	TriggerSource    string
	BatchID          string
	Attempt          int
	// DisableStream 为 true 时以非流式发起跑测。
	// 零值 false 即流式——这正是期望的默认，所以不需要额外的「是否已指定」标记。
	// 由全局设置 intelligence_check_stream_enabled 统一决定，见 resolveRequestDefaults。
	DisableStream bool
}

func (req IntelligenceCheckRequest) withDefaults() IntelligenceCheckRequest {
	if strings.TrimSpace(req.Prompt) == "" {
		req.Prompt = IntelligenceCheckDefaultPrompt
	}
	if strings.TrimSpace(req.PromptVariant) == "" {
		req.PromptVariant = IntelligenceCheckPromptVariantClassic
	}
	if req.MaxTokens <= 0 {
		req.MaxTokens = IntelligenceCheckDefaultMaxTokens
	}
	if req.MaxArtifactBytes <= 0 {
		req.MaxArtifactBytes = IntelligenceCheckDefaultMaxArtifactByte
	}
	if req.Timeout <= 0 {
		req.Timeout = IntelligenceCheckDefaultTimeout
	}
	if strings.TrimSpace(req.TriggerSource) == "" {
		req.TriggerSource = IntelligenceCheckTriggerManual
	}
	if req.Attempt <= 0 {
		req.Attempt = 1
	}
	return req
}

// IntelligenceCheckService 编排智力检测跑测：入队记录、调用账号跑测探针、提取作品、落库，
// 并承载管理员的人工评审（含可选的账号状态联动）。
type IntelligenceCheckService struct {
	accountTest *AccountTestService
	runRepo     IntelligenceCheckRunRepository
	// accountRepo / settingSvc 只服务于人工评审的账号状态联动。
	// 评审判定本身不依赖它们，所以联动关闭时评审依旧完全可用。
	accountRepo AccountRepository
	settingSvc  *SettingService
	// 下面三个仓储只服务于「跑测消费是否进使用记录」：
	// usageLogRepo 写记账行，userRepo/apiKeyRepo 解析记账归属（见 intelligence_check_usage.go）。
	// 三者缺席时跑测照常执行，只是不写使用记录。
	usageLogRepo UsageLogRepository
	userRepo     UserRepository
	apiKeyRepo   APIKeyRepository
}

// NewIntelligenceCheckService 构造智力检测服务。
func NewIntelligenceCheckService(
	accountTest *AccountTestService,
	runRepo IntelligenceCheckRunRepository,
	accountRepo AccountRepository,
	settingSvc *SettingService,
	usageLogRepo UsageLogRepository,
	userRepo UserRepository,
	apiKeyRepo APIKeyRepository,
) *IntelligenceCheckService {
	return &IntelligenceCheckService{
		accountTest:  accountTest,
		runRepo:      runRepo,
		accountRepo:  accountRepo,
		settingSvc:   settingSvc,
		usageLogRepo: usageLogRepo,
		userRepo:     userRepo,
		apiKeyRepo:   apiKeyRepo,
	}
}

// resolveRequestDefaults 用「账号级覆盖 → 全局设置」补齐请求里缺失的跑测参数。
//
// 为什么需要它：调度器路径（IntelligenceCheckRunnerService.runDueAccount）会先自行解析好
// 模型与各项参数再显式传入，因此一切正常；但手动重跑只带一个 account_id，模型/参数全是零值，
// 此时若直接执行就会撞上 ExecuteRun 的「未配置跑测模型」守卫 —— 表现为点「重跑」瞬间失败
// （latency_ms=0），让人误以为是账号状态或后台执行出了问题。
//
// 语义：只在字段为空时填充，绝不覆盖调用方的显式选择，所以调度器路径的行为逐字不变。
// 调用时机必须在 withDefaults 之前 —— 后者会把 Timeout 等填成静态默认值，
// 一旦先跑就再也判断不出「调用方没给」了。
func (s *IntelligenceCheckService) resolveRequestDefaults(ctx context.Context, req IntelligenceCheckRequest) IntelligenceCheckRequest {
	if s.settingSvc == nil || s.accountRepo == nil {
		return req
	}

	needModel := strings.TrimSpace(req.ModelID) == ""
	needEffort := strings.TrimSpace(req.ReasoningEffort) == ""
	needTokens := req.MaxTokens <= 0
	needTimeout := req.Timeout <= 0

	settings, err := s.settingSvc.GetIntelligenceCheckGlobalSettings(ctx)
	if err != nil {
		// 读设置失败不该让跑测直接崩：保持原样，由 ExecuteRun 如实记成「未配置模型」。
		return req
	}

	// 流式开关没有「字段缺失」语义：bool 零值无法区分「没给」和「显式要流式」，
	// 而这里的期望默认恰好就是流式，所以无条件以全局设置为准。
	// 注意它必须在任何早返回之前执行，否则字段齐全的调用会绕过这个开关。
	req.DisableStream = !settings.StreamEnabled

	if !needModel && !needEffort && !needTokens && !needTimeout {
		return req
	}

	if needModel || needEffort {
		// 账号级覆盖优先，与调度器同一套语义。
		var config IntelligenceCheckAccountConfig
		if account, accErr := s.accountRepo.GetByID(ctx, req.AccountID); accErr == nil && account != nil {
			config = ReadIntelligenceCheckAccountConfig(account.Extra)
		}
		if needModel {
			modelID := strings.TrimSpace(config.ModelID)
			if modelID == "" {
				modelID = strings.TrimSpace(settings.ModelID)
			}
			req.ModelID = modelID
		}
		if needEffort {
			effort := strings.TrimSpace(config.ReasoningEffort)
			if effort == "" {
				effort = strings.TrimSpace(settings.ReasoningEffort)
			}
			req.ReasoningEffort = effort
		}
	}
	if needTokens {
		req.MaxTokens = settings.MaxTokens
	}
	if needTimeout {
		req.Timeout = settings.Timeout()
	}
	return req
}

// MarkStaleRunsInterrupted 回收僵尸跑测：把开始时间早于 deadline 且仍停在
// queued / running 的记录落成 interrupted 终态，返回处理条数。
//
// 为什么必须有它：跑测在后台 goroutine 里执行，进程退出（容器重建、部署、OOM）
// 会直接杀掉它，记录就永远停在 running —— 没有终态、也没有人会再推进它。
// 前端据此显示「跑测中」，用户怎么刷新都不会变，而他其实早就不跑了。
func (s *IntelligenceCheckService) MarkStaleRunsInterrupted(ctx context.Context, deadline time.Time) (int64, error) {
	if s == nil || s.runRepo == nil {
		return 0, nil
	}
	return s.runRepo.MarkStaleRunsInterrupted(ctx, deadline)
}

// StartRun 落一条 queued 记录并立即返回，供 HTTP 层马上响应；
// 真正的跑测由 ExecuteRun 执行（通常放进后台 goroutine）。
func (s *IntelligenceCheckService) StartRun(ctx context.Context, req IntelligenceCheckRequest) (*IntelligenceCheckRun, error) {
	req = s.resolveRequestDefaults(ctx, req)
	req = req.withDefaults()
	if req.AccountID <= 0 {
		return nil, fmt.Errorf("account id is required")
	}

	run := &IntelligenceCheckRun{
		AccountID:       req.AccountID,
		BatchID:         req.BatchID,
		TriggerSource:   req.TriggerSource,
		ModelID:         req.ModelID,
		ReasoningEffort: req.ReasoningEffort,
		PromptVariant:   req.PromptVariant,
		Status:          IntelligenceCheckStatusQueued,
		Verdict:         IntelligenceCheckVerdictUnknown,
		Attempt:         req.Attempt,
		StartedAt:       time.Now(),
	}
	return s.runRepo.Create(ctx, run)
}

// ExecuteRun 执行一次跑测并回写终态。
// 所有业务性失败（未配模型、超时、没有 HTML、作品过大）都写进记录，不作为 error 返回；
// 只有持久化本身出错才返回 error。
func (s *IntelligenceCheckService) ExecuteRun(ctx context.Context, runID int64, req IntelligenceCheckRequest) error {
	// 手动重跑传进来的 req 可能只带 account_id（前端不传模型），这里按
	// 「账号级覆盖 → 全局设置」补齐；调度器路径已显式传全，不会被覆盖。
	// 必须在 withDefaults 之前调用，否则 Timeout 已被填成静态默认值、判断不出缺失。
	req = s.resolveRequestDefaults(ctx, req)
	req = req.withDefaults()

	run, err := s.runRepo.GetByID(ctx, runID)
	if err != nil {
		return fmt.Errorf("load intelligence check run %d: %w", runID, err)
	}

	run.Status = IntelligenceCheckStatusRunning
	run.ModelID = req.ModelID
	run.ReasoningEffort = req.ReasoningEffort
	run.PromptVariant = req.PromptVariant
	if err := s.runRepo.UpdateResult(ctx, run); err != nil {
		return fmt.Errorf("mark run %d running: %w", runID, err)
	}

	missingModel := strings.TrimSpace(req.ModelID) == ""
	var probe *IntelligenceCheckProbeResult
	timedOut := false
	if missingModel {
		// 未配置模型时绝不下发请求：否则一上线就是全站失败。
		now := time.Now()
		probe = &IntelligenceCheckProbeResult{
			Status:       IntelligenceCheckStatusFailed,
			StartedAt:    now,
			FinishedAt:   now,
			ErrorMessage: "未配置跑测模型，本次已跳过",
		}
	} else {
		execCtx, cancel := context.WithTimeout(ctx, req.Timeout)
		probe, err = s.accountTest.RunIntelligenceCheckProbe(
			execCtx, req.AccountID, req.ModelID, req.Prompt, req.MaxTokens, req.ReasoningEffort, req.DisableStream,
		)
		timedOut = errors.Is(execCtx.Err(), context.DeadlineExceeded)
		cancel()
		if err != nil {
			return fmt.Errorf("run intelligence check probe for account %d: %w", req.AccountID, err)
		}
	}
	if probe == nil {
		now := time.Now()
		probe = &IntelligenceCheckProbeResult{Status: IntelligenceCheckStatusFailed, StartedAt: now, FinishedAt: now}
	}

	finishedAt := probe.FinishedAt
	if finishedAt.IsZero() {
		finishedAt = time.Now()
	}
	run.Status = IntelligenceCheckStatusCompleted
	// 跑测本身不做质量判定：这里只代表「产出流程走完了」，结论交给管理员人工评审。
	// 因此 verdict 一律重置为 unknown（待评审）；重跑即自动作废上一次评审结论。
	run.Verdict = IntelligenceCheckVerdictUnknown
	run.HasHTML = false
	run.HTMLBytes = 0
	run.UpstreamModel = probe.UpstreamModel
	run.LatencyMs = probe.LatencyMs
	run.FinishedAt = &finishedAt

	fail := func(code, message string) {
		run.Status = IntelligenceCheckStatusFailed
		run.Verdict = IntelligenceCheckVerdictFail
		run.HasHTML = false
		run.HTMLBytes = 0
		run.ErrorCode = code
		run.ErrorMessage = message
	}

	html := ""
	switch {
	case missingModel:
		fail(IntelligenceCheckErrNoModel, probe.ErrorMessage)
	case probe.Truncated:
		fail(IntelligenceCheckErrResponseTooLarge, "上游响应超过内存捕获上限，作品已不完整")
	case probe.Status != IntelligenceCheckStatusCompleted:
		if timedOut {
			fail(IntelligenceCheckErrTimeout, nonEmpty(probe.ErrorMessage, "跑测超时"))
		} else {
			fail(IntelligenceCheckErrStreamIncomplete, nonEmpty(probe.ErrorMessage, "上游流未正常结束"))
		}
	default:
		html = extractIntelligenceCheckHTML(probe.ResponseText)
		switch {
		case html == "":
			fail(IntelligenceCheckErrNoHTML, "响应里没有找到可用的 HTML/SVG 作品")
		case len(html) > req.MaxArtifactBytes:
			fail(IntelligenceCheckErrResponseTooLarge,
				fmt.Sprintf("作品体积 %d 字节，超过上限 %d 字节", len(html), req.MaxArtifactBytes))
		default:
			run.HasHTML = true
			run.HTMLBytes = len(html)
		}
	}

	if run.HasHTML && html != "" {
		sum := sha256.Sum256([]byte(html))
		if err := s.runRepo.SaveArtifact(ctx, &IntelligenceCheckArtifact{
			RunID:    run.ID,
			HTMLText: html,
			ByteLen:  len(html),
			SHA256:   hex.EncodeToString(sum[:]),
		}); err != nil {
			return fmt.Errorf("save artifact for run %d: %w", runID, err)
		}
	}

	if err := s.runRepo.UpdateResult(ctx, run); err != nil {
		return fmt.Errorf("persist intelligence check run %d: %w", runID, err)
	}

	// 跑测记录落定之后再补记消费：这笔钱是在上游真实花掉的，不能只留在跑测记录里。
	// 记账本身是 best-effort——写使用记录失败绝不能反过来把一次成功的跑测判成失败。
	s.recordRunUsage(ctx, run, req, probe)
	return nil
}

// IntelligenceCheckStatusSync 描述一次人工评审带来的账号状态联动结果。
type IntelligenceCheckStatusSync struct {
	// Enabled 表示全局联动开关是否打开。
	Enabled bool
	// Applied 表示本次是否真的改了账号状态（开关已开，且状态确实需要变化）。
	Applied        bool
	AccountID      int64
	PreviousStatus string
	CurrentStatus  string
}

// ReviewRun 记录管理员对一次跑测的人工评审结论，并按全局开关决定是否联动账号状态。
//
// 设计取舍：
//   - 只有「跑测成功且产出了作品」的记录才可评审。执行失败的记录本身没作品，无从评起。
//   - 联动是可选副作用。开关关闭时只落评审结论、账号状态分毫不动；开关打开但联动写库失败时，
//     评审结论依然保存（评审本身有效），同时把错误返回给调用方，明确告知「评上了，但账号没改成功」。
func (s *IntelligenceCheckService) ReviewRun(
	ctx context.Context, runID int64, verdict string, reviewerID int64,
) (*IntelligenceCheckRun, *IntelligenceCheckStatusSync, error) {
	verdict = strings.TrimSpace(verdict)
	if verdict != IntelligenceCheckVerdictPass && verdict != IntelligenceCheckVerdictFail {
		return nil, nil, ErrIntelligenceCheckInvalidVerdict
	}

	run, err := s.runRepo.GetByID(ctx, runID)
	if err != nil {
		return nil, nil, err
	}
	if run.Status != IntelligenceCheckStatusCompleted || !run.HasHTML {
		return nil, nil, ErrIntelligenceCheckNotReviewable
	}

	reviewedAt := time.Now()
	run.Verdict = verdict
	run.ReviewedBy = reviewerID
	run.ReviewedAt = &reviewedAt
	if err := s.runRepo.UpdateReview(ctx, run); err != nil {
		return nil, nil, err
	}

	sync, syncErr := s.syncAccountStatus(ctx, run)
	return run, sync, syncErr
}

// syncAccountStatus 按评审结论联动账号状态：fail -> error，pass -> active。
// 开关关闭、结论无法映射、账号已是目标状态时都不写库。
func (s *IntelligenceCheckService) syncAccountStatus(
	ctx context.Context, run *IntelligenceCheckRun,
) (*IntelligenceCheckStatusSync, error) {
	out := &IntelligenceCheckStatusSync{AccountID: run.AccountID}
	if s.settingSvc == nil || s.accountRepo == nil {
		return out, nil
	}

	settings, err := s.settingSvc.GetIntelligenceCheckGlobalSettings(ctx)
	if err != nil {
		return out, err
	}
	out.Enabled = settings.StatusSyncEnabled
	if !out.Enabled {
		return out, nil
	}

	var target string
	switch run.Verdict {
	case IntelligenceCheckVerdictFail:
		target = StatusError
	case IntelligenceCheckVerdictPass:
		target = StatusActive
	default:
		return out, nil
	}

	account, err := s.accountRepo.GetByID(ctx, run.AccountID)
	if err != nil {
		return out, err
	}
	out.PreviousStatus = account.Status
	out.CurrentStatus = target
	if account.Status == target {
		// 已经是目标状态，不产生无意义的写入。
		return out, nil
	}

	account.Status = target
	if err := s.accountRepo.Update(ctx, account); err != nil {
		return out, err
	}
	out.Applied = true
	return out, nil
}

// extractIntelligenceCheckHTML 从模型回复里提取 HTML 作品。
// 模型常把作品包在 ```html 代码块里，也可能带一段说明文字后再输出文档，
// 因此先尝试取代码块内容，再从第一个文档起始标签处截取。
func extractIntelligenceCheckHTML(responseText string) string {
	text := strings.TrimSpace(responseText)
	if text == "" {
		return ""
	}
	if fenced := extractFencedCodeBlock(text); strings.TrimSpace(fenced) != "" {
		text = strings.TrimSpace(fenced)
	}

	lower := strings.ToLower(text)
	start := -1
	for _, marker := range []string{"<!doctype", "<html", "<svg"} {
		idx := strings.Index(lower, marker)
		if idx < 0 {
			continue
		}
		if start < 0 || idx < start {
			start = idx
		}
	}
	if start < 0 {
		return ""
	}
	return strings.TrimSpace(text[start:])
}

// extractFencedCodeBlock 取出第一个 markdown 代码块的内容；没有代码块时返回空串。
func extractFencedCodeBlock(text string) string {
	open := strings.Index(text, "```")
	if open < 0 {
		return ""
	}
	rest := text[open+3:]
	newline := strings.Index(rest, "\n")
	if newline < 0 {
		return ""
	}
	rest = rest[newline+1:]
	if close := strings.Index(rest, "```"); close >= 0 {
		return rest[:close]
	}
	// 围栏未闭合：通常是被输出上限截断，交给上层判定。
	return rest
}

// ListRuns 分页查询跑测记录（按创建时间倒序）。
func (s *IntelligenceCheckService) ListRuns(ctx context.Context, filter IntelligenceCheckRunFilter) ([]*IntelligenceCheckRun, int64, error) {
	return s.runRepo.List(ctx, filter)
}

// GetRun 取单条跑测记录。
func (s *IntelligenceCheckService) GetRun(ctx context.Context, id int64) (*IntelligenceCheckRun, error) {
	return s.runRepo.GetByID(ctx, id)
}

// GetArtifact 取作品原文。
func (s *IntelligenceCheckService) GetArtifact(ctx context.Context, runID int64) (*IntelligenceCheckArtifact, error) {
	return s.runRepo.GetArtifact(ctx, runID)
}

// ListLatestByAccount 返回每个账号最近一次跑测，供卡片墙使用。
func (s *IntelligenceCheckService) ListLatestByAccount(ctx context.Context, accountIDs []int64) (map[int64]*IntelligenceCheckRun, error) {
	return s.runRepo.ListLatestByAccount(ctx, accountIDs)
}

// ListLatestPerAccount 返回每个账号最近一次跑测（按 account_id 升序）。
// 公开作品墙用它生成展示序号，因此调用方拿到的切片顺序即展示顺序。
func (s *IntelligenceCheckService) ListLatestPerAccount(ctx context.Context, limit int) ([]*IntelligenceCheckRun, error) {
	return s.runRepo.ListLatestPerAccount(ctx, limit)
}

// PruneRuns 裁剪历史跑测：每个账号只保留 keepPerAccount 条；
// before 为零值时不按时间裁，只按条数裁。返回被删除的记录数。
func (s *IntelligenceCheckService) PruneRuns(ctx context.Context, keepPerAccount int, before time.Time) (int64, error) {
	if keepPerAccount <= 0 {
		return 0, nil
	}
	return s.runRepo.PruneRuns(ctx, keepPerAccount, before)
}

func nonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
