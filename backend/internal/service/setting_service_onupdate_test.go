package service

import (
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAddOnUpdateCallback_ChainsWithoutReplacing(t *testing.T) {
	svc := &SettingService{}
	var first, second int32
	svc.SetOnUpdateCallback(func() { atomic.AddInt32(&first, 1) })
	svc.AddOnUpdateCallback(func() { atomic.AddInt32(&second, 1) })
	require.NotNil(t, svc.onUpdate)
	svc.onUpdate()
	require.Equal(t, int32(1), atomic.LoadInt32(&first))
	require.Equal(t, int32(1), atomic.LoadInt32(&second))
}