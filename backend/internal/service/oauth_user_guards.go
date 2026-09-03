package service

// RejectProxyID returns ErrProxyNotAllowed when a non-nil, non-zero proxy_id is present.
// User-owned account OAuth paths must not accept proxies.
func RejectProxyID(proxyID *int64) error {
	if proxyID != nil && *proxyID != 0 {
		return ErrProxyNotAllowed
	}
	return nil
}
