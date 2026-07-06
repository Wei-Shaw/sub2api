package admin

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type stubAdminService struct {
	users                               []service.User
	apiKeys                             []service.APIKey
	groups                              []service.Group
	accounts                            []service.Account
	accountSchedulerScoreFilterAccounts []service.Account
	openAISchedulerScorePoolAccounts    []service.Account
	proxies                             []service.Proxy
	proxyCounts                         []service.ProxyWithAccountCount
	redeems                             []service.RedeemCode
	boundAuthIdentity                   *service.AdminBindAuthIdentityInput
	boundAuthIdentityFor                int64
	createdAccounts                     []*service.CreateAccountInput
	createdProxies                      []*service.CreateProxyInput
	updatedProxyIDs                     []int64
	updatedProxies                      []*service.UpdateProxyInput
	testedProxyIDs                      []int64
	getUserErr                          error
	createAccountErr                    error
	createSparkShadowErr                error
	updateAccountErr                    error
	bulkUpdateAccountErr                error
	checkMixedErr                       error
	lastMixedCheck                      struct {
		accountID int64
		platform  string
		groupIDs  []int64
REDACTED
	lastListAccounts struct {
		platform    string
		accountType string
		status      string
		search      string
		groupID     int64
		privacyMode string
		sortBy      string
		sortOrder   string
		calls       int
REDACTED
	lastListUsers struct {
		page      int
		pageSize  int
		filters   service.UserListFilters
		sortBy    string
		sortOrder string
		calls     int
REDACTED
	lastListProxies struct {
		protocol  string
		status    string
		search    string
		sortBy    string
		sortOrder string
		calls     int
REDACTED
	lastListRedeemCodes struct {
		codeType  string
		status    string
		search    string
		sortBy    string
		sortOrder string
		calls     int
REDACTED
	mu sync.Mutex
REDACTED

func newStubAdminService() *stubAdminService {
	now := time.Now().UTC()
	user := service.User{
		ID:        1,
		Email:     "user@example.com",
		Role:      service.RoleUser,
		Status:    service.StatusActive,
		CreatedAt: now,
		UpdatedAt: now,
REDACTED
	apiKey := service.APIKey{
		ID:        10,
		UserID:    user.ID,
		Key:       "sk-test",
		Name:      "test",
		Status:    service.StatusActive,
		CreatedAt: now,
		UpdatedAt: now,
REDACTED
	group := service.Group{
		ID:        2,
		Name:      "group",
		Platform:  service.PlatformAnthropic,
		Status:    service.StatusActive,
		CreatedAt: now,
		UpdatedAt: now,
REDACTED
	account := service.Account{
		ID:        3,
		Name:      "account",
		Platform:  service.PlatformAnthropic,
		Type:      service.AccountTypeOAuth,
		Status:    service.StatusActive,
		CreatedAt: now,
		UpdatedAt: now,
REDACTED
	proxy := service.Proxy{
		ID:        4,
		Name:      "proxy",
		Protocol:  "http",
		Host:      "127.0.0.1",
		Port:      8080,
		Status:    service.StatusActive,
		CreatedAt: now,
		UpdatedAt: now,
REDACTED
	redeem := service.RedeemCode{
		ID:        5,
		Code:      "R-TEST",
		Type:      service.RedeemTypeBalance,
		Value:     10,
		Status:    service.StatusUnused,
		CreatedAt: now,
REDACTED
	return &stubAdminService{
		users:       []service.User{userREDACTED,
		apiKeys:     []service.APIKey{apiKeyREDACTED,
		groups:      []service.Group{groupREDACTED,
		accounts:    []service.Account{accountREDACTED,
		proxies:     []service.Proxy{proxyREDACTED,
		proxyCounts: []service.ProxyWithAccountCount{{Proxy: proxy, AccountCount: 1REDACTEDREDACTED,
		redeems:     []service.RedeemCode{redeemREDACTED,
REDACTED
REDACTED

func (s *stubAdminService) ListUsers(ctx context.Context, page, pageSize int, filters service.UserListFilters, sortBy, sortOrder string) ([]service.User, int64, error) {
	s.lastListUsers.page = page
	s.lastListUsers.pageSize = pageSize
	s.lastListUsers.filters = filters
	s.lastListUsers.sortBy = sortBy
	s.lastListUsers.sortOrder = sortOrder
	s.lastListUsers.calls++
	return s.users, int64(len(s.users)), nil
REDACTED

func (s *stubAdminService) GetUser(ctx context.Context, id int64) (*service.User, error) {
	if s.getUserErr != nil {
		return nil, s.getUserErr
REDACTED
	for i := range s.users {
		if s.users[i].ID == id {
			return &s.users[i], nil
	REDACTED
REDACTED
	user := service.User{ID: id, Email: "user@example.com", Status: service.StatusActiveREDACTED
	return &user, nil
REDACTED

func (s *stubAdminService) GetUserIncludeDeleted(ctx context.Context, id int64) (*service.User, error) {
	return s.GetUser(ctx, id)
REDACTED

func (s *stubAdminService) CreateUser(ctx context.Context, input *service.CreateUserInput) (*service.User, error) {
	user := service.User{ID: 100, Email: input.Email, Status: service.StatusActiveREDACTED
	return &user, nil
REDACTED

func (s *stubAdminService) UpdateUser(ctx context.Context, id int64, input *service.UpdateUserInput) (*service.User, error) {
	user := service.User{ID: id, Email: "updated@example.com", Status: service.StatusActiveREDACTED
	return &user, nil
REDACTED

func (s *stubAdminService) DeleteUser(ctx context.Context, id int64) error {
	return nil
REDACTED

func (s *stubAdminService) UpdateUserBalance(ctx context.Context, userID int64, balance float64, operation string, notes string) (*service.User, error) {
	user := service.User{ID: userID, Balance: balance, Status: service.StatusActiveREDACTED
	return &user, nil
REDACTED

func (s *stubAdminService) BatchUpdateConcurrency(ctx context.Context, userIDs []int64, value int, mode string) (int, error) {
	return len(userIDs), nil
REDACTED

func (s *stubAdminService) GetUserAPIKeys(ctx context.Context, userID int64, page, pageSize int, sortBy, sortOrder string) ([]service.APIKey, int64, error) {
	return s.apiKeys, int64(len(s.apiKeys)), nil
REDACTED

func (s *stubAdminService) GetUserUsageStats(ctx context.Context, userID int64, period string) (any, error) {
	return map[string]any{"user_id": userIDREDACTED, nil
REDACTED

func (s *stubAdminService) GetUserRPMStatus(ctx context.Context, userID int64) (*service.UserRPMStatus, error) {
	user, err := s.GetUser(ctx, userID)
	if err != nil {
		return nil, err
REDACTED
	return &service.UserRPMStatus{
		UserRPMUsed:  0,
		UserRPMLimit: user.RPMLimit,
REDACTED, nil
REDACTED

func (s *stubAdminService) BindUserAuthIdentity(ctx context.Context, userID int64, input service.AdminBindAuthIdentityInput) (*service.AdminBoundAuthIdentity, error) {
	s.boundAuthIdentityFor = userID
	copied := input
	if input.Metadata != nil {
		copied.Metadata = map[string]any{REDACTED
		for key, value := range input.Metadata {
			copied.Metadata[key] = value
	REDACTED
REDACTED
	if input.Channel != nil {
		channel := *input.Channel
		if input.Channel.Metadata != nil {
			channel.Metadata = map[string]any{REDACTED
			for key, value := range input.Channel.Metadata {
				channel.Metadata[key] = value
		REDACTED
	REDACTED
		copied.Channel = &channel
REDACTED
	s.boundAuthIdentity = &copied

	now := time.Now().UTC()
	result := &service.AdminBoundAuthIdentity{
		UserID:          userID,
		ProviderType:    input.ProviderType,
		ProviderKey:     input.ProviderKey,
		ProviderSubject: input.ProviderSubject,
		VerifiedAt:      &now,
		Issuer:          input.Issuer,
		Metadata:        input.Metadata,
		CreatedAt:       now,
		UpdatedAt:       now,
REDACTED
	if input.Channel != nil {
		result.Channel = &service.AdminBoundAuthIdentityChannel{
			Channel:        input.Channel.Channel,
			ChannelAppID:   input.Channel.ChannelAppID,
			ChannelSubject: input.Channel.ChannelSubject,
			Metadata:       input.Channel.Metadata,
			CreatedAt:      now,
			UpdatedAt:      now,
	REDACTED
REDACTED
	return result, nil
REDACTED

func (s *stubAdminService) ListGroups(ctx context.Context, page, pageSize int, platform, status, search string, isExclusive *bool, sortBy, sortOrder string) ([]service.Group, int64, error) {
	return s.groups, int64(len(s.groups)), nil
REDACTED

func (s *stubAdminService) GetAllGroups(ctx context.Context) ([]service.Group, error) {
	return s.groups, nil
REDACTED

func (s *stubAdminService) GetAllGroupsByPlatform(ctx context.Context, platform string) ([]service.Group, error) {
	return s.groups, nil
REDACTED

func (s *stubAdminService) GetAllGroupsIncludingInactive(ctx context.Context) ([]service.Group, error) {
	return s.groups, nil
REDACTED

func (s *stubAdminService) GetGroup(ctx context.Context, id int64) (*service.Group, error) {
	group := service.Group{ID: id, Name: "group", Status: service.StatusActiveREDACTED
	return &group, nil
REDACTED

func (s *stubAdminService) GetGroupModelsListCandidates(ctx context.Context, id int64, platform string) ([]string, error) {
	if platform == service.PlatformOpenAI {
		return []string{"gpt-5.5", "gpt-5.4"REDACTED, nil
REDACTED
	return []string{"claude-sonnet-4-6"REDACTED, nil
REDACTED

func (s *stubAdminService) CreateGroup(ctx context.Context, input *service.CreateGroupInput) (*service.Group, error) {
	group := service.Group{ID: 200, Name: input.Name, Status: service.StatusActiveREDACTED
	return &group, nil
REDACTED

func (s *stubAdminService) UpdateGroup(ctx context.Context, id int64, input *service.UpdateGroupInput) (*service.Group, error) {
	group := service.Group{ID: id, Name: input.Name, Status: service.StatusActiveREDACTED
	return &group, nil
REDACTED

func (s *stubAdminService) DeleteGroup(ctx context.Context, id int64) error {
	return nil
REDACTED

func (s *stubAdminService) GetGroupAPIKeys(ctx context.Context, groupID int64, page, pageSize int) ([]service.APIKey, int64, error) {
	return s.apiKeys, int64(len(s.apiKeys)), nil
REDACTED

func (s *stubAdminService) GetGroupRateMultipliers(_ context.Context, _ int64) ([]service.UserGroupRateEntry, error) {
	return nil, nil
REDACTED

func (s *stubAdminService) ClearGroupRateMultipliers(_ context.Context, _ int64) error {
	return nil
REDACTED

func (s *stubAdminService) BatchSetGroupRateMultipliers(_ context.Context, _ int64, _ []service.GroupRateMultiplierInput) error {
	return nil
REDACTED

func (s *stubAdminService) ClearGroupRPMOverrides(_ context.Context, _ int64) error {
	return nil
REDACTED

func (s *stubAdminService) BatchSetGroupRPMOverrides(_ context.Context, _ int64, _ []service.GroupRPMOverrideInput) error {
	return nil
REDACTED

func (s *stubAdminService) ListAccounts(ctx context.Context, page, pageSize int, platform, accountType, status, search string, groupID int64, privacyMode string, sortBy, sortOrder string) ([]service.Account, int64, error) {
	s.lastListAccounts.platform = platform
	s.lastListAccounts.accountType = accountType
	s.lastListAccounts.status = status
	s.lastListAccounts.search = search
	s.lastListAccounts.groupID = groupID
	s.lastListAccounts.privacyMode = privacyMode
	s.lastListAccounts.sortBy = sortBy
	s.lastListAccounts.sortOrder = sortOrder
	s.lastListAccounts.calls++
	accounts := s.accounts
	total := len(accounts)
	if page < 1 {
		page = 1
REDACTED
	if pageSize < 1 {
		pageSize = total
REDACTED
	start := (page - 1) * pageSize
	if start >= total {
		return []service.Account{REDACTED, int64(total), nil
REDACTED
	end := start + pageSize
	if end > total {
		end = total
REDACTED
	return accounts[start:end], int64(total), nil
REDACTED

func (s *stubAdminService) ListAccountsForSchedulerScoreFilter(_ context.Context, platform, accountType, status, search string, groupID int64, privacyMode string) ([]service.Account, error) {
	if s.accountSchedulerScoreFilterAccounts != nil {
		return s.accountSchedulerScoreFilterAccounts, nil
REDACTED
	return s.accounts, nil
REDACTED

func (s *stubAdminService) ListOpenAISchedulableAccountsForSchedulerScore(_ context.Context, groupID *int64) ([]service.Account, error) {
	accounts := s.openAISchedulerScorePoolAccounts
	if accounts == nil {
		accounts = s.accounts
REDACTED
	out := make([]service.Account, 0, len(accounts))
	for _, account := range accounts {
		if account.Platform != service.PlatformOpenAI || !account.IsSchedulable() {
			continue
	REDACTED
		if groupID == nil {
			if len(account.AccountGroups) == 0 && len(account.GroupIDs) == 0 {
				out = append(out, account)
		REDACTED
			continue
	REDACTED
		for _, accountGroup := range account.AccountGroups {
			if accountGroup.GroupID == *groupID {
				out = append(out, account)
				break
		REDACTED
	REDACTED
REDACTED
	return out, nil
REDACTED

func (s *stubAdminService) GetAccount(ctx context.Context, id int64) (*service.Account, error) {
	account := service.Account{ID: id, Name: "account", Status: service.StatusActiveREDACTED
	return &account, nil
REDACTED

func (s *stubAdminService) GetAccountsByIDs(ctx context.Context, ids []int64) ([]*service.Account, error) {
	out := make([]*service.Account, 0, len(ids))
	for _, id := range ids {
		account := service.Account{ID: id, Name: "account", Status: service.StatusActiveREDACTED
		out = append(out, &account)
REDACTED
	return out, nil
REDACTED

func (s *stubAdminService) CreateAccount(ctx context.Context, input *service.CreateAccountInput) (*service.Account, error) {
	s.mu.Lock()
	s.createdAccounts = append(s.createdAccounts, input)
	s.mu.Unlock()
	if s.createAccountErr != nil {
		return nil, s.createAccountErr
REDACTED
	account := service.Account{ID: 300, Name: input.Name, Status: service.StatusActiveREDACTED
	return &account, nil
REDACTED

func (s *stubAdminService) UpdateAccount(ctx context.Context, id int64, input *service.UpdateAccountInput) (*service.Account, error) {
	if s.updateAccountErr != nil {
		return nil, s.updateAccountErr
REDACTED
	account := service.Account{ID: id, Name: input.Name, Status: service.StatusActiveREDACTED
	return &account, nil
REDACTED

func (s *stubAdminService) UpdateAccountExtra(ctx context.Context, id int64, updates map[string]any) error {
	return nil
REDACTED

func (s *stubAdminService) DeleteAccount(ctx context.Context, id int64) error {
	return nil
REDACTED

func (s *stubAdminService) RefreshAccountCredentials(ctx context.Context, id int64) (*service.Account, error) {
	account := service.Account{ID: id, Name: "account", Status: service.StatusActiveREDACTED
	return &account, nil
REDACTED

func (s *stubAdminService) ClearAccountError(ctx context.Context, id int64) (*service.Account, error) {
	account := service.Account{ID: id, Name: "account", Status: service.StatusActiveREDACTED
	return &account, nil
REDACTED

func (s *stubAdminService) SetAccountError(ctx context.Context, id int64, errorMsg string) error {
	return nil
REDACTED

func (s *stubAdminService) SetAccountSchedulable(ctx context.Context, id int64, schedulable bool) (*service.Account, error) {
	account := service.Account{ID: id, Name: "account", Status: service.StatusActive, Schedulable: schedulableREDACTED
	return &account, nil
REDACTED

func (s *stubAdminService) BulkUpdateAccounts(ctx context.Context, input *service.BulkUpdateAccountsInput) (*service.BulkUpdateAccountsResult, error) {
	if s.bulkUpdateAccountErr != nil {
		return nil, s.bulkUpdateAccountErr
REDACTED
	return &service.BulkUpdateAccountsResult{Success: len(input.AccountIDs), Failed: 0, SuccessIDs: input.AccountIDsREDACTED, nil
REDACTED

func (s *stubAdminService) CheckMixedChannelRisk(ctx context.Context, currentAccountID int64, currentAccountPlatform string, groupIDs []int64) error {
	s.lastMixedCheck.accountID = currentAccountID
	s.lastMixedCheck.platform = currentAccountPlatform
	s.lastMixedCheck.groupIDs = append([]int64(nil), groupIDs...)
	return s.checkMixedErr
REDACTED

func (s *stubAdminService) ListProxies(ctx context.Context, page, pageSize int, protocol, status, search string, sortBy, sortOrder string) ([]service.Proxy, int64, error) {
	s.lastListProxies.protocol = protocol
	s.lastListProxies.status = status
	s.lastListProxies.search = search
	s.lastListProxies.sortBy = sortBy
	s.lastListProxies.sortOrder = sortOrder
	s.lastListProxies.calls++
	search = strings.TrimSpace(strings.ToLower(search))
	filtered := make([]service.Proxy, 0, len(s.proxies))
	for _, proxy := range s.proxies {
		if protocol != "" && proxy.Protocol != protocol {
			continue
	REDACTED
		if status != "" && proxy.Status != status {
			continue
	REDACTED
		if search != "" {
			name := strings.ToLower(proxy.Name)
			host := strings.ToLower(proxy.Host)
			if !strings.Contains(name, search) && !strings.Contains(host, search) {
				continue
		REDACTED
	REDACTED
		filtered = append(filtered, proxy)
REDACTED
	return filtered, int64(len(filtered)), nil
REDACTED

func (s *stubAdminService) ListProxiesWithAccountCount(ctx context.Context, page, pageSize int, protocol, status, search string, sortBy, sortOrder string) ([]service.ProxyWithAccountCount, int64, error) {
	return s.proxyCounts, int64(len(s.proxyCounts)), nil
REDACTED

func (s *stubAdminService) GetAllProxies(ctx context.Context) ([]service.Proxy, error) {
	return s.proxies, nil
REDACTED

func (s *stubAdminService) GetAllProxiesWithAccountCount(ctx context.Context) ([]service.ProxyWithAccountCount, error) {
	return s.proxyCounts, nil
REDACTED

func (s *stubAdminService) GetProxy(ctx context.Context, id int64) (*service.Proxy, error) {
	for i := range s.proxies {
		proxy := s.proxies[i]
		if proxy.ID == id {
			return &proxy, nil
	REDACTED
REDACTED
	proxy := service.Proxy{ID: id, Name: "proxy", Status: service.StatusActiveREDACTED
	return &proxy, nil
REDACTED

func (s *stubAdminService) GetProxiesByIDs(ctx context.Context, ids []int64) ([]service.Proxy, error) {
	if len(ids) == 0 {
		return []service.Proxy{REDACTED, nil
REDACTED
	out := make([]service.Proxy, 0, len(ids))
	seen := make(map[int64]struct{REDACTED, len(ids))
	for _, id := range ids {
		seen[id] = struct{REDACTED{REDACTED
REDACTED
	for i := range s.proxies {
		proxy := s.proxies[i]
		if _, ok := seen[proxy.ID]; ok {
			out = append(out, proxy)
	REDACTED
REDACTED
	return out, nil
REDACTED

func (s *stubAdminService) CreateProxy(ctx context.Context, input *service.CreateProxyInput) (*service.Proxy, error) {
	s.mu.Lock()
	s.createdProxies = append(s.createdProxies, input)
	s.mu.Unlock()
	proxy := service.Proxy{ID: 400, Name: input.Name, Status: service.StatusActiveREDACTED
	return &proxy, nil
REDACTED

func (s *stubAdminService) UpdateProxy(ctx context.Context, id int64, input *service.UpdateProxyInput) (*service.Proxy, error) {
	s.mu.Lock()
	s.updatedProxyIDs = append(s.updatedProxyIDs, id)
	s.updatedProxies = append(s.updatedProxies, input)
	s.mu.Unlock()
	proxy := service.Proxy{ID: id, Name: input.Name, Status: service.StatusActiveREDACTED
	return &proxy, nil
REDACTED

func (s *stubAdminService) DeleteProxy(ctx context.Context, id int64) error {
	return nil
REDACTED

func (s *stubAdminService) BatchDeleteProxies(ctx context.Context, ids []int64) (*service.ProxyBatchDeleteResult, error) {
	return &service.ProxyBatchDeleteResult{DeletedIDs: idsREDACTED, nil
REDACTED

func (s *stubAdminService) GetProxyAccounts(ctx context.Context, proxyID int64) ([]service.ProxyAccountSummary, error) {
	return []service.ProxyAccountSummary{{ID: 1, Name: "account"REDACTEDREDACTED, nil
REDACTED

func (s *stubAdminService) CheckProxyExists(ctx context.Context, host string, port int, username, password string) (bool, error) {
	return false, nil
REDACTED

func (s *stubAdminService) TestProxy(ctx context.Context, id int64) (*service.ProxyTestResult, error) {
	s.mu.Lock()
	s.testedProxyIDs = append(s.testedProxyIDs, id)
	s.mu.Unlock()
	return &service.ProxyTestResult{Success: true, Message: "ok"REDACTED, nil
REDACTED

func (s *stubAdminService) CheckProxyQuality(ctx context.Context, id int64) (*service.ProxyQualityCheckResult, error) {
	return &service.ProxyQualityCheckResult{
		ProxyID:        id,
		Score:          95,
		Grade:          "A",
		Summary:        "通过 5 项，告警 0 项，失败 0 项，挑战 0 项",
		PassedCount:    5,
		WarnCount:      0,
		FailedCount:    0,
		ChallengeCount: 0,
		CheckedAt:      time.Now().Unix(),
		Items: []service.ProxyQualityCheckItem{
			{Target: "base_connectivity", Status: "pass", Message: "ok"REDACTED,
			{Target: "openai", Status: "pass", HTTPStatus: 401REDACTED,
			{Target: "anthropic", Status: "pass", HTTPStatus: 401REDACTED,
			{Target: "gemini", Status: "pass", HTTPStatus: 200REDACTED,
	REDACTED,
REDACTED, nil
REDACTED

func (s *stubAdminService) ListRedeemCodes(ctx context.Context, page, pageSize int, codeType, status, search string, sortBy, sortOrder string) ([]service.RedeemCode, int64, error) {
	s.lastListRedeemCodes.codeType = codeType
	s.lastListRedeemCodes.status = status
	s.lastListRedeemCodes.search = search
	s.lastListRedeemCodes.sortBy = sortBy
	s.lastListRedeemCodes.sortOrder = sortOrder
	s.lastListRedeemCodes.calls++
	return s.redeems, int64(len(s.redeems)), nil
REDACTED

func (s *stubAdminService) GetRedeemCode(ctx context.Context, id int64) (*service.RedeemCode, error) {
	code := service.RedeemCode{ID: id, Code: "R-TEST", Status: service.StatusUnusedREDACTED
	return &code, nil
REDACTED

func (s *stubAdminService) GenerateRedeemCodes(ctx context.Context, input *service.GenerateRedeemCodesInput) ([]service.RedeemCode, error) {
	return s.redeems, nil
REDACTED

func (s *stubAdminService) DeleteRedeemCode(ctx context.Context, id int64) error {
	return nil
REDACTED

func (s *stubAdminService) BatchDeleteRedeemCodes(ctx context.Context, ids []int64) (int64, error) {
	return int64(len(ids)), nil
REDACTED

func (s *stubAdminService) ExpireRedeemCode(ctx context.Context, id int64) (*service.RedeemCode, error) {
	code := service.RedeemCode{ID: id, Code: "R-TEST", Status: service.StatusUsedREDACTED
	return &code, nil
REDACTED

func (s *stubAdminService) GetUserBalanceHistory(ctx context.Context, userID int64, page, pageSize int, codeType string) ([]service.RedeemCode, int64, float64, error) {
	return s.redeems, int64(len(s.redeems)), 100.0, nil
REDACTED

func (s *stubAdminService) UpdateGroupSortOrders(ctx context.Context, updates []service.GroupSortOrderUpdate) error {
	return nil
REDACTED

func (s *stubAdminService) AdminUpdateAPIKeyGroupID(ctx context.Context, keyID int64, groupID *int64) (*service.AdminUpdateAPIKeyGroupIDResult, error) {
	for i := range s.apiKeys {
		if s.apiKeys[i].ID == keyID {
			k := s.apiKeys[i]
			if groupID != nil {
				if *groupID == 0 {
					k.GroupID = nil
			REDACTED else {
					gid := *groupID
					k.GroupID = &gid
			REDACTED
		REDACTED
			return &service.AdminUpdateAPIKeyGroupIDResult{APIKey: &kREDACTED, nil
	REDACTED
REDACTED
	return nil, service.ErrAPIKeyNotFound
REDACTED

func (s *stubAdminService) AdminResetAPIKeyRateLimitUsage(ctx context.Context, keyID int64) (*service.APIKey, error) {
	for i := range s.apiKeys {
		if s.apiKeys[i].ID == keyID {
			s.apiKeys[i].Usage5h = 0
			s.apiKeys[i].Usage1d = 0
			s.apiKeys[i].Usage7d = 0
			s.apiKeys[i].Window5hStart = nil
			s.apiKeys[i].Window1dStart = nil
			s.apiKeys[i].Window7dStart = nil
			k := s.apiKeys[i]
			return &k, nil
	REDACTED
REDACTED
	return nil, service.ErrAPIKeyNotFound
REDACTED

func (s *stubAdminService) ResetAccountQuota(ctx context.Context, id int64) error {
	return nil
REDACTED

func (s *stubAdminService) EnsureOpenAIPrivacy(ctx context.Context, account *service.Account) string {
	return ""
REDACTED

func (s *stubAdminService) EnsureAntigravityPrivacy(ctx context.Context, account *service.Account) string {
	return ""
REDACTED

func (s *stubAdminService) ForceOpenAIPrivacy(ctx context.Context, account *service.Account) string {
	return ""
REDACTED

func (s *stubAdminService) ForceAntigravityPrivacy(ctx context.Context, account *service.Account) string {
	return ""
REDACTED

func (s *stubAdminService) ReplaceUserGroup(ctx context.Context, userID, oldGroupID, newGroupID int64) (*service.ReplaceUserGroupResult, error) {
	return &service.ReplaceUserGroupResult{MigratedKeys: 0REDACTED, nil
REDACTED

func (s *stubAdminService) RevertAccountProxyFallback(ctx context.Context, id int64) error {
	return nil
REDACTED

func (s *stubAdminService) CreateShadow(ctx context.Context, parentID int64, opts service.ShadowOptions) (*service.Account, error) {
	if s.createSparkShadowErr != nil {
		return nil, s.createSparkShadowErr
REDACTED
	pid := parentID
	return &service.Account{
		ID:              9001,
		Name:            opts.Name,
		Platform:        service.PlatformOpenAI,
		Type:            service.AccountTypeOAuth,
		Priority:        opts.Priority,
		Concurrency:     opts.Concurrency,
		GroupIDs:        opts.GroupIDs,
		ParentAccountID: &pid,
		QuotaDimension:  service.QuotaDimensionSpark,
		Credentials:     map[string]any{REDACTED,
		Extra:           map[string]any{REDACTED,
REDACTED, nil
REDACTED

// Ensure stub implements interface.
var _ service.AdminService = (*stubAdminService)(nil)
