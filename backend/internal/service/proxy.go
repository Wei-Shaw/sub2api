package service

import (
	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// Fallback mode aliases keep existing service call sites compiling.
const (
	FallbackModeNone   = domain.FallbackModeNone
	FallbackModeProxy  = domain.FallbackModeProxy
	FallbackModeDirect = domain.FallbackModeDirect
)

// Type aliases keep existing service call sites compiling while the proxy BC
// owns its domain types. Mirror of redeem/promo/announcement.
type Proxy = domain.Proxy
type ProxyWithAccountCount = domain.ProxyWithAccountCount
type ProxyAccountSummary = domain.ProxyAccountSummary
