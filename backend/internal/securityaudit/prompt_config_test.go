package securityaudit

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

type prefixEncryptor struct{REDACTED

func (prefixEncryptor) Encrypt(value string) (string, error) { return "enc:" + value, nil REDACTED
func (prefixEncryptor) Decrypt(value string) (string, error) { return value[4:], nil REDACTED

func TestDefaultConfigIsOff(t *testing.T) {
	storage, err := ParseStorageConfig("")
REDACTED
	require.False(t, storage.Enabled)
	active, err := ActiveFromStorage(storage, true, prefixEncryptor{REDACTED)
REDACTED
	require.Equal(t, ModeOff, active.EffectiveMode())
	require.Equal(t, AllScannerIDs, storage.Scanners)
	publicJSON, err := json.Marshal(PublicFromStorage(storage, true))
REDACTED
	require.Contains(t, string(publicJSON), `"group_ids":[]`)
	require.Contains(t, string(publicJSON), `"endpoints":[]`)
REDACTED

func TestConfigRejectsBlockingWithoutAudit(t *testing.T) {
	storage := DefaultStorageConfig()
	storage.BlockingEnabled = true
	require.Error(t, validateStorageConfig(storage))
REDACTED

func TestPublicConfigNeverMarshalsToken(t *testing.T) {
	storage := DefaultStorageConfig()
	storage.Endpoints = []StorageEndpoint{{ID: "one", Name: "One", Protocol: "openai_compatible", BaseURL: "http://127.0.0.1:8080", Model: DefaultGuardModel, TokenCiphertext: "GUARD_TOKEN_CANARY_SECRET", TimeoutMS: 1000, InputLimit: 1000, Enabled: trueREDACTEDREDACTED
	public := PublicFromStorage(storage, true)
	raw, err := json.Marshal(public)
REDACTED
	require.NotContains(t, string(raw), "GUARD_TOKEN_CANARY_SECRET")
	require.NotContains(t, string(raw), "ciphertext")
	require.True(t, public.Endpoints[0].HasToken)
REDACTED

func TestConfigRuntimeLoadErrorIsStableBoundedAndSecretFree(t *testing.T) {
	const canary = "CONFIG_LOAD_CANARY_SECRET"
	manager := &ConfigManager{clock: fixedClock{REDACTEDREDACTED
	manager.recordLoadError(errors.New("decrypt failed for token " + canary + " Authorization: Bearer " + canary))
	_, _, _, message := manager.RuntimeState()
	require.Equal(t, stableErrorMessage("config_load_failed"), message)
	require.NotContains(t, message, canary)
	require.LessOrEqual(t, len([]rune(message)), 160)
REDACTED

func TestConfigManagerPublicRequiresSuccessfullyLoadedSnapshot(t *testing.T) {
	t.Run("absent persisted setting is legitimate default", func(t *testing.T) {
		manager := NewConfigManager(nil, staticSettingRepository{values: map[string]string{
			SettingKeyPromptAuditConfig: "",
			SettingKeyRiskControl:       "false",
REDACTED nil, prefixEncryptor{REDACTED)
		require.NoError(t, manager.Reload(context.Background()))

		public, err := manager.Public()
	REDACTED
		require.Equal(t, int64(1), public.ConfigVersion)
		require.False(t, public.Enabled)
REDACTED)

	t.Run("persisted config activation failure is unavailable", func(t *testing.T) {
		const canary = "persisted-token-canary"
		manager := NewConfigManager(nil, staticSettingRepository{values: map[string]string{
			SettingKeyPromptAuditConfig: `{"enabled":true,"config_version":9,"endpoints":[{"token_ciphertext":"` + canary + `"REDACTED]REDACTED`,
			SettingKeyRiskControl:       "true",
REDACTED nil, prefixEncryptor{REDACTED)
		require.Error(t, manager.Reload(context.Background()))

		public, err := manager.Public()
	REDACTED
		require.Empty(t, public)
		require.Equal(t, ErrorCodeConfigUnavailable, infraerrors.Reason(err))
		require.NotContains(t, err.Error(), canary)
REDACTED)

	t.Run("reload failure preserves last successfully loaded snapshot", func(t *testing.T) {
		storage := DefaultStorageConfig()
		storage.ConfigVersion = 4
		storage.ChangeSummary = "trusted snapshot"
		raw, err := json.Marshal(storage)
	REDACTED
		repository := &switchableSettingRepository{staticSettingRepository: staticSettingRepository{values: map[string]string{
			SettingKeyPromptAuditConfig: string(raw),
			SettingKeyRiskControl:       "false",
	REDACTEDREDACTEDREDACTED
		manager := NewConfigManager(nil, repository, nil, prefixEncryptor{REDACTED)
		require.NoError(t, manager.Reload(context.Background()))
		repository.loadErr = errors.New("settings unavailable")
		require.Error(t, manager.Reload(context.Background()))

		public, err := manager.Public()
	REDACTED
		require.Equal(t, int64(4), public.ConfigVersion)
		require.Equal(t, "trusted snapshot", public.ChangeSummary)
REDACTED)
REDACTED

func TestBuildNextStoragePreserveReplaceAndClearToken(t *testing.T) {
	manager := &ConfigManager{encryptor: prefixEncryptor{REDACTEDREDACTED
	current := DefaultStorageConfig()
	current.Endpoints = []StorageEndpoint{{ID: "one", Name: "One", Protocol: "openai_compatible", BaseURL: "http://127.0.0.1:8080", Model: DefaultGuardModel, TokenCiphertext: "enc:old", TimeoutMS: 1000, InputLimit: 1000REDACTEDREDACTED
	base := UpdateConfigRequest{ExpectedConfigVersion: 1, Strategy: "priority", WorkerCount: 1, QueueCapacity: 10, Scanners: []string{"PII"REDACTED, AllGroups: true,
		Endpoints: []UpdateEndpoint{{ID: "one", Name: "One", Protocol: "openai_compatible", BaseURL: "http://127.0.0.1:8080", TimeoutMS: 1000, InputLimit: 1000REDACTEDREDACTEDREDACTED
	preserved, err := manager.buildNextStorage(current, base, 9)
REDACTED
	require.Equal(t, "enc:old", preserved.Endpoints[0].TokenCiphertext)
	replacedReq := base
	replacedReq.Endpoints = append([]UpdateEndpoint(nil), base.Endpoints...)
	replacedReq.Endpoints[0].Token = "new"
	replaced, err := manager.buildNextStorage(current, replacedReq, 9)
REDACTED
	require.Equal(t, "enc:new", replaced.Endpoints[0].TokenCiphertext)
	clearedReq := base
	clearedReq.Endpoints = append([]UpdateEndpoint(nil), base.Endpoints...)
	clearedReq.Endpoints[0].ClearToken = true
	cleared, err := manager.buildNextStorage(current, clearedReq, 9)
REDACTED
	require.Empty(t, cleared.Endpoints[0].TokenCiphertext)
REDACTED

func TestEffectiveModeTruthTable(t *testing.T) {
	tests := []struct {
		risk, enabled, blocking bool
		want                    Mode
REDACTED{
		{false, false, false, ModeOffREDACTED, {false, true, true, ModeOffREDACTED, {true, false, false, ModeOffREDACTED,
		{true, true, false, ModeAsyncREDACTED, {true, true, true, ModeBlockingREDACTED,
REDACTED
	for _, tt := range tests {
		cfg := ActiveConfig{RiskControlEnabled: tt.risk, Enabled: tt.enabled, BlockingEnabled: tt.blockingREDACTED
		require.Equal(t, tt.want, cfg.EffectiveMode())
REDACTED
REDACTED

func TestConfigManagerColdStartOnlyFailsClosedForExplicitBlockingIntent(t *testing.T) {
	manager := &ConfigManager{REDACTED

	manager.observeExpectedState(`{"enabled":true,"blocking_enabled":false,"config_version":42REDACTED`, true)
	require.Equal(t, int64(42), manager.expected.Load())
	require.Equal(t, ModeOff, manager.EffectiveMode(), "an async config version must not imply blocking")
	require.False(t, manager.BlockingActivationDegraded())

	manager.observeExpectedState(`{"enabled":true,"blocking_enabled":true,"config_version":43REDACTED`, false)
	require.Equal(t, ModeOff, manager.EffectiveMode(), "the global risk-control switch still gates blocking")

	manager.observeExpectedState(`{"enabled":true,"blocking_enabled":true,"config_version":44REDACTED`, true)
	require.Equal(t, ModeBlocking, manager.EffectiveMode())
	require.True(t, manager.BlockingActivationDegraded())

	manager.observeExpectedState(`{"enabled":true`, true)
	require.Equal(t, ModeBlocking, manager.EffectiveMode(), "undecodable storage must not erase the last known strict intent")
REDACTED

func TestConfigManagerStaleWeakerSnapshotFailsClosedWhenBlockingExpected(t *testing.T) {
	manager := &ConfigManager{REDACTED
	async := ActiveConfig{RiskControlEnabled: true, Enabled: true, BlockingEnabled: false, ConfigVersion: 1REDACTED
	manager.snapshot.Store(&activeConfigSnapshot{active: async, storage: DefaultStorageConfig(), loadedAt: fixedClock{REDACTED.Now()REDACTED)
	manager.expected.Store(2)
	manager.expectedBlocking.Store(true)

	require.True(t, manager.BlockingActivationDegraded())
	require.Equal(t, ModeBlocking, manager.EffectiveMode())

	service := &PromptService{config: manager, evaluator: NewGuardEvaluator(nil, nil, nil)REDACTED
	decision, err := service.Evaluate(context.Background(), Request{Protocol: "openai_chat_completions", Body: []byte(`{"messages":[{"role":"user","content":"hi"REDACTED]REDACTED`)REDACTED)
REDACTED
	require.Nil(t, decision)
	var guardErr *GuardError
	require.ErrorAs(t, err, &guardErr)
	require.Equal(t, ErrorCodeUnavailable, guardErr.Code)
REDACTED

type errorSettingRepository struct{ staticSettingRepository REDACTED

func (errorSettingRepository) GetMultiple(context.Context, []string) (map[string]string, error) {
	return nil, errors.New("settings unavailable")
REDACTED

type switchableSettingRepository struct {
	staticSettingRepository
	loadErr error
REDACTED

func (r *switchableSettingRepository) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	if r.loadErr != nil {
		return nil, r.loadErr
REDACTED
	return r.staticSettingRepository.GetMultiple(ctx, keys)
REDACTED

func TestConfigManagerStartupLoadFailureDoesNotBlockWhenBlockingNotIntended(t *testing.T) {
	// Settings unavailable and no prior blocking intent: stay ModeOff so the
	// gateway remains usable and admins can still disable/configure Prompt Audit.
	manager := NewConfigManager(nil, errorSettingRepository{REDACTED, nil, prefixEncryptor{REDACTED)
	err := manager.Start(context.Background())
REDACTED
	require.True(t, manager.configUntrusted.Load())
	require.False(t, manager.BlockingActivationDegraded())
	require.Equal(t, ModeOff, manager.EffectiveMode())

	service := &PromptService{config: manager, evaluator: NewGuardEvaluator(nil, nil, nil)REDACTED
	decision, evalErr := service.Evaluate(context.Background(), Request{
		Protocol: "openai_chat_completions",
		Body:     []byte(`{"messages":[{"role":"user","content":"hi"REDACTED]REDACTED`),
REDACTED)
	require.NoError(t, evalErr)
	require.NotNil(t, decision)
	require.Equal(t, DecisionAllow, decision.Kind)
	require.NoError(t, manager.Shutdown(context.Background()))
REDACTED

func TestConfigManagerStartupLoadFailureFailsClosedWhenBlockingIntended(t *testing.T) {
	manager := NewConfigManager(nil, errorSettingRepository{REDACTED, nil, prefixEncryptor{REDACTED)
	// Simulate intent observed before a later load failure (e.g. decrypt error).
	manager.observeExpectedState(`{"enabled":true,"blocking_enabled":true,"config_version":3REDACTED`, true)
	manager.markConfigUntrusted()
	require.True(t, manager.BlockingActivationDegraded())
	require.Equal(t, ModeBlocking, manager.EffectiveMode())

	service := &PromptService{config: manager, evaluator: NewGuardEvaluator(nil, nil, nil)REDACTED
	decision, err := service.Evaluate(context.Background(), Request{
		Protocol: "openai_chat_completions",
		Body:     []byte(`{"messages":[{"role":"user","content":"hi"REDACTED]REDACTED`),
REDACTED)
REDACTED
	require.Nil(t, decision)
	var guardErr *GuardError
	require.ErrorAs(t, err, &guardErr)
	require.Equal(t, ErrorCodeUnavailable, guardErr.Code)
REDACTED

func TestConfigManagerUntrustedClearsOnSuccessfulDisable(t *testing.T) {
	// After a degraded fail-closed period, saving disabled config must restore ModeOff.
	manager := &ConfigManager{encryptor: prefixEncryptor{REDACTED, clock: fixedClock{REDACTEDREDACTED
	manager.observeExpectedState(`{"enabled":true,"blocking_enabled":true,"config_version":5REDACTED`, true)
	manager.markConfigUntrusted()
	require.Equal(t, ModeBlocking, manager.EffectiveMode())

	// Install a trusted disabled snapshot the same way Save does after commit.
	disabled := DefaultStorageConfig()
	disabled.ConfigVersion = 6
	disabled.Enabled = false
	disabled.BlockingEnabled = false
	active, err := ActiveFromStorage(disabled, true, manager.encryptor)
REDACTED
	manager.expected.Store(disabled.ConfigVersion)
	manager.expectedBlocking.Store(false)
	manager.snapshot.Store(&activeConfigSnapshot{storage: disabled, active: active, loadedAt: manager.clock.Now()REDACTED)
	manager.configUntrusted.Store(false)

	require.False(t, manager.BlockingActivationDegraded())
	require.Equal(t, ModeOff, manager.EffectiveMode())

	service := &PromptService{config: manager, evaluator: NewGuardEvaluator(nil, nil, nil)REDACTED
	decision, evalErr := service.Evaluate(context.Background(), Request{
		Protocol: "openai_chat_completions",
		Body:     []byte(`{"messages":[{"role":"user","content":"hi"REDACTED]REDACTED`),
REDACTED)
	require.NoError(t, evalErr)
	require.Equal(t, DecisionAllow, decision.Kind)
REDACTED

func TestConfigManagerUntrustedWithoutBlockingDoesNotForceBlockingMode(t *testing.T) {
	manager := &ConfigManager{REDACTED
	manager.observeExpectedState(`{"enabled":true,"blocking_enabled":false,"config_version":2REDACTED`, true)
	manager.markConfigUntrusted()
	require.False(t, manager.expectedBlocking.Load())
	require.False(t, manager.BlockingActivationDegraded())
	require.Equal(t, ModeOff, manager.EffectiveMode(), "async intent + untrusted must not force blocking unavailable")
REDACTED

func TestParseLegacyConfigDefaultsMissingFieldsWithoutEnablingBlocking(t *testing.T) {
	storage, err := ParseStorageConfig(`{"enabled":false,"config_version":9REDACTED`)
REDACTED
	require.False(t, storage.BlockingEnabled)
	require.Equal(t, "priority", storage.Strategy)
	require.Equal(t, DefaultWorkerCount, storage.WorkerCount)
	require.Equal(t, DefaultQueueCapacity, storage.QueueCapacity)
	require.Equal(t, AllScannerIDs, storage.Scanners)
	require.True(t, storage.AllGroups)
REDACTED

func TestUpdateConfigStrictBoundsAndKnownValues(t *testing.T) {
	valid := promptAuditUpdateRequest(1, 1, "")
	require.NoError(t, validateUpdateConfigRequest(valid))

	tests := []struct {
		name   string
		mutate func(*UpdateConfigRequest)
		reason string
REDACTED{
		{name: "strategy", mutate: func(req *UpdateConfigRequest) { req.Strategy = "round_robin" REDACTED, reason: "prompt_audit_invalid_strategy"REDACTED,
		{name: "worker low", mutate: func(req *UpdateConfigRequest) { req.WorkerCount = 0 REDACTED, reason: "prompt_audit_invalid_worker_count"REDACTED,
		{name: "worker high", mutate: func(req *UpdateConfigRequest) { req.WorkerCount = MaxWorkerCount + 1 REDACTED, reason: "prompt_audit_invalid_worker_count"REDACTED,
		{name: "capacity low", mutate: func(req *UpdateConfigRequest) { req.QueueCapacity = 0 REDACTED, reason: "prompt_audit_invalid_queue_capacity"REDACTED,
		{name: "capacity high", mutate: func(req *UpdateConfigRequest) { req.QueueCapacity = MaxQueueCapacity + 1 REDACTED, reason: "prompt_audit_invalid_queue_capacity"REDACTED,
		{name: "unknown scanner", mutate: func(req *UpdateConfigRequest) { req.Scanners = []string{"made_up"REDACTED REDACTED, reason: "prompt_audit_invalid_scanner"REDACTED,
		{name: "group required", mutate: func(req *UpdateConfigRequest) { req.AllGroups = false; req.GroupIDs = nil REDACTED, reason: "prompt_audit_groups_required"REDACTED,
		{name: "group positive", mutate: func(req *UpdateConfigRequest) { req.AllGroups = false; req.GroupIDs = []int64{0REDACTED REDACTED, reason: "prompt_audit_invalid_group"REDACTED,
		{name: "timeout low", mutate: func(req *UpdateConfigRequest) { req.Endpoints[0].TimeoutMS = MinTimeoutMS - 1 REDACTED, reason: "prompt_audit_invalid_timeout"REDACTED,
		{name: "timeout high", mutate: func(req *UpdateConfigRequest) { req.Endpoints[0].TimeoutMS = MaxTimeoutMS + 1 REDACTED, reason: "prompt_audit_invalid_timeout"REDACTED,
		{name: "input low", mutate: func(req *UpdateConfigRequest) { req.Endpoints[0].InputLimit = MinInputLimit - 1 REDACTED, reason: "prompt_audit_invalid_input_limit"REDACTED,
		{name: "input high", mutate: func(req *UpdateConfigRequest) { req.Endpoints[0].InputLimit = MaxInputLimit + 1 REDACTED, reason: "prompt_audit_invalid_input_limit"REDACTED,
REDACTED
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := valid
			req.Scanners = append([]string(nil), valid.Scanners...)
			req.GroupIDs = append([]int64(nil), valid.GroupIDs...)
			req.Endpoints = append([]UpdateEndpoint(nil), valid.Endpoints...)
			tt.mutate(&req)
			err := validateUpdateConfigRequest(req)
		REDACTED
			require.Equal(t, tt.reason, infraerrors.Reason(err))
	REDACTED)
REDACTED
REDACTED
