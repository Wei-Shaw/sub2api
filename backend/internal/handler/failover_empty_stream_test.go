package handler

import (
	"context"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// releasingTempUnscheduler also records empty-stream cooldown releases.
type releasingTempUnscheduler struct {
	mockTempUnscheduler
	released []int64
}

func (m *releasingTempUnscheduler) ReleaseEmptyStreamTempUnschedule(_ context.Context, accountID int64) bool {
	m.released = append(m.released, accountID)
	return true
}

func emptyStreamFailoverErr() *service.UpstreamFailoverError {
	return &service.UpstreamFailoverError{StatusCode: http.StatusBadGateway, RetryableOnSameAccount: true, EmptyStream: true}
}

func TestHandleFailoverError_EmptyStreamOnTwoAccountsIsRequestScoped(t *testing.T) {
	t.Run("second account also empty: release the first account's cooldown", func(t *testing.T) {
		mock := &releasingTempUnscheduler{}
		fs := NewFailoverState(10, false)

		require.Equal(t, FailoverContinue, exhaustSameAccount(fs, mock, 13, emptyStreamFailoverErr()))
		require.Equal(t, FailoverContinue, exhaustSameAccount(fs, mock, 2, emptyStreamFailoverErr()))
		require.Equal(t, FailoverContinue, exhaustSameAccount(fs, mock, 6, emptyStreamFailoverErr()))

		require.Len(t, mock.calls, 1)
		require.Equal(t, int64(13), mock.calls[0].accountID)
		require.Equal(t, []int64{13}, mock.released, "released once, only the account this request cooled")
		require.Len(t, fs.FailedAccountIDs, 3, "still switches past every failed account")
	})

	t.Run("empty stream on one account only keeps its cooldown", func(t *testing.T) {
		mock := &releasingTempUnscheduler{}
		fs := NewFailoverState(10, false)

		exhaustSameAccount(fs, mock, 13, emptyStreamFailoverErr())
		exhaustSameAccount(fs, mock, 2, newTestFailoverErr(http.StatusBadGateway, true, false))

		require.Len(t, mock.calls, 1)
		require.Empty(t, mock.released)
	})

	t.Run("a cooldown for another error is never released", func(t *testing.T) {
		mock := &releasingTempUnscheduler{}
		fs := NewFailoverState(10, false)

		exhaustSameAccount(fs, mock, 7, newTestFailoverErr(http.StatusBadGateway, true, false))
		exhaustSameAccount(fs, mock, 2, emptyStreamFailoverErr())
		exhaustSameAccount(fs, mock, 6, emptyStreamFailoverErr())

		require.Len(t, mock.calls, 1)
		require.Equal(t, int64(7), mock.calls[0].accountID)
		require.Empty(t, mock.released)
	})

	t.Run("an unscheduler without release support still works", func(t *testing.T) {
		mock := &mockTempUnscheduler{}
		fs := NewFailoverState(10, false)

		exhaustSameAccount(fs, mock, 13, emptyStreamFailoverErr())
		exhaustSameAccount(fs, mock, 2, emptyStreamFailoverErr())

		require.Len(t, mock.calls, 1)
	})
}
