package byteplusbilling

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func fixture(t *testing.T) (*Client, *int, *string, *bool) {
	t.Helper()
	calls := 0
	failure := ""
	mismatch := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		action := r.URL.Query().Get("Action")
		if r.Method != "POST" || r.URL.Query().Get("Version") != "2022-01-01" || !strings.HasPrefix(r.Header.Get("Authorization"), "HMAC-SHA256 Credential=fixture-access/") {
			t.Error("unsigned or invalid billing read")
		}
		if failure == action {
			w.WriteHeader(403)
			_, _ = w.Write([]byte("private-provider-error-fixture"))
			return
		}
		result := "{}"
		switch action {
		case "QueryBalanceAcct":
			id := 123
			if mismatch {
				id = 456
			}
			result = fmt.Sprintf(`{"AccountID":%d,"AvailableBalance":"12.5000","Currency":"USD"}`, id)
		case "ListResourcePackages":
			var in map[string]any
			_ = json.NewDecoder(r.Body).Decode(&in)
			if in["ResourceType"] != "Package" {
				t.Error("unexpected resource type")
			}
			next := `"next"`
			id := "pack-a"
			if in["NextToken"] == "next" {
				next = `""`
				id = "pack-b"
			}
			result = fmt.Sprintf(`{"List":[{"InstanceNo":%q,"AvailableAmount":"0.015625","Unit":"Token","ExpiryTime":"2030-01-01T23:59:59+08:00","ConfigurationCode":"fixture-seedance"}],"NextToken":%s}`, id, next)
		case "ListCoupons":
			result = `{"List":[{"CouponID":"coupon-a","RemainingAmount":1.2500,"ExpiredTime":"2030-02-01T00:00:00Z","ProductLimitList":[{"ProductCode":"ark","BillingMethod":"postpaid"}]}],"Total":1}`
		default:
			t.Errorf("unexpected/inference action %s", action)
		}
		_, _ = fmt.Fprintf(w, `{"ResponseMetadata":{},"Result":%s}`, result)
	}))
	t.Cleanup(srv.Close)
	c := newClient()
	c.endpoint = srv.URL + "/"
	c.configs["pool"] = Config{PoolID: "pool", AccountID: "123", AccessKey: "fixture-access", SecretKey: "fixture-secret", Region: "ap-southeast-1", ResourceTypes: []string{"Package"}}
	return c, &calls, &failure, &mismatch
}
func TestQueryPaginationPrecisionCooldownAndStaleSources(t *testing.T) {
	c, calls, failure, mismatch := fixture(t)
	now := time.Now()
	c.now = func() time.Time { return now }
	if c.Read("pool").Balance.State != "never_queried" || *calls != 0 {
		t.Fatal("passive read called provider")
	}
	s := c.Query(context.Background(), "pool")
	if *calls != 4 || s.Packages.State != "ok" || s.Coupons.State != "ok" || !strings.Contains(string(s.Packages.Data), "0.015625") || !strings.Contains(string(s.Coupons.Data), "1.2500") {
		t.Fatalf("incomplete/exact observation: %+v calls %d", s, *calls)
	}
	_ = c.Query(context.Background(), "pool")
	if *calls != 4 {
		t.Fatal("cooldown did not coalesce")
	}
	old := *s.Packages.ObservedAt
	*failure = "ListResourcePackages"
	now = now.Add(cooldown + time.Second)
	s = c.Query(context.Background(), "pool")
	if s.Packages.State != "forbidden" || !s.Packages.ObservedAt.Equal(old) || !strings.Contains(string(s.Packages.Data), "pack-a") || s.Balance.State != "ok" || s.Coupons.State != "ok" {
		t.Fatal("partial failure erased data or falsified freshness")
	}
	raw, _ := json.Marshal(s)
	if strings.Contains(string(raw), "fixture-secret") || strings.Contains(string(raw), "private-provider-error") {
		t.Fatal("secret/provider error leaked")
	}
	*failure = ""
	*mismatch = true
	now = now.Add(cooldown + time.Second)
	before := *calls
	s = c.Query(context.Background(), "pool")
	if *calls != before+1 || s.Balance.State != "account_mismatch" || s.Packages.Data != nil || s.Coupons.Data != nil {
		t.Fatal("owner mismatch exposed assets")
	}
}
func TestMissingCredentialsAndConfigFile(t *testing.T) {
	t.Setenv(ConfigEnv, "")
	c := New()
	s := c.Query(context.Background(), "pool")
	if s.Configured || s.Setup != "missing_credentials" || s.Balance.ObservedAt != nil {
		t.Fatal("missing credentials became a zero balance")
	}
	p := filepath.Join(t.TempDir(), "billing.json")
	_ = os.WriteFile(p, []byte(`[{"pool_id":"pool","account_id":"123","access_key":"fixture-ak","secret_key":"fixture-sk"}]`), 0600)
	t.Setenv(ConfigEnv, p)
	if !New().Read("pool").Configured || New().Read("other").Configured {
		t.Fatal("pool isolation failed")
	}
	_ = os.Chmod(p, 0644)
	if New().Read("pool").Setup != "invalid_configuration" {
		t.Fatal("world-readable credentials accepted")
	}
}
func TestConcurrentQueriesCoalesce(t *testing.T) {
	c, _, _, _ := fixture(t)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if c.Query(context.Background(), "pool").Packages.State != "ok" {
				t.Error("query incomplete")
			}
		}()
	}
	wg.Wait()
}
func TestMalformedAndIncompleteResponsesDoNotLookEmpty(t *testing.T) {
	for _, body := range []string{`{"Result":{}}`, `{"Result":{"List":[],"NextToken":"again"}}`, `{"Result":{"List":[{"InstanceNo":"x","AvailableAmount":"NaN","ExpiryTime":"2030-01-01T00:00:00Z","Unit":"Token"}]}}`} {
		t.Run(body, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(body)) }))
			defer srv.Close()
			c := newClient()
			c.endpoint = srv.URL + "/"
			data, state := c.packages(context.Background(), Config{Region: "ap-southeast-1", ResourceTypes: []string{"Package"}})
			if state == "ok" || data != nil {
				t.Fatal("failed/incomplete pagination became successful empty result")
			}
		})
	}
}

// Expected signatures were independently generated with the official BytePlus
// SDK base.Sign4, using the same fixed request and synthetic credentials.
func TestSigningMatchesOfficialSDK(t *testing.T) {
	for _, tc := range []struct{ token, signature string }{{"", "490d613ea5d1ac28bc01f295d065932f2671807a8c4467241f1ecc39e8997c76"}, {"fixture-token", "65214ca90bb548b31b65297577b5afa5a715343abc546a82568dc622a61f7e40"}} {
		req, _ := http.NewRequest("POST", "https://open.byteplusapi.com/?Action=QueryBalanceAcct&Version=2022-01-01", strings.NewReader("{}"))
		now, _ := time.Parse(time.RFC3339, "2026-11-04T10:00:00Z")
		sign(req, []byte("{}"), Config{AccessKey: "fixture-access", SecretKey: "fixture-secret", Region: "ap-southeast-1", SessionToken: tc.token}, now)
		if !strings.HasSuffix(req.Header.Get("Authorization"), "Signature="+tc.signature) {
			t.Fatalf("signature differs from official SDK: %s", req.Header.Get("Authorization"))
		}
	}
}
