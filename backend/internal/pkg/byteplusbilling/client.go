// Package byteplusbilling reads provider observations without changing funding.
package byteplusbilling

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

const ConfigEnv = "BYTEPLUS_BILLING_CONFIG_FILE"
const maxBody = 2 << 20
const maxPages = 20
const cooldown = 30 * time.Second

// Config is server-only; never return or log it. AccountID binds the signer to
// the owner verified by QueryBalanceAcct before any asset data is exposed.
type Config struct {
	PoolID        string   `json:"pool_id"`
	AccountID     string   `json:"account_id"`
	AccessKey     string   `json:"access_key"`
	SecretKey     string   `json:"secret_key"`
	SessionToken  string   `json:"session_token"`
	Region        string   `json:"region"`
	ResourceTypes []string `json:"resource_types"`
}
type Source struct {
	State       string          `json:"state"`
	ObservedAt  *time.Time      `json:"observed_at"`
	AttemptedAt *time.Time      `json:"attempted_at"`
	Data        json.RawMessage `json:"data"`
}
type Status struct {
	Configured bool   `json:"configured"`
	Setup      string `json:"setup"`
	Balance    Source `json:"balance"`
	Packages   Source `json:"packages"`
	Coupons    Source `json:"coupons"`
}
type entry struct {
	status  Status
	running chan struct{}
	last    time.Time
}
type Client struct {
	configs     map[string]Config
	configError bool
	http        *http.Client
	endpoint    string
	now         func() time.Time
	mu          sync.Mutex
	cache       map[string]*entry
}

func New() *Client {
	c := newClient()
	path := strings.TrimSpace(os.Getenv(ConfigEnv))
	if path == "" {
		return c
	}
	// Bound reads also handle accidentally configured FIFOs/devices safely.
	// #nosec G703 -- The path is an explicit server-operator secret-file setting, never HTTP or account input.
	before, statErr := os.Stat(path)
	if statErr != nil || !before.Mode().IsRegular() {
		c.configError = true
		return c
	}
	// #nosec G703 -- Only the server operator chooses this private credential file; regular-file and permission checks follow.
	f, err := os.Open(path)
	if err != nil {
		c.configError = true
		return c
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		c.configError = true
		return c
	}
	var configs []Config
	dec := json.NewDecoder(io.LimitReader(f, 64<<10))
	dec.DisallowUnknownFields()
	if dec.Decode(&configs) != nil || dec.Decode(new(any)) != io.EOF {
		c.configError = true
		return c
	}
	for _, cfg := range configs {
		if cfg.Region == "" {
			cfg.Region = "ap-southeast-1"
		}
		if len(cfg.ResourceTypes) == 0 {
			cfg.ResourceTypes = []string{"Package", "RSC"}
		}
		if !validConfig(cfg) {
			c.configError = true
			c.configs = map[string]Config{}
			return c
		}
		if _, exists := c.configs[cfg.PoolID]; exists {
			c.configError = true
			c.configs = map[string]Config{}
			return c
		}
		c.configs[cfg.PoolID] = cfg
	}
	return c
}
func newClient() *Client {
	return &Client{configs: map[string]Config{}, cache: map[string]*entry{}, endpoint: "https://open.byteplusapi.com/", now: time.Now,
		http: &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}
func validConfig(c Config) bool {
	if c.PoolID == "" || !regexp.MustCompile(`^[0-9]{1,24}$`).MatchString(c.AccountID) || strings.TrimSpace(c.AccessKey) == "" || strings.TrimSpace(c.SecretKey) == "" || !regexp.MustCompile(`^[a-z0-9-]{1,40}$`).MatchString(c.Region) {
		return false
	}
	if strings.ContainsAny(c.AccessKey+c.SecretKey+c.SessionToken, "\r\n") || len(c.ResourceTypes) > 3 {
		return false
	}
	seen := map[string]bool{}
	for _, kind := range c.ResourceTypes {
		if seen[kind] || (kind != "Package" && kind != "RSC" && kind != "RI") {
			return false
		}
		seen[kind] = true
	}
	return true
}
func emptyStatus() Status {
	return Status{Setup: "missing_credentials", Balance: Source{State: "never_queried"}, Packages: Source{State: "never_queried"}, Coupons: Source{State: "never_queried"}}
}
func clone(s Status) Status {
	s.Balance.Data = bytes.Clone(s.Balance.Data)
	s.Packages.Data = bytes.Clone(s.Packages.Data)
	s.Coupons.Data = bytes.Clone(s.Coupons.Data)
	return s
}
func (c *Client) Read(pool string) Status {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.readLocked(pool)
}
func (c *Client) readLocked(pool string) Status {
	if e := c.cache[pool]; e != nil {
		return clone(e.status)
	}
	s := emptyStatus()
	if c.configError {
		s.Setup = "invalid_configuration"
	} else if _, ok := c.configs[pool]; ok {
		s.Configured = true
		s.Setup = "ready"
	}
	return s
}
func (c *Client) Query(ctx context.Context, pool string) Status {
	c.mu.Lock()
	if _, ok := c.configs[pool]; !ok {
		s := c.readLocked(pool)
		c.mu.Unlock()
		return s
	}
	e := c.cache[pool]
	if e == nil {
		e = &entry{status: c.readLocked(pool)}
		c.cache[pool] = e
	}
	if e.running != nil {
		done := e.running
		c.mu.Unlock()
		select {
		case <-done:
		case <-ctx.Done():
		}
		return c.Read(pool)
	}
	if !e.last.IsZero() && c.now().Sub(e.last) < cooldown {
		s := clone(e.status)
		c.mu.Unlock()
		return s
	}
	e.running = make(chan struct{})
	e.last = c.now()
	previous := clone(e.status)
	cfg := c.configs[pool]
	c.mu.Unlock()
	result := c.query(ctx, cfg, previous)
	c.mu.Lock()
	e.status = result
	close(e.running)
	e.running = nil
	c.mu.Unlock()
	return clone(result)
}
func observed(old Source, data json.RawMessage, state string, now time.Time) Source {
	old.AttemptedAt = &now
	if state != "ok" {
		old.State = state
		return old
	}
	return Source{State: "ok", ObservedAt: &now, AttemptedAt: &now, Data: data}
}
func (c *Client) query(ctx context.Context, cfg Config, s Status) Status {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	now := c.now().UTC()
	raw, state := c.call(ctx, cfg, "QueryBalanceAcct", map[string]any{})
	var balance struct {
		AccountID        json.Number
		AvailableBalance *string
		CashBalance      *string
		CreditLimit      *string
		ArrearsBalance   *string
		FreezeAmount     *string
		Currency         *string
	}
	if state == "ok" {
		if json.Unmarshal(raw, &balance) != nil || string(balance.AccountID) != cfg.AccountID {
			state = "account_mismatch"
		} else if balance.Currency == nil || len(*balance.Currency) != 3 || !decimal(balance.AvailableBalance) || !optionalDecimals(balance.CashBalance, balance.CreditLimit, balance.ArrearsBalance, balance.FreezeAmount) {
			state = "invalid_response"
		}
	}
	if state != "ok" {
		s.Balance = observed(s.Balance, nil, state, now)
		// An unverified billing owner must not disclose any cached account assets.
		if state == "account_mismatch" {
			s.Balance.Data = nil
			s.Balance.ObservedAt = nil
			s.Packages = Source{State: state, AttemptedAt: &now}
			s.Coupons = Source{State: state, AttemptedAt: &now}
		} else {
			s.Packages = observed(s.Packages, nil, "owner_unverified", now)
			s.Coupons = observed(s.Coupons, nil, "owner_unverified", now)
		}
		return s
	}
	data, _ := json.Marshal(balance)
	s.Balance = observed(s.Balance, data, "ok", c.now().UTC())
	data, state = c.packages(ctx, cfg)
	s.Packages = observed(s.Packages, data, state, c.now().UTC())
	data, state = c.coupons(ctx, cfg)
	s.Coupons = observed(s.Coupons, data, state, c.now().UTC())
	return s
}

var decimalPattern = regexp.MustCompile(`^-?[0-9]+(?:\.[0-9]+)?$`)

func decimal(s *string) bool { return s != nil && len(*s) <= 80 && decimalPattern.MatchString(*s) }
func optionalDecimals(values ...*string) bool {
	for _, v := range values {
		if v != nil && !decimal(v) {
			return false
		}
	}
	return true
}
func validTime(s string) bool { _, err := time.Parse(time.RFC3339, s); return err == nil }

type Package struct {
	OwnerID           string
	InstanceNo        string
	AvailableAmount   *string
	TotalAmount       *string
	EffectiveTime     string
	ExpiryTime        string
	Product           string
	ConfigurationCode string
	ConfigurationName string
	RegionCode        string
	Unit              string
	Status            string
	ResourceType      string
}

func (c *Client) packages(ctx context.Context, cfg Config) (json.RawMessage, string) {
	all := []Package{}
	ids := map[string]bool{}
	for _, kind := range cfg.ResourceTypes {
		next := ""
		seen := map[string]bool{}
		for page := 0; ; page++ {
			if page == maxPages {
				return nil, "incomplete"
			}
			input := map[string]any{"ResourceType": kind, "MaxResults": "100"}
			if next != "" {
				input["NextToken"] = next
			}
			raw, state := c.call(ctx, cfg, "ListResourcePackages", input)
			if state != "ok" {
				return nil, state
			}
			var out struct {
				List      *[]Package
				NextToken string
			}
			if json.Unmarshal(raw, &out) != nil || out.List == nil {
				return nil, "invalid_response"
			}
			for _, p := range *out.List {
				if (p.OwnerID != "" && p.OwnerID != cfg.AccountID) || p.InstanceNo == "" || !decimal(p.AvailableAmount) || !optionalDecimals(p.TotalAmount) || p.Unit == "" || !validTime(p.ExpiryTime) || (p.EffectiveTime != "" && !validTime(p.EffectiveTime)) {
					return nil, "invalid_response"
				}
				p.ResourceType = kind
				if ids[p.InstanceNo] {
					return nil, "incomplete"
				}
				ids[p.InstanceNo] = true
				all = append(all, p)
			}
			next = out.NextToken
			if next == "" {
				break
			}
			if seen[next] {
				return nil, "incomplete"
			}
			seen[next] = true
		}
	}
	data, _ := json.Marshal(all)
	return data, "ok"
}

type Coupon struct {
	AmountLimit      *json.Number
	UsageLimit       *int
	CouponID         string
	CouponName       string
	RemainingAmount  *json.Number
	TotalAmount      *json.Number
	BeginTime        string
	ExpiredTime      string
	Status           *int
	OrderTypeLimit   string
	PayTypeLimit     string
	ProductLimitList []CouponRestriction
}
type CouponRestriction struct {
	ChargeItemType    *int
	ConfigurationType *int
	TimeLimitLower    *int
	TimeLimitUpper    *int
	ProductCode       string
	ProductName       string
	ConfigurationCode string
	ConfigurationName string
	BillingMethod     string
	ChargeItemCode    string
}

func (c *Client) coupons(ctx context.Context, cfg Config) (json.RawMessage, string) {
	all := []Coupon{}
	ids := map[string]bool{}
	expected := -1
	for page := 0; page < maxPages; page++ {
		raw, state := c.call(ctx, cfg, "ListCoupons", map[string]any{"Limit": 100, "Offset": len(all)})
		if state != "ok" {
			return nil, state
		}
		var out struct {
			List  *[]Coupon
			Total *int
		}
		if json.Unmarshal(raw, &out) != nil || out.List == nil || out.Total == nil || *out.Total < 0 {
			return nil, "invalid_response"
		}
		if expected >= 0 && expected != *out.Total {
			return nil, "incomplete"
		}
		expected = *out.Total
		for _, p := range *out.List {
			if p.CouponID == "" || p.RemainingAmount == nil || !decimalPattern.MatchString(string(*p.RemainingAmount)) || !validTime(p.ExpiredTime) || (p.BeginTime != "" && !validTime(p.BeginTime)) {
				return nil, "invalid_response"
			}
			if ids[p.CouponID] {
				return nil, "incomplete"
			}
			ids[p.CouponID] = true
			all = append(all, p)
		}
		if len(all) == expected {
			data, _ := json.Marshal(all)
			return data, "ok"
		}
		if len(all) > expected || len(*out.List) == 0 {
			return nil, "incomplete"
		}
	}
	return nil, "incomplete"
}
func (c *Client) call(ctx context.Context, cfg Config, action string, input any) (json.RawMessage, string) {
	body, err := json.Marshal(input)
	if err != nil {
		return nil, "invalid_response"
	}
	target, err := url.Parse(c.endpoint)
	if err != nil {
		return nil, "unavailable"
	}
	q := target.Query()
	q.Set("Action", action)
	q.Set("Version", "2022-01-01")
	target.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target.String(), bytes.NewReader(body))
	if err != nil {
		return nil, "unavailable"
	}
	sign(req, body, cfg, c.now().UTC())
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, "unavailable"
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return nil, "forbidden"
	}
	if resp.StatusCode == 429 {
		return nil, "rate_limited"
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "unavailable"
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil || len(data) > maxBody {
		return nil, "invalid_response"
	}
	var envelope struct {
		ResponseMetadata struct{ Error *json.RawMessage }
		Result           json.RawMessage
	}
	if json.Unmarshal(data, &envelope) != nil || envelope.ResponseMetadata.Error != nil || len(envelope.Result) == 0 || string(envelope.Result) == "null" {
		return nil, "invalid_response"
	}
	return envelope.Result, "ok"
}
func digest(b []byte) string { sum := sha256.Sum256(b); return hex.EncodeToString(sum[:]) }
func mac(key []byte, s string) []byte {
	m := hmac.New(sha256.New, key)
	_, _ = m.Write([]byte(s))
	return m.Sum(nil)
}

// Billing OpenAPI signing follows the official BytePlus Sign4 canonical request.
// Only the fixed root path and these three read actions are used by this client.
func sign(req *http.Request, body []byte, c Config, now time.Time) {
	stamp := now.Format("20060102T150405Z")
	day := stamp[:8]
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("X-Date", stamp)
	req.Header.Set("X-Content-Sha256", digest(body))
	headers := "content-type;host;x-content-sha256;x-date"
	canonicalHeaders := "content-type:" + req.Header.Get("Content-Type") + "\nhost:" + req.URL.Host + "\nx-content-sha256:" + digest(body) + "\nx-date:" + stamp + "\n"
	if c.SessionToken != "" {
		req.Header.Set("X-Security-Token", c.SessionToken)
		headers += ";x-security-token"
		canonicalHeaders += "x-security-token:" + c.SessionToken + "\n"
	}
	canonical := strings.Join([]string{req.Method, "/", strings.ReplaceAll(req.URL.Query().Encode(), "+", "%20"), canonicalHeaders, headers, digest(body)}, "\n")
	scope := day + "/" + c.Region + "/billing/request"
	toSign := "HMAC-SHA256\n" + stamp + "\n" + scope + "\n" + digest([]byte(canonical))
	key := []byte(c.SecretKey)
	for _, v := range []string{day, c.Region, "billing", "request"} {
		key = mac(key, v)
	}
	req.Header.Set("Authorization", fmt.Sprintf("HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s", c.AccessKey, scope, headers, hex.EncodeToString(mac(key, toSign))))
}

// Preserve provider monetary decimal text through JSON into JavaScript.
func (p Coupon) MarshalJSON() ([]byte, error) {
	type plain Coupon
	decimalString := func(n *json.Number) *string {
		if n == nil {
			return nil
		}
		s := string(*n)
		return &s
	}
	return json.Marshal(struct {
		plain
		RemainingAmount *string
		TotalAmount     *string
		AmountLimit     *string
	}{plain(p), decimalString(p.RemainingAmount), decimalString(p.TotalAmount), decimalString(p.AmountLimit)})
}
