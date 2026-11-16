package service

// UsageProxySnapshot records the selected proxy identity without probing its
// exit IP or storing its address and credentials.
type UsageProxySnapshot struct {
	ID   int64  `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

func usageProxySnapshot(account *Account) *UsageProxySnapshot {
	if account == nil {
		return nil
	}
	if account.Proxy == nil || account.ProxyID == nil {
		return &UsageProxySnapshot{}
	}
	return &UsageProxySnapshot{ID: account.Proxy.ID, Name: account.Proxy.Name}
}
