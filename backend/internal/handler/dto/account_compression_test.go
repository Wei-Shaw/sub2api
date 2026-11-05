package dto

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/pkg/reqcompress"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestAccountResponsesCarrySettledRequestCompressionAlgorithm(t *testing.T) {
	const accountID int64 = 771
	enabled := true
	reqcompress.SyncAccount(accountID, true, time.Time{})
	t.Cleanup(func() { reqcompress.ResetAccount(context.Background(), accountID) })

	req, err := http.NewRequest(http.MethodPost, "https://upstream.test/v1/messages",
		bytes.NewReader(bytes.Repeat([]byte("a"), 2048)))
	require.NoError(t, err)
	_, err = reqcompress.Execute(accountID, req, func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("ok")),
		}, nil
	})
	require.NoError(t, err)

	full := AccountFromServiceShallow(&service.Account{ID: accountID, CompressRequestEnabled: &enabled})
	require.Equal(t, "br", full.CompressRequestAlgorithm)
	lite := AccountListItemFromAccount(full)
	require.Equal(t, "br", lite.CompressRequestAlgorithm)
}
