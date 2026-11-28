package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// ZhipuResetCardsExtraKey 是受管的加密授权，不参与普通账号编辑、导入或导出。
const ZhipuResetCardsExtraKey = "zhipu_reset_cards_binding"

var (
	ErrZhipuResetUnavailable = infraerrors.ServiceUnavailable("ZHIPU_RESET_UNAVAILABLE", "GLM reset cards are unavailable")
	errZhipuResetAccount     = infraerrors.BadRequest("ZHIPU_RESET_ACCOUNT", "Select a personal BigModel Coding Plan API key account")
	errZhipuResetChanged     = infraerrors.Conflict("ZHIPU_RESET_CHANGED", "GLM account or authorization changed; reload this panel")
	errZhipuResetBound       = infraerrors.Conflict("ZHIPU_RESET_ALREADY_BOUND", "Remove the existing GLM authorization before binding again")
	errZhipuResetBusy        = infraerrors.TooManyRequests("ZHIPU_RESET_BUSY", "GLM reset-card operation is busy")
	errZhipuResetInput       = infraerrors.BadRequest("ZHIPU_RESET_INVALID", "Invalid GLM reset-card operation")
)

const (
	zhipuDeviceTTL   = 10 * time.Minute
	zhipuMaxAttempts = 32
	zhipuMaxInFlight = 4
	zhipuFiveHour    = "FIVE_HOUR"
	zhipuWeek        = "WEEK"
)

// ZhipuResetDevicePrompt 是浏览器批准所需的短期信息，不含官方轮询凭据。
type ZhipuResetDevicePrompt struct {
	AttemptID    string    `json:"attempt_id"`    // 本地发起标识。
	AuthorizeURL string    `json:"authorize_url"` // 智谱官方登录地址。
	ExpiresAt    time.Time `json:"expires_at"`    // 本地授权截止时间。
	PollInterval int       `json:"poll_interval"` // 最小轮询间隔，秒。
}

// ZhipuResetCard 只返回官方卡片的到期时间。
type ZhipuResetCard struct {
	ExpiresAt int64 `json:"expire_at"` // Unix 毫秒。
}

// ZhipuResetHistory 只返回上次重置时间，不返回登录资料。
type ZhipuResetHistory struct {
	UsedAt int64 `json:"used_at"` // Unix 毫秒。
}

// ZhipuResetCardStatus 是官方卡片与核销历史的脱敏视图。
type ZhipuResetCardStatus struct {
	FiveHour       []ZhipuResetCard   `json:"available_five_hour_resets"`     // 可用 5 小时卡。
	Week           []ZhipuResetCard   `json:"available_week_resets"`          // 可用周卡。
	LatestFiveHour *ZhipuResetHistory `json:"latest_five_hour_reset_history"` // 未重置时为空。
	LatestWeek     *ZhipuResetHistory `json:"latest_week_reset_history"`      // 未重置时为空。
}

// ZhipuResetState 仅返回安全的本地绑定标识和当前卡片状态。
type ZhipuResetState struct {
	BindingID string                `json:"binding_id,omitempty"` // 换绑后失效，防止旧页面向新账户用卡。
	Status    string                `json:"status"`               // unbound、bound 或 unauthorized。
	Cards     *ZhipuResetCardStatus `json:"cards,omitempty"`      // 只有成功读取官方状态时返回。
}

// ZhipuResetDeviceStatus 表示浏览器授权是否完成。
type ZhipuResetDeviceStatus struct {
	Status string           `json:"status"`          // pending、expired、rejected 或 bound。
	State  *ZhipuResetState `json:"state,omitempty"` // 绑定成功后返回状态。
}

// ZhipuResetActionResult 区分官方操作结果与后续状态读回。
type ZhipuResetActionResult struct {
	Outcome   string                `json:"outcome"`               // granted、not_eligible 或 used。
	NextTryAt int64                 `json:"next_try_at,omitempty"` // 领取未获资格时官方给出的 Unix 毫秒。
	Cards     *ZhipuResetCardStatus `json:"cards,omitempty"`       // 回读失败时为空，不代表操作失败。
}

type zhipuResetBinding struct {
	ID         string `json:"id"`
	Ciphertext string `json:"ciphertext"`
}

type zhipuResetSealedSession struct {
	zhipuResetSession
	Stamp [sha256.Size]byte `json:"account_stamp"`
}

type zhipuResetAttempt struct {
	prompt   ZhipuResetDevicePrompt
	actor    int64
	stamp    [sha256.Size]byte
	flowID   string
	token    string
	nextPoll time.Time
}

type zhipuResetRepository interface {
	GetByID(context.Context, int64) (*Account, error)
	CompareAndSwapZhipuResetCards(context.Context, *Account, json.RawMessage) (bool, error)
}

// ZhipuResetCardsService 管理独立授权及手动重置操作，不将推理密钥用于控制面鉴权。
type ZhipuResetCardsService struct {
	repo      zhipuResetRepository
	upstream  HTTPUpstream
	encryptor SecretEncryptor
	keyReady  bool
	mu        sync.Mutex
	active    map[int64]struct{}
	attempts  map[int64]*zhipuResetAttempt
}

// NewZhipuResetCardsService 使用现有持久化加密密钥；未配置时拒绝开启授权。
func NewZhipuResetCardsService(repo AccountRepository, upstream HTTPUpstream, encryptor SecretEncryptor, cfg *config.Config) *ZhipuResetCardsService {
	storage, _ := repo.(zhipuResetRepository)
	return &ZhipuResetCardsService{repo: storage, upstream: upstream, encryptor: encryptor,
		keyReady: cfg != nil && cfg.Totp.EncryptionKeyConfigured,
		active:   make(map[int64]struct{}), attempts: make(map[int64]*zhipuResetAttempt)}
}

// IsZhipuResetCardsAccount 只接受国内官方个人 Coding Plan，不将团队或国际站套餐回落为个人。
func IsZhipuResetCardsAccount(a *Account) bool {
	if a == nil || a.Platform != PlatformZhipu || a.Type != AccountTypeAPIKey || a.IsShadow() || !a.IsCodingPlan() ||
		strings.TrimSpace(a.GetCredential("zhipu_organization")) != "" || strings.TrimSpace(a.GetCredential("zhipu_project")) != "" {
		return false
	}
	u, err := url.Parse(a.GetOpenAIBaseURL())
	return err == nil && u.Scheme == "https" && u.Host == "open.bigmodel.cn" && u.User == nil
}

func (s *ZhipuResetCardsService) account(ctx context.Context, id int64) (*Account, error) {
	if s == nil || s.repo == nil || s.upstream == nil || s.encryptor == nil || !s.keyReady {
		return nil, ErrZhipuResetUnavailable
	}
	a, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !IsZhipuResetCardsAccount(a) {
		return nil, errZhipuResetAccount
	}
	return a, nil
}

func (s *ZhipuResetCardsService) enter(id int64) (func(), error) {
	if s == nil || s.repo == nil || s.upstream == nil || s.encryptor == nil || !s.keyReady {
		return nil, ErrZhipuResetUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, busy := s.active[id]; busy || len(s.active) >= zhipuMaxInFlight {
		return nil, errZhipuResetBusy
	}
	s.active[id] = struct{}{}
	return func() {
		s.mu.Lock()
		delete(s.active, id)
		s.mu.Unlock()
	}, nil
}

func zhipuResetStamp(a *Account) ([sha256.Size]byte, error) {
	raw, err := json.Marshal([]any{a.Credentials, a.ProxyID})
	return sha256.Sum256(raw), err
}

// Start 发起官方浏览器授权。待批准状态只在本实例保留，限定管理员与账号。
func (s *ZhipuResetCardsService) Start(ctx context.Context, id, actor int64) (*ZhipuResetDevicePrompt, error) {
	exit, err := s.enter(id)
	if err != nil {
		return nil, err
	}
	defer exit()
	a, err := s.account(ctx, id)
	if err != nil {
		return nil, err
	}
	if a.Extra[ZhipuResetCardsExtraKey] != nil {
		return nil, errZhipuResetBound
	}
	stamp, err := zhipuResetStamp(a)
	if err != nil || actor <= 0 {
		return nil, errZhipuResetInput
	}
	now := time.Now()
	s.mu.Lock()
	for key, attempt := range s.attempts {
		if !now.Before(attempt.prompt.ExpiresAt) {
			delete(s.attempts, key)
		}
	}
	if p := s.attempts[id]; p != nil {
		s.mu.Unlock()
		if p.actor != actor || p.stamp != stamp {
			return nil, errZhipuResetChanged
		}
		prompt := p.prompt
		return &prompt, nil
	}
	if len(s.attempts)+len(s.active) > zhipuMaxAttempts {
		s.mu.Unlock()
		return nil, errZhipuResetBusy
	}
	s.mu.Unlock()

	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		return nil, ErrZhipuResetUnavailable
	}
	token := hex.EncodeToString(secret[:])
	code, data, err := s.call(ctx, id, http.MethodPost, zhipuDevicePath+"/init", token, nil, map[string]string{"provider": "bigmodel"}, false)
	if err != nil {
		return nil, err
	}
	var init zhipuDeviceInit
	if code != 0 || json.Unmarshal(data, &init) != nil || !zhipuFlowID.MatchString(init.FlowID) ||
		!safeZhipuAuthorizeURL(init.AuthorizeURL) || init.PollInterval < 1 || init.PollInterval > 60 {
		return nil, errZhipuResetUpstream
	}
	now = time.Now()
	serverTime := init.ServerTime
	if serverTime <= 0 {
		serverTime = now.Unix()
	}
	lifetime := min(init.ExpiresAt-serverTime, int64(zhipuDeviceTTL/time.Second))
	if lifetime <= 0 {
		return nil, errZhipuDeviceExpired
	}
	interval := max(2, init.PollInterval)
	prompt := ZhipuResetDevicePrompt{rand.Text(), init.AuthorizeURL, now.Add(time.Duration(lifetime) * time.Second), interval}
	s.mu.Lock()
	s.attempts[id] = &zhipuResetAttempt{prompt, actor, stamp, init.FlowID, token, now.Add(time.Duration(interval) * time.Second)}
	s.mu.Unlock()
	return &prompt, nil
}

// Poll 只处理本次授权；先验证卡片读取权限再持久化，不替换已有绑定。
func (s *ZhipuResetCardsService) Poll(ctx context.Context, id, actor int64, attemptID string) (*ZhipuResetDeviceStatus, error) {
	exit, err := s.enter(id)
	if err != nil {
		return nil, err
	}
	defer exit()
	a, err := s.account(ctx, id)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	s.mu.Lock()
	p := s.attempts[id]
	if p == nil || p.actor != actor || p.prompt.AttemptID != attemptID {
		s.mu.Unlock()
		return nil, errZhipuDeviceExpired
	}
	if !now.Before(p.prompt.ExpiresAt) {
		delete(s.attempts, id)
		s.mu.Unlock()
		return &ZhipuResetDeviceStatus{Status: "expired"}, nil
	}
	if now.Before(p.nextPoll) {
		s.mu.Unlock()
		return &ZhipuResetDeviceStatus{Status: "pending"}, nil
	}
	p.nextPoll = now.Add(time.Duration(p.prompt.PollInterval) * time.Second)
	s.mu.Unlock()
	stamp, err := zhipuResetStamp(a)
	if err != nil || p.stamp != stamp || a.Extra[ZhipuResetCardsExtraKey] != nil {
		s.dropAttempt(id)
		return nil, errZhipuResetChanged
	}
	code, data, err := s.call(ctx, id, http.MethodGet, zhipuDevicePath+"/poll/"+p.flowID, p.token, nil, nil, false)
	if err == errZhipuDeviceExpired {
		s.dropAttempt(id)
		return &ZhipuResetDeviceStatus{Status: "expired"}, nil
	}
	if err != nil {
		return nil, err
	}
	var result zhipuDeviceResult
	if code != 0 || json.Unmarshal(data, &result) != nil {
		return nil, errZhipuResetUpstream
	}
	if result.Status == "pending" {
		return &ZhipuResetDeviceStatus{Status: "pending"}, nil
	}
	s.dropAttempt(id)
	if result.Status != "ready" {
		return &ZhipuResetDeviceStatus{Status: "rejected"}, nil
	}
	if !time.Now().Before(p.prompt.ExpiresAt) {
		return &ZhipuResetDeviceStatus{Status: "expired"}, nil
	}
	accessToken := result.BigModel.AccessToken
	if accessToken == "" {
		accessToken = result.BigModel.LegacyToken
	}
	zcodeToken := strings.TrimSpace(result.Token)
	if len(zcodeToken) > 7 && strings.EqualFold(zcodeToken[:7], "Bearer ") {
		zcodeToken = strings.TrimSpace(zcodeToken[7:])
	}
	session := zhipuResetSession{zcodeToken, strings.TrimSpace(accessToken)}
	if result.User.ID == "" || !session.valid() {
		return nil, errZhipuResetAuth
	}
	cards, err := s.cards(ctx, id, session)
	if err != nil {
		return nil, err
	}
	plain, err := json.Marshal(zhipuResetSealedSession{session, stamp})
	if err != nil {
		return nil, ErrZhipuResetUnavailable
	}
	ciphertext, err := s.encryptor.Encrypt(string(plain))
	if err != nil {
		return nil, ErrZhipuResetUnavailable
	}
	bindingID := rand.Text()
	raw, err := json.Marshal(zhipuResetBinding{bindingID, ciphertext})
	if err != nil {
		return nil, ErrZhipuResetUnavailable
	}
	if err = s.persist(ctx, a, raw); err != nil {
		return nil, err
	}
	return &ZhipuResetDeviceStatus{"bound", &ZhipuResetState{bindingID, "bound", cards}}, nil
}

func (s *ZhipuResetCardsService) dropAttempt(id int64) {
	s.mu.Lock()
	delete(s.attempts, id)
	s.mu.Unlock()
}

func zhipuResetBindingFromAccount(a *Account) (zhipuResetBinding, bool) {
	var binding zhipuResetBinding
	raw, err := json.Marshal(a.Extra[ZhipuResetCardsExtraKey])
	ok := err == nil && json.Unmarshal(raw, &binding) == nil && zhipuFlowID.MatchString(binding.ID) && binding.Ciphertext != ""
	return binding, ok
}

// HasZhipuResetCardsBinding 表示已保存的授权，不代表上游会话仍有效。
func HasZhipuResetCardsBinding(a *Account) bool {
	if !IsZhipuResetCardsAccount(a) || a.Extra[ZhipuResetCardsExtraKey] == nil {
		return false
	}
	_, ok := zhipuResetBindingFromAccount(a)
	return ok
}

func (s *ZhipuResetCardsService) session(a *Account) (zhipuResetBinding, zhipuResetSession, error) {
	binding, ok := zhipuResetBindingFromAccount(a)
	if !ok {
		return binding, zhipuResetSession{}, errZhipuResetAuth
	}
	var sealed zhipuResetSealedSession
	stamp, stampErr := zhipuResetStamp(a)
	plain, err := s.encryptor.Decrypt(binding.Ciphertext)
	if stampErr != nil || err != nil || json.Unmarshal([]byte(plain), &sealed) != nil ||
		sealed.Stamp != stamp || !sealed.valid() {
		return binding, zhipuResetSession{}, errZhipuResetAuth
	}
	return binding, sealed.zhipuResetSession, nil
}

// Status 读取官方卡片与核销历史，查询不领取也不消耗卡片。
func (s *ZhipuResetCardsService) Status(ctx context.Context, id int64) (*ZhipuResetState, error) {
	exit, err := s.enter(id)
	if err != nil {
		return nil, err
	}
	defer exit()
	a, err := s.account(ctx, id)
	if err != nil {
		return nil, err
	}
	if a.Extra[ZhipuResetCardsExtraKey] == nil {
		return &ZhipuResetState{Status: "unbound"}, nil
	}
	b, session, err := s.session(a)
	var cards *ZhipuResetCardStatus
	if err == nil {
		cards, err = s.cards(ctx, id, session)
	}
	if err == errZhipuResetAuth {
		return &ZhipuResetState{Status: "unauthorized", BindingID: b.ID}, nil
	}
	if err != nil {
		return nil, err
	}
	return &ZhipuResetState{b.ID, "bound", cards}, nil
}

func (s *ZhipuResetCardsService) cards(ctx context.Context, id int64, session zhipuResetSession) (*ZhipuResetCardStatus, error) {
	code, data, err := s.call(ctx, id, http.MethodGet, zhipuResetPath+"/status", "", &session, nil, false)
	if err != nil {
		return nil, err
	}
	var cards ZhipuResetCardStatus
	if code != 0 || json.Unmarshal(data, &cards) != nil || cards.FiveHour == nil || cards.Week == nil ||
		len(cards.FiveHour) > 100 || len(cards.Week) > 100 {
		return nil, errZhipuResetUpstream
	}
	for _, window := range [][]ZhipuResetCard{cards.FiveHour, cards.Week} {
		for _, card := range window {
			if card.ExpiresAt <= 0 {
				return nil, errZhipuResetUpstream
			}
		}
	}
	if (cards.LatestFiveHour != nil && cards.LatestFiveHour.UsedAt <= 0) ||
		(cards.LatestWeek != nil && cards.LatestWeek.UsedAt <= 0) {
		return nil, errZhipuResetUpstream
	}
	return &cards, nil
}

// Act 仅执行明确发起的一次领卡或用卡；对超时不自动重放，保留官方幂等键。
func (s *ZhipuResetCardsService) Act(ctx context.Context, id int64, bindingID, op, resetType, idempotencyKey string) (*ZhipuResetActionResult, error) {
	if !zhipuFlowID.MatchString(bindingID) || len(idempotencyKey) < 16 || len(idempotencyKey) > 64 ||
		!zhipuFlowID.MatchString(idempotencyKey) ||
		(op == "use" && resetType != zhipuFiveHour && resetType != zhipuWeek) ||
		(op != "use" && (op != "opportunity" || resetType != "")) {
		return nil, errZhipuResetInput
	}
	exit, err := s.enter(id)
	if err != nil {
		return nil, err
	}
	defer exit()
	a, err := s.account(ctx, id)
	if err != nil {
		return nil, err
	}
	b, session, err := s.session(a)
	if err != nil {
		return nil, err
	}
	if b.ID != bindingID {
		return nil, errZhipuResetChanged
	}
	// 同次请求重试沿用同一上游键；不同绑定的请求不能复用旧结果。
	hash := sha256.Sum256([]byte(bindingID + ":" + op + ":" + idempotencyKey))
	body := struct {
		Key       string `json:"idempotency_key"`
		ResetType string `json:"reset_type,omitempty"`
	}{hex.EncodeToString(hash[:]), resetType}
	code, data, err := s.call(ctx, id, http.MethodPost, zhipuResetPath+"/"+op, "", &session, body, true)
	if err != nil {
		return nil, err
	}
	var reply struct {
		Used      bool  `json:"used"`
		Granted   *bool `json:"granted"`
		NextTryAt int64 `json:"next_try_at"`
	}
	if code != 0 && (code != 3301 || op != "opportunity") {
		return nil, errZhipuResetRejected
	}
	if json.Unmarshal(data, &reply) != nil {
		return nil, errZhipuResetUnknown
	}
	var result ZhipuResetActionResult
	switch {
	case op == "use" && reply.Used:
		result.Outcome = "used"
	case op == "opportunity" && code == 0 && reply.Granted != nil && *reply.Granted:
		result.Outcome = "granted"
	case op == "opportunity" && code == 3301 && reply.Granted != nil && !*reply.Granted && reply.NextTryAt > 0:
		return &ZhipuResetActionResult{Outcome: "not_eligible", NextTryAt: reply.NextTryAt}, nil
	default:
		return nil, errZhipuResetUnknown
	}
	// 已收到成功应答后，状态读取失败不应诱导操作者再次用卡。
	result.Cards, _ = s.cards(ctx, id, session)
	return &result, nil
}

// Unbind 取消本地授权和本实例待批准流程，不撤销账号推理 Key 或供应商套餐。
func (s *ZhipuResetCardsService) Unbind(ctx context.Context, id int64) error {
	exit, err := s.enter(id)
	if err != nil {
		return err
	}
	defer exit()
	a, err := s.account(ctx, id)
	if err != nil {
		return err
	}
	s.dropAttempt(id)
	if a.Extra[ZhipuResetCardsExtraKey] == nil {
		return nil
	}
	return s.persist(ctx, a, nil)
}

func (s *ZhipuResetCardsService) persist(ctx context.Context, a *Account, data json.RawMessage) error {
	ok, err := s.repo.CompareAndSwapZhipuResetCards(ctx, a, data)
	if err != nil {
		return err
	}
	if !ok {
		return errZhipuResetChanged
	}
	return nil
}
