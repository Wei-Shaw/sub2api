package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type upstreamBillingTestTransport func(*http.Request) (*http.Response, error)

func (fn upstreamBillingTestTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func upstreamBillingTestClient(fn upstreamBillingTestTransport) *http.Client {
	return &http.Client{Transport: fn, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
}

func upstreamBillingTestResponse(body string) *http.Response {
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}

func upstreamBillingAzureConfig() BillingProviderConfig {
	return BillingProviderConfig{
		Provider: "azure",
		Settings: map[string]string{
			"tenant_id": "11111111-1111-1111-1111-111111111111", "client_id": "22222222-2222-2222-2222-222222222222",
			"subscription_id": "33333333-3333-3333-3333-333333333333",
			"resource_id":     "/subscriptions/33333333-3333-3333-3333-333333333333/resourceGroups/OpenAI/providers/Microsoft.CognitiveServices/accounts/Demo",
		},
		Secrets: map[string]string{"client_secret": "sensitive-client-secret"},
	}
}

func upstreamBillingTencentConfig() BillingProviderConfig {
	return BillingProviderConfig{Provider: "tencent", Settings: map[string]string{"business_code": "example_verified_product"}, Secrets: map[string]string{"secret_id": "AKIDexample", "secret_key": "sensitive-secret-key"}}
}

func upstreamBillingAnthropicConfig() BillingProviderConfig {
	return BillingProviderConfig{Provider: "anthropic", Settings: map[string]string{}, Secrets: map[string]string{"admin_api_key": "sk-ant-admin-example"}}
}

func upstreamBillingTestMonth() (time.Time, time.Time) {
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	return start, start.AddDate(0, 1, 0)
}

func TestUpstreamBillingProviderValidate(t *testing.T) {
	for _, cfg := range []BillingProviderConfig{upstreamBillingAzureConfig(), upstreamBillingTencentConfig(), upstreamBillingAnthropicConfig()} {
		if err := ValidateBillingProviderConfig(cfg); err != nil {
			t.Fatalf("%s: %v", cfg.Provider, err)
		}
	}
	tests := []struct {
		name   string
		config func() BillingProviderConfig
		change func(*BillingProviderConfig)
	}{
		{"unknown provider", upstreamBillingAzureConfig, func(c *BillingProviderConfig) { c.Provider = "arbitrary" }},
		{"secret in plaintext settings", upstreamBillingAzureConfig, func(c *BillingProviderConfig) { c.Settings["client_secret"] = "never-show-me" }},
		{"unknown secret", upstreamBillingAzureConfig, func(c *BillingProviderConfig) { c.Secrets["account_password"] = "never-show-me" }},
		{"empty secret", upstreamBillingAzureConfig, func(c *BillingProviderConfig) { c.Secrets["client_secret"] = "" }},
		{"tenant path injection", upstreamBillingAzureConfig, func(c *BillingProviderConfig) { c.Settings["tenant_id"] = "../common?secret=never-show-me" }},
		{"mismatched subscription", upstreamBillingAzureConfig, func(c *BillingProviderConfig) { c.Settings["subscription_id"] = "44444444-4444-4444-4444-444444444444" }},
		{"non-AI resource", upstreamBillingAzureConfig, func(c *BillingProviderConfig) {
			c.Settings["resource_id"] = strings.ReplaceAll(c.Settings["resource_id"], "Microsoft.CognitiveServices", "Microsoft.Compute")
		}},
		{"international unspecified currency", upstreamBillingTencentConfig, func(c *BillingProviderConfig) { c.Settings["region"] = "international" }},
		{"missing product filter", upstreamBillingTencentConfig, func(c *BillingProviderConfig) { delete(c.Settings, "business_code") }},
		{"header injection", upstreamBillingTencentConfig, func(c *BillingProviderConfig) { c.Secrets["secret_id"] = "abc\r\nx-secret:never-show-me" }},
		{"arbitrary endpoint", upstreamBillingAnthropicConfig, func(c *BillingProviderConfig) { c.Settings["base_url"] = "https://never-show-me" }},
		{"invalid workspace", upstreamBillingAnthropicConfig, func(c *BillingProviderConfig) { c.Settings["workspace_id"] = "never-show-me" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := test.config()
			test.change(&cfg)
			err := ValidateBillingProviderConfig(cfg)
			if err == nil {
				t.Fatal("expected invalid configuration")
			}
			if strings.Contains(err.Error(), "never-show-me") {
				t.Fatal("configuration value leaked")
			}
		})
	}
}

func TestUpstreamBillingProviderAzurePaginationAndExactMoney(t *testing.T) {
	cfg := upstreamBillingAzureConfig()
	start, end := upstreamBillingTestMonth()
	resource := strings.ToLower(cfg.Settings["resource_id"])
	pages := 0
	client := upstreamBillingTestClient(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host == "login.microsoftonline.com" {
			if err := req.ParseForm(); err != nil {
				t.Fatal(err)
			}
			if req.PostForm.Get("client_secret") != cfg.Secrets["client_secret"] || req.PostForm.Get("scope") != "https://management.azure.com/.default" {
				t.Fatal("incorrect token request")
			}
			return upstreamBillingTestResponse(`{"access_token":"private-access-token"}`), nil
		}
		if req.URL.Host != "management.azure.com" || req.Header.Get("Authorization") != "Bearer private-access-token" || req.Method != http.MethodPost {
			t.Fatal("unexpected Azure request")
		}
		var body struct {
			TimePeriod map[string]string `json:"timePeriod"`
			Dataset    struct {
				Filter struct {
					Dimensions struct {
						Values []string `json:"values"`
					} `json:"dimensions"`
				} `json:"filter"`
			} `json:"dataset"`
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.TimePeriod["to"] != "2025-01-31T23:59:59Z" {
			t.Fatalf("exclusive end converted incorrectly: %v", body.TimePeriod)
		}
		if len(body.Dataset.Filter.Dimensions.Values) != 1 || body.Dataset.Filter.Dimensions.Values[0] != resource {
			t.Fatal("missing resource filter")
		}
		pages++
		next := ""
		if pages == 1 {
			next = "https://management.azure.com" + req.URL.Path + "?api-version=2025-03-01&$skiptoken=next"
		}
		amount := "0.100000000000001"
		if pages == 2 {
			amount = "-0.025"
		}
		return upstreamBillingTestResponse(fmt.Sprintf(`{"properties":{"columns":[{"name":"Currency"},{"name":"ResourceId"},{"name":"PreTaxCost"},{"name":"UsageDate"}],"rows":[["USD",%q,%s,2025010%d]],"nextLink":%q}}`, resource, amount, pages, next)), nil
	})
	bills, err := fetchProviderBillsWithClient(context.Background(), client, cfg, start, end)
	if err != nil {
		t.Fatal(err)
	}
	if pages != 2 || len(bills) != 2 {
		t.Fatalf("wrong page/record count: %d/%d", pages, len(bills))
	}
	amounts := map[string]bool{}
	for _, bill := range bills {
		amounts[bill.Amount] = true
		if bill.ResourceID != resource || bill.Currency != "USD" || bill.PeriodEnd.Sub(bill.PeriodStart) != 24*time.Hour {
			t.Fatalf("invalid normalized bill: %+v", bill)
		}
		if bill.UsageStatus != "unsupported" || !json.Valid(bill.RawSource) || !strings.Contains(string(bill.RawSource), `"columns"`) || strings.Contains(string(bill.RawSource), "private-access-token") {
			t.Fatal("invalid original Azure bill snapshot")
		}
	}
	if !amounts["0.100000000000001"] || !amounts["-0.025"] {
		t.Fatal("precision or credit sign was lost")
	}
}

func TestUpstreamBillingProviderAzureRejectUnsafePagination(t *testing.T) {
	cfg := upstreamBillingAzureConfig()
	path := "/subscriptions/" + cfg.Settings["subscription_id"] + "/providers/Microsoft.CostManagement/query"
	for _, endpoint := range []string{
		"http://management.azure.com" + path,
		"https://management.azure.com.evil.test" + path,
		"https://management.azure.com@evil.test" + path,
		"https://management.azure.com:444" + path,
		"https://management.azure.com/subscriptions/other/providers/Microsoft.CostManagement/query",
		"https://management.azure.com" + path + "#fragment",
		"https://management.azure.com" + strings.Replace(path, "query", "%71uery", 1),
	} {
		if billingAzurePageAllowed(endpoint, path) {
			t.Fatalf("accepted unsafe URL: %s", endpoint)
		}
	}
	start, end := upstreamBillingTestMonth()
	requests := 0
	client := upstreamBillingTestClient(func(req *http.Request) (*http.Response, error) {
		requests++
		if req.URL.Host == "login.microsoftonline.com" {
			return upstreamBillingTestResponse(`{"access_token":"private"}`), nil
		}
		return upstreamBillingTestResponse(`{"properties":{"rows":[],"nextLink":"https://attacker.invalid/steal-token"}}`), nil
	})
	bills, err := fetchProviderBillsWithClient(context.Background(), client, cfg, start, end)
	if err == nil || bills != nil || requests != 2 {
		t.Fatalf("unsafe pagination did not abort: %v, %d", err, requests)
	}
}

func TestUpstreamBillingProviderAzureInvalidResponse(t *testing.T) {
	cfg := upstreamBillingAzureConfig()
	start, end := upstreamBillingTestMonth()
	for _, response := range []string{
		`{"error":{"message":"sensitive-client-secret"}}`,
		`{"properties":{"columns":[{"name":"PreTaxCost"},{"name":"ResourceId"},{"name":"UsageDate"},{"name":"Currency"}],"rows":[[1,"unrelated",20250101,"USD"]]}}`,
		`{"properties":{"columns":[{"name":"PreTaxCost"}],"rows":[["NaN"]]}}`,
	} {
		client := upstreamBillingTestClient(func(req *http.Request) (*http.Response, error) {
			if req.URL.Host == "login.microsoftonline.com" {
				return upstreamBillingTestResponse(`{"access_token":"private"}`), nil
			}
			return upstreamBillingTestResponse(response), nil
		})
		bills, err := fetchProviderBillsWithClient(context.Background(), client, cfg, start, end)
		if err == nil || bills != nil || strings.Contains(err.Error(), "sensitive") {
			t.Fatalf("invalid response accepted/leaked: %v", err)
		}
	}
}

func TestUpstreamBillingProviderTencentWholeMonthPaginationAndCredits(t *testing.T) {
	cfg := upstreamBillingTencentConfig()
	zone := time.FixedZone("Asia/Shanghai", 8*3600)
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, zone)
	end := start.AddDate(0, 1, 0)
	requests := 0
	client := upstreamBillingTestClient(func(req *http.Request) (*http.Response, error) {
		requests++
		if req.URL.Host != "billing.tencentcloudapi.com" || req.Header.Get("X-TC-Action") != "DescribeBillDetail" || !strings.HasPrefix(req.Header.Get("Authorization"), "TC3-HMAC-SHA256 Credential=AKIDexample/") {
			t.Fatal("incorrect Tencent request/signature")
		}
		var payload map[string]any
		if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload["Month"] != "2025-01" || payload["BusinessCode"] != cfg.Settings["business_code"] || payload["Offset"] != float64((requests-1)*300) {
			t.Fatalf("incorrect request: %v", payload)
		}
		rows := make([]string, 0, 300)
		if requests == 1 {
			for i := 0; i < 300; i++ {
				rows = append(rows, fmt.Sprintf(`{"Id":"row-%d","BusinessCode":"example_verified_product","ResourceId":"Resource-CaseSensitive","ComponentSet":[{"RealCost":"0.1"},{"RealCost":"0.2"}]}`, i))
			}
		} else {
			if payload["Context"] != "next-context" {
				t.Fatal("missing Tencent continuation")
			}
			rows = append(rows, `{"Id":"refund","BusinessCode":"example_verified_product","ResourceId":"Resource-CaseSensitive","ComponentSet":[{"RealCost":"-2.7"}]}`)
		}
		// Total is intentionally wrong: Tencent documents that it is stale.
		return upstreamBillingTestResponse(`{"Response":{"Total":1,"Context":"next-context","DetailSet":[` + strings.Join(rows, ",") + `]}}`), nil
	})
	bills, err := fetchProviderBillsWithClient(context.Background(), client, cfg, start, end)
	if err != nil {
		t.Fatal(err)
	}
	if len(bills) != 301 || requests != 2 {
		t.Fatalf("pagination: %d bills in %d requests", len(bills), requests)
	}
	credit := false
	for _, bill := range bills {
		if bill.Currency != "CNY" || bill.ResourceID != "Resource-CaseSensitive" || !bill.PeriodStart.Equal(start) || !bill.PeriodEnd.Equal(end) {
			t.Fatalf("invalid bill attribution: %+v", bill)
		}
		if bill.Amount == "-2.7" {
			credit = true
		} else if bill.Amount != "0.3" {
			t.Fatal("inexact component sum")
		}
	}
	if !credit {
		t.Fatal("refund missing")
	}
}

func TestUpstreamBillingProviderTencentRejectsPartialOrUTCMontH(t *testing.T) {
	start, end := upstreamBillingTestMonth()
	called := false
	client := upstreamBillingTestClient(func(req *http.Request) (*http.Response, error) {
		called = true
		return upstreamBillingTestResponse(`{}`), nil
	})
	if _, err := fetchProviderBillsWithClient(context.Background(), client, upstreamBillingTencentConfig(), start, end); err == nil || called {
		t.Fatal("UTC month should be rejected before network access")
	}
}

func TestUpstreamBillingProviderTencentSignatureGolden(t *testing.T) {
	req, _ := http.NewRequest(http.MethodPost, "https://billing.tencentcloudapi.com/", strings.NewReader("{}"))
	billingSignTencent(req, []byte("{}"), "AKIDexample", "test-secret", time.Unix(1735689600, 0))
	want := "TC3-HMAC-SHA256 Credential=AKIDexample/2025-01-01/billing/tc3_request, SignedHeaders=content-type;host;x-tc-action, Signature=c8d858c3b8b05e37eab7270483cb673408e7ab4c19a362e6649b5e2cf0c5e7e7"
	if got := req.Header.Get("Authorization"); got != want {
		t.Fatalf("signature differs: %s", got)
	}
}

func TestUpstreamBillingProviderTencentRejectsWrongProductAndMissingCosts(t *testing.T) {
	zone := time.FixedZone("China", 8*3600)
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, zone)
	for _, response := range []string{
		`{"Response":{"Error":{"Code":"AuthFailure","Message":"sensitive-secret-key"}}}`,
		`{"Response":{"DetailSet":[{"Id":"1","BusinessCode":"other-product","ComponentSet":[{"RealCost":"1"}]}]}}`,
		`{"Response":{"DetailSet":[{"Id":"1","BusinessCode":"example_verified_product","ComponentSet":[]}]}}`,
		`{"Response":{"DetailSet":[{"Id":"1","BusinessCode":"example_verified_product","ComponentSet":[{"RealCost":""}]}]}}`,
	} {
		client := upstreamBillingTestClient(func(*http.Request) (*http.Response, error) { return upstreamBillingTestResponse(response), nil })
		bills, err := fetchProviderBillsWithClient(context.Background(), client, upstreamBillingTencentConfig(), start, start.AddDate(0, 1, 0))
		if err == nil || bills != nil || strings.Contains(err.Error(), "sensitive") {
			t.Fatalf("invalid/tencent data accepted/leaked: %v", err)
		}
	}
}

func TestUpstreamBillingProviderAnthropicCentsPaginationAndWorkspace(t *testing.T) {
	cfg := upstreamBillingAnthropicConfig()
	start, end := upstreamBillingTestMonth()
	requests := 0
	client := upstreamBillingTestClient(func(req *http.Request) (*http.Response, error) {
		requests++
		if req.URL.Host != "api.anthropic.com" || req.Header.Get("x-api-key") != cfg.Secrets["admin_api_key"] {
			t.Fatal("invalid Anthropic request")
		}
		if req.URL.Query().Get("bucket_width") != "1d" || len(req.URL.Query()["group_by[]"]) != 2 {
			t.Fatal("workspace/description grouping missing")
		}
		if requests == 1 {
			return upstreamBillingTestResponse(`{"data":[{"starting_at":"2025-01-01T00:00:00Z","ending_at":"2025-01-02T00:00:00Z","results":[{"amount":"123.78912","currency":"USD","description":"Input tokens","workspace_id":"wrkspc_one"}]}],"has_more":true,"next_page":"safe-opaque-token"}`), nil
		}
		if req.URL.Query().Get("page") != "safe-opaque-token" {
			t.Fatal("missing continuation token")
		}
		return upstreamBillingTestResponse(`{"data":[{"starting_at":"2025-01-02T00:00:00Z","ending_at":"2025-01-03T00:00:00Z","results":[{"amount":"-25","currency":"USD","description":"Credit","workspace_id":null}]}],"has_more":false}`), nil
	})
	bills, err := fetchProviderBillsWithClient(context.Background(), client, cfg, start, end)
	if err != nil {
		t.Fatal(err)
	}
	if len(bills) != 2 || requests != 2 {
		t.Fatalf("wrong bill count: %d", len(bills))
	}
	amounts := map[string]string{}
	for _, bill := range bills {
		amounts[bill.ResourceID] = bill.Amount
	}
	if amounts["wrkspc_one"] != "1.2378912" || amounts["__organization__"] != "-0.25" {
		t.Fatalf("cents conversion: %v", amounts)
	}
}

func TestUpstreamBillingProviderAnthropicFiltersWorkspaceAndIDsSurviveRevision(t *testing.T) {
	cfg := upstreamBillingAnthropicConfig()
	cfg.Settings["workspace_id"] = "wrkspc_selected"
	start, end := upstreamBillingTestMonth()
	amount := "100"
	client := upstreamBillingTestClient(func(*http.Request) (*http.Response, error) {
		return upstreamBillingTestResponse(fmt.Sprintf(`{"data":[{"starting_at":"2025-01-01T00:00:00Z","ending_at":"2025-01-02T00:00:00Z","results":[{"amount":%q,"currency":"USD","description":"Input tokens","workspace_id":"wrkspc_selected"},{"amount":"9999","currency":"USD","description":"Input tokens","workspace_id":"wrkspc_other"}]}],"has_more":false}`, amount)), nil
	})
	first, err := fetchProviderBillsWithClient(context.Background(), client, cfg, start, end)
	if err != nil {
		t.Fatal(err)
	}
	amount = "200"
	revised, err := fetchProviderBillsWithClient(context.Background(), client, cfg, start, end)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 1 || len(revised) != 1 || first[0].ID != revised[0].ID || first[0].Amount == revised[0].Amount {
		t.Fatal("unstable IDs or unfiltered workspaces")
	}
}

func TestUpstreamBillingProviderAnthropicInvalidResponsesAbort(t *testing.T) {
	start, end := upstreamBillingTestMonth()
	for _, response := range []string{
		`{"data":null}`,
		`{"data":[],"has_more":true}`,
		`{"data":[{"starting_at":"2024-12-31T00:00:00Z","ending_at":"2025-01-01T00:00:00Z"}]}`,
		`{"data":[{"starting_at":"2025-01-01T00:00:00Z","ending_at":"2025-01-02T00:00:00Z","results":[{"amount":"12","currency":"CNY"}]}]}`,
		`{"data":[{"starting_at":"2025-01-01T00:00:00Z","ending_at":"2025-01-02T00:00:00Z","results":[{"amount":"NaN","currency":"USD"}]}]}`,
	} {
		client := upstreamBillingTestClient(func(*http.Request) (*http.Response, error) { return upstreamBillingTestResponse(response), nil })
		bills, err := fetchProviderBillsWithClient(context.Background(), client, upstreamBillingAnthropicConfig(), start, end)
		if err == nil || bills != nil {
			t.Fatal("invalid response was accepted")
		}
	}
}

func TestUpstreamBillingProviderConflictingDuplicateIsNotDoubleCharged(t *testing.T) {
	start, end := upstreamBillingTestMonth()
	client := upstreamBillingTestClient(func(*http.Request) (*http.Response, error) {
		return upstreamBillingTestResponse(`{"data":[{"starting_at":"2025-01-01T00:00:00Z","ending_at":"2025-01-02T00:00:00Z","results":[{"amount":"100","currency":"USD","description":"Tokens"},{"amount":"200","currency":"USD","description":"Tokens"}]}]}`), nil
	})
	bills, err := fetchProviderBillsWithClient(context.Background(), client, upstreamBillingAnthropicConfig(), start, end)
	if err == nil || bills != nil {
		t.Fatal("conflicting records should abort transaction")
	}
}

func TestUpstreamBillingProviderRequestSafety(t *testing.T) {
	makeRequest := func() (*http.Request, error) {
		return http.NewRequestWithContext(context.Background(), http.MethodGet, "https://api.anthropic.com/", nil)
	}
	t.Run("HTTP errors never reveal body", func(t *testing.T) {
		client := upstreamBillingTestClient(func(*http.Request) (*http.Response, error) {
			r := upstreamBillingTestResponse(`{"error":"sensitive-secret-key"}`)
			r.StatusCode = 403
			return r, nil
		})
		err := billingProviderRequest(context.Background(), client, makeRequest, new(any))
		if err == nil || strings.Contains(err.Error(), "sensitive") {
			t.Fatalf("error leaked: %v", err)
		}
	})
	t.Run("transport errors never reveal URL", func(t *testing.T) {
		client := upstreamBillingTestClient(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("secret in https://host?token=sensitive-secret-key")
		})
		err := billingProviderRequest(context.Background(), client, makeRequest, new(any))
		if err == nil || strings.Contains(err.Error(), "sensitive") {
			t.Fatalf("error leaked: %v", err)
		}
	})
	t.Run("long Retry-After is honored without early retry", func(t *testing.T) {
		count := 0
		client := upstreamBillingTestClient(func(*http.Request) (*http.Response, error) {
			count++
			r := upstreamBillingTestResponse(`{}`)
			r.StatusCode = 429
			r.Header.Set("Retry-After", "3600")
			return r, nil
		})
		if err := billingProviderRequest(context.Background(), client, makeRequest, new(any)); err == nil || count != 1 {
			t.Fatal("retried before rate limit expiry")
		}
	})
	t.Run("bounded response", func(t *testing.T) {
		client := upstreamBillingTestClient(func(*http.Request) (*http.Response, error) {
			return upstreamBillingTestResponse(strings.Repeat("x", billingProviderMaxBody+1)), nil
		})
		if err := billingProviderRequest(context.Background(), client, makeRequest, new(any)); err == nil {
			t.Fatal("unbounded response accepted")
		}
	})
	t.Run("trailing JSON rejected", func(t *testing.T) {
		client := upstreamBillingTestClient(func(*http.Request) (*http.Response, error) { return upstreamBillingTestResponse(`{} {}`), nil })
		if err := billingProviderRequest(context.Background(), client, makeRequest, new(any)); err == nil {
			t.Fatal("trailing JSON accepted")
		}
	})
	t.Run("retry cancellable", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		client := upstreamBillingTestClient(func(*http.Request) (*http.Response, error) {
			r := upstreamBillingTestResponse(`{}`)
			r.StatusCode = 503
			return r, nil
		})
		if err := billingProviderRequest(ctx, client, makeRequest, new(any)); err == nil {
			t.Fatal("cancel ignored")
		}
	})
}

func TestUpstreamBillingProviderRetryAfterParsing(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	if billingRetryDelay("10", now) != 10*time.Second {
		t.Fatal("seconds not parsed")
	}
	if billingRetryDelay(now.Add(time.Minute).Format(http.TimeFormat), now) != time.Minute {
		t.Fatal("date not parsed")
	}
	if billingRetryDelay("-2", now) != 0 || billingRetryDelay("invalid", now) != 0 {
		t.Fatal("invalid delay accepted")
	}
}

func upstreamBillingAnthropicTokenCosts(tokenTypes ...string) string {
	rows := make([]string, 0, len(tokenTypes))
	for _, tokenType := range tokenTypes {
		rows = append(rows, fmt.Sprintf(`{"amount":"123.78912","currency":"USD","description":%q,"workspace_id":"wrkspc_one","model":"claude-model-exact","cost_type":"tokens","token_type":%q,"context_window":"0-200k","service_tier":"standard","inference_geo":"global"}`, tokenType, tokenType))
	}
	return `{"data":[{"starting_at":"2025-01-01T00:00:00Z","ending_at":"2025-01-02T00:00:00Z","results":[` + strings.Join(rows, ",") + `]}],"has_more":false}`
}

func TestUpstreamBillingProviderAnthropicExactCategoryEvidence(t *testing.T) {
	start, end := upstreamBillingTestMonth()
	tokenTypes := []string{"uncached_input_tokens", "output_tokens", "cache_read_input_tokens", "cache_creation.ephemeral_5m_input_tokens", "cache_creation.ephemeral_1h_input_tokens"}
	requests := 0
	client := upstreamBillingTestClient(func(req *http.Request) (*http.Response, error) {
		requests++
		if strings.HasSuffix(req.URL.Path, "/cost_report") {
			return upstreamBillingTestResponse(upstreamBillingAnthropicTokenCosts(tokenTypes...)), nil
		}
		if req.URL.Path != "/v1/organizations/usage_report/messages" || len(req.URL.Query()["group_by[]"]) != 5 {
			t.Fatal("incorrect usage query")
		}
		return upstreamBillingTestResponse(`{"data":[{"starting_at":"2025-01-01T00:00:00Z","ending_at":"2025-01-02T00:00:00Z","results":[{"workspace_id":"wrkspc_one","model":"claude-model-exact","context_window":"0-200k","service_tier":"standard","inference_geo":"global","uncached_input_tokens":1500,"output_tokens":500,"cache_read_input_tokens":200,"cache_creation":{"ephemeral_5m_input_tokens":100,"ephemeral_1h_input_tokens":50}},{"workspace_id":"wrkspc_one","model":"different-model","context_window":"0-200k","service_tier":"standard","inference_geo":"global","uncached_input_tokens":9000000}]}],"has_more":false}`), nil
	})
	bills, err := fetchProviderBillsWithClient(context.Background(), client, upstreamBillingAnthropicConfig(), start, end)
	if err != nil {
		t.Fatal(err)
	}
	if requests != 2 || len(bills) != 5 {
		t.Fatalf("unexpected requests/bills: %d/%d", requests, len(bills))
	}
	want := map[string]int64{"input": 1500, "output": 500, "cache_read": 200, "cache_creation_5m": 100, "cache_creation_1h": 50}
	for _, bill := range bills {
		if bill.UsageStatus != "available" || bill.Usage == nil || !bill.Usage.Complete || bill.Usage.Tokens != want[bill.Usage.TokenType] {
			t.Fatalf("incorrect category evidence: %+v", bill.Usage)
		}
		if bill.Usage.Model != "claude-model-exact" || bill.Usage.ContextWindow != "0-200k" || bill.Usage.ServiceTier != "standard" || bill.Usage.InferenceGeo != "global" {
			t.Fatal("lost matching dimensions")
		}
		if !strings.Contains(string(bill.RawSource), `"amount":"123.78912"`) || !strings.Contains(string(bill.RawSource), `"starting_at":"2025-01-01T00:00:00Z"`) || bill.Amount != "1.2378912" {
			t.Fatal("raw source lost original cents or bucket")
		}
	}
}

func TestUpstreamBillingProviderAnthropicEvidenceUnavailablePreservesCosts(t *testing.T) {
	start, end := upstreamBillingTestMonth()
	for _, usageResponse := range []string{
		`{"data":[],"has_more":false}`,
		`{"data":[{"starting_at":"2025-01-01T00:00:00Z","ending_at":"2025-01-02T00:00:00Z","results":[{"workspace_id":"wrkspc_one","model":"claude-model-exact","context_window":"0-200k","service_tier":"batch","inference_geo":"global","uncached_input_tokens":1500}]}]}`,
		`{"data":[{"starting_at":"2025-01-01T00:00:00Z","ending_at":"2025-01-02T00:00:00Z","results":[{"workspace_id":"wrkspc_one","model":"claude-model-exact","context_window":"0-200k","service_tier":"standard","inference_geo":"global","uncached_input_tokens":-1}]}]}`,
		`{"data":null}`,
		"http-error",
	} {
		client := upstreamBillingTestClient(func(req *http.Request) (*http.Response, error) {
			if strings.HasSuffix(req.URL.Path, "/cost_report") {
				return upstreamBillingTestResponse(upstreamBillingAnthropicTokenCosts("uncached_input_tokens")), nil
			}
			if usageResponse == "http-error" {
				res := upstreamBillingTestResponse(`{"error":"private-admin-key"}`)
				res.StatusCode = 403
				return res, nil
			}
			return upstreamBillingTestResponse(usageResponse), nil
		})
		bills, err := fetchProviderBillsWithClient(context.Background(), client, upstreamBillingAnthropicConfig(), start, end)
		if err != nil || len(bills) != 1 || bills[0].Usage != nil || bills[0].UsageStatus != "unavailable" || bills[0].Amount != "1.2378912" {
			t.Fatalf("costs were lost or evidence fabricated: %v %+v", err, bills)
		}
	}
}

func TestUpstreamBillingProviderAnthropicUnknownDimensionsAreUnsupported(t *testing.T) {
	start, end := upstreamBillingTestMonth()
	for _, replacement := range []struct{ from, to string }{
		{`"token_type":"uncached_input_tokens"`, `"token_type":"unknown_tokens"`},
		{`"inference_geo":"global"`, `"inference_geo":null`},
		{`"context_window":"0-200k"`, `"context_window":null`},
		{`"cost_type":"tokens"`, `"cost_type":"web_search"`},
	} {
		requests := 0
		client := upstreamBillingTestClient(func(req *http.Request) (*http.Response, error) {
			requests++
			return upstreamBillingTestResponse(strings.ReplaceAll(upstreamBillingAnthropicTokenCosts("uncached_input_tokens"), replacement.from, replacement.to)), nil
		})
		bills, err := fetchProviderBillsWithClient(context.Background(), client, upstreamBillingAnthropicConfig(), start, end)
		if err != nil || len(bills) != 1 || bills[0].Usage != nil || bills[0].UsageStatus != "unsupported" || requests != 1 {
			t.Fatalf("unsupported category used for pricing: %v %+v", err, bills)
		}
	}
}

func TestUpstreamBillingProviderAnthropicAmbiguousCostCategoriesNotPriced(t *testing.T) {
	start, end := upstreamBillingTestMonth()
	costs := upstreamBillingAnthropicTokenCosts("uncached_input_tokens", "uncached_input_tokens")
	// Same parsed dimensions but distinct descriptions can indicate a pricing
	// distinction absent from our comparable data. Do not reuse a denominator.
	costs = strings.Replace(costs, `"description":"uncached_input_tokens"`, `"description":"Different speed input tokens"`, 1)
	client := upstreamBillingTestClient(func(req *http.Request) (*http.Response, error) {
		if strings.HasSuffix(req.URL.Path, "/cost_report") {
			return upstreamBillingTestResponse(costs), nil
		}
		return upstreamBillingTestResponse(`{"data":[{"starting_at":"2025-01-01T00:00:00Z","ending_at":"2025-01-02T00:00:00Z","results":[{"workspace_id":"wrkspc_one","model":"claude-model-exact","context_window":"0-200k","service_tier":"standard","inference_geo":"global","uncached_input_tokens":1500}]}]}`), nil
	})
	bills, err := fetchProviderBillsWithClient(context.Background(), client, upstreamBillingAnthropicConfig(), start, end)
	if err != nil || len(bills) != 2 {
		t.Fatalf("bad costs: %v", err)
	}
	for _, bill := range bills {
		if bill.UsageStatus != "unsupported" || bill.Usage != nil {
			t.Fatal("ambiguous denominator attached")
		}
	}
}

func TestUpstreamBillingProviderRawSourceLimitsAndTencentQuantity(t *testing.T) {
	zone := time.FixedZone("China", 8*3600)
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, zone)
	row := `{"Id":"bill1","BusinessCode":"example_verified_product","ResourceId":"resource","ComponentSet":[{"RealCost":"1.200","UsedAmount":"3.5","UsedAmountUnit":"万Token","RealTotalMeasure":"5.0","DeductedMeasure":"1.5"}]}`
	client := upstreamBillingTestClient(func(*http.Request) (*http.Response, error) {
		return upstreamBillingTestResponse(`{"Response":{"DetailSet":[` + row + `]}}`), nil
	})
	bills, err := fetchProviderBillsWithClient(context.Background(), client, upstreamBillingTencentConfig(), start, start.AddDate(0, 1, 0))
	if err != nil || len(bills) != 1 {
		t.Fatalf("fetch: %v", err)
	}
	if string(bills[0].RawSource) != row || bills[0].Usage != nil || bills[0].UsageStatus != "unsupported" {
		t.Fatal("Tencent original quantity was lost or interpreted as tokens")
	}
	row = fmt.Sprintf(`{"Id":"bill1","BusinessCode":"example_verified_product","ResourceId":"resource","ReserveDetail":%q,"ComponentSet":[{"RealCost":"1"}]}`, strings.Repeat("x", billingProviderMaxRawRow))
	bills, err = fetchProviderBillsWithClient(context.Background(), client, upstreamBillingTencentConfig(), start, start.AddDate(0, 1, 0))
	if err == nil || bills != nil {
		t.Fatal("oversized original bill must fail, never silently truncate")
	}
}
