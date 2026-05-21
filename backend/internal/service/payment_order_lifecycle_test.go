//go:build unit

package service

import (
	"context"
	"database/sql"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

type paymentOrderLifecycleQueryProvider struct {
	key               string
	lastQueryTradeNo  string
	lastCancelTradeNo string
	queryCalls        int
	cancelCalls       int
	responses         []*payment.QueryOrderResponse
	resp              *payment.QueryOrderResponse
REDACTED

type paymentOrderLifecycleRedeemRepo struct {
	codesByCode map[string]*RedeemCode
	useCalls    []struct {
		id     int64
		userID int64
REDACTED
REDACTED

func (p *paymentOrderLifecycleQueryProvider) Name() string {
	return "payment-order-lifecycle-query-provider"
REDACTED

func (p *paymentOrderLifecycleQueryProvider) ProviderKey() string {
	if p.key != "" {
		return p.key
REDACTED
	return payment.TypeAlipay
REDACTED

func (p *paymentOrderLifecycleQueryProvider) SupportedTypes() []payment.PaymentType {
	return []payment.PaymentType{p.ProviderKey()REDACTED
REDACTED

func (p *paymentOrderLifecycleQueryProvider) CreatePayment(context.Context, payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	panic("unexpected call")
REDACTED

func (p *paymentOrderLifecycleQueryProvider) QueryOrder(_ context.Context, tradeNo string) (*payment.QueryOrderResponse, error) {
	p.lastQueryTradeNo = tradeNo
	p.queryCalls++
	if len(p.responses) > 0 {
		resp := p.responses[0]
		if len(p.responses) > 1 {
			p.responses = p.responses[1:]
	REDACTED
		return resp, nil
REDACTED
	return p.resp, nil
REDACTED

func (p *paymentOrderLifecycleQueryProvider) VerifyNotification(context.Context, string, map[string]string) (*payment.PaymentNotification, error) {
	panic("unexpected call")
REDACTED

func (p *paymentOrderLifecycleQueryProvider) Refund(context.Context, payment.RefundRequest) (*payment.RefundResponse, error) {
	panic("unexpected call")
REDACTED

func (p *paymentOrderLifecycleQueryProvider) CancelPayment(_ context.Context, tradeNo string) error {
	p.lastCancelTradeNo = tradeNo
	p.cancelCalls++
	return nil
REDACTED

func (r *paymentOrderLifecycleRedeemRepo) Create(context.Context, *RedeemCode) error {
	panic("unexpected call")
REDACTED

func (r *paymentOrderLifecycleRedeemRepo) CreateBatch(context.Context, []RedeemCode) error {
	panic("unexpected call")
REDACTED

func (r *paymentOrderLifecycleRedeemRepo) GetByID(_ context.Context, id int64) (*RedeemCode, error) {
	for _, code := range r.codesByCode {
		if code.ID != id {
			continue
	REDACTED
		cloned := *code
		return &cloned, nil
REDACTED
	return nil, ErrRedeemCodeNotFound
REDACTED

func (r *paymentOrderLifecycleRedeemRepo) GetByCode(_ context.Context, code string) (*RedeemCode, error) {
	redeemCode, ok := r.codesByCode[code]
	if !ok {
		return nil, ErrRedeemCodeNotFound
REDACTED
	cloned := *redeemCode
	return &cloned, nil
REDACTED

func (r *paymentOrderLifecycleRedeemRepo) Update(context.Context, *RedeemCode) error {
	panic("unexpected call")
REDACTED

func (r *paymentOrderLifecycleRedeemRepo) BatchUpdate(context.Context, []int64, RedeemCodeBatchUpdateFields) (int64, error) {
	panic("unexpected call")
REDACTED

func (r *paymentOrderLifecycleRedeemRepo) Delete(context.Context, int64) error {
	panic("unexpected call")
REDACTED

func (r *paymentOrderLifecycleRedeemRepo) Use(_ context.Context, id, userID int64) error {
	for code, redeemCode := range r.codesByCode {
		if redeemCode.ID != id {
			continue
	REDACTED
		now := time.Now().UTC()
		redeemCode.Status = StatusUsed
		redeemCode.UsedBy = &userID
		redeemCode.UsedAt = &now
		r.codesByCode[code] = redeemCode
		r.useCalls = append(r.useCalls, struct {
			id     int64
			userID int64
	REDACTED{id: id, userID: userIDREDACTED)
		return nil
REDACTED
	return ErrRedeemCodeNotFound
REDACTED

func (r *paymentOrderLifecycleRedeemRepo) List(context.Context, pagination.PaginationParams) ([]RedeemCode, *pagination.PaginationResult, error) {
	panic("unexpected call")
REDACTED

func (r *paymentOrderLifecycleRedeemRepo) ListWithFilters(context.Context, pagination.PaginationParams, string, string, string) ([]RedeemCode, *pagination.PaginationResult, error) {
	panic("unexpected call")
REDACTED

func (r *paymentOrderLifecycleRedeemRepo) ListByUser(context.Context, int64, int) ([]RedeemCode, error) {
	panic("unexpected call")
REDACTED

func (r *paymentOrderLifecycleRedeemRepo) ListByUserPaginated(context.Context, int64, pagination.PaginationParams, string) ([]RedeemCode, *pagination.PaginationResult, error) {
	panic("unexpected call")
REDACTED

func (r *paymentOrderLifecycleRedeemRepo) SumPositiveBalanceByUser(context.Context, int64) (float64, error) {
	panic("unexpected call")
REDACTED

func TestVerifyOrderByOutTradeNoBackfillsTradeNoFromPaidQuery(t *testing.T) {
	ctx := context.Background()
	client := newPaymentOrderLifecycleTestClient(t)

	user, err := client.User.Create().
		SetEmail("checkpaid@example.com").
		SetPasswordHash("hash").
		SetUsername("checkpaid-user").
		Save(ctx)
REDACTED

	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(88).
		SetPayAmount(88).
		SetFeeRate(0).
		SetRechargeCode("CHECKPAID-UPSTREAM-TRADE-NO").
		SetOutTradeNo("sub2_checkpaid_trade_no_missing").
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusPending).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		Save(ctx)
REDACTED

	userRepo := &mockUserRepo{
		getByIDUser: &User{
			ID:       user.ID,
			Email:    user.Email,
			Username: user.Username,
			Balance:  0,
	REDACTED,
REDACTED
	userRepo.updateBalanceFn = func(ctx context.Context, id int64, amount float64) error {
		require.Equal(t, user.ID, id)
		if userRepo.getByIDUser != nil {
			userRepo.getByIDUser.Balance += amount
	REDACTED
		return nil
REDACTED
	redeemRepo := &paymentOrderLifecycleRedeemRepo{
		codesByCode: map[string]*RedeemCode{
			order.RechargeCode: {
				ID:     1,
				Code:   order.RechargeCode,
				Type:   RedeemTypeBalance,
				Value:  order.Amount,
				Status: StatusUnused,
		REDACTED,
	REDACTED,
REDACTED
	redeemService := NewRedeemService(
		redeemRepo,
		userRepo,
		nil,
		nil,
		nil,
		client,
		nil,
		nil,
	)
	registry := payment.NewRegistry()
	provider := &paymentOrderLifecycleQueryProvider{
		resp: &payment.QueryOrderResponse{
			TradeNo: "upstream-trade-123",
			Status:  payment.ProviderStatusPaid,
			Amount:  88,
	REDACTED,
REDACTED
	registry.Register(provider)

	svc := &PaymentService{
		entClient:       client,
		registry:        registry,
		redeemService:   redeemService,
		userRepo:        userRepo,
		providersLoaded: true,
REDACTED

	got, err := svc.VerifyOrderByOutTradeNo(ctx, order.OutTradeNo, user.ID)
REDACTED
	require.Equal(t, order.OutTradeNo, provider.lastQueryTradeNo)
	require.Equal(t, OrderStatusCompleted, got.Status)
	require.Equal(t, "upstream-trade-123", got.PaymentTradeNo)

	reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
REDACTED
	require.Equal(t, OrderStatusCompleted, reloaded.Status)
	require.Equal(t, "upstream-trade-123", reloaded.PaymentTradeNo)

	require.Equal(t, 88.0, userRepo.getByIDUser.Balance)
	require.Len(t, redeemRepo.useCalls, 1)
	require.Equal(t, int64(1), redeemRepo.useCalls[0].id)
	require.Equal(t, user.ID, redeemRepo.useCalls[0].userID)
REDACTED

func TestVerifyOrderByOutTradeNoRetriesZeroAmountPaidQueryOnce(t *testing.T) {
	ctx := context.Background()
	client := newPaymentOrderLifecycleTestClient(t)

	user, err := client.User.Create().
		SetEmail("checkpaid-retry@example.com").
		SetPasswordHash("hash").
		SetUsername("checkpaid-retry-user").
		Save(ctx)
REDACTED

	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(88).
		SetPayAmount(88).
		SetFeeRate(0).
		SetRechargeCode("CHECKPAID-UPSTREAM-RETRY").
		SetOutTradeNo("sub2_checkpaid_retry_zero_amount").
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusPending).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		Save(ctx)
REDACTED

	userRepo := &mockUserRepo{
		getByIDUser: &User{
			ID:       user.ID,
			Email:    user.Email,
			Username: user.Username,
			Balance:  0,
	REDACTED,
REDACTED
	userRepo.updateBalanceFn = func(ctx context.Context, id int64, amount float64) error {
		require.Equal(t, user.ID, id)
		if userRepo.getByIDUser != nil {
			userRepo.getByIDUser.Balance += amount
	REDACTED
		return nil
REDACTED
	redeemRepo := &paymentOrderLifecycleRedeemRepo{
		codesByCode: map[string]*RedeemCode{
			order.RechargeCode: {
				ID:     1,
				Code:   order.RechargeCode,
				Type:   RedeemTypeBalance,
				Value:  order.Amount,
				Status: StatusUnused,
		REDACTED,
	REDACTED,
REDACTED
	redeemService := NewRedeemService(
		redeemRepo,
		userRepo,
		nil,
		nil,
		nil,
		client,
		nil,
		nil,
	)
	registry := payment.NewRegistry()
	provider := &paymentOrderLifecycleQueryProvider{
		responses: []*payment.QueryOrderResponse{
			{
				TradeNo: "upstream-trade-zero",
				Status:  payment.ProviderStatusPaid,
				Amount:  0,
		REDACTED,
			{
				TradeNo: "upstream-trade-retry",
				Status:  payment.ProviderStatusPaid,
				Amount:  88,
		REDACTED,
	REDACTED,
REDACTED
	registry.Register(provider)

	svc := &PaymentService{
		entClient:       client,
		registry:        registry,
		redeemService:   redeemService,
		userRepo:        userRepo,
		providersLoaded: true,
REDACTED

	got, err := svc.VerifyOrderByOutTradeNo(ctx, order.OutTradeNo, user.ID)
REDACTED
	require.Equal(t, 2, provider.queryCalls)
	require.Equal(t, OrderStatusCompleted, got.Status)
	require.Equal(t, "upstream-trade-retry", got.PaymentTradeNo)
REDACTED

func TestVerifyOrderByOutTradeNoRejectsPaidQueryWithZeroAmount(t *testing.T) {
	ctx := context.Background()
	client := newPaymentOrderLifecycleTestClient(t)

	user, err := client.User.Create().
		SetEmail("checkpaid-zero-amount@example.com").
		SetPasswordHash("hash").
		SetUsername("checkpaid-zero-amount-user").
		Save(ctx)
REDACTED

	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(88).
		SetPayAmount(88).
		SetFeeRate(0).
		SetRechargeCode("CHECKPAID-ZERO-AMOUNT").
		SetOutTradeNo("sub2_checkpaid_zero_amount").
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusPending).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		Save(ctx)
REDACTED

	userRepo := &mockUserRepo{
		getByIDUser: &User{
			ID:       user.ID,
			Email:    user.Email,
			Username: user.Username,
			Balance:  0,
	REDACTED,
REDACTED
	redeemRepo := &paymentOrderLifecycleRedeemRepo{
		codesByCode: map[string]*RedeemCode{
			order.RechargeCode: {
				ID:     1,
				Code:   order.RechargeCode,
				Type:   RedeemTypeBalance,
				Value:  order.Amount,
				Status: StatusUnused,
		REDACTED,
	REDACTED,
REDACTED
	redeemService := NewRedeemService(
		redeemRepo,
		userRepo,
		nil,
		nil,
		nil,
		client,
		nil,
		nil,
	)
	registry := payment.NewRegistry()
	provider := &paymentOrderLifecycleQueryProvider{
		resp: &payment.QueryOrderResponse{
			TradeNo: "upstream-trade-zero",
			Status:  payment.ProviderStatusPaid,
			Amount:  0,
	REDACTED,
REDACTED
	registry.Register(provider)

	svc := &PaymentService{
		entClient:       client,
		registry:        registry,
		redeemService:   redeemService,
		userRepo:        userRepo,
		providersLoaded: true,
REDACTED

	got, err := svc.VerifyOrderByOutTradeNo(ctx, order.OutTradeNo, user.ID)
REDACTED
	require.Equal(t, order.OutTradeNo, provider.lastQueryTradeNo)
	require.Equal(t, OrderStatusPending, got.Status)
	require.Empty(t, got.PaymentTradeNo)

	reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
REDACTED
	require.Equal(t, OrderStatusPending, reloaded.Status)
	require.Empty(t, reloaded.PaymentTradeNo)

	require.Equal(t, 0.0, userRepo.getByIDUser.Balance)
	require.Empty(t, redeemRepo.useCalls)
REDACTED

func TestVerifyOrderByOutTradeNoDoesNotCancelUnpaidUpstreamOrder(t *testing.T) {
	ctx := context.Background()
	client := newPaymentOrderLifecycleTestClient(t)

	user, err := client.User.Create().
		SetEmail("checkpaid-pending@example.com").
		SetPasswordHash("hash").
		SetUsername("checkpaid-pending-user").
		Save(ctx)
REDACTED

	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(88).
		SetPayAmount(88).
		SetFeeRate(0).
		SetRechargeCode("CHECKPAID-PENDING").
		SetOutTradeNo("sub2_checkpaid_pending").
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusPending).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		Save(ctx)
REDACTED

	registry := payment.NewRegistry()
	provider := &paymentOrderLifecycleQueryProvider{
		resp: &payment.QueryOrderResponse{
			TradeNo: order.OutTradeNo,
			Status:  payment.ProviderStatusPending,
			Amount:  0,
	REDACTED,
REDACTED
	registry.Register(provider)

	svc := &PaymentService{
		entClient:       client,
		registry:        registry,
		providersLoaded: true,
REDACTED

	got, err := svc.VerifyOrderByOutTradeNo(ctx, order.OutTradeNo, user.ID)
REDACTED
	require.Equal(t, OrderStatusPending, got.Status)
	require.Equal(t, order.OutTradeNo, provider.lastQueryTradeNo)
	require.Zero(t, provider.cancelCalls)

	reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
REDACTED
	require.Equal(t, OrderStatusPending, reloaded.Status)
REDACTED

func TestCancelOrderStillClosesUnpaidUpstreamOrder(t *testing.T) {
	ctx := context.Background()
	client := newPaymentOrderLifecycleTestClient(t)

	user, err := client.User.Create().
		SetEmail("cancel-pending@example.com").
		SetPasswordHash("hash").
		SetUsername("cancel-pending-user").
		Save(ctx)
REDACTED

	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(88).
		SetPayAmount(88).
		SetFeeRate(0).
		SetRechargeCode("CANCEL-PENDING").
		SetOutTradeNo("sub2_cancel_pending").
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusPending).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		Save(ctx)
REDACTED

	registry := payment.NewRegistry()
	provider := &paymentOrderLifecycleQueryProvider{
		resp: &payment.QueryOrderResponse{
			TradeNo: order.OutTradeNo,
			Status:  payment.ProviderStatusPending,
			Amount:  0,
	REDACTED,
REDACTED
	registry.Register(provider)

	svc := &PaymentService{
		entClient:       client,
		registry:        registry,
		providersLoaded: true,
REDACTED

	outcome, err := svc.CancelOrder(ctx, order.ID, user.ID)
REDACTED
	require.Equal(t, checkPaidResultCancelled, outcome)
	require.Equal(t, order.OutTradeNo, provider.lastCancelTradeNo)
	require.Equal(t, 1, provider.cancelCalls)

	reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
REDACTED
	require.Equal(t, OrderStatusCancelled, reloaded.Status)
REDACTED

func TestReconcilePendingWxpayOrdersBackfillsPaidOrder(t *testing.T) {
	ctx := context.Background()
	client := newPaymentOrderLifecycleTestClient(t)

	user, err := client.User.Create().
		SetEmail("wxpay-reconcile@example.com").
		SetPasswordHash("hash").
		SetUsername("wxpay-reconcile-user").
		Save(ctx)
REDACTED

	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(50).
		SetPayAmount(50).
		SetFeeRate(0).
		SetRechargeCode("WXPAY-RECONCILE").
		SetOutTradeNo("sub2_wxpay_reconcile").
		SetPaymentType(payment.TypeWxpay).
		SetPaymentTradeNo("").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusPending).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		Save(ctx)
REDACTED

	userRepo := &mockUserRepo{
		getByIDUser: &User{
			ID:       user.ID,
			Email:    user.Email,
			Username: user.Username,
			Balance:  0,
	REDACTED,
REDACTED
	userRepo.updateBalanceFn = func(ctx context.Context, id int64, amount float64) error {
		require.Equal(t, user.ID, id)
		if userRepo.getByIDUser != nil {
			userRepo.getByIDUser.Balance += amount
	REDACTED
		return nil
REDACTED
	redeemRepo := &paymentOrderLifecycleRedeemRepo{
		codesByCode: map[string]*RedeemCode{
			order.RechargeCode: {
				ID:     1,
				Code:   order.RechargeCode,
				Type:   RedeemTypeBalance,
				Value:  order.Amount,
				Status: StatusUnused,
		REDACTED,
	REDACTED,
REDACTED
	redeemService := NewRedeemService(
		redeemRepo,
		userRepo,
		nil,
		nil,
		nil,
		client,
		nil,
		nil,
	)
	registry := payment.NewRegistry()
	provider := &paymentOrderLifecycleQueryProvider{
		key: payment.TypeWxpay,
		resp: &payment.QueryOrderResponse{
			TradeNo: "wxpay-upstream-trade-123",
			Status:  payment.ProviderStatusPaid,
			Amount:  50,
			Metadata: map[string]string{
				"trade_state": "SUCCESS",
		REDACTED,
	REDACTED,
REDACTED
	registry.Register(provider)

	svc := &PaymentService{
		entClient:       client,
		registry:        registry,
		redeemService:   redeemService,
		userRepo:        userRepo,
		providersLoaded: true,
REDACTED

	recovered, err := svc.ReconcilePendingWxpayOrders(ctx)
REDACTED
	require.Equal(t, 1, recovered)
	require.Equal(t, order.OutTradeNo, provider.lastQueryTradeNo)
	require.Zero(t, provider.cancelCalls)

	reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
REDACTED
	require.Equal(t, OrderStatusCompleted, reloaded.Status)
	require.Equal(t, "wxpay-upstream-trade-123", reloaded.PaymentTradeNo)
	require.Equal(t, 50.0, userRepo.getByIDUser.Balance)
	require.Len(t, redeemRepo.useCalls, 1)
REDACTED

func TestVerifyOrderByOutTradeNoUsesOutTradeNoWhenPaymentTradeNoAlreadyExistsForAlipay(t *testing.T) {
	ctx := context.Background()
	client := newPaymentOrderLifecycleTestClient(t)

	user, err := client.User.Create().
		SetEmail("checkpaid-existing-trade@example.com").
		SetPasswordHash("hash").
		SetUsername("checkpaid-existing-trade-user").
		Save(ctx)
REDACTED

	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(88).
		SetPayAmount(88).
		SetFeeRate(0).
		SetRechargeCode("CHECKPAID-EXISTING-TRADE-NO").
		SetOutTradeNo("sub2_checkpaid_use_out_trade_no").
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("upstream-trade-existing").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusPending).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		Save(ctx)
REDACTED

	userRepo := &mockUserRepo{
		getByIDUser: &User{
			ID:       user.ID,
			Email:    user.Email,
			Username: user.Username,
			Balance:  0,
	REDACTED,
REDACTED
	userRepo.updateBalanceFn = func(ctx context.Context, id int64, amount float64) error {
		require.Equal(t, user.ID, id)
		if userRepo.getByIDUser != nil {
			userRepo.getByIDUser.Balance += amount
	REDACTED
		return nil
REDACTED
	redeemRepo := &paymentOrderLifecycleRedeemRepo{
		codesByCode: map[string]*RedeemCode{
			order.RechargeCode: {
				ID:     1,
				Code:   order.RechargeCode,
				Type:   RedeemTypeBalance,
				Value:  order.Amount,
				Status: StatusUnused,
		REDACTED,
	REDACTED,
REDACTED
	redeemService := NewRedeemService(
		redeemRepo,
		userRepo,
		nil,
		nil,
		nil,
		client,
		nil,
		nil,
	)
	registry := payment.NewRegistry()
	provider := &paymentOrderLifecycleQueryProvider{
		resp: &payment.QueryOrderResponse{
			TradeNo: "upstream-trade-existing",
			Status:  payment.ProviderStatusPaid,
			Amount:  88,
	REDACTED,
REDACTED
	registry.Register(provider)

	svc := &PaymentService{
		entClient:       client,
		registry:        registry,
		redeemService:   redeemService,
		userRepo:        userRepo,
		providersLoaded: true,
REDACTED

	got, err := svc.VerifyOrderByOutTradeNo(ctx, order.OutTradeNo, user.ID)
REDACTED
	require.Equal(t, order.OutTradeNo, provider.lastQueryTradeNo)
	require.Equal(t, "upstream-trade-existing", got.PaymentTradeNo)
REDACTED

func TestPaymentOrderAllowsRegistryFallbackOnlyForLegacyOrdersWithoutPinnedProviderState(t *testing.T) {
	t.Parallel()

	require.True(t, paymentOrderAllowsRegistryFallback(&dbent.PaymentOrder{
		PaymentType: payment.TypeAlipay,
REDACTED))

	instanceID := "12"
	require.False(t, paymentOrderAllowsRegistryFallback(&dbent.PaymentOrder{
		PaymentType:        payment.TypeAlipay,
		ProviderInstanceID: &instanceID,
REDACTED))

	require.False(t, paymentOrderAllowsRegistryFallback(&dbent.PaymentOrder{
		PaymentType: payment.TypeAlipay,
		ProviderSnapshot: map[string]any{
			"schema_version":       2,
			"provider_instance_id": "12",
	REDACTED,
REDACTED))
REDACTED

func TestPaymentOrderQueryReferenceUsesOutTradeNoForOfficialProviders(t *testing.T) {
	t.Parallel()

	order := &dbent.PaymentOrder{
		PaymentType:    payment.TypeWxpay,
		OutTradeNo:     "sub2_out_trade_no",
		PaymentTradeNo: "wx-transaction-id",
REDACTED

	require.Equal(t, "sub2_out_trade_no", paymentOrderQueryReference(order, &paymentOrderLifecycleQueryProvider{REDACTED))
	require.Equal(t, "sub2_out_trade_no", paymentOrderQueryReference(order, paymentFulfillmentTestProvider{
		key: payment.TypeWxpay,
REDACTED))
REDACTED

func newPaymentOrderLifecycleTestClient(t *testing.T) *dbent.Client {
REDACTED

	db, err := sql.Open("sqlite", "file:payment_order_lifecycle?mode=memory&cache=shared&_fk=1")
REDACTED
	t.Cleanup(func() { _ = db.Close() REDACTED)

	_, err = db.Exec("PRAGMA foreign_keys = ON")
REDACTED

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() REDACTED)
	return client
REDACTED
