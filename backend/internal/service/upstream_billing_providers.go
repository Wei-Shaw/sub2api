package service

// Official cost APIs, not model inference APIs or balance snapshots.
// References (verified 2026-09):
// https://learn.microsoft.com/en-us/rest/api/cost-management/query/usage
// https://learn.microsoft.com/en-us/entra/identity-platform/v2-oauth2-client-creds-grant-flow
// https://cloud.tencent.com/document/api/555/30756
// https://cloud.tencent.com/document/api/555/35761
// https://cloud.tencent.com/document/api/213/30654
// https://platform.claude.com/docs/en/build-with-claude/usage-cost-api
// These APIs report resource/workspace costs, NOT exact model API-key costs.

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

type BillingProviderConfig struct {
	Provider string            `json:"provider"`
	Settings map[string]string `json:"settings"`
	Secrets  map[string]string `json:"secrets"`
}

// ProviderBill is an official cost bucket, in major currency units. Its stable
// ID deliberately excludes the amount so that re-syncing corrected bills replaces
// the old value. PeriodEnd is exclusive. No request/key precision is implied.
type ProviderBill struct {
	ID          string             `json:"id"`
	ResourceID  string             `json:"resource_id"`
	Description string             `json:"description"`
	Amount      string             `json:"amount"`
	Currency    string             `json:"currency"`
	PeriodStart time.Time          `json:"period_start"`
	PeriodEnd   time.Time          `json:"period_end"`
	RawSource   json.RawMessage    `json:"raw_source,omitempty"`
	Usage       *ProviderBillUsage `json:"usage,omitempty"`
	UsageStatus string             `json:"usage_status,omitempty"`
}

// ProviderBillUsage is a comparable provider-observed denominator for exactly
// ONE cost category, not the sum of differently-priced token types. Complete
// means the matching provider usage report was fully read; it does not certify
// that all provider traffic flowed through this sub2api instance.
type ProviderBillUsage struct {
	Model         string `json:"model"`
	TokenType     string `json:"token_type"`
	Tokens        int64  `json:"tokens"`
	ContextWindow string `json:"context_window"`
	ServiceTier   string `json:"service_tier"`
	InferenceGeo  string `json:"inference_geo"`
	Complete      bool   `json:"complete"`
}

const (
	billingProviderMaxPages    = 100
	billingProviderMaxBills    = 30000
	billingProviderMaxBody     = 8 << 20
	billingProviderRetries     = 3
	billingProviderMaxRawRow   = 64 << 10
	billingProviderMaxRawTotal = 32 << 20
)

var (
	errBillingProviderNoContent = errors.New("provider returned no billing content")
	billingUUIDPattern          = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	billingAzureResource        = regexp.MustCompile(`(?i)^/subscriptions/([0-9a-f-]{36})/resourcegroups/[a-z0-9_.()-]+/providers/microsoft\.cognitiveservices/accounts/[a-z0-9_-]+$`)
	billingBusinessPattern      = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
	billingWorkspacePattern     = regexp.MustCompile(`^(wrkspc_[A-Za-z0-9]+|__organization__)$`)
	billingAmountPattern        = regexp.MustCompile(`^-?[0-9]{1,24}(\.[0-9]{1,24})?([eE][+-]?[0-9]{1,2})?$`)
	billingCurrencyPattern      = regexp.MustCompile(`^[A-Z]{3}$`)
)

// ValidateBillingProviderConfig rejects unknown fields, including secrets
// accidentally placed in Settings (which are intentionally stored unencrypted).
// It validates complete credentials; callers merging saved credentials must do
// that before validation. Error strings never contain field values.
func ValidateBillingProviderConfig(cfg BillingProviderConfig) error {
	var settings, secrets, requiredSettings []string
	switch cfg.Provider {
	case "azure":
		settings = []string{"tenant_id", "client_id", "subscription_id", "resource_id"}
		requiredSettings = settings
		secrets = []string{"client_secret"}
	case "tencent":
		settings = []string{"business_code", "region"}
		requiredSettings = []string{"business_code"}
		secrets = []string{"secret_id", "secret_key"}
	case "anthropic":
		settings = []string{"workspace_id"}
		secrets = []string{"admin_api_key"}
	case "aliyun", "volcengine":
		settings = []string{"account_id", "product_code"}
		requiredSettings = settings
		secrets = []string{"access_key_id", "access_key_secret"}
	default:
		return errors.New("unsupported billing provider")
	}
	if err := billingCheckFields(cfg.Settings, settings, requiredSettings, false); err != nil {
		return err
	}
	if err := billingCheckFields(cfg.Secrets, secrets, secrets, true); err != nil {
		return err
	}
	switch cfg.Provider {
	case "azure":
		for _, name := range []string{"tenant_id", "client_id", "subscription_id"} {
			if !billingUUIDPattern.MatchString(cfg.Settings[name]) {
				return fmt.Errorf("invalid Azure %s", name)
			}
		}
		match := billingAzureResource.FindStringSubmatch(cfg.Settings["resource_id"])
		if len(match) != 2 || !strings.EqualFold(match[1], cfg.Settings["subscription_id"]) {
			return errors.New("Azure resource_id must identify a Cognitive Services account in the configured subscription")
		}
	case "tencent":
		if !billingBusinessPattern.MatchString(cfg.Settings["business_code"]) {
			return errors.New("invalid Tencent business_code; use the code from DescribeBillSummaryByProduct")
		}
		// International billing does not return a currency on BillDetail. Do not
		// guess currency or mix its different billing rules with domestic CNY.
		if region := cfg.Settings["region"]; region != "" && region != "domestic" {
			return errors.New("Tencent billing currently supports the domestic CNY account only")
		}
		if !billingBusinessPattern.MatchString(cfg.Secrets["secret_id"]) {
			return errors.New("invalid Tencent secret_id")
		}
	case "anthropic":
		if workspace := cfg.Settings["workspace_id"]; workspace != "" && !billingWorkspacePattern.MatchString(workspace) {
			return errors.New("invalid Anthropic workspace_id")
		}
	case "aliyun", "volcengine":
		accountID, err := strconv.ParseInt(cfg.Settings["account_id"], 10, 64)
		if err != nil || accountID <= 0 || strconv.FormatInt(accountID, 10) != cfg.Settings["account_id"] {
			return errors.New("billing account_id must be the resource-owning cloud account's positive numeric ID")
		}
		if !billingBusinessPattern.MatchString(cfg.Settings["product_code"]) {
			return errors.New("invalid billing product_code; copy the code from the cloud billing console")
		}
		if !billingBusinessPattern.MatchString(cfg.Secrets["access_key_id"]) {
			return errors.New("invalid billing access_key_id")
		}
	}
	return nil
}

func billingCheckFields(values map[string]string, allowed, required []string, secret bool) error {
	known := make(map[string]bool, len(allowed))
	for _, key := range allowed {
		known[key] = true
	}
	for key, value := range values {
		if !known[key] {
			return errors.New("unrecognized billing configuration field")
		}
		if len(value) > 4096 || strings.ContainsAny(value, "\r\n\x00") || (!secret && value != strings.TrimSpace(value)) {
			return fmt.Errorf("invalid billing configuration field: %s", key)
		}
	}
	for _, key := range required {
		if strings.TrimSpace(values[key]) == "" {
			return fmt.Errorf("required billing configuration field: %s", key)
		}
	}
	return nil
}

// FetchProviderBills is restricted to fixed official HTTPS endpoints. Redirects
// are disabled so credentials cannot be forwarded to another host. A failed or
// incomplete page aborts the whole fetch; callers must never commit partial data.
func FetchProviderBills(ctx context.Context, cfg BillingProviderConfig, start, end time.Time) ([]ProviderBill, error) {
	client := &http.Client{
		Timeout: 40 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return fetchProviderBillsWithClient(ctx, client, cfg, start, end)
}

func fetchProviderBillsWithClient(ctx context.Context, client *http.Client, cfg BillingProviderConfig, start, end time.Time) ([]ProviderBill, error) {
	if err := ValidateBillingProviderConfig(cfg); err != nil {
		return nil, err
	}
	if start.IsZero() || !start.Before(end) || end.Sub(start) > 32*24*time.Hour {
		return nil, errors.New("billing date range must be positive and no longer than one month")
	}
	ctx, cancel := context.WithTimeout(ctx, 4*time.Minute)
	defer cancel()
	var bills []ProviderBill
	var err error
	switch cfg.Provider {
	case "azure":
		bills, err = fetchAzureProviderBills(ctx, client, cfg, start.UTC(), end.UTC())
	case "tencent":
		bills, err = fetchTencentProviderBills(ctx, client, cfg, start, end)
	case "anthropic":
		bills, err = fetchAnthropicProviderBills(ctx, client, cfg, start.UTC(), end.UTC())
	case "aliyun":
		bills, err = fetchAliyunProviderBills(ctx, client, cfg, start, end)
	case "volcengine":
		bills, err = fetchVolcengineProviderBills(ctx, client, cfg, start, end)
	}
	if err != nil {
		return nil, err
	}
	// Even repeated pages must not silently double-charge a bucket. Providers
	// returning conflicting duplicates need a later complete re-sync.
	seen := make(map[string]ProviderBill, len(bills))
	for _, bill := range bills {
		if previous, ok := seen[bill.ID]; ok {
			if !reflect.DeepEqual(previous, bill) {
				return nil, errors.New("provider returned conflicting duplicate billing records")
			}
			continue
		}
		seen[bill.ID] = bill
	}
	bills = make([]ProviderBill, 0, len(seen))
	for _, bill := range seen {
		bills = append(bills, bill)
	}
	sort.Slice(bills, func(i, j int) bool { return bills[i].ID < bills[j].ID })
	return bills, nil
}

// billingProviderRequest does not include transport errors, response bodies,
// URL query strings, or provider error messages in returned errors.
func billingProviderRequest(ctx context.Context, client *http.Client, makeRequest func() (*http.Request, error), output any) error {
	for attempt := 0; attempt < billingProviderRetries; attempt++ {
		req, err := makeRequest()
		if err != nil {
			return errors.New("cannot create provider billing request")
		}
		res, err := client.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return errors.New("provider billing request cancelled or timed out")
			}
			return errors.New("provider billing connection failed")
		}
		body, readErr := io.ReadAll(io.LimitReader(res.Body, billingProviderMaxBody+1))
		_ = res.Body.Close()
		if readErr != nil || len(body) > billingProviderMaxBody {
			return errors.New("invalid or oversized provider billing response")
		}
		transient := res.StatusCode == http.StatusTooManyRequests || res.StatusCode >= 500 && res.StatusCode <= 504
		// Tencent API 3.0 commonly reports throttling inside an HTTP 200
		// envelope. Inspect only the code; never surface its Message content.
		if req.URL.Host == "billing.tencentcloudapi.com" && res.StatusCode == http.StatusOK {
			var envelope struct {
				Response struct {
					Error *struct {
						Code string `json:"Code"`
					} `json:"Error"`
				} `json:"Response"`
			}
			if json.Unmarshal(body, &envelope) == nil && envelope.Response.Error != nil {
				code := envelope.Response.Error.Code
				transient = strings.HasPrefix(code, "RequestLimitExceeded") || strings.HasPrefix(code, "InternalError")
			}
		}
		if res.StatusCode == http.StatusOK && billingDomesticTransientError(req.URL.Host, body) {
			transient = true
		}
		if transient {
			if attempt+1 == billingProviderRetries {
				if res.StatusCode == http.StatusOK {
					return errors.New("provider billing temporarily unavailable; retry later")
				}
				return fmt.Errorf("provider billing temporarily unavailable (HTTP %d)", res.StatusCode)
			}
			delay := time.Duration(attempt+1) * time.Second
			for _, header := range []string{"Retry-After", "x-ms-ratelimit-microsoft.costmanagement-qpu-retry-after", "x-ms-ratelimit-microsoft.costmanagement-clienttype-retry-after", "x-ms-ratelimit-microsoft.costmanagement-tenant-retry-after"} {
				if retry := billingRetryDelay(res.Header.Get(header), time.Now()); retry > delay {
					delay = retry
				}
			}
			// Honor long Retry-After by deferring to the next scheduled sync, not
			// retrying sooner than the provider instructed.
			if delay > 30*time.Second {
				return errors.New("provider billing rate limited; retry on the next sync")
			}
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return errors.New("provider billing request cancelled or timed out")
			case <-timer.C:
			}
			continue
		}
		if res.StatusCode < 200 || res.StatusCode >= 300 {
			return fmt.Errorf("provider billing request failed (HTTP %d); check read-only billing permissions", res.StatusCode)
		}
		if res.StatusCode == http.StatusNoContent {
			return errBillingProviderNoContent
		}
		decoder := json.NewDecoder(bytes.NewReader(body))
		decoder.UseNumber()
		if err := decoder.Decode(output); err != nil {
			return errors.New("invalid provider billing JSON response")
		}
		if decoder.Decode(new(any)) != io.EOF {
			return errors.New("invalid provider billing JSON response")
		}
		return nil
	}
	return errors.New("provider billing retry limit exceeded")
}

func billingRetryDelay(value string, now time.Time) time.Duration {
	if seconds, err := strconv.ParseInt(value, 10, 32); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	if deadline, err := http.ParseTime(value); err == nil && deadline.After(now) {
		return deadline.Sub(now)
	}
	return 0
}

func billingJSONRequest(ctx context.Context, method, endpoint string, body []byte, headers map[string]string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	return req, nil
}

func fetchAzureProviderBills(ctx context.Context, client *http.Client, cfg BillingProviderConfig, start, end time.Time) ([]ProviderBill, error) {
	form := url.Values{
		"grant_type": {"client_credentials"}, "client_id": {cfg.Settings["client_id"]},
		"client_secret": {cfg.Secrets["client_secret"]}, "scope": {"https://management.azure.com/.default"},
	}
	var token struct {
		AccessToken string `json:"access_token"`
	}
	err := billingProviderRequest(ctx, client, func() (*http.Request, error) {
		return billingJSONRequest(ctx, http.MethodPost, "https://login.microsoftonline.com/"+cfg.Settings["tenant_id"]+"/oauth2/v2.0/token", []byte(form.Encode()), map[string]string{"Content-Type": "application/x-www-form-urlencoded"})
	}, &token)
	if err != nil {
		return nil, err
	}
	if token.AccessToken == "" || strings.ContainsAny(token.AccessToken, "\r\n") {
		return nil, errors.New("Azure billing authorization failed")
	}
	resource := strings.ToLower(cfg.Settings["resource_id"])
	queryEnd := end
	if now := time.Now().UTC(); queryEnd.After(now) {
		queryEnd = now
	}
	if !start.Before(queryEnd) {
		return []ProviderBill{}, nil
	}
	// Cost Management timePeriod.to is inclusive; our public interval is not.
	query, _ := json.Marshal(map[string]any{
		"type": "Usage", "timeframe": "Custom",
		"timePeriod": map[string]string{"from": start.Format(time.RFC3339), "to": queryEnd.Add(-time.Second).Format(time.RFC3339)},
		"dataset": map[string]any{
			"granularity": "Daily",
			"aggregation": map[string]any{"totalCost": map[string]string{"name": "PreTaxCost", "function": "Sum"}},
			"grouping":    []map[string]string{{"type": "Dimension", "name": "ResourceId"}},
			"filter":      map[string]any{"dimensions": map[string]any{"name": "ResourceId", "operator": "In", "values": []string{resource}}},
		},
	})
	path := "/subscriptions/" + cfg.Settings["subscription_id"] + "/providers/Microsoft.CostManagement/query"
	endpoint := "https://management.azure.com" + path + "?api-version=2025-03-01"
	seenPages := map[string]bool{}
	bills := make([]ProviderBill, 0)
	rawBytes := 0
	for page := 0; page < billingProviderMaxPages; page++ {
		if !billingAzurePageAllowed(endpoint, path) || seenPages[endpoint] {
			return nil, errors.New("invalid or repeated Azure billing pagination link")
		}
		seenPages[endpoint] = true
		var response struct {
			Properties *struct {
				Columns  json.RawMessage      `json:"columns"`
				Rows     *[][]json.RawMessage `json:"rows"`
				NextLink string               `json:"nextLink"`
			} `json:"properties"`
		}
		err = billingProviderRequest(ctx, client, func() (*http.Request, error) {
			return billingJSONRequest(ctx, http.MethodPost, endpoint, query, map[string]string{"Authorization": "Bearer " + token.AccessToken})
		}, &response)
		if errors.Is(err, errBillingProviderNoContent) {
			return bills, nil
		}
		if err != nil {
			return nil, err
		}
		if response.Properties == nil || response.Properties.Rows == nil {
			return nil, errors.New("invalid Azure billing response")
		}
		columns := make(map[string]int)
		var columnList []struct {
			Name string `json:"name"`
		}
		if len(*response.Properties.Rows) > 0 && json.Unmarshal(response.Properties.Columns, &columnList) != nil {
			return nil, errors.New("invalid Azure billing columns")
		}
		for i, column := range columnList {
			columns[strings.ToLower(column.Name)] = i
		}
		for _, row := range *response.Properties.Rows {
			value := func(name string) string {
				i, ok := columns[name]
				if !ok || i >= len(row) {
					return ""
				}
				var text string
				if json.Unmarshal(row[i], &text) == nil {
					return text
				}
				return string(row[i])
			}
			if !strings.EqualFold(value("resourceid"), resource) {
				return nil, errors.New("Azure billing response contains an unexpected resource")
			}
			day, parseErr := time.Parse("20060102", value("usagedate"))
			if parseErr != nil || day.Before(start) || !day.Before(end) {
				return nil, errors.New("Azure billing response contains an invalid usage date")
			}
			amount, parseErr := billingParseAmount(value("pretaxcost"))
			currency := strings.ToUpper(value("currency"))
			if parseErr != nil || !billingCurrencyPattern.MatchString(currency) {
				return nil, errors.New("invalid Azure billing amount or currency")
			}
			raw, rawErr := billingRawCostSource(map[string]any{"columns": response.Properties.Columns, "row": row})
			if rawErr != nil {
				return nil, rawErr
			}
			rawBytes += len(raw)
			if rawBytes > billingProviderMaxRawTotal {
				return nil, errors.New("provider original billing snapshot exceeds 32 MiB")
			}
			bills = append(bills, ProviderBill{
				ID: billingBucketID("azure", resource, day.Format("2006-01-02"), currency), ResourceID: resource,
				Description: "Azure resource daily cost (pre-tax)", Amount: amount.String(), Currency: currency,
				PeriodStart: day, PeriodEnd: day.AddDate(0, 0, 1),
				RawSource: raw, UsageStatus: "unsupported",
			})
			if len(bills) > billingProviderMaxBills {
				return nil, errors.New("provider billing record limit exceeded")
			}
		}
		endpoint = response.Properties.NextLink
		if endpoint == "" {
			return bills, nil
		}
	}
	return nil, errors.New("Azure billing pagination limit exceeded")
}

func billingAzurePageAllowed(endpoint, path string) bool {
	u, err := url.Parse(endpoint)
	return err == nil && u.Scheme == "https" && u.Host == "management.azure.com" && u.User == nil && u.Fragment == "" && strings.EqualFold(u.EscapedPath(), path) && u.RawPath == ""
}

type billingTencentDetail struct {
	ID               string `json:"Id"`
	BillID           string `json:"BillId"`
	BusinessCode     string `json:"BusinessCode"`
	BusinessCodeName string `json:"BusinessCodeName"`
	ProductCode      string `json:"ProductCode"`
	ResourceID       string `json:"ResourceId"`
	PayTime          string `json:"PayTime"`
	FeeBeginTime     string `json:"FeeBeginTime"`
	FeeEndTime       string `json:"FeeEndTime"`
	ComponentSet     []struct {
		ItemCode      string `json:"ItemCode"`
		ComponentCode string `json:"ComponentCode"`
		RealCost      string `json:"RealCost"`
	} `json:"ComponentSet"`
}

type billingAnthropicCostResult struct {
	Amount        string  `json:"amount"`
	Currency      string  `json:"currency"`
	Description   string  `json:"description"`
	WorkspaceID   *string `json:"workspace_id"`
	Model         string  `json:"model"`
	CostType      string  `json:"cost_type"`
	TokenType     string  `json:"token_type"`
	ContextWindow string  `json:"context_window"`
	ServiceTier   string  `json:"service_tier"`
	InferenceGeo  string  `json:"inference_geo"`
}

func fetchTencentProviderBills(ctx context.Context, client *http.Client, cfg BillingProviderConfig, start, end time.Time) ([]ProviderBill, error) {
	// Domestic billing is a calendar month in China Standard Time. The API's
	// BeginTime/EndTime do NOT filter individual days: its contract says entire
	// month data is returned. Reject partial months instead of misattributing it.
	zone := time.FixedZone("Asia/Shanghai", 8*60*60)
	localStart := start.In(zone)
	monthStart := time.Date(localStart.Year(), localStart.Month(), 1, 0, 0, 0, 0, zone)
	if !start.Equal(monthStart) || !end.Equal(monthStart.AddDate(0, 1, 0)) {
		return nil, errors.New("Tencent billing requires a complete calendar month in Asia/Shanghai")
	}
	var bills []ProviderBill
	continuation := ""
	rawBytes := 0
	for page := 0; page < billingProviderMaxPages; page++ {
		payload, _ := json.Marshal(map[string]any{
			"Month": monthStart.Format("2006-01"), "Offset": page * 300, "Limit": 300,
			"BusinessCode": cfg.Settings["business_code"], "NeedRecordNum": 0, "Context": continuation,
		})
		var response struct {
			Response *struct {
				Error *struct {
					Code string `json:"Code"`
				} `json:"Error"`
				Context   string             `json:"Context"`
				DetailSet *[]json.RawMessage `json:"DetailSet"`
			} `json:"Response"`
		}
		err := billingProviderRequest(ctx, client, func() (*http.Request, error) {
			req, err := billingJSONRequest(ctx, http.MethodPost, "https://billing.tencentcloudapi.com/", payload, nil)
			if err == nil {
				billingSignTencent(req, payload, cfg.Secrets["secret_id"], cfg.Secrets["secret_key"], time.Now())
			}
			return req, err
		}, &response)
		if err != nil {
			return nil, err
		}
		if response.Response == nil {
			return nil, errors.New("invalid Tencent billing response")
		}
		if response.Response.Error != nil {
			return nil, errors.New("Tencent billing API rejected the request; check permissions, month availability and product code")
		}
		if response.Response.DetailSet == nil {
			return nil, errors.New("invalid Tencent billing detail list")
		}
		for _, sourceRow := range *response.Response.DetailSet {
			raw, rawErr := billingRawCostSource(sourceRow)
			if rawErr != nil {
				return nil, rawErr
			}
			rawBytes += len(raw)
			if rawBytes > billingProviderMaxRawTotal {
				return nil, errors.New("provider original billing snapshot exceeds 32 MiB")
			}
			var row billingTencentDetail
			if json.Unmarshal(sourceRow, &row) != nil {
				return nil, errors.New("invalid Tencent billing row")
			}
			if row.BusinessCode != cfg.Settings["business_code"] {
				return nil, errors.New("Tencent billing response contains an unexpected product")
			}
			if len(row.ComponentSet) == 0 {
				return nil, errors.New("Tencent bill is missing component costs")
			}
			amount := decimal.Zero
			components := make([]string, 0, len(row.ComponentSet))
			for _, component := range row.ComponentSet {
				value, parseErr := billingParseAmount(component.RealCost)
				if parseErr != nil {
					return nil, errors.New("invalid Tencent billing amount")
				}
				amount = amount.Add(value)
				components = append(components, component.ItemCode+":"+component.ComponentCode)
			}
			sort.Strings(components)
			identity := row.ID
			if identity == "" {
				if row.BillID == "" {
					return nil, errors.New("Tencent bill is missing a stable billing identifier")
				}
				identity = billingBucketID(row.BillID, row.ResourceID, row.ProductCode, row.PayTime, row.FeeBeginTime, row.FeeEndTime, strings.Join(components, ","))
			}
			bills = append(bills, ProviderBill{
				ID:         billingBucketID("tencent", monthStart.Format("2006-01"), cfg.Settings["business_code"], identity),
				ResourceID: row.ResourceID, Description: "Tencent domestic discounted cost: " + billingDescription(row.BusinessCodeName),
				Amount: amount.String(), Currency: "CNY", PeriodStart: start.UTC(), PeriodEnd: end.UTC(),
				RawSource: raw, UsageStatus: "unsupported",
			})
			if len(bills) > billingProviderMaxBills {
				return nil, errors.New("provider billing record limit exceeded")
			}
		}
		// Total is cached for 24h and may be stale; only the page length is a
		// reliable termination condition, not Offset >= Total.
		if len(*response.Response.DetailSet) < 300 {
			return bills, nil
		}
		continuation = response.Response.Context
	}
	return nil, errors.New("Tencent billing pagination limit exceeded")
}

func billingSignTencent(req *http.Request, payload []byte, secretID, secretKey string, now time.Time) {
	const contentType = "application/json; charset=utf-8"
	const action = "DescribeBillDetail"
	const signedHeaders = "content-type;host;x-tc-action"
	date := now.UTC().Format("2006-01-02")
	timestamp := strconv.FormatInt(now.Unix(), 10)
	scope := date + "/billing/tc3_request"
	canonical := "POST\n/\n\ncontent-type:" + contentType + "\nhost:" + req.URL.Host + "\nx-tc-action:" + strings.ToLower(action) + "\n\n" + signedHeaders + "\n" + billingSHA256(payload)
	toSign := "TC3-HMAC-SHA256\n" + timestamp + "\n" + scope + "\n" + billingSHA256([]byte(canonical))
	signDate := billingHMAC([]byte("TC3"+secretKey), date)
	signService := billingHMAC(signDate, "billing")
	signKey := billingHMAC(signService, "tc3_request")
	signature := hex.EncodeToString(billingHMAC(signKey, toSign))
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("X-TC-Action", action)
	req.Header.Set("X-TC-Version", "2018-07-09")
	req.Header.Set("X-TC-Timestamp", timestamp)
	req.Header.Set("Authorization", "TC3-HMAC-SHA256 Credential="+secretID+"/"+scope+", SignedHeaders="+signedHeaders+", Signature="+signature)
}

func fetchAnthropicProviderBills(ctx context.Context, client *http.Client, cfg BillingProviderConfig, start, end time.Time) ([]ProviderBill, error) {
	// The Cost API returns whole daily buckets. Do not ask for an incomplete
	// current day, which would either be omitted or contain still-changing cost.
	if today := time.Now().UTC().Truncate(24 * time.Hour); end.After(today) {
		end = today
	}
	if !start.Before(end) {
		return []ProviderBill{}, nil
	}
	query := url.Values{
		"starting_at": {start.Format(time.RFC3339)}, "ending_at": {end.Format(time.RFC3339)},
		"bucket_width": {"1d"}, "limit": {"31"}, "group_by[]": {"workspace_id", "description"},
	}
	var bills []ProviderBill
	seenPages := map[string]bool{}
	rawBytes := 0
	for page := 0; page < billingProviderMaxPages; page++ {
		var response struct {
			Data *[]struct {
				StartingAt time.Time          `json:"starting_at"`
				EndingAt   time.Time          `json:"ending_at"`
				Results    *[]json.RawMessage `json:"results"`
			} `json:"data"`
			HasMore  bool   `json:"has_more"`
			NextPage string `json:"next_page"`
		}
		err := billingProviderRequest(ctx, client, func() (*http.Request, error) {
			return billingJSONRequest(ctx, http.MethodGet, "https://api.anthropic.com/v1/organizations/cost_report?"+query.Encode(), nil, map[string]string{"x-api-key": cfg.Secrets["admin_api_key"], "anthropic-version": "2023-06-01"})
		}, &response)
		if err != nil {
			return nil, err
		}
		if response.Data == nil {
			return nil, errors.New("invalid Anthropic billing response")
		}
		for _, bucket := range *response.Data {
			if bucket.StartingAt.Before(start) || bucket.EndingAt.After(end) || !bucket.StartingAt.Before(bucket.EndingAt) {
				return nil, errors.New("invalid Anthropic billing period")
			}
			if bucket.Results == nil {
				return nil, errors.New("invalid Anthropic billing results list")
			}
			for _, sourceRow := range *bucket.Results {
				var row billingAnthropicCostResult
				if json.Unmarshal(sourceRow, &row) != nil {
					return nil, errors.New("invalid Anthropic billing row")
				}
				resource := "__organization__"
				if row.WorkspaceID != nil && *row.WorkspaceID != "" {
					resource = *row.WorkspaceID
				}
				if configured := cfg.Settings["workspace_id"]; configured != "" && resource != configured {
					continue
				}
				amount, parseErr := billingParseAmount(row.Amount)
				// Official Cost API amounts are decimal strings in cents, not USD
				// dollars. Preserve sub-cent precision instead of rounding.
				currency := strings.ToUpper(row.Currency)
				if parseErr != nil || currency != "USD" {
					return nil, errors.New("invalid Anthropic billing amount or unsupported currency")
				}
				raw, rawErr := billingRawCostSource(map[string]any{"starting_at": bucket.StartingAt, "ending_at": bucket.EndingAt, "result": sourceRow})
				if rawErr != nil {
					return nil, rawErr
				}
				rawBytes += len(raw)
				if rawBytes > billingProviderMaxRawTotal {
					return nil, errors.New("provider original billing snapshot exceeds 32 MiB")
				}
				usage := billingAnthropicUsageCandidate(row)
				usageStatus := "unsupported"
				if usage != nil {
					usageStatus = "unavailable"
				}
				bills = append(bills, ProviderBill{
					ID:         billingBucketID("anthropic", resource, bucket.StartingAt.UTC().Format(time.RFC3339), bucket.EndingAt.UTC().Format(time.RFC3339), row.Description, currency),
					ResourceID: resource, Description: billingDescription(row.Description), Amount: amount.Shift(-2).String(), Currency: currency,
					PeriodStart: bucket.StartingAt.UTC(), PeriodEnd: bucket.EndingAt.UTC(),
					RawSource: raw, Usage: usage, UsageStatus: usageStatus,
				})
				if len(bills) > billingProviderMaxBills {
					return nil, errors.New("provider billing record limit exceeded")
				}
			}
		}
		if !response.HasMore {
			attachAnthropicProviderUsage(ctx, client, cfg, start, end, bills)
			return bills, nil
		}
		if response.NextPage == "" || len(response.NextPage) > 4096 || seenPages[response.NextPage] {
			return nil, errors.New("invalid or repeated Anthropic billing pagination token")
		}
		seenPages[response.NextPage] = true
		query.Set("page", response.NextPage)
	}
	return nil, errors.New("Anthropic billing pagination limit exceeded")
}

func billingRawCostSource(value any) (json.RawMessage, error) {
	encoded, err := json.Marshal(value)
	if err != nil || len(encoded) > billingProviderMaxRawRow {
		return nil, errors.New("provider raw billing row exceeds the 64 KiB limit or is invalid")
	}
	return json.RawMessage(encoded), nil
}

func billingAnthropicUsageCandidate(row billingAnthropicCostResult) *ProviderBillUsage {
	if row.CostType != "tokens" || row.Model == "" || len(row.Model) > 200 ||
		(row.ContextWindow != "0-200k" && row.ContextWindow != "200k-1M") ||
		(row.ServiceTier != "standard" && row.ServiceTier != "batch") ||
		(row.InferenceGeo != "global" && row.InferenceGeo != "us" && row.InferenceGeo != "not_available") {
		return nil
	}
	// These exact enum strings are documented in the Cost Report response
	// schema. Do not infer category/model by searching the human description.
	tokenTypes := map[string]string{
		"uncached_input_tokens": "input", "output_tokens": "output", "cache_read_input_tokens": "cache_read",
		"cache_creation.ephemeral_5m_input_tokens": "cache_creation_5m",
		"cache_creation.ephemeral_1h_input_tokens": "cache_creation_1h",
	}
	category, ok := tokenTypes[row.TokenType]
	if !ok {
		return nil
	}
	return &ProviderBillUsage{Model: row.Model, TokenType: category, ContextWindow: row.ContextWindow, ServiceTier: row.ServiceTier, InferenceGeo: row.InferenceGeo}
}

func billingAnthropicUsageKey(resource string, start, end time.Time, usage *ProviderBillUsage) string {
	return billingBucketID(resource, start.UTC().Format(time.RFC3339), end.UTC().Format(time.RFC3339),
		usage.Model, usage.TokenType, usage.ContextWindow, usage.ServiceTier, usage.InferenceGeo)
}

// A usage failure must not discard a valid official cost snapshot. Missing or
// ambiguous evidence is explicit and never silently replaced by guessed tokens.
func attachAnthropicProviderUsage(ctx context.Context, client *http.Client, cfg BillingProviderConfig, start, end time.Time, bills []ProviderBill) {
	matchCount := make(map[string]int)
	for _, bill := range bills {
		if bill.Usage != nil {
			matchCount[billingAnthropicUsageKey(bill.ResourceID, bill.PeriodStart, bill.PeriodEnd, bill.Usage)]++
		}
	}
	if len(matchCount) == 0 {
		return
	}
	usageCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	counts, err := fetchAnthropicProviderUsage(usageCtx, client, cfg, start, end)
	for i := range bills {
		bill := &bills[i]
		if bill.Usage == nil {
			continue
		}
		key := billingAnthropicUsageKey(bill.ResourceID, bill.PeriodStart, bill.PeriodEnd, bill.Usage)
		if matchCount[key] != 1 {
			// E.g. two differently-priced descriptions sharing parsed fields.
			// A shared denominator cannot safely price each one independently.
			bill.Usage = nil
			bill.UsageStatus = "unsupported"
			continue
		}
		count, found := counts[key]
		if err != nil || !found {
			bill.Usage = nil
			bill.UsageStatus = "unavailable"
			continue
		}
		bill.Usage.Tokens = count
		bill.Usage.Complete = true
		bill.UsageStatus = "available"
	}
}

func fetchAnthropicProviderUsage(ctx context.Context, client *http.Client, cfg BillingProviderConfig, start, end time.Time) (map[string]int64, error) {
	query := url.Values{
		"starting_at": {start.Format(time.RFC3339)}, "ending_at": {end.Format(time.RFC3339)},
		"bucket_width": {"1d"}, "limit": {"31"},
		"group_by[]": {"workspace_id", "model", "context_window", "service_tier", "inference_geo"},
	}
	if workspace := cfg.Settings["workspace_id"]; workspace != "" && workspace != "__organization__" {
		query.Set("workspace_ids[]", workspace)
	}
	counts := make(map[string]int64)
	seenPages := make(map[string]bool)
	records := 0
	for page := 0; page < billingProviderMaxPages; page++ {
		var response struct {
			Data *[]struct {
				StartingAt time.Time `json:"starting_at"`
				EndingAt   time.Time `json:"ending_at"`
				Results    *[]struct {
					WorkspaceID          *string `json:"workspace_id"`
					Model                string  `json:"model"`
					ContextWindow        string  `json:"context_window"`
					ServiceTier          string  `json:"service_tier"`
					InferenceGeo         string  `json:"inference_geo"`
					UncachedInputTokens  *int64  `json:"uncached_input_tokens"`
					OutputTokens         *int64  `json:"output_tokens"`
					CacheReadInputTokens *int64  `json:"cache_read_input_tokens"`
					CacheCreation        struct {
						Ephemeral5mInputTokens *int64 `json:"ephemeral_5m_input_tokens"`
						Ephemeral1hInputTokens *int64 `json:"ephemeral_1h_input_tokens"`
					} `json:"cache_creation"`
				} `json:"results"`
			} `json:"data"`
			HasMore  bool   `json:"has_more"`
			NextPage string `json:"next_page"`
		}
		err := billingProviderRequest(ctx, client, func() (*http.Request, error) {
			return billingJSONRequest(ctx, http.MethodGet, "https://api.anthropic.com/v1/organizations/usage_report/messages?"+query.Encode(), nil,
				map[string]string{"x-api-key": cfg.Secrets["admin_api_key"], "anthropic-version": "2023-06-01"})
		}, &response)
		if err != nil {
			return nil, err
		}
		if response.Data == nil {
			return nil, errors.New("invalid Anthropic usage report")
		}
		for _, bucket := range *response.Data {
			if bucket.StartingAt.Before(start) || bucket.EndingAt.After(end) || bucket.EndingAt.Sub(bucket.StartingAt) != 24*time.Hour || bucket.Results == nil {
				return nil, errors.New("invalid Anthropic usage bucket")
			}
			for _, row := range *bucket.Results {
				records++
				if records > billingProviderMaxBills {
					return nil, errors.New("Anthropic usage record limit exceeded")
				}
				resource := "__organization__"
				if row.WorkspaceID != nil && *row.WorkspaceID != "" {
					resource = *row.WorkspaceID
				}
				if configured := cfg.Settings["workspace_id"]; configured != "" && resource != configured {
					continue
				}
				values := map[string]*int64{
					"input": row.UncachedInputTokens, "output": row.OutputTokens, "cache_read": row.CacheReadInputTokens,
					"cache_creation_5m": row.CacheCreation.Ephemeral5mInputTokens, "cache_creation_1h": row.CacheCreation.Ephemeral1hInputTokens,
				}
				for category, tokens := range values {
					if tokens == nil {
						continue
					} // Missing is not evidence of zero.
					if *tokens < 0 {
						return nil, errors.New("negative Anthropic usage count")
					}
					usage := &ProviderBillUsage{Model: row.Model, TokenType: category, ContextWindow: row.ContextWindow, ServiceTier: row.ServiceTier, InferenceGeo: row.InferenceGeo}
					key := billingAnthropicUsageKey(resource, bucket.StartingAt, bucket.EndingAt, usage)
					if _, exists := counts[key]; exists {
						return nil, errors.New("duplicate Anthropic usage bucket")
					}
					counts[key] = *tokens
				}
			}
		}
		if !response.HasMore {
			return counts, nil
		}
		if response.NextPage == "" || len(response.NextPage) > 4096 || seenPages[response.NextPage] {
			return nil, errors.New("invalid or repeated Anthropic usage pagination token")
		}
		seenPages[response.NextPage] = true
		query.Set("page", response.NextPage)
	}
	return nil, errors.New("Anthropic usage pagination limit exceeded")
}

func billingParseAmount(value string) (decimal.Decimal, error) {
	if !billingAmountPattern.MatchString(value) {
		return decimal.Zero, errors.New("invalid monetary amount")
	}
	amount, err := decimal.NewFromString(value)
	if err != nil || amount.Exponent() < -24 || amount.Exponent() > 24 {
		return decimal.Zero, errors.New("invalid monetary amount")
	}
	return amount, nil
}

func billingBucketID(parts ...string) string {
	encoded, _ := json.Marshal(parts)
	return billingSHA256(encoded)
}

func billingSHA256(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

func billingHMAC(key []byte, value string) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(value))
	return mac.Sum(nil)
}

func billingDescription(value string) string {
	value = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, value)
	runes := []rune(value)
	if len(runes) > 500 {
		return string(runes[:500])
	}
	return value
}
