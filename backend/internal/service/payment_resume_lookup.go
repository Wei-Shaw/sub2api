package service

import (
	"context"
	"fmt"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

func (s *PaymentService) GetPublicOrderByResumeToken(ctx context.Context, token string) (*dbent.PaymentOrder, error) {
	claims, err := s.paymentResume().ParseToken(strings.TrimSpace(token))
	if err != nil {
		return nil, err
REDACTED

	order, err := s.entClient.PaymentOrder.Get(ctx, claims.OrderID)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, infraerrors.NotFound("NOT_FOUND", "order not found")
	REDACTED
		return nil, fmt.Errorf("get order by resume token: %w", err)
REDACTED
	if claims.UserID > 0 && order.UserID != claims.UserID {
		return nil, invalidResumeTokenMatchError()
REDACTED
	snapshot := psOrderProviderSnapshot(order)
	orderProviderInstanceID := strings.TrimSpace(psStringValue(order.ProviderInstanceID))
	orderProviderKey := strings.TrimSpace(psStringValue(order.ProviderKey))
	if snapshot != nil {
		if snapshot.ProviderInstanceID != "" {
			orderProviderInstanceID = snapshot.ProviderInstanceID
	REDACTED
		if snapshot.ProviderKey != "" {
			orderProviderKey = snapshot.ProviderKey
	REDACTED
REDACTED
	if claims.ProviderInstanceID != "" && orderProviderInstanceID != claims.ProviderInstanceID {
		return nil, invalidResumeTokenMatchError()
REDACTED
	if claims.ProviderKey != "" && !strings.EqualFold(orderProviderKey, claims.ProviderKey) {
		return nil, invalidResumeTokenMatchError()
REDACTED
	if claims.PaymentType != "" && NormalizeVisibleMethod(order.PaymentType) != NormalizeVisibleMethod(claims.PaymentType) {
		return nil, invalidResumeTokenMatchError()
REDACTED
	if order.Status == OrderStatusPending || order.Status == OrderStatusExpired {
		result := s.reconcilePaid(ctx, order)
		if result == checkPaidResultAlreadyPaid {
			order, err = s.entClient.PaymentOrder.Get(ctx, order.ID)
			if err != nil {
				return nil, fmt.Errorf("reload order by resume token: %w", err)
		REDACTED
	REDACTED
REDACTED

	return order, nil
REDACTED

func invalidResumeTokenMatchError() error {
	return infraerrors.BadRequest("INVALID_RESUME_TOKEN", "resume token does not match the payment order")
REDACTED

func (s *PaymentService) ParseWeChatPaymentResumeToken(token string) (*WeChatPaymentResumeClaims, error) {
	return s.paymentResume().ParseWeChatPaymentResumeToken(strings.TrimSpace(token))
REDACTED
