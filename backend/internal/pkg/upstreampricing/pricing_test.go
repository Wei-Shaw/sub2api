package upstreampricing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func ptr(s string) *string { return &s }
func payload(t *testing.T, s string) map[string]any {
	t.Helper()
	d := json.NewDecoder(strings.NewReader(s))
	d.UseNumber()
	var v map[string]any
	require.NoError(t, d.Decode(&v))
	return v
}
func fixtureCatalog() Catalog {
	return Catalog{Group: Group{Name: "paid", Multiplier: "0.5"}, Models: []Price{{Model: "mapped", Mode: "token", Input: ptr("2"), Output: ptr("8"), CacheRead: ptr("0.2"), CacheWrite: ptr("3"), CacheWrite1h: ptr("6"), CacheWrite30m: ptr("4"), CacheWrite12h: ptr("7")}}}
}

func TestUpstreamPricingCalculate(t *testing.T) {
	cat := fixtureCatalog()
	u := Usage{Model: "mapped", Input: 100, Output: 20, CacheRead: 200, CacheWrite: 100, CacheWrite5m: 25, CacheWrite1h: 25, CacheWrite30m: 25, CacheWrite12h: 25}
	e := Calculate(cat, u)
	require.Equal(t, "estimated", e.Status)
	require.Equal(t, "0.00045", *e.Amount)
	tests := []struct {
		name   string
		change func(*Catalog, *Usage)
		reason string
	}{
		{"missing_model", func(c *Catalog, u *Usage) { u.Model = "client-model" }, "目录"},
		{"unknown_cache_price", func(c *Catalog, u *Usage) { c.Models[0].CacheRead = nil }, "缺少单价"},
		{"unknown_hour_price", func(c *Catalog, u *Usage) { c.Models[0].CacheWrite1h = nil }, "缺少单价"},
		{"negative", func(c *Catalog, u *Usage) { u.Input = -1 }, "负值"},
		{"inconsistent_ttl", func(c *Catalog, u *Usage) { u.CacheWrite = 99 }, "不一致"},
		{"extra_units", func(c *Catalog, u *Usage) { u.ExtraUnits = true }, "尚未适配"},
		{"priority", func(c *Catalog, u *Usage) { u.ServiceTier = "priority" }, "服务层级"},
		{"invalid_ratio", func(c *Catalog, u *Usage) { c.Group.Multiplier = "-1" }, "倍率"},
		{"unsupported", func(c *Catalog, u *Usage) { c.Models[0].Mode = "unavailable"; c.Models[0].Reason = "unsupported" }, "unsupported"},
		{"empty", func(c *Catalog, u *Usage) { *u = Usage{Model: "mapped"} }, "缺少上游"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := fixtureCatalog()
			copy := u
			tt.change(&c, &copy)
			e := Calculate(c, copy)
			require.Equal(t, "unavailable", e.Status)
			require.Nil(t, e.Amount)
			require.Contains(t, e.Reason, tt.reason)
		})
	}
	cat.Models[0].CacheRead = ptr("0")
	e = Calculate(cat, u)
	require.Equal(t, "estimated", e.Status)
	require.Equal(t, "0.00043", *e.Amount)
	cat.Models[0] = Price{Model: "mapped", Mode: "per_request", Request: ptr("0.01")}
	e = Calculate(cat, Usage{Model: "mapped"})
	require.Equal(t, "0.005", *e.Amount)
	cat.Group.Multiplier = "0"
	e = Calculate(cat, Usage{Model: "mapped"})
	require.Equal(t, "0", *e.Amount)
	cat = fixtureCatalog()
	cat.Group.Multiplier = "1"
	cat.Models[0].Input = ptr("0.000000000000000001")
	e = Calculate(cat, Usage{Model: "mapped", Input: 1})
	require.Equal(t, "0.000000000000000000000001", *e.Amount)
}

func TestUpstreamPricingNewAPINormalization(t *testing.T) {
	v := payload(t, `{"pricing_version":"v1","group_ratio":{"paid":9},"data":[{"model_name":"mapped","quota_type":0,"model_ratio":1.25,"completion_ratio":4,"cache_ratio":0.1,"create_cache_ratio":1.5,"enable_groups":["paid"]},{"model_name":"other-group","quota_type":0,"enable_groups":["other"]},{"model_name":"single","quota_type":1,"model_price":0.02,"enable_groups":["all"]},{"model_name":"expr","quota_type":0,"billing_mode":"tiered_expr","enable_groups":["paid"]}]}`)
	c, err := NormalizeNewAPI(v, Group{Name: "paid", Multiplier: "0.5"})
	require.NoError(t, err)
	require.Equal(t, 2, c.Supported)
	require.Equal(t, 1, c.Unsupported)
	require.Equal(t, "0.5", c.Group.Multiplier)
	p := c.Models[1]
	require.Equal(t, "mapped", p.Model)
	require.Equal(t, "2.5", *p.Input)
	require.Equal(t, "10", *p.Output)
	require.Equal(t, "0.25", *p.CacheRead)
	require.Equal(t, "3.75", *p.CacheWrite)
	manual := Price{Model: "mapped", Mode: "token", Input: ptr("0"), Output: ptr("1")}
	require.NoError(t, finalize(&c, []Price{manual}))
	require.True(t, c.Models[1].Manual)
	require.Equal(t, "0", *c.Models[1].Input)
	hash := c.Fingerprint
	require.NoError(t, finalize(&c, []Price{manual}))
	require.Equal(t, hash, c.Fingerprint)
	duplicate := payload(t, `{"data":[{"model_name":"x","quota_type":1,"model_price":1,"enable_groups":["paid"]},{"model_name":"x","quota_type":1,"model_price":1,"enable_groups":["paid"]}]}`)
	_, err = NormalizeNewAPI(duplicate, Group{Name: "paid", Multiplier: "1"})
	require.Error(t, err)
}

func TestUpstreamPricingSub2APINormalization(t *testing.T) {
	v := payload(t, `{"data":{"groups":[{"id":5,"name":"paid","models":[{"name":"mapped","pricing":{"billing_mode":"token","input_price":0.000002,"output_price":0.000008,"cache_read_price":2e-7,"cache_write_price":0.000003,"cache_write_1h_price":0.000006,"cache_write_12h_price":0.000007}},{"name":"reference-only","official_pricing":{"input_price":1}},{"name":"time","time_pricing":{"periods":[{"multiplier":2}]},"pricing":{"billing_mode":"token","input_price":1,"output_price":1}},{"name":"tiered","pricing":{"billing_mode":"token","input_price":1,"output_price":1,"intervals":[{"min_tokens":0,"max_tokens":100}]}},{"name":"effort","pricing":{"billing_mode":"token","input_price":1,"output_price":1,"reasoning_effort_multipliers":{"high":2}}}]}]}}`)
	c, err := NormalizeSub2API(v, Group{ID: "5", Name: "paid", Multiplier: "0.4"})
	require.NoError(t, err)
	require.Equal(t, 1, c.Supported)
	require.Equal(t, 4, c.Unsupported)
	var p Price
	for _, v := range c.Models {
		if v.Model == "mapped" {
			p = v
		}
	}
	require.Equal(t, "2", *p.Input)
	require.Equal(t, "0.2", *p.CacheRead)
	require.Equal(t, "6", *p.CacheWrite1h)
	_, err = NormalizeSub2API(v, Group{ID: "6", Name: "paid", Multiplier: "1"})
	require.Error(t, err)
	_, err = NormalizeSub2API(v, Group{ID: "5", Name: "other", Multiplier: "1"})
	require.Error(t, err)
}

func TestUpstreamPricingDecimalAndValidation(t *testing.T) {
	for _, v := range []any{json.Number("1e-100000000"), json.Number("1e-30"), json.Number("-1"), json.Number("1000000000001"), "NaN", "1/2", "0.0000000000000000001"} {
		_, err := number(v)
		require.Error(t, err)
	}
	p, err := number(json.Number("2e-7"))
	require.NoError(t, err)
	require.Equal(t, "0.0000002", *p)
	for _, address := range []string{"https://user:password@example.com", "https://example.com?token=x", "file:///tmp/pricing", "http://example.com", "https://example.com#token"} {
		c := Config{Provider: "newapi", BaseURL: address}
		require.Error(t, ValidateConfig(&c))
	}
	for _, address := range []string{"https://example.com/relay", "http://127.0.0.1:8080", "http://localhost:8080"} {
		c := Config{Provider: "sub2api", BaseURL: address}
		require.NoError(t, ValidateConfig(&c))
		require.Equal(t, 60, c.IntervalMinutes)
	}
	message, status := PublicMessage(errors.New("database error with synthetic credential"))
	require.Equal(t, 503, status)
	require.NotContains(t, message, "credential")
}

func TestUpstreamPricingDoesNotRoundUnsupportedConvertedPricesToFree(t *testing.T) {
	v := payload(t, `{"data":[{"model_name":"tiny","quota_type":0,"model_ratio":0.000000000000000001,"completion_ratio":0.000000000000000001,"enable_groups":["paid"]}]}`)
	c, err := NormalizeNewAPI(v, Group{Name: "paid", Multiplier: "1"})
	require.NoError(t, err)
	require.Equal(t, 1, c.Unsupported)
	require.Equal(t, "unavailable", c.Models[0].Mode)
}

func TestUpstreamPricingOpenAI30mPriceCard(t *testing.T) {
	v := payload(t, `{"data":{"groups":[{"id":5,"name":"paid","models":[{"name":"mapped","platform":"openai","pricing":{"billing_mode":"token","input_price":0.000002,"output_price":0.000008,"cache_write_price":0.000003,"cache_write_12h_price":0.000007}}]}]}}`)
	c, err := NormalizeSub2API(v, Group{ID: "5", Name: "paid", Multiplier: "1"})
	require.NoError(t, err)
	require.Equal(t, "3", *c.Models[0].CacheWrite30m)
	e := Calculate(c, Usage{Model: "mapped", Input: 1000000, CacheWrite: 1000000, CacheWrite30m: 500000, CacheWrite12h: 500000})
	require.Equal(t, "estimated", e.Status)
	require.Equal(t, "7", *e.Amount)
}

func TestUpstreamPricingRemoteDoesNotFollowRedirectOrEchoBody(t *testing.T) {
	visited := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { visited = true }))
	defer target.Close()
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, http.StatusFound) }))
	defer site.Close()
	r := NewRemote("")
	_, err := r.get(context.Background(), Config{BaseURL: site.URL, Token: "synthetic-panel-credential"}, "/api/pricing")
	require.Error(t, err)
	require.False(t, visited)
	require.NotContains(t, err.Error(), "synthetic-panel-credential")
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"success":false,"message":"synthetic-panel-credential"}`))
	}))
	defer bad.Close()
	_, err = r.get(context.Background(), Config{BaseURL: bad.URL, Token: "synthetic-panel-credential"}, "/api/pricing")
	require.Error(t, err)
	require.NotContains(t, err.Error(), "synthetic-panel-credential")
}

func TestUpstreamPricingRemoteGroupsAndExactIdentity(t *testing.T) {
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer synthetic-panel-credential", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/groups/available":
			require.Equal(t, "Asia/Shanghai", r.URL.Query().Get("timezone"))
			require.Equal(t, "1", r.Header.Get("X-User-UI-Request"))
			_, _ = w.Write([]byte(`{"data":[{"id":5,"name":"paid","rate_multiplier":9}]}`))
		case "/api/v1/groups/rates":
			_, _ = w.Write([]byte(`{"data":{"5":0.5}}`))
		case "/api/v1/model-plaza":
			_, _ = w.Write([]byte(`{"data":{"groups":[{"id":5,"name":"paid","models":[{"name":"mapped","pricing":{"billing_mode":"token","input_price":0.000002,"output_price":0.000008}}]}]}}`))
		default:
			w.WriteHeader(404)
		}
	}))
	defer site.Close()
	r := NewRemote("")
	c := Config{Provider: "sub2api", BaseURL: site.URL, Token: "synthetic-panel-credential", Group: "paid", GroupID: "5"}
	cat, err := r.Fetch(context.Background(), c)
	require.NoError(t, err)
	require.Equal(t, "0.5", cat.Group.Multiplier)
	c.GroupID = "6"
	_, err = r.Fetch(context.Background(), c)
	require.Error(t, err)
}

func TestUpstreamPricingNewAPIAccessTokenMultiplier(t *testing.T) {
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer synthetic-panel-token", r.Header.Get("Authorization"))
		require.Equal(t, "123", r.Header.Get("New-Api-User"))
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/user/self/groups":
			fmt.Fprint(w, `{"success":true,"data":{"vip":{"ratio":0.15},"astra":{"ratio":0.27}}}`)
		case "/api/pricing":
			// The authenticated user rate wins over a public catalog rate.
			fmt.Fprint(w, `{"success":true,"group_ratio":{"astra":9},"data":[{"model_name":"gpt","quota_type":0,"model_ratio":1,"completion_ratio":4,"enable_groups":["astra"]}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer site.Close()
	remote := NewRemote("")
	cfg := Config{Provider: "newapi", BaseURL: site.URL, Token: "synthetic-panel-token", UserID: "123", Group: "astra"}
	groups, err := remote.Groups(context.Background(), cfg)
	require.NoError(t, err)
	require.ElementsMatch(t, []Group{{Name: "vip", Multiplier: "0.15"}, {Name: "astra", Multiplier: "0.27"}}, groups)
	cat, err := remote.Fetch(context.Background(), cfg)
	require.NoError(t, err)
	require.Equal(t, "0.27", cat.Group.Multiplier)
	estimate := Calculate(cat, Usage{Model: "gpt", Input: 1000000})
	require.Equal(t, "estimated", estimate.Status)
	require.Equal(t, "0.54", *estimate.Amount)
}
