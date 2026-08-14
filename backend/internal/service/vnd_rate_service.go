package service

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/util/urlvalidator"
)

// Vietcombank publishes its board rates as XML and states the polling contract in
// a comment at the top of every response: "Only one request every 5 minutes!".
// The floor below is what keeps a misconfigured interval from hammering them.
const (
	vndRateMinIntervalMinutes = 5
	vndRateFetchTimeout       = 15 * time.Second
	vndRateMaxBodyBytes       = 1 << 20 // 1 MiB; the real document is ~2.5 KB
)

// VNDRateColumn selects which side of the bank's board is authoritative.
const (
	VNDRateColumnBuy      = "buy"
	VNDRateColumnTransfer = "transfer"
	VNDRateColumnSell     = "sell"
)

// vcbExrateList mirrors the Vietcombank pXML.aspx document.
type vcbExrateList struct {
	XMLName  xml.Name    `xml:"ExrateList"`
	DateTime string      `xml:"DateTime"`
	Rates    []vcbExrate `xml:"Exrate"`
}

type vcbExrate struct {
	CurrencyCode string `xml:"CurrencyCode,attr"`
	CurrencyName string `xml:"CurrencyName,attr"`
	Buy          string `xml:"Buy,attr"`
	Transfer     string `xml:"Transfer,attr"`
	Sell         string `xml:"Sell,attr"`
}

// VNDRateService keeps SUBSCRIPTION_USD_TO_VND_RATE in step with a bank's
// published board rate.
//
// It owns the setting while enabled: an administrator editing the field in the
// console will see the next tick overwrite it. That is deliberate — a rate that
// is half automatic and half manual is worse than either, because nobody can
// tell which number priced a given order.
type VNDRateService struct {
	cfg         *config.Config
	settingRepo SettingRepository
	httpClient  *http.Client

	stopCh chan struct{}
	wg     sync.WaitGroup
	once   sync.Once
}

func NewVNDRateService(cfg *config.Config, settingRepo SettingRepository) *VNDRateService {
	return &VNDRateService{
		cfg:         cfg,
		settingRepo: settingRepo,
		httpClient:  &http.Client{Timeout: vndRateFetchTimeout},
		stopCh:      make(chan struct{}),
	}
}

func (s *VNDRateService) enabled() bool {
	return s != nil && s.cfg != nil && s.cfg.VNDRate.Enabled && strings.TrimSpace(s.cfg.VNDRate.URL) != ""
}

// interval clamps the configured poll interval to the bank's stated minimum.
func (s *VNDRateService) interval() time.Duration {
	minutes := s.cfg.VNDRate.IntervalMinutes
	if minutes < vndRateMinIntervalMinutes {
		minutes = vndRateMinIntervalMinutes
	}
	return time.Duration(minutes) * time.Minute
}

// Start performs one sync and then schedules the polling loop. A failed first
// sync is logged, not fatal: the previously stored rate stays in force, which is
// the safe outcome — orders keep pricing off the last known-good number instead
// of falling back to 0 (conversion off) and charging plan prices as-is.
func (s *VNDRateService) Start(ctx context.Context) error {
	if !s.enabled() {
		logger.LegacyPrintf("service.vndrate", "%s", "[VNDRate] Auto sync disabled")
		return nil
	}

	if _, err := s.SyncOnce(ctx); err != nil {
		logger.LegacyPrintf("service.vndrate", "[VNDRate] Initial sync failed, keeping stored rate: %v", err)
	}

	every := s.interval()
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(every)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if _, err := s.SyncOnce(context.Background()); err != nil {
					logger.LegacyPrintf("service.vndrate", "[VNDRate] Sync failed, keeping stored rate: %v", err)
				}
			case <-s.stopCh:
				return
			}
		}
	}()

	logger.LegacyPrintf("service.vndrate", "[VNDRate] Auto sync started (every %v, column=%s)", every, s.column())
	return nil
}

func (s *VNDRateService) Stop() {
	if s == nil {
		return
	}
	s.once.Do(func() { close(s.stopCh) })
	s.wg.Wait()
}

func (s *VNDRateService) column() string {
	col := strings.ToLower(strings.TrimSpace(s.cfg.VNDRate.Column))
	switch col {
	case VNDRateColumnBuy, VNDRateColumnTransfer, VNDRateColumnSell:
		return col
	default:
		return VNDRateColumnSell
	}
}

// SyncOnce fetches the board, applies the configured margin, and persists the
// result. It returns the rate it stored.
func (s *VNDRateService) SyncOnce(ctx context.Context) (float64, error) {
	if !s.enabled() {
		return 0, fmt.Errorf("vnd rate sync disabled")
	}

	body, err := s.fetch(ctx)
	if err != nil {
		return 0, err
	}

	currency := strings.ToUpper(strings.TrimSpace(s.cfg.VNDRate.Currency))
	if currency == "" {
		currency = "USD"
	}

	rate, err := parseVCBRate(body, currency, s.column())
	if err != nil {
		return 0, err
	}

	rate = applyVNDRateMargin(rate, s.cfg.VNDRate.MarginPercent)
	if rate <= 0 {
		return 0, fmt.Errorf("computed rate is not positive: %v", rate)
	}

	// The dong has no minor unit, so persist a whole number: a fractional rate
	// would only reappear as rounding noise on every order.
	stored := strconv.FormatFloat(rate, 'f', 0, 64)
	if err := s.settingRepo.Set(ctx, SettingSubscriptionUSDToVNDRate, stored); err != nil {
		return 0, fmt.Errorf("persist %s: %w", SettingSubscriptionUSDToVNDRate, err)
	}

	logger.LegacyPrintf("service.vndrate", "[VNDRate] %s/%s %s = %s VND", currency, "VND", s.column(), stored)
	return rate, nil
}

func (s *VNDRateService) fetch(ctx context.Context) ([]byte, error) {
	normalized, err := urlvalidator.ValidateHTTPSURL(s.cfg.VNDRate.URL, urlvalidator.ValidationOptions{
		AllowedHosts:     s.cfg.Security.URLAllowlist.FxHosts,
		RequireAllowlist: true,
		AllowPrivate:     s.cfg.Security.URLAllowlist.AllowPrivateHosts,
	})
	if err != nil {
		return nil, fmt.Errorf("validate fx url: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, normalized, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fx endpoint returned %d", resp.StatusCode)
	}

	return io.ReadAll(io.LimitReader(resp.Body, vndRateMaxBodyBytes))
}

// parseVCBRate pulls one currency/column pair out of the Vietcombank document.
func parseVCBRate(body []byte, currency, column string) (float64, error) {
	var list vcbExrateList
	if err := xml.Unmarshal(body, &list); err != nil {
		return 0, fmt.Errorf("parse fx xml: %w", err)
	}

	currency = strings.ToUpper(strings.TrimSpace(currency))
	for _, entry := range list.Rates {
		if !strings.EqualFold(strings.TrimSpace(entry.CurrencyCode), currency) {
			continue
		}

		var raw string
		switch column {
		case VNDRateColumnBuy:
			raw = entry.Buy
		case VNDRateColumnTransfer:
			raw = entry.Transfer
		default:
			raw = entry.Sell
		}

		value, err := parseVCBAmount(raw)
		if err != nil {
			return 0, fmt.Errorf("%s %s column: %w", currency, column, err)
		}
		if value <= 0 {
			return 0, fmt.Errorf("%s %s column is not positive: %q", currency, column, raw)
		}
		return value, nil
	}

	return 0, fmt.Errorf("currency %s not present in fx document", currency)
}

// parseVCBAmount reads "26,270.00" as 26270. The bank groups thousands with
// commas, which strconv will not accept.
func parseVCBAmount(raw string) (float64, error) {
	cleaned := strings.ReplaceAll(strings.TrimSpace(raw), ",", "")
	if cleaned == "" {
		return 0, fmt.Errorf("empty amount")
	}
	value, err := strconv.ParseFloat(cleaned, 64)
	if err != nil {
		return 0, fmt.Errorf("parse amount %q: %w", raw, err)
	}
	return value, nil
}

// applyVNDRateMargin widens the bank rate by a percentage. Negative margins are
// ignored rather than clamped to zero so a typo cannot quietly undercut the
// board rate.
func applyVNDRateMargin(rate, marginPercent float64) float64 {
	if marginPercent <= 0 {
		return rate
	}
	return rate * (1 + marginPercent/100)
}
