package securityaudit

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

type activeConfigSnapshot struct {
	storage  storageConfig
	active   ActiveConfig
	loadedAt time.Time
REDACTED

type ConfigManager struct {
	db        *sql.DB
	settings  service.SettingRepository
	redis     *redis.Client
	encryptor SecretEncryptor
	clock     Clock

	snapshot atomic.Pointer[activeConfigSnapshot]
	expected atomic.Int64
	// expectedBlocking records the last storage intent that could be decoded,
	// independently of whether endpoint credentials or the full config could be
	// activated. A config version alone cannot distinguish async from blocking.
	expectedBlocking atomic.Bool
	// configUntrusted is set when a load/reload fails before a trustworthy
	// snapshot is installed. While set, EffectiveMode fails closed so a
	// persisted blocking policy cannot be silently skipped after startup or
	// invalidation errors.
	configUntrusted atomic.Bool

	stateMu       sync.RWMutex
	lastLoadError string
	lastErrorAt   *time.Time

	lifecycleMu sync.Mutex
	cancel      context.CancelFunc
	wg          sync.WaitGroup
REDACTED

func NewConfigManager(db *sql.DB, settings service.SettingRepository, redisClient *redis.Client, encryptor service.SecretEncryptor) *ConfigManager {
	return &ConfigManager{db: db, settings: settings, redis: redisClient, encryptor: encryptor, clock: realClock{REDACTEDREDACTED
REDACTED

func (m *ConfigManager) Start(ctx context.Context) error {
	if m == nil {
		return errors.New("prompt audit config manager unavailable")
REDACTED
	m.lifecycleMu.Lock()
	if m.cancel != nil {
		m.lifecycleMu.Unlock()
		return nil
REDACTED
	runCtx, cancel := context.WithCancel(ctx)
	m.cancel = cancel
	m.lifecycleMu.Unlock()
	loadErr := m.Reload(runCtx)
	if loadErr != nil {
		m.markConfigUntrusted()
REDACTED
	m.wg.Add(1)
	go m.refreshLoop(runCtx)
	if m.redis != nil {
		m.wg.Add(1)
		go m.subscribeLoop(runCtx)
REDACTED
	return loadErr
REDACTED

func (m *ConfigManager) Shutdown(_ context.Context) error {
	if m == nil {
		return nil
REDACTED
	m.lifecycleMu.Lock()
	cancel := m.cancel
	m.cancel = nil
	m.lifecycleMu.Unlock()
	if cancel != nil {
		cancel()
REDACTED
	m.wg.Wait()
	return nil
REDACTED

func (m *ConfigManager) Reload(ctx context.Context) error {
	if m == nil || m.settings == nil {
		m.markUntrustedIfNoActiveSnapshot()
		return errors.New("prompt audit setting repository unavailable")
REDACTED
	values, err := m.settings.GetMultiple(ctx, []string{SettingKeyPromptAuditConfig, SettingKeyRiskControlREDACTED)
	if err != nil {
		m.recordLoadError(err)
		m.markUntrustedIfNoActiveSnapshot()
		return err
REDACTED
	m.observeExpectedState(values[SettingKeyPromptAuditConfig], values[SettingKeyRiskControl] == "true")
	storage, err := ParseStorageConfig(values[SettingKeyPromptAuditConfig])
	if err != nil {
		m.recordLoadError(err)
		m.markUntrustedIfNoActiveSnapshot()
		return err
REDACTED
	m.expected.Store(storage.ConfigVersion)
	m.expectedBlocking.Store(values[SettingKeyRiskControl] == "true" && storage.Enabled && storage.BlockingEnabled)
	active, err := ActiveFromStorage(storage, values[SettingKeyRiskControl] == "true", m.encryptor)
	if err != nil {
		m.recordLoadError(err)
		// expectedBlocking may already require fail-closed via BlockingActivationDegraded.
		m.markUntrustedIfNoActiveSnapshot()
		return err
REDACTED
	now := m.clock.Now()
	m.snapshot.Store(&activeConfigSnapshot{storage: cloneStorageConfig(storage), active: cloneActiveConfig(active), loadedAt: nowREDACTED)
	m.configUntrusted.Store(false)
	m.clearLoadError()
	LogInfo(EventConfigLoaded, map[string]any{
		"config_version": storage.ConfigVersion, "status": "loaded",
REDACTED)
	return nil
REDACTED

func (m *ConfigManager) Active() (ActiveConfig, bool) {
	if m == nil {
		return ActiveConfig{REDACTED, false
REDACTED
	snapshot := m.snapshot.Load()
	if snapshot == nil {
		return ActiveConfig{REDACTED, false
REDACTED
	return cloneActiveConfig(snapshot.active), true
REDACTED

func (m *ConfigManager) BlockingActivationDegraded() bool {
	if m == nil {
		return false
REDACTED
	if m.configUntrusted.Load() {
		return true
REDACTED
	if !m.expectedBlocking.Load() {
		return false
REDACTED
	active, ok := m.Active()
	if !ok {
		return true
REDACTED
	// A still-active weaker snapshot after a failed blocking activation must not
	// keep serving allow decisions under the old off/async mode.
	return active.EffectiveMode() != ModeBlocking
REDACTED

func (m *ConfigManager) EffectiveMode() Mode {
	if m != nil && m.BlockingActivationDegraded() {
		return ModeBlocking
REDACTED
	active, ok := m.Active()
	if !ok {
		return ModeOff
REDACTED
	return active.EffectiveMode()
REDACTED

func (m *ConfigManager) markConfigUntrusted() {
	if m == nil {
		return
REDACTED
	m.configUntrusted.Store(true)
REDACTED

func (m *ConfigManager) markUntrustedIfNoActiveSnapshot() {
	if m == nil {
		return
REDACTED
	if _, ok := m.Active(); !ok {
		m.markConfigUntrusted()
REDACTED
REDACTED

func (m *ConfigManager) Public() PublicConfig {
	if m == nil {
		return PublicFromStorage(DefaultStorageConfig(), false)
REDACTED
	snapshot := m.snapshot.Load()
	if snapshot == nil {
		return PublicFromStorage(DefaultStorageConfig(), false)
REDACTED
	return PublicFromStorage(cloneStorageConfig(snapshot.storage), snapshot.active.RiskControlEnabled)
REDACTED

func (m *ConfigManager) Save(ctx context.Context, req UpdateConfigRequest, actorID int64) (PublicConfig, error) {
	if m == nil || m.db == nil || m.encryptor == nil {
		return PublicConfig{REDACTED, errors.New("prompt audit config persistence unavailable")
REDACTED
	if req.ExpectedConfigVersion < 1 {
		return PublicConfig{REDACTED, infraerrors.BadRequest("prompt_audit_expected_config_version_required", "必须提供有效的配置版本")
REDACTED
	tx, err := m.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommittedREDACTED)
	if err != nil {
		return PublicConfig{REDACTED, err
REDACTED
	defer func() { _ = tx.Rollback() REDACTED()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, promptAuditConfigLockKey); err != nil {
		return PublicConfig{REDACTED, err
REDACTED
	current := DefaultStorageConfig()
	var raw string
	err = tx.QueryRowContext(ctx, `SELECT value FROM settings WHERE key=$1 FOR UPDATE`, SettingKeyPromptAuditConfig).Scan(&raw)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return PublicConfig{REDACTED, err
REDACTED
	if err == nil {
		current, err = ParseStorageConfig(raw)
		if err != nil {
			return PublicConfig{REDACTED, err
	REDACTED
REDACTED
	if current.ConfigVersion != req.ExpectedConfigVersion {
		return PublicConfig{REDACTED, infraerrors.Conflict(ErrorCodeConfigConflict, "提示词审计配置已被其他管理员更新")
REDACTED
	next, err := m.buildNextStorage(current, req, actorID)
	if err != nil {
		return PublicConfig{REDACTED, err
REDACTED
	next.ConfigVersion = current.ConfigVersion + 1
	next.UpdatedAt = m.clock.Now()
	next.UpdatedBy = actorID
	next.ChangeSummary = changeSummary(next)
	rawNext, err := json.Marshal(next)
	if err != nil {
		return PublicConfig{REDACTED, err
REDACTED
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO settings (key,value,updated_at) VALUES ($1,$2,NOW())
		ON CONFLICT (key) DO UPDATE SET value=EXCLUDED.value, updated_at=EXCLUDED.updated_at`,
		SettingKeyPromptAuditConfig, string(rawNext)); err != nil {
		return PublicConfig{REDACTED, err
REDACTED
	if err := tx.Commit(); err != nil {
		return PublicConfig{REDACTED, err
REDACTED
	// Install the snapshot with the current global gate, not merely the value
	// cached when this process last reloaded Prompt Audit configuration.
	riskControlEnabled := m.currentRiskControlEnabled()
	if values, getErr := m.settings.GetMultiple(ctx, []string{SettingKeyRiskControlREDACTED); getErr == nil {
		riskControlEnabled = values[SettingKeyRiskControl] == "true"
REDACTED
	active, err := ActiveFromStorage(next, riskControlEnabled, m.encryptor)
	if err != nil {
		return PublicConfig{REDACTED, err
REDACTED
	m.expected.Store(next.ConfigVersion)
	m.expectedBlocking.Store(active.RiskControlEnabled && next.Enabled && next.BlockingEnabled)
	m.snapshot.Store(&activeConfigSnapshot{storage: cloneStorageConfig(next), active: cloneActiveConfig(active), loadedAt: m.clock.Now()REDACTED)
	m.clearLoadError()
	LogInfo(EventConfigUpdated, map[string]any{
		"config_version": next.ConfigVersion, "status": "updated",
REDACTED)
	if m.redis != nil {
		if err := m.redis.Publish(ctx, ConfigInvalidationChannel, strconv.FormatInt(next.ConfigVersion, 10)).Err(); err != nil {
			LogWarn(EventConfigReloadDegraded, map[string]any{
				"config_version": next.ConfigVersion, "status": "degraded", "error_code": "config_invalidation_publish_failed",
		REDACTED)
	REDACTED
REDACTED
	return PublicFromStorage(next, active.RiskControlEnabled), nil
REDACTED

func (m *ConfigManager) buildNextStorage(current storageConfig, req UpdateConfigRequest, actorID int64) (storageConfig, error) {
	if err := validateUpdateConfigRequest(req); err != nil {
		return storageConfig{REDACTED, err
REDACTED
	currentByID := make(map[string]StorageEndpoint, len(current.Endpoints))
	for _, endpoint := range current.Endpoints {
		currentByID[endpoint.ID] = endpoint
REDACTED
	next := storageConfig{
		Enabled: req.Enabled, BlockingEnabled: req.BlockingEnabled, StorePassEvents: req.StorePassEvents,
		Strategy: strings.TrimSpace(req.Strategy), WorkerCount: req.WorkerCount,
		QueueCapacity: req.QueueCapacity, Scanners: append([]string(nil), req.Scanners...),
		AllGroups: req.AllGroups, GroupIDs: append([]int64(nil), req.GroupIDs...),
		ConfigVersion: current.ConfigVersion, UpdatedBy: actorID,
		Endpoints: make([]StorageEndpoint, 0, len(req.Endpoints)),
REDACTED
	for _, endpoint := range req.Endpoints {
		baseURL, err := NormalizeBaseURL(endpoint.BaseURL)
		if err != nil {
			return storageConfig{REDACTED, err
	REDACTED
		stored := StorageEndpoint{
			ID: strings.TrimSpace(endpoint.ID), Name: strings.TrimSpace(endpoint.Name),
			Protocol: strings.TrimSpace(endpoint.Protocol), BaseURL: baseURL, Model: strings.TrimSpace(endpoint.Model),
			TimeoutMS: endpoint.TimeoutMS, InputLimit: endpoint.InputLimit, Enabled: endpoint.Enabled,
	REDACTED
		old, hadOld := currentByID[stored.ID]
		switch {
		case endpoint.ClearToken:
			stored.TokenCiphertext = ""
		case strings.TrimSpace(endpoint.Token) != "":
			ciphertext, err := m.encryptor.Encrypt(strings.TrimSpace(endpoint.Token))
			if err != nil {
				return storageConfig{REDACTED, fmt.Errorf("encrypt prompt audit endpoint token: %w", err)
		REDACTED
			stored.TokenCiphertext = ciphertext
		case hadOld:
			stored.TokenCiphertext = old.TokenCiphertext
	REDACTED
		next.Endpoints = append(next.Endpoints, stored)
REDACTED
	normalizeStorageConfig(&next)
	if err := validateStorageConfig(next); err != nil {
		return storageConfig{REDACTED, err
REDACTED
	return next, nil
REDACTED

func (m *ConfigManager) RuntimeState() (expected int64, active int64, loadedAt *time.Time, loadError string) {
	if m == nil {
		return 1, 0, nil, "config_manager_unavailable"
REDACTED
	expected = m.expected.Load()
	if expected < 1 {
		expected = 1
REDACTED
	if snapshot := m.snapshot.Load(); snapshot != nil {
		active = snapshot.active.ConfigVersion
		value := snapshot.loadedAt
		loadedAt = &value
REDACTED
	m.stateMu.RLock()
	loadError = m.lastLoadError
	m.stateMu.RUnlock()
	return
REDACTED

func (m *ConfigManager) Encrypt(value string) (string, error) { return m.encryptor.Encrypt(value) REDACTED
func (m *ConfigManager) Decrypt(value string) (string, error) { return m.encryptor.Decrypt(value) REDACTED

func (m *ConfigManager) currentRiskControlEnabled() bool {
	if snapshot := m.snapshot.Load(); snapshot != nil {
		return snapshot.active.RiskControlEnabled
REDACTED
	return false
REDACTED

func (m *ConfigManager) observeExpectedState(raw string, riskControlEnabled bool) {
	if m == nil {
		return
REDACTED
	if strings.TrimSpace(raw) == "" {
		m.expected.Store(1)
		m.expectedBlocking.Store(false)
		return
REDACTED
	var intent struct {
		Enabled         bool  `json:"enabled"`
		BlockingEnabled bool  `json:"blocking_enabled"`
		ConfigVersion   int64 `json:"config_version"`
REDACTED
	if err := json.Unmarshal([]byte(raw), &intent); err != nil {
		return
REDACTED
	if intent.ConfigVersion < 1 {
		intent.ConfigVersion = 1
REDACTED
	m.expected.Store(intent.ConfigVersion)
	m.expectedBlocking.Store(riskControlEnabled && intent.Enabled && intent.BlockingEnabled)
REDACTED

func (m *ConfigManager) refreshLoop(ctx context.Context) {
	defer m.wg.Done()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := m.Reload(ctx); err != nil {
				LogWarn(EventConfigReloadDegraded, map[string]any{"status": "degraded", "error_code": "config_ttl_reload_failed"REDACTED)
		REDACTED
	REDACTED
REDACTED
REDACTED

func (m *ConfigManager) subscribeLoop(ctx context.Context) {
	defer m.wg.Done()
	pubsub := m.redis.Subscribe(ctx, ConfigInvalidationChannel)
	defer func() { _ = pubsub.Close() REDACTED()
	channel := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case message, ok := <-channel:
			if !ok {
				return
		REDACTED
			version, err := strconv.ParseInt(strings.TrimSpace(message.Payload), 10, 64)
			if err != nil || version < 1 {
				continue
		REDACTED
			m.expected.Store(version)
			if err := m.Reload(ctx); err != nil {
				// A newer published version failed to activate. Until reload
				// succeeds, do not keep serving a potentially stale weaker mode.
				if active, ok := m.Active(); !ok || active.ConfigVersion < version {
					m.markConfigUntrusted()
			REDACTED
				LogWarn(EventConfigReloadDegraded, map[string]any{
					"config_version": version, "status": "degraded", "error_code": "config_invalidation_reload_failed",
			REDACTED)
		REDACTED
	REDACTED
REDACTED
REDACTED

func (m *ConfigManager) recordLoadError(_ error) {
	if m == nil {
		return
REDACTED
	now := m.clock.Now()
	m.stateMu.Lock()
	m.lastLoadError = stableErrorMessage("config_load_failed")
	m.lastErrorAt = &now
	m.stateMu.Unlock()
REDACTED

func (m *ConfigManager) clearLoadError() {
	m.stateMu.Lock()
	m.lastLoadError = ""
	m.lastErrorAt = nil
	m.stateMu.Unlock()
REDACTED

func cloneStorageConfig(cfg storageConfig) storageConfig {
	cfg.Scanners = append([]string(nil), cfg.Scanners...)
	cfg.GroupIDs = append([]int64(nil), cfg.GroupIDs...)
	cfg.Endpoints = append([]StorageEndpoint(nil), cfg.Endpoints...)
	return cfg
REDACTED

func cloneActiveConfig(cfg ActiveConfig) ActiveConfig {
	cfg.Scanners = append([]string(nil), cfg.Scanners...)
	cfg.GroupIDs = append([]int64(nil), cfg.GroupIDs...)
	cfg.Endpoints = append([]ActiveEndpoint(nil), cfg.Endpoints...)
	return cfg
REDACTED
