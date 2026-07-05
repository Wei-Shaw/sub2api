package provider

import "testing"

func TestEasyPayResolveType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		config      map[string]string
		paymentType string
		want        string
	}{
		{
			name:        "alipay with override",
			config:      map[string]string{"typeAlipay": "alipay_h5"},
			paymentType: "alipay",
			want:        "alipay_h5",
		},
		{
			name:        "alipay_direct with override",
			config:      map[string]string{"typeAlipay": "alipay_h5"},
			paymentType: "alipay_direct",
			want:        "alipay_h5",
		},
		{
			name:        "alipay no override",
			config:      map[string]string{},
			paymentType: "alipay",
			want:        "alipay",
		},
		{
			name:        "wxpay with override",
			config:      map[string]string{"typeWxpay": "wxpay_h5"},
			paymentType: "wxpay",
			want:        "wxpay_h5",
		},
		{
			name:        "wxpay_direct with override",
			config:      map[string]string{"typeWxpay": "wxpay_h5"},
			paymentType: "wxpay_direct",
			want:        "wxpay_h5",
		},
		{
			name:        "wxpay no override",
			config:      map[string]string{},
			paymentType: "wxpay",
			want:        "wxpay",
		},
		{
			name:        "both configured alipay input",
			config:      map[string]string{"typeAlipay": "alipay_h5", "typeWxpay": "wxpay_h5"},
			paymentType: "alipay",
			want:        "alipay_h5",
		},
		{
			name:        "both configured wxpay input",
			config:      map[string]string{"typeAlipay": "alipay_h5", "typeWxpay": "wxpay_h5"},
			paymentType: "wxpay",
			want:        "wxpay_h5",
		},
		{
			name:        "empty override ignored for alipay",
			config:      map[string]string{"typeAlipay": ""},
			paymentType: "alipay",
			want:        "alipay",
		},
		{
			name:        "empty override ignored for wxpay",
			config:      map[string]string{"typeWxpay": ""},
			paymentType: "wxpay",
			want:        "wxpay",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := &EasyPay{config: tt.config}
			got := e.resolveType(tt.paymentType)
			if got != tt.want {
				t.Errorf("resolveType(%q) = %q, want %q", tt.paymentType, got, tt.want)
			}
		})
	}
}

func TestEasyPayResolveCID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		config      map[string]string
		paymentType string
		want        string
	}{
		{
			name:        "alipay with override",
			config:      map[string]string{"cid": "default", "cidAlipay": "ali_cid"},
			paymentType: "alipay",
			want:        "ali_cid",
		},
		{
			name:        "alipay no override falls back to cid",
			config:      map[string]string{"cid": "default"},
			paymentType: "alipay",
			want:        "default",
		},
		{
			name:        "wxpay with override",
			config:      map[string]string{"cid": "default", "cidWxpay": "wx_cid"},
			paymentType: "wxpay",
			want:        "wx_cid",
		},
		{
			name:        "wxpay no override falls back to cid",
			config:      map[string]string{"cid": "default"},
			paymentType: "wxpay",
			want:        "default",
		},
		{
			name:        "empty override ignored for alipay",
			config:      map[string]string{"cid": "default", "cidAlipay": ""},
			paymentType: "alipay",
			want:        "default",
		},
		{
			name:        "empty override ignored for wxpay",
			config:      map[string]string{"cid": "default", "cidWxpay": ""},
			paymentType: "wxpay",
			want:        "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := &EasyPay{config: tt.config}
			got := e.resolveCID(tt.paymentType)
			if got != tt.want {
				t.Errorf("resolveCID(%q) = %q, want %q", tt.paymentType, got, tt.want)
			}
		})
	}
}
