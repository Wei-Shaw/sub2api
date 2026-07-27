package service

import "testing"

// TestAccountProxyURL 覆盖 Account.ProxyURL() 的全部分支。
//
// 该方法是账号代理取值的唯一入口（替代原先散落在 58 处的
// `if account.ProxyID != nil && account.Proxy != nil { account.Proxy.URL() }` 写法）。
func TestAccountProxyURL(t *testing.T) {
	t.Parallel()

	proxyID := int64(7)

	tests := []struct {
		name    string
		account *Account
		want    string
	}{
		{
			name:    "nil 接收者返回空串且不 panic",
			account: nil,
			want:    "",
		},
		{
			name:    "未绑定代理返回空串",
			account: &Account{ID: 1},
			want:    "",
		},
		{
			name: "ProxyID 非空但 Proxy 未 hydrate 时返回空串",
			account: &Account{
				ID:      1,
				ProxyID: &proxyID,
			},
			want: "",
		},
		{
			name: "无认证代理",
			account: &Account{
				ID:      1,
				ProxyID: &proxyID,
				Proxy: &Proxy{
					ID:       proxyID,
					Protocol: "http",
					Host:     "proxy.example.com",
					Port:     3128,
				},
			},
			want: "http://proxy.example.com:3128",
		},
		{
			name: "带认证代理",
			account: &Account{
				ID:      1,
				ProxyID: &proxyID,
				Proxy: &Proxy{
					ID:       proxyID,
					Protocol: "socks5",
					Host:     "proxy.example.com",
					Port:     1080,
					Username: "user",
					Password: "pass",
				},
			},
			want: "socks5://user:pass@proxy.example.com:1080",
		},
		{
			// 前向兼容：代理池模式下账号可能只有 Proxy 而无 ProxyID。
			// 本用例锁定该语义，防止后续有人给守卫加回 ProxyID != nil 判断。
			name: "仅有 Proxy 而无 ProxyID 时仍返回代理 URL",
			account: &Account{
				ID: 1,
				Proxy: &Proxy{
					ID:       proxyID,
					Protocol: "http",
					Host:     "pool-member.example.com",
					Port:     8080,
				},
			},
			want: "http://pool-member.example.com:8080",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.account.ProxyURL(); got != tc.want {
				t.Fatalf("Account.ProxyURL() mismatch: got=%q want=%q", got, tc.want)
			}
		})
	}
}
