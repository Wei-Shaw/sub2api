//go:build unit

package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type gatewayAdmissionLeaseTTLStoreStub struct {
	GatewayAdmissionStore
	leaseTTL time.Duration
}

func (s *gatewayAdmissionLeaseTTLStoreStub) GatewayAdmissionLeaseTTL() time.Duration {
	return s.leaseTTL
}

func TestNewGatewayAdmissionDerivesRenewIntervalFromStoreLeaseTTL(t *testing.T) {
	tests := []struct {
		name string
		ttl  time.Duration
		want time.Duration
	}{
		{name: "custom lease ttl", ttl: 9 * time.Second, want: 3 * time.Second},
		{name: "zero lease ttl", ttl: 0, want: gatewayAdmissionRenewInterval},
		{name: "negative lease ttl", ttl: -time.Second, want: gatewayAdmissionRenewInterval},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			admission := NewGatewayAdmission(&gatewayAdmissionLeaseTTLStoreStub{leaseTTL: tt.ttl}, nil, nil)

			require.Equal(t, tt.want, admission.renewInterval)
		})
	}
}
