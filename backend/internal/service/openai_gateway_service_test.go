package service

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
)

type stubOpenAIAccountRepo struct {
	AccountRepository
	accounts []Account
REDACTED

func (r stubOpenAIAccountRepo) GetByID(ctx context.Context, id int64) (*Account, error) {
	for i := range r.accounts {
		if r.accounts[i].ID == id {
			return &r.accounts[i], nil
	REDACTED
REDACTED
	return nil, errors.New("account not found")
REDACTED

func (r stubOpenAIAccountRepo) ListSchedulableByGroupIDAndPlatform(ctx context.Context, groupID int64, platform string) ([]Account, error) {
	var result []Account
	for _, acc := range r.accounts {
		if acc.Platform == platform {
			result = append(result, acc)
	REDACTED
REDACTED
	return result, nil
REDACTED

func (r stubOpenAIAccountRepo) ListSchedulableByPlatform(ctx context.Context, platform string) ([]Account, error) {
	var result []Account
	for _, acc := range r.accounts {
		if acc.Platform == platform {
			result = append(result, acc)
	REDACTED
REDACTED
	return result, nil
REDACTED

type stubConcurrencyCache struct {
	ConcurrencyCache
	loadBatchErr    error
	loadMap         map[int64]*AccountLoadInfo
	acquireResults  map[int64]bool
	waitCounts      map[int64]int
	skipDefaultLoad bool
REDACTED

func (c stubConcurrencyCache) AcquireAccountSlot(ctx context.Context, accountID int64, maxConcurrency int, requestID string) (bool, error) {
	if c.acquireResults != nil {
		if result, ok := c.acquireResults[accountID]; ok {
			return result, nil
	REDACTED
REDACTED
	return true, nil
REDACTED

func (c stubConcurrencyCache) ReleaseAccountSlot(ctx context.Context, accountID int64, requestID string) error {
	return nil
REDACTED

func (c stubConcurrencyCache) GetAccountsLoadBatch(ctx context.Context, accounts []AccountWithConcurrency) (map[int64]*AccountLoadInfo, error) {
	if c.loadBatchErr != nil {
		return nil, c.loadBatchErr
REDACTED
	out := make(map[int64]*AccountLoadInfo, len(accounts))
	if c.skipDefaultLoad && c.loadMap != nil {
		for _, acc := range accounts {
			if load, ok := c.loadMap[acc.ID]; ok {
				out[acc.ID] = load
		REDACTED
	REDACTED
		return out, nil
REDACTED
	for _, acc := range accounts {
		if c.loadMap != nil {
			if load, ok := c.loadMap[acc.ID]; ok {
				out[acc.ID] = load
				continue
		REDACTED
	REDACTED
		out[acc.ID] = &AccountLoadInfo{AccountID: acc.ID, LoadRate: 0REDACTED
REDACTED
	return out, nil
REDACTED

func (c stubConcurrencyCache) GetAccountWaitingCount(ctx context.Context, accountID int64) (int, error) {
	if c.waitCounts != nil {
		if count, ok := c.waitCounts[accountID]; ok {
			return count, nil
	REDACTED
REDACTED
	return 0, nil
REDACTED

type stubGatewayCache struct {
	sessionBindings map[string]int64
	deletedSessions map[string]int
REDACTED

func (c *stubGatewayCache) GetSessionAccountID(ctx context.Context, groupID int64, sessionHash string) (int64, error) {
	if id, ok := c.sessionBindings[sessionHash]; ok {
		return id, nil
REDACTED
	return 0, errors.New("not found")
REDACTED

func (c *stubGatewayCache) SetSessionAccountID(ctx context.Context, groupID int64, sessionHash string, accountID int64, ttl time.Duration) error {
	if c.sessionBindings == nil {
		c.sessionBindings = make(map[string]int64)
REDACTED
	c.sessionBindings[sessionHash] = accountID
	return nil
REDACTED

func (c *stubGatewayCache) RefreshSessionTTL(ctx context.Context, groupID int64, sessionHash string, ttl time.Duration) error {
	return nil
REDACTED

func (c *stubGatewayCache) DeleteSessionAccountID(ctx context.Context, groupID int64, sessionHash string) error {
	if c.sessionBindings == nil {
		return nil
REDACTED
	if c.deletedSessions == nil {
		c.deletedSessions = make(map[string]int)
REDACTED
	c.deletedSessions[sessionHash]++
	delete(c.sessionBindings, sessionHash)
	return nil
REDACTED

func TestOpenAISelectAccountWithLoadAwareness_FiltersUnschedulable(t *testing.T) {
	now := time.Now()
	resetAt := now.Add(10 * time.Minute)
	groupID := int64(1)

	rateLimited := Account{
		ID:               1,
		Platform:         PlatformOpenAI,
		Type:             AccountTypeAPIKey,
		Status:           StatusActive,
		Schedulable:      true,
		Concurrency:      1,
		Priority:         0,
		RateLimitResetAt: &resetAt,
REDACTED
	available := Account{
		ID:          2,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Priority:    1,
REDACTED

	svc := &OpenAIGatewayService{
		accountRepo:        stubOpenAIAccountRepo{accounts: []Account{rateLimited, availableREDACTEDREDACTED,
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{REDACTED),
REDACTED

	selection, err := svc.SelectAccountWithLoadAwareness(context.Background(), &groupID, "", "gpt-5.2", nil)
	if err != nil {
		t.Fatalf("SelectAccountWithLoadAwareness error: %v", err)
REDACTED
	if selection == nil || selection.Account == nil {
		t.Fatalf("expected selection with account")
REDACTED
	if selection.Account.ID != available.ID {
		t.Fatalf("expected account %d, got %d", available.ID, selection.Account.ID)
REDACTED
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
REDACTED
REDACTED

func TestOpenAISelectAccountWithLoadAwareness_FiltersUnschedulableWhenNoConcurrencyService(t *testing.T) {
	now := time.Now()
	resetAt := now.Add(10 * time.Minute)
	groupID := int64(1)

	rateLimited := Account{
		ID:               1,
		Platform:         PlatformOpenAI,
		Type:             AccountTypeAPIKey,
		Status:           StatusActive,
		Schedulable:      true,
		Concurrency:      1,
		Priority:         0,
		RateLimitResetAt: &resetAt,
REDACTED
	available := Account{
		ID:          2,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Priority:    1,
REDACTED

	svc := &OpenAIGatewayService{
		accountRepo: stubOpenAIAccountRepo{accounts: []Account{rateLimited, availableREDACTEDREDACTED,
		// concurrencyService is nil, forcing the non-load-batch selection path.
REDACTED

	selection, err := svc.SelectAccountWithLoadAwareness(context.Background(), &groupID, "", "gpt-5.2", nil)
	if err != nil {
		t.Fatalf("SelectAccountWithLoadAwareness error: %v", err)
REDACTED
	if selection == nil || selection.Account == nil {
		t.Fatalf("expected selection with account")
REDACTED
	if selection.Account.ID != available.ID {
		t.Fatalf("expected account %d, got %d", available.ID, selection.Account.ID)
REDACTED
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
REDACTED
REDACTED

func TestOpenAISelectAccountForModelWithExclusions_StickyUnschedulableClearsSession(t *testing.T) {
	sessionHash := "session-1"
	repo := stubOpenAIAccountRepo{
		accounts: []Account{
			{ID: 1, Platform: PlatformOpenAI, Status: StatusDisabled, Schedulable: true, Concurrency: 1REDACTED,
			{ID: 2, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1REDACTED,
	REDACTED,
REDACTED
	cache := &stubGatewayCache{
		sessionBindings: map[string]int64{"openai:" + sessionHash: 1REDACTED,
REDACTED

	svc := &OpenAIGatewayService{
		accountRepo: repo,
		cache:       cache,
REDACTED

	acc, err := svc.SelectAccountForModelWithExclusions(context.Background(), nil, sessionHash, "gpt-4", nil)
	if err != nil {
		t.Fatalf("SelectAccountForModelWithExclusions error: %v", err)
REDACTED
	if acc == nil || acc.ID != 2 {
		t.Fatalf("expected account 2, got %+v", acc)
REDACTED
	if cache.deletedSessions["openai:"+sessionHash] != 1 {
		t.Fatalf("expected sticky session to be deleted")
REDACTED
	if cache.sessionBindings["openai:"+sessionHash] != 2 {
		t.Fatalf("expected sticky session to bind to account 2")
REDACTED
REDACTED

func TestOpenAISelectAccountWithLoadAwareness_StickyUnschedulableClearsSession(t *testing.T) {
	sessionHash := "session-2"
	groupID := int64(1)
	repo := stubOpenAIAccountRepo{
		accounts: []Account{
			{ID: 1, Platform: PlatformOpenAI, Status: StatusDisabled, Schedulable: true, Concurrency: 1REDACTED,
			{ID: 2, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1REDACTED,
	REDACTED,
REDACTED
	cache := &stubGatewayCache{
		sessionBindings: map[string]int64{"openai:" + sessionHash: 1REDACTED,
REDACTED

	svc := &OpenAIGatewayService{
		accountRepo:        repo,
		cache:              cache,
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{REDACTED),
REDACTED

	selection, err := svc.SelectAccountWithLoadAwareness(context.Background(), &groupID, sessionHash, "gpt-4", nil)
	if err != nil {
		t.Fatalf("SelectAccountWithLoadAwareness error: %v", err)
REDACTED
	if selection == nil || selection.Account == nil || selection.Account.ID != 2 {
		t.Fatalf("expected account 2, got %+v", selection)
REDACTED
	if cache.deletedSessions["openai:"+sessionHash] != 1 {
		t.Fatalf("expected sticky session to be deleted")
REDACTED
	if cache.sessionBindings["openai:"+sessionHash] != 2 {
		t.Fatalf("expected sticky session to bind to account 2")
REDACTED
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
REDACTED
REDACTED

func TestOpenAISelectAccountForModelWithExclusions_NoModelSupport(t *testing.T) {
	repo := stubOpenAIAccountRepo{
		accounts: []Account{
			{
				ID:          1,
				Platform:    PlatformOpenAI,
				Status:      StatusActive,
				Schedulable: true,
		REDACTED"model_mapping": map[string]any{"gpt-3.5-turbo": "gpt-3.5-turbo"REDACTEDREDACTED,
		REDACTED,
	REDACTED,
REDACTED
	cache := &stubGatewayCache{REDACTED

	svc := &OpenAIGatewayService{
		accountRepo: repo,
		cache:       cache,
REDACTED

	acc, err := svc.SelectAccountForModelWithExclusions(context.Background(), nil, "", "gpt-4", nil)
	if err == nil {
		t.Fatalf("expected error for unsupported model")
REDACTED
	if acc != nil {
		t.Fatalf("expected nil account for unsupported model")
REDACTED
	if !strings.Contains(err.Error(), "supporting model") {
		t.Fatalf("unexpected error: %v", err)
REDACTED
REDACTED

func TestOpenAISelectAccountWithLoadAwareness_LoadBatchErrorFallback(t *testing.T) {
	groupID := int64(1)
	repo := stubOpenAIAccountRepo{
		accounts: []Account{
			{ID: 1, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 2REDACTED,
			{ID: 2, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1REDACTED,
	REDACTED,
REDACTED
	cache := &stubGatewayCache{REDACTED
	concurrencyCache := stubConcurrencyCache{
		loadBatchErr: errors.New("load batch failed"),
REDACTED

	svc := &OpenAIGatewayService{
		accountRepo:        repo,
		cache:              cache,
		concurrencyService: NewConcurrencyService(concurrencyCache),
REDACTED

	selection, err := svc.SelectAccountWithLoadAwareness(context.Background(), &groupID, "fallback", "gpt-4", nil)
	if err != nil {
		t.Fatalf("SelectAccountWithLoadAwareness error: %v", err)
REDACTED
	if selection == nil || selection.Account == nil {
		t.Fatalf("expected selection")
REDACTED
	if selection.Account.ID != 2 {
		t.Fatalf("expected account 2, got %d", selection.Account.ID)
REDACTED
	if cache.sessionBindings["openai:fallback"] != 2 {
		t.Fatalf("expected sticky session updated")
REDACTED
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
REDACTED
REDACTED

func TestOpenAISelectAccountWithLoadAwareness_NoSlotFallbackWait(t *testing.T) {
	groupID := int64(1)
	repo := stubOpenAIAccountRepo{
		accounts: []Account{
			{ID: 1, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1REDACTED,
	REDACTED,
REDACTED
	cache := &stubGatewayCache{REDACTED
	concurrencyCache := stubConcurrencyCache{
		acquireResults: map[int64]bool{1: falseREDACTED,
		loadMap: map[int64]*AccountLoadInfo{
			1: {AccountID: 1, LoadRate: 10REDACTED,
	REDACTED,
REDACTED

	svc := &OpenAIGatewayService{
		accountRepo:        repo,
		cache:              cache,
		concurrencyService: NewConcurrencyService(concurrencyCache),
REDACTED

	selection, err := svc.SelectAccountWithLoadAwareness(context.Background(), &groupID, "", "gpt-4", nil)
	if err != nil {
		t.Fatalf("SelectAccountWithLoadAwareness error: %v", err)
REDACTED
	if selection == nil || selection.WaitPlan == nil {
		t.Fatalf("expected wait plan fallback")
REDACTED
	if selection.Account == nil || selection.Account.ID != 1 {
		t.Fatalf("expected account 1")
REDACTED
REDACTED

func TestOpenAISelectAccountForModelWithExclusions_SetsStickyBinding(t *testing.T) {
	sessionHash := "bind"
	repo := stubOpenAIAccountRepo{
		accounts: []Account{
			{ID: 1, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1REDACTED,
	REDACTED,
REDACTED
	cache := &stubGatewayCache{REDACTED

	svc := &OpenAIGatewayService{
		accountRepo: repo,
		cache:       cache,
REDACTED

	acc, err := svc.SelectAccountForModelWithExclusions(context.Background(), nil, sessionHash, "gpt-4", nil)
	if err != nil {
		t.Fatalf("SelectAccountForModelWithExclusions error: %v", err)
REDACTED
	if acc == nil || acc.ID != 1 {
		t.Fatalf("expected account 1")
REDACTED
	if cache.sessionBindings["openai:"+sessionHash] != 1 {
		t.Fatalf("expected sticky session binding")
REDACTED
REDACTED

func TestOpenAISelectAccountWithLoadAwareness_StickyWaitPlan(t *testing.T) {
	sessionHash := "sticky-wait"
	groupID := int64(1)
	repo := stubOpenAIAccountRepo{
		accounts: []Account{
			{ID: 1, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1REDACTED,
	REDACTED,
REDACTED
	cache := &stubGatewayCache{
		sessionBindings: map[string]int64{"openai:" + sessionHash: 1REDACTED,
REDACTED
	concurrencyCache := stubConcurrencyCache{
		acquireResults: map[int64]bool{1: falseREDACTED,
		waitCounts:     map[int64]int{1: 0REDACTED,
REDACTED

	svc := &OpenAIGatewayService{
		accountRepo:        repo,
		cache:              cache,
		concurrencyService: NewConcurrencyService(concurrencyCache),
REDACTED

	selection, err := svc.SelectAccountWithLoadAwareness(context.Background(), &groupID, sessionHash, "gpt-4", nil)
	if err != nil {
		t.Fatalf("SelectAccountWithLoadAwareness error: %v", err)
REDACTED
	if selection == nil || selection.WaitPlan == nil {
		t.Fatalf("expected sticky wait plan")
REDACTED
	if selection.Account == nil || selection.Account.ID != 1 {
		t.Fatalf("expected account 1")
REDACTED
REDACTED

func TestOpenAISelectAccountWithLoadAwareness_PrefersLowerLoad(t *testing.T) {
	groupID := int64(1)
	repo := stubOpenAIAccountRepo{
		accounts: []Account{
			{ID: 1, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1REDACTED,
			{ID: 2, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1REDACTED,
	REDACTED,
REDACTED
	cache := &stubGatewayCache{REDACTED
	concurrencyCache := stubConcurrencyCache{
		loadMap: map[int64]*AccountLoadInfo{
			1: {AccountID: 1, LoadRate: 80REDACTED,
			2: {AccountID: 2, LoadRate: 10REDACTED,
	REDACTED,
REDACTED

	svc := &OpenAIGatewayService{
		accountRepo:        repo,
		cache:              cache,
		concurrencyService: NewConcurrencyService(concurrencyCache),
REDACTED

	selection, err := svc.SelectAccountWithLoadAwareness(context.Background(), &groupID, "load", "gpt-4", nil)
	if err != nil {
		t.Fatalf("SelectAccountWithLoadAwareness error: %v", err)
REDACTED
	if selection == nil || selection.Account == nil || selection.Account.ID != 2 {
		t.Fatalf("expected account 2")
REDACTED
	if cache.sessionBindings["openai:load"] != 2 {
		t.Fatalf("expected sticky session updated")
REDACTED
REDACTED

func TestOpenAISelectAccountForModelWithExclusions_StickyExcludedFallback(t *testing.T) {
	sessionHash := "excluded"
	repo := stubOpenAIAccountRepo{
		accounts: []Account{
			{ID: 1, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1REDACTED,
			{ID: 2, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 2REDACTED,
	REDACTED,
REDACTED
	cache := &stubGatewayCache{
		sessionBindings: map[string]int64{"openai:" + sessionHash: 1REDACTED,
REDACTED

	svc := &OpenAIGatewayService{
		accountRepo: repo,
		cache:       cache,
REDACTED

	excluded := map[int64]struct{REDACTED{1: {REDACTEDREDACTED
	acc, err := svc.SelectAccountForModelWithExclusions(context.Background(), nil, sessionHash, "gpt-4", excluded)
	if err != nil {
		t.Fatalf("SelectAccountForModelWithExclusions error: %v", err)
REDACTED
	if acc == nil || acc.ID != 2 {
		t.Fatalf("expected account 2")
REDACTED
REDACTED

func TestOpenAISelectAccountForModelWithExclusions_StickyNonOpenAI(t *testing.T) {
	sessionHash := "non-openai"
	repo := stubOpenAIAccountRepo{
		accounts: []Account{
			{ID: 1, Platform: PlatformAnthropic, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1REDACTED,
			{ID: 2, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 2REDACTED,
	REDACTED,
REDACTED
	cache := &stubGatewayCache{
		sessionBindings: map[string]int64{"openai:" + sessionHash: 1REDACTED,
REDACTED

	svc := &OpenAIGatewayService{
		accountRepo: repo,
		cache:       cache,
REDACTED

	acc, err := svc.SelectAccountForModelWithExclusions(context.Background(), nil, sessionHash, "gpt-4", nil)
	if err != nil {
		t.Fatalf("SelectAccountForModelWithExclusions error: %v", err)
REDACTED
	if acc == nil || acc.ID != 2 {
		t.Fatalf("expected account 2")
REDACTED
REDACTED

func TestOpenAISelectAccountForModelWithExclusions_NoAccounts(t *testing.T) {
	repo := stubOpenAIAccountRepo{accounts: []Account{REDACTEDREDACTED
	cache := &stubGatewayCache{REDACTED

	svc := &OpenAIGatewayService{
		accountRepo: repo,
		cache:       cache,
REDACTED

	acc, err := svc.SelectAccountForModelWithExclusions(context.Background(), nil, "", "", nil)
	if err == nil {
		t.Fatalf("expected error for no accounts")
REDACTED
	if acc != nil {
		t.Fatalf("expected nil account")
REDACTED
	if !strings.Contains(err.Error(), "no available OpenAI accounts") {
		t.Fatalf("unexpected error: %v", err)
REDACTED
REDACTED

func TestOpenAISelectAccountWithLoadAwareness_NoCandidates(t *testing.T) {
	groupID := int64(1)
	resetAt := time.Now().Add(1 * time.Hour)
	repo := stubOpenAIAccountRepo{
		accounts: []Account{
			{ID: 1, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1, RateLimitResetAt: &resetAtREDACTED,
	REDACTED,
REDACTED
	cache := &stubGatewayCache{REDACTED
	concurrencyCache := stubConcurrencyCache{REDACTED

	svc := &OpenAIGatewayService{
		accountRepo:        repo,
		cache:              cache,
		concurrencyService: NewConcurrencyService(concurrencyCache),
REDACTED

	selection, err := svc.SelectAccountWithLoadAwareness(context.Background(), &groupID, "", "gpt-4", nil)
	if err == nil {
		t.Fatalf("expected error for no candidates")
REDACTED
	if selection != nil {
		t.Fatalf("expected nil selection")
REDACTED
REDACTED

func TestOpenAISelectAccountWithLoadAwareness_AllFullWaitPlan(t *testing.T) {
	groupID := int64(1)
	repo := stubOpenAIAccountRepo{
		accounts: []Account{
			{ID: 1, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1REDACTED,
	REDACTED,
REDACTED
	cache := &stubGatewayCache{REDACTED
	concurrencyCache := stubConcurrencyCache{
		loadMap: map[int64]*AccountLoadInfo{
			1: {AccountID: 1, LoadRate: 100REDACTED,
	REDACTED,
REDACTED

	svc := &OpenAIGatewayService{
		accountRepo:        repo,
		cache:              cache,
		concurrencyService: NewConcurrencyService(concurrencyCache),
REDACTED

	selection, err := svc.SelectAccountWithLoadAwareness(context.Background(), &groupID, "", "gpt-4", nil)
	if err != nil {
		t.Fatalf("SelectAccountWithLoadAwareness error: %v", err)
REDACTED
	if selection == nil || selection.WaitPlan == nil {
		t.Fatalf("expected wait plan")
REDACTED
REDACTED

func TestOpenAISelectAccountWithLoadAwareness_LoadBatchErrorNoAcquire(t *testing.T) {
	groupID := int64(1)
	repo := stubOpenAIAccountRepo{
		accounts: []Account{
			{ID: 1, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1REDACTED,
	REDACTED,
REDACTED
	cache := &stubGatewayCache{REDACTED
	concurrencyCache := stubConcurrencyCache{
		loadBatchErr:   errors.New("load batch failed"),
		acquireResults: map[int64]bool{1: falseREDACTED,
REDACTED

	svc := &OpenAIGatewayService{
		accountRepo:        repo,
		cache:              cache,
		concurrencyService: NewConcurrencyService(concurrencyCache),
REDACTED

	selection, err := svc.SelectAccountWithLoadAwareness(context.Background(), &groupID, "", "gpt-4", nil)
	if err != nil {
		t.Fatalf("SelectAccountWithLoadAwareness error: %v", err)
REDACTED
	if selection == nil || selection.WaitPlan == nil {
		t.Fatalf("expected wait plan")
REDACTED
REDACTED

func TestOpenAISelectAccountWithLoadAwareness_MissingLoadInfo(t *testing.T) {
	groupID := int64(1)
	repo := stubOpenAIAccountRepo{
		accounts: []Account{
			{ID: 1, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1REDACTED,
			{ID: 2, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1REDACTED,
	REDACTED,
REDACTED
	cache := &stubGatewayCache{REDACTED
	concurrencyCache := stubConcurrencyCache{
		loadMap: map[int64]*AccountLoadInfo{
			1: {AccountID: 1, LoadRate: 50REDACTED,
	REDACTED,
		skipDefaultLoad: true,
REDACTED

	svc := &OpenAIGatewayService{
		accountRepo:        repo,
		cache:              cache,
		concurrencyService: NewConcurrencyService(concurrencyCache),
REDACTED

	selection, err := svc.SelectAccountWithLoadAwareness(context.Background(), &groupID, "", "gpt-4", nil)
	if err != nil {
		t.Fatalf("SelectAccountWithLoadAwareness error: %v", err)
REDACTED
	if selection == nil || selection.Account == nil || selection.Account.ID != 2 {
		t.Fatalf("expected account 2")
REDACTED
REDACTED

func TestOpenAISelectAccountForModelWithExclusions_LeastRecentlyUsed(t *testing.T) {
	oldTime := time.Now().Add(-2 * time.Hour)
	newTime := time.Now().Add(-1 * time.Hour)
	repo := stubOpenAIAccountRepo{
		accounts: []Account{
			{ID: 1, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Priority: 1, LastUsedAt: &newTimeREDACTED,
			{ID: 2, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Priority: 1, LastUsedAt: &oldTimeREDACTED,
	REDACTED,
REDACTED
	cache := &stubGatewayCache{REDACTED

	svc := &OpenAIGatewayService{
		accountRepo: repo,
		cache:       cache,
REDACTED

	acc, err := svc.SelectAccountForModelWithExclusions(context.Background(), nil, "", "gpt-4", nil)
	if err != nil {
		t.Fatalf("SelectAccountForModelWithExclusions error: %v", err)
REDACTED
	if acc == nil || acc.ID != 2 {
		t.Fatalf("expected account 2")
REDACTED
REDACTED

func TestOpenAISelectAccountWithLoadAwareness_PreferNeverUsed(t *testing.T) {
	groupID := int64(1)
	lastUsed := time.Now().Add(-1 * time.Hour)
	repo := stubOpenAIAccountRepo{
		accounts: []Account{
			{ID: 1, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1, LastUsedAt: &lastUsedREDACTED,
			{ID: 2, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1REDACTED,
	REDACTED,
REDACTED
	cache := &stubGatewayCache{REDACTED
	concurrencyCache := stubConcurrencyCache{
		loadMap: map[int64]*AccountLoadInfo{
			1: {AccountID: 1, LoadRate: 10REDACTED,
			2: {AccountID: 2, LoadRate: 10REDACTED,
	REDACTED,
REDACTED

	svc := &OpenAIGatewayService{
		accountRepo:        repo,
		cache:              cache,
		concurrencyService: NewConcurrencyService(concurrencyCache),
REDACTED

	selection, err := svc.SelectAccountWithLoadAwareness(context.Background(), &groupID, "", "gpt-4", nil)
	if err != nil {
		t.Fatalf("SelectAccountWithLoadAwareness error: %v", err)
REDACTED
	if selection == nil || selection.Account == nil || selection.Account.ID != 2 {
		t.Fatalf("expected account 2")
REDACTED
REDACTED

func TestOpenAIStreamingTimeout(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			StreamDataIntervalTimeout: 1,
			StreamKeepaliveInterval:   0,
			MaxLineSize:               defaultMaxLineSize,
	REDACTED,
REDACTED
	svc := &OpenAIGatewayService{cfg: cfgREDACTED

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	pr, pw := io.Pipe()
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       pr,
		Header:     http.Header{REDACTED,
REDACTED

	start := time.Now()
	_, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, &Account{ID: 1REDACTED, start, "model", "model")
	_ = pw.Close()
	_ = pr.Close()

	if err == nil || !strings.Contains(err.Error(), "stream data interval timeout") {
		t.Fatalf("expected stream timeout error, got %v", err)
REDACTED
	if !strings.Contains(rec.Body.String(), "stream_timeout") {
		t.Fatalf("expected stream_timeout SSE error, got %q", rec.Body.String())
REDACTED
REDACTED

func TestOpenAIStreamingTooLong(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			StreamDataIntervalTimeout: 0,
			StreamKeepaliveInterval:   0,
			MaxLineSize:               64 * 1024,
	REDACTED,
REDACTED
	svc := &OpenAIGatewayService{cfg: cfgREDACTED

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	pr, pw := io.Pipe()
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       pr,
		Header:     http.Header{REDACTED,
REDACTED

	go func() {
		defer func() { _ = pw.Close() REDACTED()
		// 写入超过 MaxLineSize 的单行数据，触发 ErrTooLong
		payload := "data: " + strings.Repeat("a", 128*1024) + "\n"
		_, _ = pw.Write([]byte(payload))
REDACTED()

	_, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, &Account{ID: 2REDACTED, time.Now(), "model", "model")
	_ = pr.Close()

	if !errors.Is(err, bufio.ErrTooLong) {
		t.Fatalf("expected ErrTooLong, got %v", err)
REDACTED
	if !strings.Contains(rec.Body.String(), "response_too_large") {
		t.Fatalf("expected response_too_large SSE error, got %q", rec.Body.String())
REDACTED
REDACTED

func TestOpenAINonStreamingContentTypePassThrough(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Security: config.SecurityConfig{
			ResponseHeaders: config.ResponseHeaderConfig{Enabled: falseREDACTED,
	REDACTED,
REDACTED
	svc := &OpenAIGatewayService{cfg: cfgREDACTED

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	body := []byte(`{"usage":{"input_tokens":1,"output_tokens":2,"input_tokens_details":{"cached_tokens":0REDACTEDREDACTEDREDACTED`)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(body)),
		Header:     http.Header{"Content-Type": []string{"application/vnd.test+json"REDACTEDREDACTED,
REDACTED

	_, err := svc.handleNonStreamingResponse(c.Request.Context(), resp, c, &Account{REDACTED, "model", "model")
	if err != nil {
		t.Fatalf("handleNonStreamingResponse error: %v", err)
REDACTED

	if !strings.Contains(rec.Header().Get("Content-Type"), "application/vnd.test+json") {
		t.Fatalf("expected Content-Type passthrough, got %q", rec.Header().Get("Content-Type"))
REDACTED
REDACTED

func TestOpenAINonStreamingContentTypeDefault(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Security: config.SecurityConfig{
			ResponseHeaders: config.ResponseHeaderConfig{Enabled: falseREDACTED,
	REDACTED,
REDACTED
	svc := &OpenAIGatewayService{cfg: cfgREDACTED

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	body := []byte(`{"usage":{"input_tokens":1,"output_tokens":2,"input_tokens_details":{"cached_tokens":0REDACTEDREDACTEDREDACTED`)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(body)),
		Header:     http.Header{REDACTED,
REDACTED

	_, err := svc.handleNonStreamingResponse(c.Request.Context(), resp, c, &Account{REDACTED, "model", "model")
	if err != nil {
		t.Fatalf("handleNonStreamingResponse error: %v", err)
REDACTED

	if !strings.Contains(rec.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("expected default Content-Type, got %q", rec.Header().Get("Content-Type"))
REDACTED
REDACTED

func TestOpenAIStreamingHeadersOverride(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Security: config.SecurityConfig{
			ResponseHeaders: config.ResponseHeaderConfig{Enabled: falseREDACTED,
	REDACTED,
		Gateway: config.GatewayConfig{
			StreamDataIntervalTimeout: 0,
			StreamKeepaliveInterval:   0,
			MaxLineSize:               defaultMaxLineSize,
	REDACTED,
REDACTED
	svc := &OpenAIGatewayService{cfg: cfgREDACTED

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	pr, pw := io.Pipe()
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       pr,
		Header: http.Header{
			"Cache-Control": []string{"upstream"REDACTED,
			"X-Request-Id":  []string{"req-123"REDACTED,
			"Content-Type":  []string{"application/custom"REDACTED,
	REDACTED,
REDACTED

	go func() {
		defer func() { _ = pw.Close() REDACTED()
		_, _ = pw.Write([]byte("data: {REDACTED\n\n"))
REDACTED()

	_, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, &Account{ID: 1REDACTED, time.Now(), "model", "model")
	_ = pr.Close()
	if err != nil {
		t.Fatalf("handleStreamingResponse error: %v", err)
REDACTED

	if rec.Header().Get("Cache-Control") != "no-cache" {
		t.Fatalf("expected Cache-Control override, got %q", rec.Header().Get("Cache-Control"))
REDACTED
	if rec.Header().Get("Content-Type") != "text/event-stream" {
		t.Fatalf("expected Content-Type override, got %q", rec.Header().Get("Content-Type"))
REDACTED
	if rec.Header().Get("X-Request-Id") != "req-123" {
		t.Fatalf("expected X-Request-Id passthrough, got %q", rec.Header().Get("X-Request-Id"))
REDACTED
REDACTED

func TestOpenAIInvalidBaseURLWhenAllowlistDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Security: config.SecurityConfig{
			URLAllowlist: config.URLAllowlistConfig{Enabled: falseREDACTED,
	REDACTED,
REDACTED
	svc := &OpenAIGatewayService{cfg: cfgREDACTED

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	account := &Account{
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
REDACTED"base_url": "://invalid-url"REDACTED,
REDACTED

	_, err := svc.buildUpstreamRequest(c.Request.Context(), c, account, []byte("{REDACTED"), "token", false, "", false)
	if err == nil {
		t.Fatalf("expected error for invalid base_url when allowlist disabled")
REDACTED
REDACTED

func TestOpenAIValidateUpstreamBaseURLDisabledRequiresHTTPS(t *testing.T) {
	cfg := &config.Config{
		Security: config.SecurityConfig{
			URLAllowlist: config.URLAllowlistConfig{Enabled: falseREDACTED,
	REDACTED,
REDACTED
	svc := &OpenAIGatewayService{cfg: cfgREDACTED

	if _, err := svc.validateUpstreamBaseURL("http://not-https.example.com"); err == nil {
		t.Fatalf("expected http to be rejected when allow_insecure_http is false")
REDACTED
	normalized, err := svc.validateUpstreamBaseURL("https://example.com")
	if err != nil {
		t.Fatalf("expected https to be allowed when allowlist disabled, got %v", err)
REDACTED
	if normalized != "https://example.com" {
		t.Fatalf("expected raw url passthrough, got %q", normalized)
REDACTED
REDACTED

func TestOpenAIValidateUpstreamBaseURLDisabledAllowsHTTP(t *testing.T) {
	cfg := &config.Config{
		Security: config.SecurityConfig{
			URLAllowlist: config.URLAllowlistConfig{
				Enabled:           false,
				AllowInsecureHTTP: true,
		REDACTED,
	REDACTED,
REDACTED
	svc := &OpenAIGatewayService{cfg: cfgREDACTED

	normalized, err := svc.validateUpstreamBaseURL("http://not-https.example.com")
	if err != nil {
		t.Fatalf("expected http allowed when allow_insecure_http is true, got %v", err)
REDACTED
	if normalized != "http://not-https.example.com" {
		t.Fatalf("expected raw url passthrough, got %q", normalized)
REDACTED
REDACTED

func TestOpenAIValidateUpstreamBaseURLEnabledEnforcesAllowlist(t *testing.T) {
	cfg := &config.Config{
		Security: config.SecurityConfig{
			URLAllowlist: config.URLAllowlistConfig{
				Enabled:       true,
				UpstreamHosts: []string{"example.com"REDACTED,
		REDACTED,
	REDACTED,
REDACTED
	svc := &OpenAIGatewayService{cfg: cfgREDACTED

	if _, err := svc.validateUpstreamBaseURL("https://example.com"); err != nil {
		t.Fatalf("expected allowlisted host to pass, got %v", err)
REDACTED
	if _, err := svc.validateUpstreamBaseURL("https://evil.com"); err == nil {
		t.Fatalf("expected non-allowlisted host to fail")
REDACTED
REDACTED
