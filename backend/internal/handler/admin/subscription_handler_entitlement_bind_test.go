package admin

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type recordingManualSubscriptionBinder struct {
	calls []manualSubscriptionBindCall
	err   error
}

type manualSubscriptionBindCall struct {
	userID  int64
	groupID int64
}

func (b *recordingManualSubscriptionBinder) BindUserToGroupAfterPayment(ctx context.Context, userID, groupID int64) (*service.EntitlementSwitchResult, error) {
	b.calls = append(b.calls, manualSubscriptionBindCall{userID: userID, groupID: groupID})
	if b.err != nil {
		return nil, b.err
	}
	return &service.EntitlementSwitchResult{}, nil
}

func TestSubscriptionHandlerBindEntitlementAfterManualSubscription(t *testing.T) {
	binder := &recordingManualSubscriptionBinder{}
	h := NewSubscriptionHandler(nil, nil)
	h.SetEntitlementBinder(binder)

	h.bindEntitlementAfterManualSubscription(context.Background(), 42, 9, "test")

	require.Equal(t, []manualSubscriptionBindCall{{userID: 42, groupID: 9}}, binder.calls)
}

func TestSubscriptionHandlerBindEntitlementAfterManualSubscriptionDoesNotFailGrantOnBinderError(t *testing.T) {
	binder := &recordingManualSubscriptionBinder{err: errors.New("bind failed")}
	h := NewSubscriptionHandler(nil, nil)
	h.SetEntitlementBinder(binder)

	require.NotPanics(t, func() {
		h.bindEntitlementAfterManualSubscription(context.Background(), 42, 9, "test")
	})
	require.Equal(t, []manualSubscriptionBindCall{{userID: 42, groupID: 9}}, binder.calls)
}

func TestSubscriptionHandlerBindEntitlementAfterManualSubscriptionSkipsMissingInputs(t *testing.T) {
	binder := &recordingManualSubscriptionBinder{}
	h := NewSubscriptionHandler(nil, nil)
	h.SetEntitlementBinder(binder)

	h.bindEntitlementAfterManualSubscription(context.Background(), 0, 9, "test")
	h.bindEntitlementAfterManualSubscription(context.Background(), 42, 0, "test")

	require.Empty(t, binder.calls)
}
