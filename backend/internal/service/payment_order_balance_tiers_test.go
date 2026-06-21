package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

func TestCreateOrderPersistsBalanceTierSnapshot(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	user, err := client.User.Create().
		SetEmail("tier@example.com").
		SetPasswordHash("hash").
		SetUsername("tier-user").
		SetStatus(StatusActive).
		Save(ctx)
	require.NoError(t, err)

	provider := &paymentBalanceTierProvider{}
	registry := payment.NewRegistry()
	registry.Register(provider)
	svc := &PaymentService{
		entClient: client,
		registry:  registry,
		loadBalancer: &paymentBalanceTierLoadBalancer{selection: &payment.InstanceSelection{
			InstanceID:     "provider-1",
			ProviderKey:    payment.TypeEasyPay,
			SupportedTypes: "alipay",
			PaymentMode:    "popup",
			Config: map[string]string{
				"pid":         "1001",
				"pkey":        "secret",
				"apiBase":     "https://pay.example.com",
				"notifyUrl":   "https://app.example.com/notify",
				"returnUrl":   "https://app.example.com/return",
				"paymentMode": "popup",
				"currency":    payment.DefaultPaymentCurrency,
			},
		}},
		configService: &PaymentConfigService{
			entClient: client,
			settingRepo: &paymentConfigSettingRepoStub{values: map[string]string{
				SettingPaymentEnabled:      "true",
				SettingEnabledPaymentTypes: payment.TypeAlipay,
				SettingBalanceRechargeMult: "1",
				SettingBalancePricingTiers: `[{"min":100,"max":500,"multiplier":7.3,"label":"vip","enabled":true,"sortOrder":1}]`,
			}},
		},
		userRepo: &paymentBalanceTierUserRepo{user: &User{
			ID:       user.ID,
			Email:    user.Email,
			Username: user.Username,
			Status:   StatusActive,
		}},
	}

	resp, err := svc.CreateOrder(ctx, CreateOrderRequest{
		UserID:      user.ID,
		Amount:      320,
		PaymentType: payment.TypeAlipay,
		OrderType:   payment.OrderTypeBalance,
		ClientIP:    "127.0.0.1",
		SrcHost:     "example.com",
	})
	require.NoError(t, err)

	order, err := client.PaymentOrder.Get(ctx, resp.OrderID)
	require.NoError(t, err)
	require.Equal(t, 43.84, order.Amount)
	require.NotNil(t, order.AppliedRateMultiplier)
	require.Equal(t, 7.3, *order.AppliedRateMultiplier)
	require.NotNil(t, order.CreditedAmount)
	require.Equal(t, 43.84, *order.CreditedAmount)
	require.NotNil(t, order.PricingTierLabel)
	require.Equal(t, "vip", *order.PricingTierLabel)
	require.Equal(t, "vip", order.PricingTierSnapshot["label"])
}

type paymentBalanceTierProvider struct{}

func (p *paymentBalanceTierProvider) Name() string        { return "testpay" }
func (p *paymentBalanceTierProvider) ProviderKey() string { return "testpay" }
func (p *paymentBalanceTierProvider) SupportedTypes() []payment.PaymentType {
	return []payment.PaymentType{"testpay"}
}
func (p *paymentBalanceTierProvider) CreatePayment(context.Context, payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	return &payment.CreatePaymentResponse{TradeNo: "upstream-trade", PayURL: "https://pay.example.com/order"}, nil
}
func (p *paymentBalanceTierProvider) QueryOrder(context.Context, string) (*payment.QueryOrderResponse, error) {
	return nil, nil
}
func (p *paymentBalanceTierProvider) VerifyNotification(context.Context, string, map[string]string) (*payment.PaymentNotification, error) {
	return nil, nil
}
func (p *paymentBalanceTierProvider) Refund(context.Context, payment.RefundRequest) (*payment.RefundResponse, error) {
	return nil, nil
}

type paymentBalanceTierLoadBalancer struct {
	selection *payment.InstanceSelection
}

func (l *paymentBalanceTierLoadBalancer) GetInstanceConfig(context.Context, int64) (map[string]string, error) {
	return l.selection.Config, nil
}
func (l *paymentBalanceTierLoadBalancer) SelectInstance(context.Context, string, payment.PaymentType, payment.Strategy, float64) (*payment.InstanceSelection, error) {
	return l.selection, nil
}

type paymentBalanceTierUserRepo struct {
	user *User
}

func (r *paymentBalanceTierUserRepo) Create(context.Context, *User) error { return nil }
func (r *paymentBalanceTierUserRepo) GetByID(context.Context, int64) (*User, error) {
	return r.user, nil
}
func (r *paymentBalanceTierUserRepo) GetByEmail(context.Context, string) (*User, error) {
	return r.user, nil
}
func (r *paymentBalanceTierUserRepo) GetFirstAdmin(context.Context) (*User, error) {
	return r.user, nil
}
func (r *paymentBalanceTierUserRepo) Update(context.Context, *User) error { return nil }
func (r *paymentBalanceTierUserRepo) Delete(context.Context, int64) error { return nil }
func (r *paymentBalanceTierUserRepo) GetUserAvatar(context.Context, int64) (*UserAvatar, error) {
	return nil, nil
}
func (r *paymentBalanceTierUserRepo) GetByIDIncludeDeleted(ctx context.Context, id int64) (*User, error) {
	return r.GetByID(ctx, id)
}
func (r *paymentBalanceTierUserRepo) UpsertUserAvatar(context.Context, int64, UpsertUserAvatarInput) (*UserAvatar, error) {
	return nil, nil
}
func (r *paymentBalanceTierUserRepo) DeleteUserAvatar(context.Context, int64) error { return nil }
func (r *paymentBalanceTierUserRepo) List(context.Context, pagination.PaginationParams) ([]User, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (r *paymentBalanceTierUserRepo) ListWithFilters(context.Context, pagination.PaginationParams, UserListFilters) ([]User, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (r *paymentBalanceTierUserRepo) GetLatestUsedAtByUserIDs(context.Context, []int64) (map[int64]*time.Time, error) {
	return nil, nil
}
func (r *paymentBalanceTierUserRepo) GetLatestUsedAtByUserID(context.Context, int64) (*time.Time, error) {
	return nil, nil
}
func (r *paymentBalanceTierUserRepo) UpdateUserLastActiveAt(context.Context, int64, time.Time) error {
	return nil
}
func (r *paymentBalanceTierUserRepo) UpdateBalance(context.Context, int64, float64) error { return nil }
func (r *paymentBalanceTierUserRepo) DeductBalance(context.Context, int64, float64) error { return nil }
func (r *paymentBalanceTierUserRepo) UpdateConcurrency(context.Context, int64, int) error { return nil }
func (r *paymentBalanceTierUserRepo) BatchSetConcurrency(context.Context, []int64, int) (int, error) {
	return 0, nil
}
func (r *paymentBalanceTierUserRepo) BatchAddConcurrency(context.Context, []int64, int) (int, error) {
	return 0, nil
}
func (r *paymentBalanceTierUserRepo) ExistsByEmail(context.Context, string) (bool, error) {
	return false, nil
}
func (r *paymentBalanceTierUserRepo) RemoveGroupFromAllowedGroups(context.Context, int64) (int64, error) {
	return 0, nil
}
func (r *paymentBalanceTierUserRepo) AddGroupToAllowedGroups(context.Context, int64, int64) error {
	return nil
}
func (r *paymentBalanceTierUserRepo) RemoveGroupFromUserAllowedGroups(context.Context, int64, int64) error {
	return nil
}
func (r *paymentBalanceTierUserRepo) ListUserAuthIdentities(context.Context, int64) ([]UserAuthIdentityRecord, error) {
	return nil, nil
}
func (r *paymentBalanceTierUserRepo) UnbindUserAuthProvider(context.Context, int64, string) error {
	return nil
}
func (r *paymentBalanceTierUserRepo) UpdateTotpSecret(context.Context, int64, *string) error {
	return nil
}
func (r *paymentBalanceTierUserRepo) EnableTotp(context.Context, int64) error  { return nil }
func (r *paymentBalanceTierUserRepo) DisableTotp(context.Context, int64) error { return nil }
