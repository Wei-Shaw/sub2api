package service

import (
	"context"
	"fmt"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
)

func (s *PaymentService) GetPublicOrderByResumeToken(ctx context.Context, token string) (*dbent.PaymentOrder, error) {
	claims, err := s.paymentResume().ParseToken(strings.TrimSpace(token))
	if err != nil {
		return nil, err
REDACTED

	order, err := s.entClient.PaymentOrder.Get(ctx, claims.OrderID)
	if err != nil {
		return nil, fmt.Errorf("get order by resume token: %w", err)
REDACTED
	if claims.UserID > 0 && order.UserID != claims.UserID {
		return nil, fmt.Errorf("resume token user mismatch")
REDACTED
	if claims.ProviderInstanceID != "" && strings.TrimSpace(psStringValue(order.ProviderInstanceID)) != claims.ProviderInstanceID {
		return nil, fmt.Errorf("resume token provider instance mismatch")
REDACTED
	if claims.ProviderKey != "" && strings.TrimSpace(psStringValue(order.ProviderKey)) != claims.ProviderKey {
		return nil, fmt.Errorf("resume token provider key mismatch")
REDACTED
	if claims.PaymentType != "" && strings.TrimSpace(order.PaymentType) != claims.PaymentType {
		return nil, fmt.Errorf("resume token payment type mismatch")
REDACTED

	return order, nil
REDACTED

func (s *PaymentService) ParseWeChatPaymentResumeToken(token string) (*WeChatPaymentResumeClaims, error) {
	return s.paymentResume().ParseWeChatPaymentResumeToken(strings.TrimSpace(token))
REDACTED
