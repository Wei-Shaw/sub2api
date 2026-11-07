package service

import (
	"fmt"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/pkg/packguard"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
)

func isBytePlusPackDenial(resp *http.Response) bool { return packguard.IsDenial(resp) }

func forwardBytePlusPackDenial(c *gin.Context, resp *http.Response, body []byte) error {
	MarkResponseCommitted(c)
	c.Header("Cache-Control", "no-store")
	c.Data(resp.StatusCode, "application/json", body)
	return fmt.Errorf("BytePlus prepaid guard rejected request: %d", resp.StatusCode)
}

type bytePlusPackHTTP struct {
	HTTPUpstream
	guard *packguard.Transport
}

// NewBytePlusPackHTTP requires a working ledger; it never silently unwraps guarded traffic.
func NewBytePlusPackHTTP(upstream HTTPUpstream, ledger packguard.Ledger) (HTTPUpstream, error) {
	if upstream == nil {
		return nil, fmt.Errorf("prepaid upstream transport is required")
	}
	guard, err := packguard.NewTransport(ledger)
	if err != nil {
		return nil, err
	}
	return &bytePlusPackHTTP{HTTPUpstream: upstream, guard: guard}, nil
}
func (u *bytePlusPackHTTP) Do(req *http.Request, proxy string, id int64, concurrency int) (*http.Response, error) {
	return u.guard.RoundTrip(req, id, func(r *http.Request) (*http.Response, error) {
		return u.HTTPUpstream.Do(packGuardRedirectPolicy(r), proxy, id, concurrency)
	})
}
func (u *bytePlusPackHTTP) DoWithTLS(req *http.Request, proxy string, id int64, concurrency int, profile *tlsfingerprint.Profile) (*http.Response, error) {
	return u.guard.RoundTrip(req, id, func(r *http.Request) (*http.Response, error) {
		return u.HTTPUpstream.DoWithTLS(packGuardRedirectPolicy(r), proxy, id, concurrency, profile)
	})
}

func packGuardRedirectPolicy(req *http.Request) *http.Request {
	if packguard.IsGuarded(req.Context()) {
		return req.WithContext(WithHTTPUpstreamRedirectsDisabled(req.Context()))
	}
	return req
}
