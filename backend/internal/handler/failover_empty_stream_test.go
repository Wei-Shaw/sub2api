package handler

import (
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func emptyStreamFailoverErr() *service.UpstreamFailoverError {
	return &service.UpstreamFailoverError{StatusCode: http.StatusBadGateway, RetryableOnSameAccount: true, EmptyStream: true}
}

// An empty stream only affects the request that got it: the request moves on
// to another account, but no shared account is cooled for anyone else.
func TestHandleFailoverError_EmptyStreamNeverCoolsAnAccount(t *testing.T) {
	t.Run("empty stream on one account: switch, no cooldown", func(t *testing.T) {
		mock := &mockTempUnscheduler{}
		fs := NewFailoverState(10, false)

		require.Equal(t, FailoverContinue, exhaustSameAccount(fs, mock, 13, emptyStreamFailoverErr()))

		require.Empty(t, mock.calls)
		require.Contains(t, fs.FailedAccountIDs, int64(13), "this request still skips the empty account")
	})

	t.Run("empty stream on several accounts: no cooldown at all", func(t *testing.T) {
		mock := &mockTempUnscheduler{}
		fs := NewFailoverState(10, false)

		for _, id := range []int64{13, 2, 6} {
			require.Equal(t, FailoverContinue, exhaustSameAccount(fs, mock, id, emptyStreamFailoverErr()))
		}

		require.Empty(t, mock.calls)
		require.Len(t, fs.FailedAccountIDs, 3)
	})

	t.Run("an empty stream does not use up the request's one cooldown", func(t *testing.T) {
		mock := &mockTempUnscheduler{}
		fs := NewFailoverState(10, false)

		exhaustSameAccount(fs, mock, 13, emptyStreamFailoverErr())
		exhaustSameAccount(fs, mock, 7, newTestFailoverErr(http.StatusBadGateway, true, false))

		require.Len(t, mock.calls, 1)
		require.Equal(t, int64(7), mock.calls[0].accountID)
	})
}
