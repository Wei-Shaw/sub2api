package service

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/shopspring/decimal"
	"golang.org/x/sync/singleflight"
)

// 用户可以按 USD 或 VND 填充值金额，但 SePay 只结算 VND，而账户余额只记 USD。
// 两个方向的换算都要一个可信的 USD/VND 汇率，这里从越南外贸银行（Vietcombank）
// 的公开牌价接口取。
const (
	// vietcombankExchangeRateURL 是 Vietcombank 的公开牌价 XML。
	vietcombankExchangeRateURL = "https://portal.vietcombank.com.vn/Usercontrols/TVPortal.TyGia/pXML.aspx"

	// exchangeRateHTTPTimeout 限制单次牌价抓取时长。建单路径会等这个调用，
	// 不能让外部接口把下单拖死。
	exchangeRateHTTPTimeout = 5 * time.Second

	// exchangeRateFailureBackoff 是抓取失败后暂停重试的时长。期间直接走库里的
	// 快照兜底，源站挂起时不会让每次下单都等满超时。
	exchangeRateFailureBackoff = time.Minute

	// exchangeRateRefreshInterval 是内存里认为汇率仍然新鲜的时长。
	// Vietcombank 一天只更新几次，一小时足够贴近，也不会打扰对方。
	exchangeRateRefreshInterval = time.Hour

	// SettingExchangeRateCache 存放最近一次成功抓取的汇率快照。
	// 落库而不是只放内存：重启后仍然有兜底值可用。
	SettingExchangeRateCache = "EXCHANGE_RATE_USD_VND_CACHE"

	// SettingExchangeRateMarkupPercent 是叠加在牌价上的安全边际（百分比）。
	SettingExchangeRateMarkupPercent = "EXCHANGE_RATE_MARKUP_PERCENT"

	// SettingExchangeRateMaxAgeHours 是缓存汇率的最长可用时长。
	// 超过这个年龄就拒绝建单，而不是拿一个不知道有多旧的价格去收钱。
	SettingExchangeRateMaxAgeHours = "EXCHANGE_RATE_MAX_AGE_HOURS"

	defaultExchangeRateMaxAgeHours = 24
)

// ExchangeRateSnapshot 是一次牌价抓取的结果。
type ExchangeRateSnapshot struct {
	// Rate 是 1 USD 折合多少 VND，取 Vietcombank 的「卖出」价：
	// 用户拿 VND 换取以 USD 计价的余额，相当于我们把 USD 卖给用户。
	Rate      decimal.Decimal `json:"-"`
	RateText  string          `json:"rate"`
	FetchedAt time.Time       `json:"fetched_at"`
	// Source 记录牌价来源，便于排查换算争议。
	Source string `json:"source"`
}

// vietcombankExrateList 只解析我们用得上的字段。
type vietcombankExrateList struct {
	XMLName xml.Name `xml:"ExrateList"`
	Rates   []struct {
		CurrencyCode string `xml:"CurrencyCode,attr"`
		Sell         string `xml:"Sell,attr"`
	} `xml:"Exrate"`
}

// ExchangeRateService 负责取回、缓存并按配置调整 USD/VND 汇率。
type ExchangeRateService struct {
	settingRepo SettingRepository
	httpClient  *http.Client

	mu     sync.Mutex
	cached *ExchangeRateSnapshot
	// lastFetchFailure / lastFetchErr 记录最近一次抓取失败，供失败退避使用。
	lastFetchFailure time.Time
	lastFetchErr     error
	fetchGroup       singleflight.Group
}

// NewExchangeRateService 构造汇率服务。
func NewExchangeRateService(settingRepo SettingRepository) *ExchangeRateService {
	return &ExchangeRateService{
		settingRepo: settingRepo,
		httpClient:  &http.Client{Timeout: exchangeRateHTTPTimeout},
	}
}

// SetHTTPClient 供测试注入替身。
func (s *ExchangeRateService) SetHTTPClient(client *http.Client) {
	if client != nil {
		s.httpClient = client
	}
}

// EffectiveRate 返回实际用于换算的汇率，即牌价叠加安全边际之后的值。
//
// 取值顺序：内存缓存未过期就直接用；否则抓取上游；抓取失败则回落到库里的
// 快照，但只有在快照还没超过配置的最长可用时长时才允许——用一个不知道多旧的
// 价格收钱，比暂时不让下单更糟。
func (s *ExchangeRateService) EffectiveRate(ctx context.Context) (decimal.Decimal, *ExchangeRateSnapshot, error) {
	snapshot, err := s.snapshot(ctx)
	if err != nil {
		return decimal.Zero, nil, err
	}

	markup := s.markupPercent(ctx)
	effective := snapshot.Rate
	if markup.IsPositive() {
		effective = effective.Mul(decimal.NewFromInt(1).Add(markup.Div(decimal.NewFromInt(100))))
	}
	return effective, snapshot, nil
}

func (s *ExchangeRateService) snapshot(ctx context.Context) (*ExchangeRateSnapshot, error) {
	s.mu.Lock()
	cached := s.cached
	s.mu.Unlock()
	if cached != nil && time.Since(cached.FetchedAt) < exchangeRateRefreshInterval {
		return cached, nil
	}

	fetched, fetchErr := s.refresh(ctx)
	if fetchErr == nil {
		return fetched, nil
	}

	stored := s.loadStored(ctx)
	if stored == nil {
		return nil, infraerrors.ServiceUnavailable("EXCHANGE_RATE_UNAVAILABLE",
			"exchange rate is unavailable and no cached rate exists; try again shortly").
			WithMetadata(map[string]string{"detail": fetchErr.Error()})
	}

	maxAge := s.maxAge(ctx)
	age := time.Since(stored.FetchedAt)
	if age > maxAge {
		slog.Error("exchange rate cache is too old to charge against",
			"age", age.String(), "max_age", maxAge.String(), "error", fetchErr)
		return nil, infraerrors.ServiceUnavailable("EXCHANGE_RATE_STALE",
			"the cached exchange rate is too old to price a payment; try again once the rate provider recovers").
			WithMetadata(map[string]string{
				"age_hours":     fmt.Sprintf("%.1f", age.Hours()),
				"max_age_hours": fmt.Sprintf("%.0f", maxAge.Hours()),
			})
	}

	slog.Warn("exchange rate fetch failed, using cached rate",
		"age", age.String(), "error", fetchErr)
	s.mu.Lock()
	s.cached = stored
	s.mu.Unlock()
	return stored, nil
}

// refresh 合并并发抓取，并在上次失败后的退避期内直接返回那次的错误。
// 抓取不继承调用方的取消：客户端断开不能被记成源站故障；抓取加落库仍有总时限。
func (s *ExchangeRateService) refresh(ctx context.Context) (*ExchangeRateSnapshot, error) {
	v, err, _ := s.fetchGroup.Do("usd_vnd", func() (any, error) {
		s.mu.Lock()
		cached, failedAt, failErr := s.cached, s.lastFetchFailure, s.lastFetchErr
		s.mu.Unlock()
		// 上一轮合并抓取可能刚刚结束。
		if cached != nil && time.Since(cached.FetchedAt) < exchangeRateRefreshInterval {
			return cached, nil
		}
		if failErr != nil && time.Since(failedAt) < exchangeRateFailureBackoff {
			return nil, failErr
		}

		fetchCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*exchangeRateHTTPTimeout)
		defer cancel()
		fetched, err := s.fetchUpstream(fetchCtx)
		if err != nil {
			s.mu.Lock()
			s.lastFetchFailure, s.lastFetchErr = time.Now(), err
			s.mu.Unlock()
			return nil, err
		}
		s.store(fetchCtx, fetched)
		return fetched, nil
	})
	if err != nil {
		return nil, err
	}
	snapshot, _ := v.(*ExchangeRateSnapshot)
	return snapshot, nil
}

func (s *ExchangeRateService) fetchUpstream(ctx context.Context) (*ExchangeRateSnapshot, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, vietcombankExchangeRateURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build exchange rate request: %w", err)
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch exchange rate: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("exchange rate provider returned status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read exchange rate response: %w", err)
	}

	rate, err := parseVietcombankUSDSellRate(body)
	if err != nil {
		return nil, err
	}
	return &ExchangeRateSnapshot{
		Rate:      rate,
		RateText:  rate.String(),
		FetchedAt: time.Now().UTC(),
		Source:    "vietcombank",
	}, nil
}

// parseVietcombankUSDSellRate 取出 USD 行的卖出价。
//
// 牌价里的数字带千分位逗号（"26,260.00"），直接交给 decimal 会解析失败。
func parseVietcombankUSDSellRate(body []byte) (decimal.Decimal, error) {
	var list vietcombankExrateList
	if err := xml.Unmarshal(body, &list); err != nil {
		return decimal.Zero, fmt.Errorf("decode exchange rate xml: %w", err)
	}
	for _, entry := range list.Rates {
		if !strings.EqualFold(strings.TrimSpace(entry.CurrencyCode), "USD") {
			continue
		}
		raw := strings.ReplaceAll(strings.TrimSpace(entry.Sell), ",", "")
		rate, err := decimal.NewFromString(raw)
		if err != nil {
			return decimal.Zero, fmt.Errorf("parse USD sell rate %q: %w", entry.Sell, err)
		}
		if !rate.IsPositive() {
			return decimal.Zero, errors.New("USD sell rate must be positive")
		}
		return rate, nil
	}
	return decimal.Zero, errors.New("exchange rate response has no USD entry")
}

func (s *ExchangeRateService) store(ctx context.Context, snapshot *ExchangeRateSnapshot) {
	s.mu.Lock()
	s.cached = snapshot
	s.mu.Unlock()

	if s.settingRepo == nil {
		return
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		slog.Warn("encode exchange rate snapshot failed", "error", err)
		return
	}
	if err := s.settingRepo.Set(ctx, SettingExchangeRateCache, string(encoded)); err != nil {
		// 落库失败不该挡下单：内存里已经有值，重启后再抓一次就是了。
		slog.Warn("persist exchange rate snapshot failed", "error", err)
	}
}

func (s *ExchangeRateService) loadStored(ctx context.Context) *ExchangeRateSnapshot {
	if s.settingRepo == nil {
		return nil
	}
	raw, err := s.settingRepo.GetValue(ctx, SettingExchangeRateCache)
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil
	}
	var snapshot ExchangeRateSnapshot
	if err := json.Unmarshal([]byte(raw), &snapshot); err != nil {
		slog.Warn("stored exchange rate snapshot is unreadable", "error", err)
		return nil
	}
	rate, err := decimal.NewFromString(strings.TrimSpace(snapshot.RateText))
	if err != nil || !rate.IsPositive() {
		slog.Warn("stored exchange rate is not a positive number", "rate", snapshot.RateText)
		return nil
	}
	snapshot.Rate = rate
	return &snapshot
}

func (s *ExchangeRateService) markupPercent(ctx context.Context) decimal.Decimal {
	if s.settingRepo == nil {
		return decimal.Zero
	}
	raw, err := s.settingRepo.GetValue(ctx, SettingExchangeRateMarkupPercent)
	if err != nil {
		return decimal.Zero
	}
	markup, err := decimal.NewFromString(strings.TrimSpace(raw))
	if err != nil || markup.IsNegative() {
		return decimal.Zero
	}
	return markup
}

func (s *ExchangeRateService) maxAge(ctx context.Context) time.Duration {
	hours := defaultExchangeRateMaxAgeHours
	if s.settingRepo != nil {
		if raw, err := s.settingRepo.GetValue(ctx, SettingExchangeRateMaxAgeHours); err == nil {
			if parsed := pcParseInt(strings.TrimSpace(raw), 0); parsed > 0 {
				hours = parsed
			}
		}
	}
	return time.Duration(hours) * time.Hour
}
