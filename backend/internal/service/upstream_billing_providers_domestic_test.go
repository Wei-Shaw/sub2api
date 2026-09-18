package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func billingDomesticTestConfig(provider string) BillingProviderConfig {
	return BillingProviderConfig{
		Provider: provider,
		Settings: map[string]string{"account_id": "123456789", "product_code": "verified_product"},
		Secrets:  map[string]string{"access_key_id": "test-access-id", "access_key_secret": "sensitive-secret"},
	}
}

func billingDomesticTestMonth() (time.Time, time.Time) {
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.FixedZone("Asia/Shanghai", 8*60*60))
	return start, start.AddDate(0, 1, 0)
}

func TestUpstreamBillingDomesticValidate(t *testing.T) {
	for _, provider := range []string{"aliyun", "volcengine"} {
		cfg := billingDomesticTestConfig(provider)
		if err := ValidateBillingProviderConfig(cfg); err != nil {
			t.Fatal(err)
		}
		for _, account := range []string{"", "0123", "-1", "+123", "0", "9223372036854775808", "RAM-user", "123/owner"} {
			bad := billingDomesticTestConfig(provider)
			bad.Settings["account_id"] = account
			if ValidateBillingProviderConfig(bad) == nil {
				t.Fatalf("accepted invalid account ID %q", account)
			}
		}
		for _, change := range []func(*BillingProviderConfig){
			func(c *BillingProviderConfig) { c.Settings["access_key_secret"] = "never-show-me" },
			func(c *BillingProviderConfig) { c.Settings["product_code"] = "" },
			func(c *BillingProviderConfig) { c.Settings["region"] = "cn-beijing" },
			func(c *BillingProviderConfig) { c.Settings["endpoint"] = "https://never-show-me" },
			func(c *BillingProviderConfig) { c.Secrets["access_key_id"] = "bad,Signature=injected" },
			func(c *BillingProviderConfig) { c.Secrets["access_key_secret"] = "" },
		} {
			bad := billingDomesticTestConfig(provider)
			change(&bad)
			err := ValidateBillingProviderConfig(bad)
			if err == nil || strings.Contains(err.Error(), "never-show-me") {
				t.Fatalf("unsafe validation: %v", err)
			}
		}
	}
}

func TestUpstreamBillingAliyunSignatureOfficialGolden(t *testing.T) {
	// Official ACS3 fixed example; key, nonce, timestamp and expected signature
	// are public sample values, not real credentials.
	req, _ := http.NewRequest(http.MethodPost, "https://ecs.cn-shanghai.aliyuncs.com/?RegionId=cn-shanghai&ImageId=win2019_1809_x64_dtc_zh-cn_40G_alibase_20230811.vhd", nil)
	req.Header.Set("x-acs-action", "RunInstances")
	req.Header.Set("x-acs-version", "2014-05-26")
	now, _ := time.Parse(time.RFC3339, "2023-10-26T10:22:32Z")
	billingAliyunSign(req, "YourAccessKeyId", "YourAccessKeySecret", now, "3156853299f313e23d1673dc12e1703d")
	want := "ACS3-HMAC-SHA256 Credential=YourAccessKeyId,SignedHeaders=host;x-acs-action;x-acs-content-sha256;x-acs-date;x-acs-signature-nonce;x-acs-version,Signature=06563a9e1b43f5dfe96b81484da74bceab24a1d853912eee15083a6f0f3283c0"
	if req.Header.Get("Authorization") != want {
		t.Fatalf("signature differs from official fixed example: %s", req.Header.Get("Authorization"))
	}
}

func TestUpstreamBillingVolcengineSignatureIndependentGolden(t *testing.T) {
	// Cross-checked using Node crypto and the official volc-sdk-golang/base
	// canonical request / signing-key algorithm, independently of Go helpers.
	body := []byte(`{"BillPeriod":"2025-01","Limit":300}`)
	req, _ := billingJSONRequest(context.Background(), http.MethodPost, "https://billing.volcengineapi.com/?Version=2022-01-01&Action=ListBillDetail", body, map[string]string{"Content-Type": "application/json; charset=utf-8"})
	now, _ := time.Parse(time.RFC3339, "2025-03-29T18:09:37Z")
	billingVolcengineSign(req, body, "test-access-id", "test-secret", now)
	want := "HMAC-SHA256 Credential=test-access-id/20250329/cn-north-1/billing/request, SignedHeaders=content-type;host;x-content-sha256;x-date, Signature=5ab79714c432652fe2a8a7f78b140a3590a47e071c9ec2feaa7fec8473b2983b"
	if req.Header.Get("Authorization") != want || req.Header.Get("X-Content-Sha256") != "1fc89af70b75098ebf6bf7cfb0396a23e967145dccc96f775f014ee640c5bd0e" {
		t.Fatalf("incorrect signing: %s", req.Header.Get("Authorization"))
	}
}

const billingAliyunTestRow = `{"OwnerID":"123456789","BillAccountID":"987654321","InstanceID":"Instance-Case-Sensitive","ProductCode":"verified_product","ProductType":"type","PipCode":"pip","CommodityCode":"commodity","BillingItemCode":"output","BillingItem":"输出","Item":"PayAsYouGoBill","SubscriptionType":"PayAsYouGo","Currency":"USD","PretaxAmount":9007199254740993.123456789,"AfterDiscountAmount":5,"Usage":"333.111","UsageUnit":"unknown"}`
const billingVolcengineTestRow = `{"BillDetailId":"Detail-1","BillPeriod":"2025-01","OwnerID":"123456789","PayerID":"987654321","Product":"verified_product","InstanceNo":"Instance-Case-Sensitive","ResourceID":"not-the-billing-instance","Currency":"USD","CurrencySettlement":"CNY","PayableAmount":"9007199254740993.123456789","SettlePayableAmount":"200","Count":"333.111","Unit":"unknown","ChargeItem":"输出","BillCategory":"消费-使用"}`

func billingAliyunTestEnvelope(row, cursor string, total int) string {
	return fmt.Sprintf(`{"Success":true,"Code":"Success","Data":{"AccountID":"987654321","BillingCycle":"2025-01","Items":[%s],"NextToken":%q,"TotalCount":%d}}`, row, cursor, total)
}

func billingVolcengineTestEnvelope(row string, offset, total int) string {
	return fmt.Sprintf(`{"ResponseMetadata":{"RequestId":"req"},"Result":{"List":[%s],"Total":%d,"Offset":%d,"Limit":300}}`, row, total, offset)
}

func TestUpstreamBillingAliyunPaginationPrecisionAndCorrection(t *testing.T) {
	cfg := billingDomesticTestConfig("aliyun")
	start, end := billingDomesticTestMonth()
	pages, lastNonce := 0, ""
	client := upstreamBillingTestClient(func(req *http.Request) (*http.Response, error) {
		q := req.URL.Query()
		if req.URL.Host != "business.aliyuncs.com" || req.Method != http.MethodPost || req.Header.Get("x-acs-action") != "DescribeInstanceBill" || req.Header.Get("x-acs-version") != "2017-12-14" || q.Get("BillOwnerId") != "123456789" || q.Get("ProductCode") != "verified_product" || q.Get("BillingCycle") != "2025-01" || q.Get("Granularity") != "MONTHLY" || q.Get("IsBillingItem") != "true" || q.Get("IsHideZeroCharge") != "false" || q.Get("MaxResults") != "300" {
			t.Fatal("incorrect Alibaba Cloud billing request")
		}
		if req.Header.Get("Content-Type") != "" || req.Body != nil || strings.Contains(req.URL.String(), "sensitive") || strings.Contains(req.URL.String(), "test-access-id") || !strings.HasPrefix(req.Header.Get("Authorization"), "ACS3-HMAC-SHA256 Credential=test-access-id,") {
			t.Fatal("incorrect RPC parameter placement or credentials")
		}
		if nonce := req.Header.Get("x-acs-signature-nonce"); len(nonce) != 32 || nonce == lastNonce {
			t.Fatal("missing or repeated nonce")
		} else {
			lastNonce = nonce
		}
		pages++
		if pages == 1 {
			return upstreamBillingTestResponse(billingAliyunTestEnvelope(billingAliyunTestRow, "opaque +/cursor=", 2)), nil
		}
		if q.Get("NextToken") != "opaque +/cursor=" || strings.Contains(req.URL.RawQuery, "+") {
			t.Fatal("cursor must be RFC3986 encoded")
		}
		refund := strings.ReplaceAll(strings.ReplaceAll(billingAliyunTestRow, `"PayAsYouGoBill"`, `"Refund"`), "9007199254740993.123456789", "-0.000000123456789")
		return upstreamBillingTestResponse(billingAliyunTestEnvelope(refund, "", 2)), nil
	})
	bills, err := fetchProviderBillsWithClient(context.Background(), client, cfg, start, end)
	if err != nil || len(bills) != 2 || pages != 2 {
		t.Fatalf("fetch: bills=%d pages=%d err=%v", len(bills), pages, err)
	}
	amounts := map[string]bool{}
	var originalID string
	for _, bill := range bills {
		amounts[bill.Amount] = true
		if bill.Amount == "9007199254740993.123456789" {
			originalID = bill.ID
		}
		if bill.ResourceID != "Instance-Case-Sensitive" || bill.Currency != "USD" || bill.Usage != nil || bill.UsageStatus != "unsupported" || !bill.PeriodStart.Equal(start) || bill.PeriodStart.Location() != time.UTC || !bill.PeriodEnd.Equal(end) || !strings.Contains(string(bill.RawSource), `"Usage":"333.111"`) || !strings.Contains(string(bill.RawSource), `"BillAccountID":"987654321"`) {
			t.Fatalf("incorrect bill mapping: %+v", bill)
		}
	}
	if !amounts["9007199254740993.123456789"] || !amounts["-0.000000123456789"] {
		t.Fatal("lost precision or negative adjustment")
	}
	corrected := strings.ReplaceAll(billingAliyunTestRow, "9007199254740993.123456789", "1.23")
	correctedClient := upstreamBillingTestClient(func(_ *http.Request) (*http.Response, error) {
		return upstreamBillingTestResponse(billingAliyunTestEnvelope(corrected, "", 1)), nil
	})
	updated, err := fetchProviderBillsWithClient(context.Background(), correctedClient, cfg, start, end)
	if err != nil || len(updated) != 1 || updated[0].ID != originalID || updated[0].Amount != "1.23" {
		t.Fatalf("correction changed identity: %v %+v", err, updated)
	}
}

func TestUpstreamBillingVolcenginePaginationPrecisionAndCorrection(t *testing.T) {
	cfg := billingDomesticTestConfig("volcengine")
	start, end := billingDomesticTestMonth()
	pages := 0
	client := upstreamBillingTestClient(func(req *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(req.Body)
		var query struct {
			BillPeriod                                                       string
			OwnerID                                                          []int64
			Product                                                          []string
			Limit, Offset, NeedRecordNum, IgnoreZero, GroupPeriod, GroupTerm int
		}
		if err := json.Unmarshal(body, &query); err != nil {
			t.Fatal(err)
		}
		if req.URL.Host != "billing.volcengineapi.com" || req.Method != http.MethodPost || req.URL.Query().Get("Action") != "ListBillDetail" || req.URL.Query().Get("Version") != "2022-01-01" || query.BillPeriod != "2025-01" || len(query.OwnerID) != 1 || query.OwnerID[0] != 123456789 || len(query.Product) != 1 || query.Product[0] != "verified_product" || query.Limit != 300 || query.Offset != pages || query.NeedRecordNum != 1 || query.IgnoreZero != 0 || query.GroupPeriod != 2 || query.GroupTerm != 0 {
			t.Fatalf("incorrect Volcengine billing request: %s", body)
		}
		if req.Header.Get("Content-Type") != "application/json; charset=utf-8" || !strings.HasPrefix(req.Header.Get("Authorization"), "HMAC-SHA256 Credential=test-access-id/") || req.Header.Get("X-Content-Sha256") != billingSHA256(body) {
			t.Fatal("incorrect signing")
		}
		pages++
		if pages == 1 {
			return upstreamBillingTestResponse(billingVolcengineTestEnvelope(billingVolcengineTestRow, 0, 2)), nil
		}
		refund := strings.ReplaceAll(strings.ReplaceAll(billingVolcengineTestRow, "Detail-1", "Detail-2"), "9007199254740993.123456789", "-0.000000123456789")
		return upstreamBillingTestResponse(billingVolcengineTestEnvelope(refund, 1, 2)), nil
	})
	bills, err := fetchProviderBillsWithClient(context.Background(), client, cfg, start, end)
	if err != nil || len(bills) != 2 || pages != 2 {
		t.Fatalf("fetch: bills=%d pages=%d err=%v", len(bills), pages, err)
	}
	amounts := map[string]bool{}
	var originalID string
	for _, bill := range bills {
		amounts[bill.Amount] = true
		if bill.Amount == "9007199254740993.123456789" {
			originalID = bill.ID
		}
		if bill.ResourceID != "Instance-Case-Sensitive" || bill.Currency != "USD" || bill.Usage != nil || bill.UsageStatus != "unsupported" || !bill.PeriodStart.Equal(start) || bill.PeriodStart.Location() != time.UTC || !bill.PeriodEnd.Equal(end) || !strings.Contains(string(bill.RawSource), `"Count":"333.111"`) || !strings.Contains(string(bill.RawSource), `"PayerID":"987654321"`) {
			t.Fatalf("incorrect bill mapping: %+v", bill)
		}
	}
	if !amounts["9007199254740993.123456789"] || !amounts["-0.000000123456789"] {
		t.Fatal("lost precision or negative adjustment")
	}
	correctedClient := upstreamBillingTestClient(func(_ *http.Request) (*http.Response, error) {
		return upstreamBillingTestResponse(billingVolcengineTestEnvelope(strings.ReplaceAll(billingVolcengineTestRow, "9007199254740993.123456789", "1.23"), 0, 1)), nil
	})
	updated, err := fetchProviderBillsWithClient(context.Background(), correctedClient, cfg, start, end)
	if err != nil || len(updated) != 1 || updated[0].ID != originalID || updated[0].Amount != "1.23" {
		t.Fatalf("correction changed identity: %v %+v", err, updated)
	}
}

func TestUpstreamBillingDomesticRejectIncompleteAndWrongScope(t *testing.T) {
	start, end := billingDomesticTestMonth()
	for _, provider := range []string{"aliyun", "volcengine"} {
		baseRow := billingAliyunTestRow
		envelope := func(row string) string { return billingAliyunTestEnvelope(row, "", 1) }
		if provider == "volcengine" {
			baseRow = billingVolcengineTestRow
			envelope = func(row string) string { return billingVolcengineTestEnvelope(row, 0, 1) }
		}
		tests := map[string]string{
			"wrong owner":      envelope(strings.ReplaceAll(baseRow, `"OwnerID":"123456789"`, `"OwnerID":"987654321"`)),
			"missing owner":    envelope(strings.ReplaceAll(baseRow, `"OwnerID":"123456789",`, "")),
			"wrong product":    envelope(strings.ReplaceAll(baseRow, "verified_product", "another_product")),
			"missing currency": envelope(strings.ReplaceAll(baseRow, `"Currency":"USD",`, "")),
			"invalid currency": envelope(strings.ReplaceAll(baseRow, `"Currency":"USD"`, `"Currency":"usd"`)),
			"invalid amount":   envelope(strings.ReplaceAll(baseRow, "9007199254740993.123456789", "NaN")),
			"empty response":   `{}`,
			"oversized row":    envelope(strings.ReplaceAll(baseRow, `"OwnerID":`, `"Extra":"`+strings.Repeat("x", billingProviderMaxRawRow)+`","OwnerID":`)),
		}
		if provider == "aliyun" {
			tests["missing payable amount"] = envelope(strings.ReplaceAll(baseRow, `"PretaxAmount":9007199254740993.123456789,`, ""))
			tests["wrong month"] = strings.ReplaceAll(envelope(baseRow), "2025-01", "2025-02")
			tests["ambiguous identity"] = billingAliyunTestEnvelope(baseRow+","+baseRow, "", 2)
			tests["early cursor end"] = billingAliyunTestEnvelope(baseRow, "", 2)
			tests["empty continued page"] = billingAliyunTestEnvelope("", "cursor", 2)
			tests["null items"] = `{"Success":true,"Code":"Success","Data":{"BillingCycle":"2025-01","Items":null}}`
			tests["application error"] = `{"Success":false,"Code":"Forbidden","Message":"sensitive-secret"}`
		} else {
			tests["missing payable amount"] = envelope(strings.ReplaceAll(baseRow, `"PayableAmount":"9007199254740993.123456789",`, ""))
			tests["wrong month"] = envelope(strings.ReplaceAll(baseRow, "2025-01", "2025-02"))
			tests["missing detail id"] = envelope(strings.ReplaceAll(baseRow, `"BillDetailId":"Detail-1",`, ""))
			tests["repeated detail"] = billingVolcengineTestEnvelope(baseRow+","+baseRow, 0, 2)
			tests["wrong offset"] = billingVolcengineTestEnvelope(baseRow, 1, 1)
			tests["unreported total"] = billingVolcengineTestEnvelope(baseRow, 0, -1)
			tests["warning"] = strings.ReplaceAll(envelope(baseRow), `"Limit":300`, `"Limit":300,"Warning":"sensitive-secret"`)
			tests["null list"] = `{"ResponseMetadata":{},"Result":{"List":null,"Total":0,"Offset":0,"Limit":300}}`
			tests["application error"] = `{"ResponseMetadata":{"Error":{"Code":"AccessDenied","Message":"sensitive-secret"}}}`
		}
		for name, body := range tests {
			t.Run(provider+"/"+name, func(t *testing.T) {
				client := upstreamBillingTestClient(func(_ *http.Request) (*http.Response, error) { return upstreamBillingTestResponse(body), nil })
				bills, err := fetchProviderBillsWithClient(context.Background(), client, billingDomesticTestConfig(provider), start, end)
				if err == nil || bills != nil || strings.Contains(err.Error(), "sensitive-secret") {
					t.Fatalf("must fail closed with no partial records: %v, %+v", err, bills)
				}
			})
		}
	}
}

func TestUpstreamBillingDomesticEmptyAndPartialMonth(t *testing.T) {
	start, end := billingDomesticTestMonth()
	for _, provider := range []string{"aliyun", "volcengine"} {
		called := 0
		client := upstreamBillingTestClient(func(_ *http.Request) (*http.Response, error) {
			called++
			body := billingAliyunTestEnvelope("", "", 0)
			if provider == "volcengine" {
				body = billingVolcengineTestEnvelope("", 0, 0)
			}
			return upstreamBillingTestResponse(body), nil
		})
		bills, err := fetchProviderBillsWithClient(context.Background(), client, billingDomesticTestConfig(provider), start, end)
		if err != nil || len(bills) != 0 || called != 1 {
			t.Fatalf("valid empty month must succeed: %s %v", provider, err)
		}
		for _, date := range []time.Time{start.Add(time.Hour), start.AddDate(0, 0, 1), time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)} {
			if _, err := fetchProviderBillsWithClient(context.Background(), client, billingDomesticTestConfig(provider), date, end); err == nil || called != 1 {
				t.Fatal("partial or wrong-zone month must be rejected before network request")
			}
		}
	}
}

func TestUpstreamBillingDomesticPaginationFailureNoPartial(t *testing.T) {
	start, end := billingDomesticTestMonth()
	for _, provider := range []string{"aliyun", "volcengine"} {
		for _, failure := range []string{"http", "repeated", "changed_total"} {
			t.Run(provider+"/"+failure, func(t *testing.T) {
				pages := 0
				client := upstreamBillingTestClient(func(_ *http.Request) (*http.Response, error) {
					pages++
					body := billingAliyunTestEnvelope(billingAliyunTestRow, "cursor", 2)
					if provider == "volcengine" {
						body = billingVolcengineTestEnvelope(billingVolcengineTestRow, 0, 2)
					}
					if pages > 1 {
						switch failure {
						case "http":
							res := upstreamBillingTestResponse(`{"message":"sensitive-secret"}`)
							res.StatusCode = 403
							return res, nil
						case "changed_total":
							body = strings.ReplaceAll(strings.ReplaceAll(body, `"TotalCount":2`, `"TotalCount":3`), `"Total":2`, `"Total":3`)
						case "repeated":
							if provider == "volcengine" {
								body = billingVolcengineTestEnvelope(billingVolcengineTestRow, 1, 2)
							} else {
								body = strings.ReplaceAll(body, `"Item":"PayAsYouGoBill"`, `"Item":"Refund"`)
							}
						}
					}
					return upstreamBillingTestResponse(body), nil
				})
				bills, err := fetchProviderBillsWithClient(context.Background(), client, billingDomesticTestConfig(provider), start, end)
				if err == nil || bills != nil || pages != 2 || strings.Contains(err.Error(), "sensitive-secret") {
					t.Fatalf("must reject incomplete fetch after 2 pages: %v pages=%d bills=%d", err, pages, len(bills))
				}
			})
		}
	}
}

func TestUpstreamBillingDomesticEnvelopeRetry(t *testing.T) {
	start, end := billingDomesticTestMonth()
	for _, provider := range []string{"aliyun", "volcengine"} {
		calls := 0
		client := upstreamBillingTestClient(func(_ *http.Request) (*http.Response, error) {
			calls++
			if calls == 1 {
				body := `{"Success":false,"Code":"Throttling.User","Message":"sensitive-secret"}`
				if provider == "volcengine" {
					body = `{"ResponseMetadata":{"Error":{"Code":"Throttling","Message":"sensitive-secret"}}}`
				}
				return upstreamBillingTestResponse(body), nil
			}
			body := billingAliyunTestEnvelope("", "", 0)
			if provider == "volcengine" {
				body = billingVolcengineTestEnvelope("", 0, 0)
			}
			return upstreamBillingTestResponse(body), nil
		})
		bills, err := fetchProviderBillsWithClient(context.Background(), client, billingDomesticTestConfig(provider), start, end)
		if err != nil || len(bills) != 0 || calls != 2 {
			t.Fatalf("retry failed: %s %v calls=%d", provider, err, calls)
		}
	}
}

func TestUpstreamBillingDomesticPaginationLimits(t *testing.T) {
	start, end := billingDomesticTestMonth()
	for _, provider := range []string{"aliyun", "volcengine"} {
		t.Run(provider+"/page_limit", func(t *testing.T) {
			pages := 0
			client := upstreamBillingTestClient(func(_ *http.Request) (*http.Response, error) {
				pages++
				row := strings.ReplaceAll(billingAliyunTestRow, "Instance-Case-Sensitive", fmt.Sprintf("Instance-%d", pages))
				body := billingAliyunTestEnvelope(row, fmt.Sprintf("cursor-%d", pages), billingProviderMaxPages+1)
				if provider == "volcengine" {
					row = strings.ReplaceAll(billingVolcengineTestRow, "Detail-1", fmt.Sprintf("Detail-%d", pages))
					body = billingVolcengineTestEnvelope(row, pages-1, billingProviderMaxPages+1)
				}
				return upstreamBillingTestResponse(body), nil
			})
			bills, err := fetchProviderBillsWithClient(context.Background(), client, billingDomesticTestConfig(provider), start, end)
			if err == nil || bills != nil || pages != billingProviderMaxPages {
				t.Fatalf("must stop exactly at page limit without partial records: %v pages=%d", err, pages)
			}
		})
		t.Run(provider+"/record_limit", func(t *testing.T) {
			client := upstreamBillingTestClient(func(_ *http.Request) (*http.Response, error) {
				body := billingAliyunTestEnvelope("", "next", billingProviderMaxBills+1)
				if provider == "volcengine" {
					body = billingVolcengineTestEnvelope("", 0, billingProviderMaxBills+1)
				}
				return upstreamBillingTestResponse(body), nil
			})
			if bills, err := fetchProviderBillsWithClient(context.Background(), client, billingDomesticTestConfig(provider), start, end); err == nil || bills != nil {
				t.Fatal("must reject too many records")
			}
		})
	}
}

func TestUpstreamBillingDomesticNoResourceRemainsUnmatched(t *testing.T) {
	start, end := billingDomesticTestMonth()
	for _, provider := range []string{"aliyun", "volcengine"} {
		client := upstreamBillingTestClient(func(_ *http.Request) (*http.Response, error) {
			// BillingDate is a DAILY-only field: a monthly report remains
			// attributed by the verified envelope, never by this optional field.
			row := strings.ReplaceAll(billingAliyunTestRow, `"InstanceID":"Instance-Case-Sensitive",`, `"BillingDate":"2025-01",`)
			body := billingAliyunTestEnvelope(row, "", 1)
			if provider == "volcengine" {
				row = strings.ReplaceAll(billingVolcengineTestRow, `"InstanceNo":"Instance-Case-Sensitive",`, "")
				body = billingVolcengineTestEnvelope(row, 0, 1)
			}
			return upstreamBillingTestResponse(body), nil
		})
		bills, err := fetchProviderBillsWithClient(context.Background(), client, billingDomesticTestConfig(provider), start, end)
		if err != nil || len(bills) != 1 || bills[0].ResourceID != "" {
			t.Fatalf("must preserve missing resource as unmatched, not invent one: %v %+v", err, bills)
		}
	}
}
