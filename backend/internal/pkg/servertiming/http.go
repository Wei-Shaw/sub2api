package servertiming

import (
	"context"
	"net/http"
	"strings"
	"time"
)

type dependencyModuleKey struct{}

type timingRoundTripper struct {
	base http.RoundTripper
}

// WithDependencyModule overrides the safe module name used for an outbound call.
func WithDependencyModule(ctx context.Context, module string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	module = strings.TrimPrefix(normalizeMetricName(module), dependencyPrefix)
	if module == "" {
		return ctx
	}
	return context.WithValue(ctx, dependencyModuleKey{}, module)
}

// WrapRoundTripper records outbound response-header latency for active requests.
func WrapRoundTripper(base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	if _, ok := base.(*timingRoundTripper); ok {
		return base
	}
	return &timingRoundTripper{base: base}
}

// InstrumentClient returns a shallow client copy with an instrumented transport.
func InstrumentClient(client *http.Client) *http.Client {
	if client == nil {
		client = &http.Client{}
	}
	copyClient := *client
	copyClient.Transport = WrapRoundTripper(copyClient.Transport)
	return &copyClient
}

// Do records response-header latency without changing the client's transport
// type. Use it for clients whose callers inspect or configure *http.Transport.
func Do(client *http.Client, req *http.Request) (*http.Response, error) {
	if client == nil {
		client = http.DefaultClient
	}
	if req == nil || !Active(req.Context()) {
		return client.Do(req)
	}
	startedAt := time.Now()
	response, err := client.Do(req)
	RecordDependency(req.Context(), dependencyModule(req), startedAt, time.Now())
	return response, err
}

func (t *timingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil || !Active(req.Context()) {
		return t.base.RoundTrip(req)
	}
	startedAt := time.Now()
	response, err := t.base.RoundTrip(req)
	RecordDependency(req.Context(), dependencyModule(req), startedAt, time.Now())
	return response, err
}

func dependencyModule(req *http.Request) string {
	if req != nil {
		if module, ok := req.Context().Value(dependencyModuleKey{}).(string); ok && module != "" {
			return module
		}
	}
	if req == nil || req.URL == nil {
		return "http"
	}
	host := strings.ToLower(req.URL.Hostname())
	switch {
	case strings.Contains(host, "github"):
		return "github"
	case strings.Contains(host, "openai"):
		return "openai"
	case strings.Contains(host, "anthropic"):
		return "anthropic"
	case strings.Contains(host, "generativelanguage") || strings.Contains(host, "gemini"):
		return "gemini"
	case strings.Contains(host, "cloudcode") || strings.Contains(host, "antigravity"):
		return "antigravity"
	case strings.Contains(host, "googleapis") || strings.Contains(host, "google"):
		return "google"
	case strings.Contains(host, "amazonaws") || strings.Contains(host, "cloudflarestorage") || strings.Contains(host, "s3"):
		return "s3"
	case strings.Contains(host, "stripe") || strings.Contains(host, "airwallex") || strings.Contains(host, "alipay") || strings.Contains(host, "wechatpay") || strings.Contains(host, "paypal"):
		return "payment"
	default:
		return "http"
	}
}
