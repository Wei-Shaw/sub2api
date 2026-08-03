package service

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/gin-gonic/gin"
)

const (
	codexClientAdmissionFilterReason                 = "client_restriction_codex"
	codexClientAdmissionShadowParentUnresolvedReason = "shadow_parent_unresolved"
	// MaxCodexClientAdmissionVetoAttempts bounds request-local re-selection when
	// account policy changes between candidate selection and terminal admission.
	MaxCodexClientAdmissionVetoAttempts = 10
)

var ErrCodexClientRestricted = errors.New("request is not allowed by codex client restriction")

type CodexClientAdmissionError struct {
	Result CodexClientRestrictionDetectionResult
}

func (e *CodexClientAdmissionError) Error() string {
	if e == nil {
		return ErrCodexClientRestricted.Error()
	}
	return fmt.Sprintf("%s: %s", ErrCodexClientRestricted, e.Result.Reason)
}

func (e *CodexClientAdmissionError) Unwrap() error {
	return ErrCodexClientRestricted
}

type codexClientAdmissionContextKey struct{}

// codexClientAdmissionSnapshot freezes the global policy and request identity
// once at ingress. Account-specific evaluation only has two variants: the
// normal restricted account and an account that additionally enables the
// app-server escape hatch.
type codexClientAdmissionSnapshot struct {
	standard          CodexClientRestrictionDetectionResult
	appServer         CodexClientRestrictionDetectionResult
	enforcementActive bool

	mu                      sync.Mutex
	lastDenied              *CodexClientRestrictionDetectionResult
	skippedStickyAccountIDs map[int64]struct{}
}

func (s *OpenAIGatewayService) WithOpenAICodexClientAdmission(
	ctx context.Context,
	c *gin.Context,
	body []byte,
) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	policy := CodexRestrictionPolicy{EngineFingerprintSignals: openai.DefaultEngineFingerprintSignals}
	if s != nil && s.settingService != nil {
		policy = s.settingService.GetCodexRestrictionPolicy(ctx)
	}
	detector := s.getCodexClientRestrictionDetector()
	standard := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra:    map[string]any{"codex_cli_only": true},
	}
	appServer := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			"codex_cli_only":                  true,
			"codex_cli_only_allow_app_server": true,
		},
	}
	snapshot := &codexClientAdmissionSnapshot{
		standard:  detector.Detect(c, standard, policy, body),
		appServer: detector.Detect(c, appServer, policy, body),
	}
	snapshot.enforcementActive = !snapshot.standard.Matched || !snapshot.appServer.Matched
	// When both account variants are admitted, no account-level codex_cli_only
	// setting can reject this request. Keep the frozen detection snapshot for
	// the final forwarding log/guard, but leave enforcement inactive so official
	// clients preserve eager sticky binding and the zero-extra-cache-read path.
	return context.WithValue(ctx, codexClientAdmissionContextKey{}, snapshot)
}

func codexClientAdmissionSnapshotFromContext(ctx context.Context) (*codexClientAdmissionSnapshot, bool) {
	if ctx == nil {
		return nil, false
	}
	snapshot, ok := ctx.Value(codexClientAdmissionContextKey{}).(*codexClientAdmissionSnapshot)
	return snapshot, ok && snapshot != nil
}

func codexClientAdmissionActive(ctx context.Context) bool {
	snapshot, ok := codexClientAdmissionSnapshotFromContext(ctx)
	return ok && snapshot.enforcementActive
}

func OpenAICodexClientAdmissionActive(ctx context.Context) bool {
	return codexClientAdmissionActive(ctx)
}

func codexClientAdmissionAppliesToAccount(ctx context.Context, account *Account) bool {
	return codexClientAdmissionActive(ctx) && account != nil &&
		(account.IsOpenAIOAuth() || account.IsShadow())
}

func openAIStickyAdmissionDeferred(ctx context.Context) bool {
	return gatewayProfitControlGateActive(ctx) || codexClientAdmissionActive(ctx)
}

func (snapshot *codexClientAdmissionSnapshot) resultFor(account *Account) (CodexClientRestrictionDetectionResult, bool) {
	if snapshot == nil || account == nil || !account.IsCodexCLIOnlyEnabled() {
		return CodexClientRestrictionDetectionResult{}, false
	}
	if account.IsCodexCLIOnlyAppServerAllowed() {
		return snapshot.appServer, true
	}
	return snapshot.standard, true
}

func (snapshot *codexClientAdmissionSnapshot) deniedResult() (CodexClientRestrictionDetectionResult, bool) {
	if snapshot == nil {
		return CodexClientRestrictionDetectionResult{}, false
	}
	snapshot.mu.Lock()
	defer snapshot.mu.Unlock()
	if snapshot.lastDenied == nil {
		return CodexClientRestrictionDetectionResult{}, false
	}
	return *snapshot.lastDenied, true
}

// hadDenied reports whether this request actually encountered an account that
// the frozen client policy rejected. The precomputed standard/app-server
// results describe what would happen if such an account were evaluated; they
// must not by themselves turn an unrelated empty pool into a typed 403.
func (snapshot *codexClientAdmissionSnapshot) hadDenied() bool {
	if snapshot == nil {
		return false
	}
	snapshot.mu.Lock()
	defer snapshot.mu.Unlock()
	return snapshot.lastDenied != nil
}

func (snapshot *codexClientAdmissionSnapshot) recordSkippedSticky(accountID int64) {
	if snapshot == nil || accountID <= 0 {
		return
	}
	snapshot.mu.Lock()
	if snapshot.skippedStickyAccountIDs == nil {
		snapshot.skippedStickyAccountIDs = make(map[int64]struct{})
	}
	snapshot.skippedStickyAccountIDs[accountID] = struct{}{}
	snapshot.mu.Unlock()
}

func (snapshot *codexClientAdmissionSnapshot) stickyWasSkipped(accountID int64) bool {
	if snapshot == nil || accountID <= 0 {
		return false
	}
	snapshot.mu.Lock()
	defer snapshot.mu.Unlock()
	_, ok := snapshot.skippedStickyAccountIDs[accountID]
	return ok
}

func recordCodexClientAdmissionStickySkip(ctx context.Context, accountID int64) {
	snapshot, ok := codexClientAdmissionSnapshotFromContext(ctx)
	if !ok {
		return
	}
	snapshot.recordSkippedSticky(accountID)
}

func (snapshot *codexClientAdmissionSnapshot) recordDenied(result CodexClientRestrictionDetectionResult) {
	if snapshot == nil || !result.Enabled || result.Matched {
		return
	}
	snapshot.mu.Lock()
	copy := result
	snapshot.lastDenied = &copy
	snapshot.mu.Unlock()
}

func codexClientAdmissionDenied(ctx context.Context) bool {
	snapshot, ok := codexClientAdmissionSnapshotFromContext(ctx)
	if !ok {
		return false
	}
	return snapshot.hadDenied()
}

func codexClientAdmissionErrorFromContext(ctx context.Context) error {
	snapshot, ok := codexClientAdmissionSnapshotFromContext(ctx)
	if !ok {
		return ErrCodexClientRestricted
	}
	result, ok := snapshot.deniedResult()
	if !ok {
		return ErrCodexClientRestricted
	}
	return &CodexClientAdmissionError{Result: result}
}

func CodexClientAdmissionErrorFromContext(ctx context.Context) error {
	return codexClientAdmissionErrorFromContext(ctx)
}

func CodexClientRestrictionResultFromError(err error) (CodexClientRestrictionDetectionResult, bool) {
	var admissionErr *CodexClientAdmissionError
	if errors.As(err, &admissionErr) && admissionErr != nil {
		return admissionErr.Result, true
	}
	return CodexClientRestrictionDetectionResult{}, false
}

func (s *OpenAIGatewayService) codexRestrictionAccount(
	ctx context.Context,
	account *Account,
	lookup func(int64) *Account,
) (*Account, error) {
	if account == nil || account.ParentAccountID == nil {
		return account, nil
	}
	parentID := *account.ParentAccountID
	var parent *Account
	if lookup != nil {
		parent = lookup(parentID)
	} else if s != nil && s.schedulerSnapshot != nil {
		parent, _ = s.schedulerSnapshot.GetAccount(ctx, parentID)
	} else if s != nil && s.accountRepo != nil {
		parent, _ = s.accountRepo.GetByID(ctx, parentID)
	}
	if err := validateCodexRestrictionParent(parent, parentID); err != nil {
		return nil, err
	}
	return parent, nil
}

func (s *OpenAIGatewayService) codexRestrictionAccountLatest(ctx context.Context, account *Account) (*Account, error) {
	if account == nil || account.ParentAccountID == nil {
		return account, nil
	}
	parentID := *account.ParentAccountID
	var parent *Account
	if s != nil && s.schedulerSnapshot != nil {
		parent, _ = s.schedulerSnapshot.GetAccount(ctx, parentID)
	} else if s != nil && s.accountRepo != nil {
		parent, _ = s.accountRepo.GetByID(ctx, parentID)
	}
	if err := validateCodexRestrictionParent(parent, parentID); err != nil {
		return nil, err
	}
	return parent, nil
}

func validateCodexRestrictionParent(parent *Account, parentID int64) error {
	if parent == nil {
		return fmt.Errorf("shadow parent %d not found", parentID)
	}
	if parent.IsShadow() {
		return fmt.Errorf("shadow parent %d is itself a shadow", parent.ID)
	}
	if !parent.IsOpenAIOAuth() {
		return fmt.Errorf("shadow parent %d is not OpenAI OAuth", parent.ID)
	}
	return nil
}

func codexClientAdmissionShadowParentUnresolvedResult() CodexClientRestrictionDetectionResult {
	return CodexClientRestrictionDetectionResult{
		Enabled: true,
		Matched: false,
		Reason:  codexClientAdmissionShadowParentUnresolvedReason,
	}
}

func (s *OpenAIGatewayService) codexClientAdmissionVetoReason(
	ctx context.Context,
	account *Account,
	lookup func(int64) *Account,
) (bool, string) {
	snapshot, ok := codexClientAdmissionSnapshotFromContext(ctx)
	if !ok {
		return false, ""
	}
	effective, err := s.codexRestrictionAccount(ctx, account, lookup)
	if err != nil {
		// A shadow forwards with its parent's credentials. If that parent cannot
		// be resolved with the same topology guarantees as credential resolution,
		// admitting the shadow would create a policy bypass window when the later
		// token lookup recovers. Exclude it locally and preserve the generic client
		// policy response without sending anything upstream.
		result := codexClientAdmissionShadowParentUnresolvedResult()
		snapshot.recordDenied(result)
		return true, codexClientAdmissionFilterReason
	}
	result, active := snapshot.resultFor(effective)
	if !active || result.Matched {
		return false, ""
	}
	snapshot.recordDenied(result)
	return true, codexClientAdmissionFilterReason
}

func (s *OpenAIGatewayService) codexClientAdmissionVeto(
	ctx context.Context,
	account *Account,
) (bool, CodexClientRestrictionDetectionResult) {
	if account == nil {
		return false, CodexClientRestrictionDetectionResult{}
	}
	snapshot, ok := codexClientAdmissionSnapshotFromContext(ctx)
	if !ok {
		return false, CodexClientRestrictionDetectionResult{}
	}
	effective, err := s.codexRestrictionAccountLatest(ctx, account)
	if err != nil {
		logger.LegacyPrintf("service.openai_gateway", "[CodexAdmission] shadow parent refresh failed: account_id=%d parent_id=%v error=%v", account.ID, account.ParentAccountID, err)
		result := codexClientAdmissionShadowParentUnresolvedResult()
		snapshot.recordDenied(result)
		return true, result
	}
	result, active := snapshot.resultFor(effective)
	if !active || result.Matched {
		return false, result
	}
	snapshot.recordDenied(result)
	return true, result
}

func (s *OpenAIGatewayService) detectCodexClientRestrictionForForward(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
) CodexClientRestrictionDetectionResult {
	effective, err := s.codexRestrictionAccountLatest(ctx, account)
	if err != nil {
		logger.LegacyPrintf("service.openai_gateway", "[CodexAdmission] forward shadow parent resolution failed: account_id=%d parent_id=%v error=%v", account.ID, account.ParentAccountID, err)
		return codexClientAdmissionShadowParentUnresolvedResult()
	}
	if snapshot, ok := codexClientAdmissionSnapshotFromContext(ctx); ok {
		if result, active := snapshot.resultFor(effective); active {
			return result
		}
		return CodexClientRestrictionDetectionResult{Reason: CodexClientRestrictionReasonDisabled}
	}
	return s.detectCodexClientRestriction(c, effective, body)
}

func (s *OpenAIGatewayService) enforceOpenAICodexClientAdmissionForForward(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
) error {
	var result CodexClientRestrictionDetectionResult
	denied := false
	if codexClientAdmissionActive(ctx) {
		denied, result = s.codexClientAdmissionVeto(ctx, account)
	} else {
		result = s.detectCodexClientRestrictionForForward(ctx, c, account, body)
		denied = result.Enabled && !result.Matched
	}
	logCodexCLIOnlyDetection(ctx, c, account, getAPIKeyIDFromContext(c), result, body)
	if !denied {
		return nil
	}
	MarkOpsClientBusinessLimited(c, OpsClientBusinessLimitedReasonLocalPolicyDenied)
	return &CodexClientAdmissionError{Result: result}
}
