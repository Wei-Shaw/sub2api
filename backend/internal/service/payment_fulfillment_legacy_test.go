//go:build unit

package service

import (
	"errors"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/stretchr/testify/require"
)

func TestParseLegacyPaymentOrderID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		orderID   string
		lookupErr error
		wantID    int64
		wantOK    bool
	}{
		{name: "legacy numeric order", orderID: "sub2_42", lookupErr: &dbent.NotFoundError{}, wantID: 42, wantOK: true},
		{name: "trims whitespace", orderID: "  sub2_42  ", lookupErr: &dbent.NotFoundError{}, wantID: 42, wantOK: true},
		{name: "current vclaw prefix is not legacy numeric format", orderID: "vclaw_42", lookupErr: &dbent.NotFoundError{}, wantOK: false},
		{name: "bare numeric ID", orderID: "42", lookupErr: &dbent.NotFoundError{}, wantOK: false},
		{name: "empty suffix", orderID: "sub2_", lookupErr: &dbent.NotFoundError{}, wantOK: false},
		{name: "non-numeric suffix", orderID: "sub2_42abc", lookupErr: &dbent.NotFoundError{}, wantOK: false},
		{name: "zero ID", orderID: "sub2_0", lookupErr: &dbent.NotFoundError{}, wantOK: false},
		{name: "negative ID", orderID: "sub2_-42", lookupErr: &dbent.NotFoundError{}, wantOK: false},
		{name: "database error does not fallback", orderID: "sub2_42", lookupErr: errors.New("db down"), wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotID, gotOK := parseLegacyPaymentOrderID(tt.orderID, tt.lookupErr)
			require.Equal(t, tt.wantOK, gotOK)
			if tt.wantOK {
				require.Equal(t, tt.wantID, gotID)
			}
		})
	}
}
