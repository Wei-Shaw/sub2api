package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentorder"
	"github.com/Wei-Shaw/sub2api/ent/paymentproviderinstance"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/payment/provider"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// validateProviderConfig runs the provider's constructor to surface config-level
// errors at save time (e.g. wxpay missing certSerial), instead of only failing
// when an order is created. Returns the structured ApplicationError from the
// constructor so the frontend i18n layer can localize it.
//
// Only validates enabled instances — a disabled instance may be a half-filled
// draft the admin will complete later.
func (s *PaymentConfigService) validateProviderConfig(providerKey string, config map[string]string) error {
	_, err := provider.CreateProvider(providerKey, "_validate_", config)
	return err
REDACTED

// --- Provider Instance CRUD ---

func (s *PaymentConfigService) ListProviderInstances(ctx context.Context) ([]*dbent.PaymentProviderInstance, error) {
	return s.entClient.PaymentProviderInstance.Query().Order(paymentproviderinstance.BySortOrder()).All(ctx)
REDACTED

// ProviderInstanceResponse is the API response for a provider instance.
type ProviderInstanceResponse struct {
	ID              int64             `json:"id"`
	ProviderKey     string            `json:"provider_key"`
	Name            string            `json:"name"`
	Config          map[string]string `json:"config"`
	SupportedTypes  []string          `json:"supported_types"`
	Limits          string            `json:"limits"`
	Enabled         bool              `json:"enabled"`
	RefundEnabled   bool              `json:"refund_enabled"`
	AllowUserRefund bool              `json:"allow_user_refund"`
	SortOrder       int               `json:"sort_order"`
	PaymentMode     string            `json:"payment_mode"`
REDACTED

// ListProviderInstancesWithConfig returns provider instances with decrypted config.
func (s *PaymentConfigService) ListProviderInstancesWithConfig(ctx context.Context) ([]ProviderInstanceResponse, error) {
	instances, err := s.entClient.PaymentProviderInstance.Query().
		Order(paymentproviderinstance.BySortOrder()).All(ctx)
	if err != nil {
		return nil, err
REDACTED
	result := make([]ProviderInstanceResponse, 0, len(instances))
	for _, inst := range instances {
		resp := ProviderInstanceResponse{
			ID: int64(inst.ID), ProviderKey: inst.ProviderKey, Name: inst.Name,
			SupportedTypes: splitTypes(inst.SupportedTypes), Limits: inst.Limits,
			Enabled: inst.Enabled, RefundEnabled: inst.RefundEnabled, AllowUserRefund: inst.AllowUserRefund,
			SortOrder: inst.SortOrder, PaymentMode: inst.PaymentMode,
	REDACTED
		resp.Config, err = s.decryptAndMaskConfig(inst.ProviderKey, inst.Config)
		if err != nil {
			return nil, fmt.Errorf("decrypt config for instance %d: %w", inst.ID, err)
	REDACTED
		result = append(result, resp)
REDACTED
	return result, nil
REDACTED

// decryptAndMaskConfig returns the stored config with sensitive fields omitted.
// Admin UIs display masked placeholders for these; the raw values never leave
// the server. Callers that need the full config (e.g. payment runtime) must
// use decryptConfig directly.
func (s *PaymentConfigService) decryptAndMaskConfig(providerKey, encrypted string) (map[string]string, error) {
	cfg, err := s.decryptConfig(encrypted)
	if err != nil {
		return nil, err
REDACTED
	if cfg == nil {
		return nil, nil
REDACTED
	masked := make(map[string]string, len(cfg))
	for k, v := range cfg {
		if isSensitiveProviderConfigField(providerKey, k) {
			continue
	REDACTED
		masked[k] = v
REDACTED
	return masked, nil
REDACTED

// pendingOrderStatuses are order statuses considered "in progress".
var pendingOrderStatuses = []string{
	payment.OrderStatusPending,
	payment.OrderStatusPaid,
	payment.OrderStatusRecharging,
REDACTED

// providerSensitiveConfigFields is the authoritative list of config keys that
// are treated as secrets per provider. Must stay in sync with the frontend
// definition at frontend/src/components/payment/providerConfig.ts
// (PROVIDER_CONFIG_FIELDS, fields with sensitive: true).
//
// Key matching is case-insensitive. Non-listed keys (e.g. appId, notifyUrl,
// stripe publishableKey) are returned in plaintext by the admin GET API.
var providerSensitiveConfigFields = map[string]map[string]struct{REDACTED{
	payment.TypeEasyPay:   {"pkey": {REDACTEDREDACTED,
	payment.TypeAlipay:    {"privatekey": {REDACTED, "publickey": {REDACTED, "alipaypublickey": {REDACTEDREDACTED,
	payment.TypeWxpay:     {"privatekey": {REDACTED, "apiv3key": {REDACTED, "publickey": {REDACTEDREDACTED,
	payment.TypeStripe:    {"secretkey": {REDACTED, "webhooksecret": {REDACTEDREDACTED,
	payment.TypeAirwallex: {"apikey": {REDACTED, "webhooksecret": {REDACTEDREDACTED,
REDACTED

// providerPendingOrderProtectedConfigFields lists config keys that cannot be
// changed while the instance has in-progress orders. This includes secrets plus
// all provider identity fields that are snapshotted into orders or used by
// webhook/refund verification.
var providerPendingOrderProtectedConfigFields = map[string]map[string]struct{REDACTED{
	payment.TypeEasyPay:   {"pkey": {REDACTED, "pid": {REDACTEDREDACTED,
	payment.TypeAlipay:    {"privatekey": {REDACTED, "publickey": {REDACTED, "alipaypublickey": {REDACTED, "appid": {REDACTEDREDACTED,
	payment.TypeWxpay:     {"privatekey": {REDACTED, "apiv3key": {REDACTED, "publickey": {REDACTED, "appid": {REDACTED, "mpappid": {REDACTED, "mchid": {REDACTED, "publickeyid": {REDACTED, "certserial": {REDACTEDREDACTED,
	payment.TypeStripe:    {"secretkey": {REDACTED, "webhooksecret": {REDACTED, "currency": {REDACTEDREDACTED,
	payment.TypeAirwallex: {"clientid": {REDACTED, "apikey": {REDACTED, "webhooksecret": {REDACTED, "apibase": {REDACTED, "accountid": {REDACTED, "currency": {REDACTEDREDACTED,
REDACTED

func isSensitiveProviderConfigField(providerKey, fieldName string) bool {
	fields, ok := providerSensitiveConfigFields[providerKey]
	if !ok {
		return false
REDACTED
	_, found := fields[strings.ToLower(fieldName)]
	return found
REDACTED

func hasPendingOrderProtectedConfigChange(providerKey string, currentConfig, nextConfig map[string]string) bool {
	fields, ok := providerPendingOrderProtectedConfigFields[providerKey]
	if !ok {
		return false
REDACTED
	for fieldName := range fields {
		if providerConfigFieldValue(currentConfig, fieldName) != providerConfigFieldValue(nextConfig, fieldName) {
			return true
	REDACTED
REDACTED
	return false
REDACTED

func providerConfigFieldValue(config map[string]string, fieldName string) string {
	for key, value := range config {
		if strings.EqualFold(key, fieldName) {
			return value
	REDACTED
REDACTED
	return ""
REDACTED

func (s *PaymentConfigService) countPendingOrders(ctx context.Context, providerInstanceID int64) (int, error) {
	return s.entClient.PaymentOrder.Query().
		Where(
			paymentorder.ProviderInstanceIDEQ(strconv.FormatInt(providerInstanceID, 10)),
			paymentorder.StatusIn(pendingOrderStatuses...),
		).Count(ctx)
REDACTED

func (s *PaymentConfigService) countPendingOrdersByPlan(ctx context.Context, planID int64) (int, error) {
	return s.entClient.PaymentOrder.Query().
		Where(
			paymentorder.PlanIDEQ(planID),
			paymentorder.StatusIn(pendingOrderStatuses...),
		).Count(ctx)
REDACTED

var validProviderKeys = map[string]bool{
	payment.TypeEasyPay: true, payment.TypeAlipay: true, payment.TypeWxpay: true, payment.TypeStripe: true, payment.TypeAirwallex: true,
REDACTED

func (s *PaymentConfigService) CreateProviderInstance(ctx context.Context, req CreateProviderInstanceRequest) (*dbent.PaymentProviderInstance, error) {
	typesStr := joinTypes(req.SupportedTypes)
	if err := validateProviderRequest(req.ProviderKey, req.Name, typesStr); err != nil {
		return nil, err
REDACTED
	if req.ProviderKey == payment.TypeEasyPay {
		if err := validateEasyPayCustomMethods(req.Config, typesStr); err != nil {
			return nil, err
	REDACTED
REDACTED
	if err := s.validateVisibleMethodEnablementConflicts(ctx, 0, req.ProviderKey, typesStr, req.Enabled); err != nil {
		return nil, err
REDACTED
	if req.Enabled {
		if err := s.validateProviderConfig(req.ProviderKey, req.Config); err != nil {
			return nil, err
	REDACTED
REDACTED
	enc, err := s.encryptConfig(req.Config)
	if err != nil {
		return nil, err
REDACTED
	allowUserRefund := req.AllowUserRefund && req.RefundEnabled
	return s.entClient.PaymentProviderInstance.Create().
		SetProviderKey(req.ProviderKey).SetName(req.Name).SetConfig(enc).
		SetSupportedTypes(typesStr).SetEnabled(req.Enabled).SetPaymentMode(req.PaymentMode).
		SetSortOrder(req.SortOrder).SetLimits(req.Limits).SetRefundEnabled(req.RefundEnabled).
		SetAllowUserRefund(allowUserRefund).
		Save(ctx)
REDACTED

func validateProviderRequest(providerKey, name, supportedTypes string) error {
	if strings.TrimSpace(name) == "" {
		return infraerrors.BadRequest("VALIDATION_ERROR", "provider name is required")
REDACTED
	if !validProviderKeys[providerKey] {
		return infraerrors.BadRequest("VALIDATION_ERROR", fmt.Sprintf("invalid provider key: %s", providerKey))
REDACTED
	// supported_types can be empty (provider accepts no payment types until configured)
	return nil
REDACTED

var easyPayCustomMethodCodePattern = regexp.MustCompile(`^[a-z0-9_-]+$`)

type easyPayCustomMethodConfig struct {
	Type        string `json:"type"`
	UpstreamType string `json:"upstreamType"`
	DisplayName  string `json:"displayName"`
REDACTED

func validateEasyPayCustomMethods(config map[string]string, supportedTypes string) error {
	if config == nil {
		config = map[string]string{REDACTED
REDACTED
	raw := strings.TrimSpace(config["customMethods"])
	methods := make([]easyPayCustomMethodConfig, 0)
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &methods); err != nil {
			return infraerrors.BadRequest("VALIDATION_ERROR", "customMethods must be a JSON array")
	REDACTED
REDACTED

	customTypes := make(map[string]struct{REDACTED, len(methods))
	for _, method := range methods {
		method.Type = strings.TrimSpace(strings.ToLower(method.Type))
		method.UpstreamType = strings.TrimSpace(strings.ToLower(method.UpstreamType))
		if method.Type == "" || method.UpstreamType == "" {
			return infraerrors.BadRequest("VALIDATION_ERROR", "customMethods upstreamType is required")
	REDACTED
		if !easyPayCustomMethodCodePattern.MatchString(method.Type) {
			return infraerrors.BadRequest("VALIDATION_ERROR", "customMethods type may only contain lowercase letters, digits, underscores, and hyphens")
	REDACTED
		if !easyPayCustomMethodCodePattern.MatchString(method.UpstreamType) {
			return infraerrors.BadRequest("VALIDATION_ERROR", "customMethods upstreamType may only contain lowercase letters, digits, underscores, and hyphens")
	REDACTED
		if easyPayCustomMethodTypeConflictsWithBuiltin(method.Type) {
			return infraerrors.BadRequest("VALIDATION_ERROR", "customMethods type cannot start with alipay or wxpay")
	REDACTED
		if _, exists := customTypes[method.Type]; exists {
			return infraerrors.BadRequest("VALIDATION_ERROR", "duplicate customMethods type")
	REDACTED
		customTypes[method.Type] = struct{REDACTED{REDACTED
REDACTED

	for _, supportedType := range splitTypes(supportedTypes) {
		supportedType = strings.TrimSpace(strings.ToLower(supportedType))
		if supportedType == "" || supportedType == payment.TypeAlipay || supportedType == payment.TypeWxpay {
			continue
	REDACTED
		if _, exists := customTypes[supportedType]; !exists {
			return infraerrors.BadRequest("VALIDATION_ERROR", fmt.Sprintf("supported EasyPay custom type %s has no customMethods mapping", supportedType))
	REDACTED
REDACTED
	return nil
REDACTED

func easyPayCustomMethodTypeConflictsWithBuiltin(methodType string) bool {
	return strings.HasPrefix(methodType, payment.TypeAlipay) || strings.HasPrefix(methodType, payment.TypeWxpay)
REDACTED

// UpdateProviderInstance updates a provider instance by ID (patch semantics).
// NOTE: This function exceeds 30 lines due to per-field nil-check patch update
// boilerplate and pending-order safety checks.
func (s *PaymentConfigService) UpdateProviderInstance(ctx context.Context, id int64, req UpdateProviderInstanceRequest) (*dbent.PaymentProviderInstance, error) {
	current, err := s.entClient.PaymentProviderInstance.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("load provider instance: %w", err)
REDACTED
	var pendingOrderCount *int
	getPendingOrderCount := func() (int, error) {
		if pendingOrderCount != nil {
			return *pendingOrderCount, nil
	REDACTED
		count, err := s.countPendingOrders(ctx, id)
		if err != nil {
			return 0, fmt.Errorf("check pending orders: %w", err)
	REDACTED
		pendingOrderCount = &count
		return count, nil
REDACTED
	nextEnabled := current.Enabled
	if req.Enabled != nil {
		nextEnabled = *req.Enabled
REDACTED
	nextSupportedTypes := current.SupportedTypes
	if req.SupportedTypes != nil {
		nextSupportedTypes = joinTypes(req.SupportedTypes)
REDACTED
	if err := s.validateVisibleMethodEnablementConflicts(ctx, id, current.ProviderKey, nextSupportedTypes, nextEnabled); err != nil {
		return nil, err
REDACTED
	var mergedConfig map[string]string
	if req.Config != nil {
		currentConfig, err := s.decryptConfig(current.Config)
		if err != nil {
			return nil, fmt.Errorf("decrypt existing config: %w", err)
	REDACTED
		mergedConfig, err = s.mergeConfig(ctx, id, req.Config)
		if err != nil {
			return nil, err
	REDACTED
		if hasPendingOrderProtectedConfigChange(current.ProviderKey, currentConfig, mergedConfig) {
			count, err := getPendingOrderCount()
			if err != nil {
				return nil, err
		REDACTED
			if count > 0 {
				return nil, infraerrors.Conflict("PENDING_ORDERS", "instance has pending orders").
					WithMetadata(map[string]string{"count": strconv.Itoa(count)REDACTED)
		REDACTED
	REDACTED
REDACTED
	if req.Enabled != nil && !*req.Enabled {
		count, err := getPendingOrderCount()
		if err != nil {
			return nil, err
	REDACTED
		if count > 0 {
			return nil, infraerrors.Conflict("PENDING_ORDERS", "instance has pending orders").
				WithMetadata(map[string]string{"count": strconv.Itoa(count)REDACTED)
	REDACTED
REDACTED
	configToValidate := mergedConfig
	if configToValidate == nil {
		configToValidate, err = s.decryptConfig(current.Config)
		if err != nil {
			return nil, fmt.Errorf("decrypt existing config: %w", err)
	REDACTED
REDACTED
	if current.ProviderKey == payment.TypeEasyPay {
		if err := validateEasyPayCustomMethods(configToValidate, nextSupportedTypes); err != nil {
			return nil, err
	REDACTED
REDACTED
	// Validate merged config when the instance will end up enabled.
	// This surfaces provider-level errors (e.g. wxpay missing certSerial) at save time,
	// so admins see them in the dialog instead of only when an order is created.
	finalEnabled := current.Enabled
	if req.Enabled != nil {
		finalEnabled = *req.Enabled
REDACTED
	if finalEnabled {
		if err := s.validateProviderConfig(current.ProviderKey, configToValidate); err != nil {
			return nil, err
	REDACTED
REDACTED
	u := s.entClient.PaymentProviderInstance.UpdateOneID(id)
	if req.Name != nil {
		u.SetName(*req.Name)
REDACTED
	if mergedConfig != nil {
		enc, err := s.encryptConfig(mergedConfig)
		if err != nil {
			return nil, err
	REDACTED
		u.SetConfig(enc)
REDACTED
	if req.SupportedTypes != nil {
		// Check pending orders before removing payment types
		count, err := getPendingOrderCount()
		if err != nil {
			return nil, err
	REDACTED
		if count > 0 {
			// Load current instance to compare types
			oldTypes := strings.Split(current.SupportedTypes, ",")
			newTypes := req.SupportedTypes
			for _, ot := range oldTypes {
				ot = strings.TrimSpace(ot)
				if ot == "" {
					continue
			REDACTED
				found := false
				for _, nt := range newTypes {
					if strings.TrimSpace(nt) == ot {
						found = true
						break
				REDACTED
			REDACTED
				if !found {
					return nil, infraerrors.Conflict("PENDING_ORDERS", "cannot remove payment types while instance has pending orders").
						WithMetadata(map[string]string{"count": strconv.Itoa(count)REDACTED)
			REDACTED
		REDACTED
	REDACTED
		u.SetSupportedTypes(joinTypes(req.SupportedTypes))
REDACTED
	if req.Enabled != nil {
		u.SetEnabled(*req.Enabled)
REDACTED
	if req.SortOrder != nil {
		u.SetSortOrder(*req.SortOrder)
REDACTED
	if req.Limits != nil {
		u.SetLimits(*req.Limits)
REDACTED
	if req.RefundEnabled != nil {
		u.SetRefundEnabled(*req.RefundEnabled)
		// Cascade: turning off refund_enabled also disables allow_user_refund
		if !*req.RefundEnabled {
			u.SetAllowUserRefund(false)
	REDACTED
REDACTED
	if req.AllowUserRefund != nil {
		// Only allow enabling when refund_enabled is (or will be) true
		if *req.AllowUserRefund {
			refundEnabled := false
			if req.RefundEnabled != nil {
				refundEnabled = *req.RefundEnabled
		REDACTED else {
				refundEnabled = current.RefundEnabled
		REDACTED
			if refundEnabled {
				u.SetAllowUserRefund(true)
		REDACTED
	REDACTED else {
			u.SetAllowUserRefund(false)
	REDACTED
REDACTED
	if req.PaymentMode != nil {
		u.SetPaymentMode(*req.PaymentMode)
REDACTED
	return u.Save(ctx)
REDACTED

// GetUserRefundEligibleInstanceIDs returns provider instance IDs that allow user refund.
func (s *PaymentConfigService) GetUserRefundEligibleInstanceIDs(ctx context.Context) ([]string, error) {
	instances, err := s.entClient.PaymentProviderInstance.Query().
		Where(
			paymentproviderinstance.RefundEnabledEQ(true),
			paymentproviderinstance.AllowUserRefundEQ(true),
		).Select(paymentproviderinstance.FieldID).All(ctx)
	if err != nil {
		return nil, err
REDACTED
	ids := make([]string, 0, len(instances))
	for _, inst := range instances {
		ids = append(ids, strconv.FormatInt(int64(inst.ID), 10))
REDACTED
	return ids, nil
REDACTED

func (s *PaymentConfigService) mergeConfig(ctx context.Context, id int64, newConfig map[string]string) (map[string]string, error) {
	inst, err := s.entClient.PaymentProviderInstance.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("load existing provider: %w", err)
REDACTED
	existing, err := s.decryptConfig(inst.Config)
	if err != nil {
		return nil, fmt.Errorf("decrypt existing config for instance %d: %w", id, err)
REDACTED
	if existing == nil {
		existing = map[string]string{REDACTED
REDACTED
	for k, v := range newConfig {
		// Preserve existing secrets when the client submits an empty value
		// (admin UI omits the value to indicate "leave unchanged").
		if v == "" && isSensitiveProviderConfigField(inst.ProviderKey, k) {
			continue
	REDACTED
		existing[k] = v
REDACTED
	return existing, nil
REDACTED

// decryptConfig parses a stored provider config.
// New records are plaintext JSON; legacy records are AES-256-GCM ciphertext
// ("iv:authTag:ciphertext"). Values that cannot be parsed as either — including
// legacy ciphertext with no/invalid TOTP_ENCRYPTION_KEY — are treated as empty,
// letting the admin re-enter the config via the UI to complete the migration.
//
// TODO(deprecated-legacy-ciphertext): The AES fallback branch is a transitional
// shim for pre-plaintext records. Remove it (and the encryptionKey field) after
// a few releases once all live deployments have re-saved their provider configs.
func (s *PaymentConfigService) decryptConfig(stored string) (map[string]string, error) {
	if stored == "" {
		return nil, nil
REDACTED
	var cfg map[string]string
	if err := json.Unmarshal([]byte(stored), &cfg); err == nil {
		return cfg, nil
REDACTED
	// Deprecated: legacy AES-256-GCM ciphertext fallback — scheduled for removal.
	if len(s.encryptionKey) == payment.AES256KeySize {
		//nolint:staticcheck // SA1019: intentional legacy fallback, scheduled for removal
		if plaintext, err := payment.Decrypt(stored, s.encryptionKey); err == nil {
			if err := json.Unmarshal([]byte(plaintext), &cfg); err == nil {
				return cfg, nil
		REDACTED
	REDACTED
REDACTED
	slog.Warn("payment provider config unreadable, treating as empty for re-entry",
		"stored_len", len(stored))
	return nil, nil
REDACTED

func (s *PaymentConfigService) DeleteProviderInstance(ctx context.Context, id int64) error {
	count, err := s.countPendingOrders(ctx, id)
	if err != nil {
		return fmt.Errorf("check pending orders: %w", err)
REDACTED
	if count > 0 {
		return infraerrors.Conflict("PENDING_ORDERS",
			fmt.Sprintf("this instance has %d in-progress orders and cannot be deleted — wait for orders to complete or disable the instance first", count))
REDACTED
	return s.entClient.PaymentProviderInstance.DeleteOneID(id).Exec(ctx)
REDACTED

// encryptConfig serialises a provider config for storage.
// New records are written as plaintext JSON; the historical AES-GCM wrapping
// has been dropped but decryptConfig still accepts old ciphertext during migration.
func (s *PaymentConfigService) encryptConfig(cfg map[string]string) (string, error) {
	data, err := json.Marshal(cfg)
	if err != nil {
		return "", fmt.Errorf("marshal config: %w", err)
REDACTED
	return string(data), nil
REDACTED
