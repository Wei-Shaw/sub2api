package service

// Domestic cloud historical billing APIs. Model inference API keys cannot call
// these APIs. The account filter is verified against EVERY returned row; payer
// accounts and resource-owning accounts must not be silently interchanged.
// https://help.aliyun.com/zh/user-center/developer-reference/api-bssopenapi-2017-12-14-describeinstancebill
// https://help.aliyun.com/zh/sdk/product-overview/v3-request-structure-and-signature
// https://www.volcengine.com/docs/BillingCenter/ListBillDetail-Pagequerybilldetails
// https://www.volcengine.com/docs/BillingCenter/APIcallinstructions
// https://github.com/volcengine/volc-sdk-golang/blob/main/base/sign.go

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func billingDomesticMonth(start, end time.Time) (string, error) {
	zone := time.FixedZone("Asia/Shanghai", 8*60*60)
	local := start.In(zone)
	month := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, zone)
	if !start.Equal(month) || !end.Equal(month.AddDate(0, 1, 0)) {
		return "", errors.New("domestic cloud billing requires a complete calendar month in Asia/Shanghai")
	}
	return month.Format("2006-01"), nil
}

// Match only documented error classes; never return provider messages (which
// may echo request arguments or credentials). HTTP 429/5xx are handled by the
// common request helper independently of these HTTP-200 error envelopes.
func billingDomesticTransientError(host string, body []byte) bool {
	if host == "business.aliyuncs.com" {
		var response struct {
			Success bool   `json:"Success"`
			Code    string `json:"Code"`
		}
		if json.Unmarshal(body, &response) == nil && !response.Success {
			return response.Code == "Throttling" || strings.HasPrefix(response.Code, "Throttling.") || response.Code == "InternalError" || response.Code == "ServiceUnavailable"
		}
	}
	if host == "billing.volcengineapi.com" {
		var response struct {
			ResponseMetadata struct {
				Error *struct {
					Code string `json:"Code"`
				} `json:"Error"`
			} `json:"ResponseMetadata"`
		}
		if json.Unmarshal(body, &response) == nil && response.ResponseMetadata.Error != nil {
			code := response.ResponseMetadata.Error.Code
			return code == "Throttling" || strings.HasPrefix(code, "Throttling.") || code == "InternalError" || code == "ServiceUnavailable"
		}
	}
	return false
}

// RPC query values are RFC3986 encoded, not form encoded. AccessKey credentials
// are exclusively in signed headers; no secrets are placed in the request URL.
func billingAliyunSign(req *http.Request, accessID, secret string, now time.Time, nonce string) {
	const signedHeaders = "host;x-acs-action;x-acs-content-sha256;x-acs-date;x-acs-signature-nonce;x-acs-version"
	stamp := now.UTC().Format("2006-01-02T15:04:05Z")
	payloadHash := billingSHA256(nil)
	req.URL.RawQuery = strings.ReplaceAll(req.URL.Query().Encode(), "+", "%20")
	req.Header.Set("x-acs-date", stamp)
	req.Header.Set("x-acs-signature-nonce", nonce)
	req.Header.Set("x-acs-content-sha256", payloadHash)
	canonicalHeaders := "host:" + req.URL.Host + "\n" +
		"x-acs-action:" + req.Header.Get("x-acs-action") + "\n" +
		"x-acs-content-sha256:" + payloadHash + "\n" +
		"x-acs-date:" + stamp + "\n" +
		"x-acs-signature-nonce:" + nonce + "\n" +
		"x-acs-version:" + req.Header.Get("x-acs-version") + "\n"
	canonical := strings.Join([]string{req.Method, "/", req.URL.RawQuery, canonicalHeaders, signedHeaders, payloadHash}, "\n")
	signature := hex.EncodeToString(billingHMAC([]byte(secret), "ACS3-HMAC-SHA256\n"+billingSHA256([]byte(canonical))))
	req.Header.Set("Authorization", "ACS3-HMAC-SHA256 Credential="+accessID+",SignedHeaders="+signedHeaders+",Signature="+signature)
}

type billingAliyunRow struct {
	OwnerID          string      `json:"OwnerID"`
	BillAccountID    string      `json:"BillAccountID"`
	InstanceID       string      `json:"InstanceID"`
	ProductCode      string      `json:"ProductCode"`
	ProductType      string      `json:"ProductType"`
	PipCode          string      `json:"PipCode"`
	CommodityCode    string      `json:"CommodityCode"`
	BillingItemCode  string      `json:"BillingItemCode"`
	BillingItem      string      `json:"BillingItem"`
	Item             string      `json:"Item"`
	SubscriptionType string      `json:"SubscriptionType"`
	BillingType      string      `json:"BillingType"`
	BizType          string      `json:"BizType"`
	Currency         string      `json:"Currency"`
	PretaxAmount     json.Number `json:"PretaxAmount"`
	Region           string      `json:"Region"`
	Zone             string      `json:"Zone"`
}

func fetchAliyunProviderBills(ctx context.Context, client *http.Client, cfg BillingProviderConfig, start, end time.Time) ([]ProviderBill, error) {
	month, err := billingDomesticMonth(start, end)
	if err != nil {
		return nil, err
	}
	bills := make([]ProviderBill, 0)
	seenCursors, seenIDs := map[string]bool{}, map[string]bool{}
	cursor, rawBytes, expectedTotal := "", 0, -1
	for page := 0; page < billingProviderMaxPages; page++ {
		query := url.Values{
			"BillingCycle": {month}, "Granularity": {"MONTHLY"}, "IsBillingItem": {"true"},
			"IsHideZeroCharge": {"false"}, "MaxResults": {"300"},
			"BillOwnerId": {cfg.Settings["account_id"]}, "ProductCode": {cfg.Settings["product_code"]},
		}
		if cursor != "" {
			query.Set("NextToken", cursor)
		}
		var response struct {
			Success bool   `json:"Success"`
			Code    string `json:"Code"`
			Data    *struct {
				BillingCycle string            `json:"BillingCycle"`
				Items        []json.RawMessage `json:"Items"`
				NextToken    string            `json:"NextToken"`
				TotalCount   *int              `json:"TotalCount"`
			} `json:"Data"`
		}
		err := billingProviderRequest(ctx, client, func() (*http.Request, error) {
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://business.aliyuncs.com/?"+query.Encode(), nil)
			if err != nil {
				return nil, err
			}
			var nonce [16]byte
			if _, err := rand.Read(nonce[:]); err != nil {
				return nil, err
			}
			req.Header.Set("Accept", "application/json")
			req.Header.Set("x-acs-action", "DescribeInstanceBill")
			req.Header.Set("x-acs-version", "2017-12-14")
			billingAliyunSign(req, cfg.Secrets["access_key_id"], cfg.Secrets["access_key_secret"], time.Now(), hex.EncodeToString(nonce[:]))
			return req, nil
		}, &response)
		if err != nil {
			return nil, err
		}
		if !response.Success || response.Code != "Success" || response.Data == nil || response.Data.Items == nil {
			return nil, errors.New("Alibaba Cloud billing rejected or omitted billing data; check read-only permissions and month availability")
		}
		data := response.Data
		if data.BillingCycle != month || len(data.Items) > 300 || len(bills)+len(data.Items) > billingProviderMaxBills {
			return nil, errors.New("invalid Alibaba Cloud billing month or record count")
		}
		if data.TotalCount != nil {
			if *data.TotalCount < 0 || *data.TotalCount > billingProviderMaxBills || expectedTotal >= 0 && *data.TotalCount != expectedTotal {
				return nil, errors.New("Alibaba Cloud billing total changed or exceeds the record limit; retry later")
			}
			expectedTotal = *data.TotalCount
		}
		for _, source := range data.Items {
			var row billingAliyunRow
			if err := json.Unmarshal(source, &row); err != nil {
				return nil, errors.New("invalid Alibaba Cloud billing record")
			}
			if row.OwnerID != cfg.Settings["account_id"] || row.ProductCode != cfg.Settings["product_code"] {
				return nil, errors.New("Alibaba Cloud billing returned a different resource owner or product")
			}
			// BillingDate is documented only for DAILY requests. This MONTHLY
			// report is attributed by Data.BillingCycle, already checked above;
			// any optional row date is retained raw, not used as a daily ledger.
			amount, err := billingParseAmount(row.PretaxAmount.String())
			if err != nil || !billingCurrencyPattern.MatchString(row.Currency) {
				return nil, errors.New("Alibaba Cloud bill has no valid official payable amount or currency")
			}
			// DescribeInstanceBill is a monthly charge-item summary, not an
			// immutable line-ID API. Reject ambiguous collisions instead of
			// inventing identities based on mutable cost amounts or row order.
			id := billingBucketID("aliyun", month, row.OwnerID, row.BillAccountID, row.InstanceID,
				row.ProductCode, row.ProductType, row.PipCode, row.CommodityCode, row.BillingItemCode,
				row.Item, row.SubscriptionType, row.BillingType, row.BizType, row.Currency, row.Region, row.Zone)
			if seenIDs[id] {
				return nil, errors.New("Alibaba Cloud billing returned repeated or ambiguous charge-item buckets")
			}
			seenIDs[id] = true
			raw, err := billingRawCostSource(source)
			if err != nil {
				return nil, err
			}
			rawBytes += len(raw)
			if rawBytes > billingProviderMaxRawTotal {
				return nil, errors.New("Alibaba Cloud raw billing records exceed the size limit")
			}
			bills = append(bills, ProviderBill{
				ID: id, ResourceID: row.InstanceID,
				Description: billingDescription(strings.Join([]string{"Alibaba Cloud payable", row.ProductCode, row.BillingItem, row.Item}, " / ")),
				Amount:      amount.String(), Currency: row.Currency, PeriodStart: start.UTC(), PeriodEnd: end.UTC(),
				RawSource: raw, UsageStatus: "unsupported",
			})
		}
		if data.NextToken == "" {
			if expectedTotal >= 0 && len(bills) != expectedTotal {
				return nil, errors.New("Alibaba Cloud billing pagination did not return the complete total")
			}
			return bills, nil
		}
		if len(data.Items) == 0 || len(data.NextToken) > 4096 || seenCursors[data.NextToken] || expectedTotal >= 0 && len(bills) >= expectedTotal {
			return nil, errors.New("invalid or repeated Alibaba Cloud billing cursor")
		}
		seenCursors[data.NextToken] = true
		cursor = data.NextToken
	}
	return nil, errors.New("Alibaba Cloud billing pagination exceeds the page limit")
}

func billingVolcengineSign(req *http.Request, body []byte, accessID, secret string, now time.Time) {
	const signedHeaders = "content-type;host;x-content-sha256;x-date"
	stamp := now.UTC().Format("20060102T150405Z")
	date := now.UTC().Format("20060102")
	payloadHash := billingSHA256(body)
	req.URL.RawQuery = strings.ReplaceAll(req.URL.Query().Encode(), "+", "%20")
	req.Header.Set("X-Date", stamp)
	req.Header.Set("X-Content-Sha256", payloadHash)
	canonicalHeaders := "content-type:" + req.Header.Get("Content-Type") + "\n" +
		"host:" + req.URL.Host + "\n" + "x-content-sha256:" + payloadHash + "\n" + "x-date:" + stamp + "\n"
	canonical := strings.Join([]string{req.Method, "/", req.URL.RawQuery, canonicalHeaders, signedHeaders, payloadHash}, "\n")
	scope := date + "/cn-north-1/billing/request"
	signingKey := billingHMAC([]byte(secret), date)
	for _, part := range []string{"cn-north-1", "billing", "request"} {
		signingKey = billingHMAC(signingKey, part)
	}
	signature := hex.EncodeToString(billingHMAC(signingKey, "HMAC-SHA256\n"+stamp+"\n"+scope+"\n"+billingSHA256([]byte(canonical))))
	req.Header.Set("Authorization", "HMAC-SHA256 Credential="+accessID+"/"+scope+", SignedHeaders="+signedHeaders+", Signature="+signature)
}

func fetchVolcengineProviderBills(ctx context.Context, client *http.Client, cfg BillingProviderConfig, start, end time.Time) ([]ProviderBill, error) {
	month, err := billingDomesticMonth(start, end)
	if err != nil {
		return nil, err
	}
	owner, _ := strconv.ParseInt(cfg.Settings["account_id"], 10, 64) // validated before dispatch
	bills, seenIDs := make([]ProviderBill, 0), map[string]bool{}
	rawBytes, offset, expectedTotal := 0, 0, -1
	for page := 0; page < billingProviderMaxPages; page++ {
		payload, _ := json.Marshal(map[string]any{
			"BillPeriod": month, "OwnerID": []int64{owner}, "Product": []string{cfg.Settings["product_code"]},
			"Limit": 300, "Offset": offset, "NeedRecordNum": 1, "IgnoreZero": 0,
			"GroupPeriod": 2, "GroupTerm": 0, // original charge-item detail, with BillDetailId
		})
		var response struct {
			ResponseMetadata *struct {
				Error *struct {
					Code string `json:"Code"`
				} `json:"Error"`
			} `json:"ResponseMetadata"`
			Result *struct {
				List    []json.RawMessage `json:"List"`
				Total   *int              `json:"Total"`
				Offset  *int              `json:"Offset"`
				Limit   *int              `json:"Limit"`
				Warning string            `json:"Warning"`
			} `json:"Result"`
		}
		err := billingProviderRequest(ctx, client, func() (*http.Request, error) {
			req, err := billingJSONRequest(ctx, http.MethodPost, "https://billing.volcengineapi.com/?Action=ListBillDetail&Version=2022-01-01", payload, map[string]string{"Content-Type": "application/json; charset=utf-8"})
			if err != nil {
				return nil, err
			}
			billingVolcengineSign(req, payload, cfg.Secrets["access_key_id"], cfg.Secrets["access_key_secret"], time.Now())
			return req, nil
		}, &response)
		if err != nil {
			return nil, err
		}
		if response.ResponseMetadata == nil || response.ResponseMetadata.Error != nil || response.Result == nil || response.Result.List == nil {
			return nil, errors.New("Volcengine billing rejected or omitted billing data; check read-only permissions and month availability")
		}
		result := response.Result
		if result.Warning != "" || result.Total == nil || *result.Total < 0 || *result.Total > billingProviderMaxBills || expectedTotal >= 0 && *result.Total != expectedTotal {
			return nil, errors.New("Volcengine billing is incomplete, changed, or exceeds the record limit; retry later")
		}
		expectedTotal = *result.Total
		if result.Offset == nil || *result.Offset != offset || result.Limit == nil || *result.Limit != 300 || len(result.List) > 300 || offset+len(result.List) > expectedTotal {
			return nil, errors.New("invalid Volcengine billing pagination metadata")
		}
		for _, source := range result.List {
			var row struct {
				BillDetailID  string `json:"BillDetailId"`
				BillPeriod    string `json:"BillPeriod"`
				OwnerID       string `json:"OwnerID"`
				Product       string `json:"Product"`
				InstanceNo    string `json:"InstanceNo"`
				ChargeItem    string `json:"ChargeItem"`
				BillCategory  string `json:"BillCategory"`
				Currency      string `json:"Currency"`
				PayableAmount string `json:"PayableAmount"`
			}
			if err := json.Unmarshal(source, &row); err != nil {
				return nil, errors.New("invalid Volcengine billing record")
			}
			if row.OwnerID != cfg.Settings["account_id"] || row.Product != cfg.Settings["product_code"] || row.BillPeriod != month {
				return nil, errors.New("Volcengine billing returned a different owner, product, or billing month")
			}
			if row.BillDetailID == "" || len(row.BillDetailID) > 512 || seenIDs[row.BillDetailID] {
				return nil, errors.New("Volcengine billing omitted or repeated a detail ID")
			}
			seenIDs[row.BillDetailID] = true
			amount, err := billingParseAmount(row.PayableAmount)
			if err != nil || !billingCurrencyPattern.MatchString(row.Currency) {
				return nil, errors.New("Volcengine bill has no valid official payable amount or currency")
			}
			raw, err := billingRawCostSource(source)
			if err != nil {
				return nil, err
			}
			rawBytes += len(raw)
			if rawBytes > billingProviderMaxRawTotal {
				return nil, errors.New("Volcengine raw billing records exceed the size limit")
			}
			bills = append(bills, ProviderBill{
				ID: billingBucketID("volcengine", month, row.OwnerID, row.Product, row.BillDetailID), ResourceID: row.InstanceNo,
				Description: billingDescription(strings.Join([]string{"Volcengine payable", row.Product, row.ChargeItem, row.BillCategory}, " / ")),
				Amount:      amount.String(), Currency: row.Currency, PeriodStart: start.UTC(), PeriodEnd: end.UTC(),
				RawSource: raw, UsageStatus: "unsupported",
			})
		}
		offset += len(result.List)
		if offset == expectedTotal {
			return bills, nil
		}
		if len(result.List) == 0 {
			return nil, errors.New("Volcengine billing pagination ended before the complete total")
		}
	}
	return nil, errors.New("Volcengine billing pagination exceeds the page limit")
}
