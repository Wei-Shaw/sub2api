package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
)

type paymentOrderProviderSnapshot struct {
	SchemaVersion      int
	ProviderInstanceID string
	ProviderKey        string
	PaymentMode        string
	MerchantAppID      string
	MerchantID         string
	Currency           string
REDACTED

func psOrderProviderSnapshot(order *dbent.PaymentOrder) *paymentOrderProviderSnapshot {
	if order == nil || len(order.ProviderSnapshot) == 0 {
		return nil
REDACTED

	snapshot := &paymentOrderProviderSnapshot{
		SchemaVersion:      psSnapshotIntValue(order.ProviderSnapshot["schema_version"]),
		ProviderInstanceID: psSnapshotStringValue(order.ProviderSnapshot["provider_instance_id"]),
		ProviderKey:        psSnapshotStringValue(order.ProviderSnapshot["provider_key"]),
		PaymentMode:        psSnapshotStringValue(order.ProviderSnapshot["payment_mode"]),
		MerchantAppID:      psSnapshotStringValue(order.ProviderSnapshot["merchant_app_id"]),
		MerchantID:         psSnapshotStringValue(order.ProviderSnapshot["merchant_id"]),
		Currency:           psSnapshotStringValue(order.ProviderSnapshot["currency"]),
REDACTED
	if snapshot.SchemaVersion == 0 &&
		snapshot.ProviderInstanceID == "" &&
		snapshot.ProviderKey == "" &&
		snapshot.PaymentMode == "" &&
		snapshot.MerchantAppID == "" &&
		snapshot.MerchantID == "" &&
		snapshot.Currency == "" {
		return nil
REDACTED
	return snapshot
REDACTED

func psSnapshotStringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return ""
REDACTED
REDACTED

func psSnapshotIntValue(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float32:
		return int(typed)
	case float64:
		return int(typed)
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(typed))
		if err == nil {
			return n
	REDACTED
REDACTED
	return 0
REDACTED

func (s *PaymentService) resolveSnapshotOrderProviderInstance(ctx context.Context, order *dbent.PaymentOrder, snapshot *paymentOrderProviderSnapshot) (*dbent.PaymentProviderInstance, error) {
	if s == nil || s.entClient == nil || order == nil || snapshot == nil {
		return nil, nil
REDACTED

	snapshotInstanceID := strings.TrimSpace(snapshot.ProviderInstanceID)
	columnInstanceID := strings.TrimSpace(psStringValue(order.ProviderInstanceID))
	if snapshotInstanceID == "" {
		snapshotInstanceID = columnInstanceID
REDACTED
	if snapshotInstanceID == "" {
		return nil, fmt.Errorf("order %d provider snapshot is missing provider_instance_id", order.ID)
REDACTED
	if columnInstanceID != "" && snapshot.ProviderInstanceID != "" && !strings.EqualFold(columnInstanceID, snapshot.ProviderInstanceID) {
		return nil, fmt.Errorf("order %d provider snapshot instance mismatch: snapshot=%s order=%s", order.ID, snapshot.ProviderInstanceID, columnInstanceID)
REDACTED

	instID, err := strconv.ParseInt(snapshotInstanceID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("order %d provider snapshot instance id is invalid: %s", order.ID, snapshotInstanceID)
REDACTED

	inst, err := s.entClient.PaymentProviderInstance.Get(ctx, instID)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, fmt.Errorf("order %d provider snapshot instance %s is missing", order.ID, snapshotInstanceID)
	REDACTED
		return nil, err
REDACTED

	if snapshot.ProviderKey != "" && !strings.EqualFold(strings.TrimSpace(inst.ProviderKey), snapshot.ProviderKey) {
		return nil, fmt.Errorf("order %d provider snapshot key mismatch: snapshot=%s instance=%s", order.ID, snapshot.ProviderKey, inst.ProviderKey)
REDACTED

	return inst, nil
REDACTED

func expectedNotificationProviderKeyForOrder(registry *payment.Registry, order *dbent.PaymentOrder, instanceProviderKey string) string {
	if order == nil {
		return strings.TrimSpace(instanceProviderKey)
REDACTED

	orderProviderKey := psStringValue(order.ProviderKey)
	if snapshot := psOrderProviderSnapshot(order); snapshot != nil && snapshot.ProviderKey != "" {
		orderProviderKey = snapshot.ProviderKey
REDACTED

	return expectedNotificationProviderKey(registry, order.PaymentType, orderProviderKey, instanceProviderKey)
REDACTED
