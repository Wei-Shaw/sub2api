//go:build unit

package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func mkProxy(id int64, mode string, backup *int64, expiresInDays *int, now time.Time) Proxy {
	p := Proxy{ID: id, Status: StatusActive, FallbackMode: mode, BackupProxyID: backup}
	if expiresInDays != nil {
		t := now.AddDate(0, 0, *expiresInDays)
		p.ExpiresAt = &t
	}
	return p
}
func i64(v int64) *int64 { return &v }
func di(v int) *int      { return &v }

func TestResolveFallbackTarget(t *testing.T) {
	now := time.Now()
	t.Run("none keeps original", func(t *testing.T) {
		a := mkProxy(1, FallbackModeNone, nil, di(-1), now)
		by := map[int64]Proxy{1: a}
		target, change := ResolveProxyFallbackTarget(a, by, now)
		require.False(t, change)
		require.Nil(t, target)
	})
	t.Run("direct -> nil target, change", func(t *testing.T) {
		a := mkProxy(1, FallbackModeDirect, nil, di(-1), now)
		by := map[int64]Proxy{1: a}
		target, change := ResolveProxyFallbackTarget(a, by, now)
		require.True(t, change)
		require.Nil(t, target)
	})
	t.Run("proxy -> healthy backup", func(t *testing.T) {
		b := mkProxy(2, FallbackModeNone, nil, di(30), now)
		a := mkProxy(1, FallbackModeProxy, i64(2), di(-1), now)
		by := map[int64]Proxy{1: a, 2: b}
		target, change := ResolveProxyFallbackTarget(a, by, now)
		require.True(t, change)
		require.NotNil(t, target)
		require.Equal(t, int64(2), *target)
	})
	t.Run("chain A->B(expired)->C(healthy)", func(t *testing.T) {
		c := mkProxy(3, FallbackModeNone, nil, di(30), now)
		b := mkProxy(2, FallbackModeProxy, i64(3), di(-1), now)
		a := mkProxy(1, FallbackModeProxy, i64(2), di(-1), now)
		by := map[int64]Proxy{1: a, 2: b, 3: c}
		target, change := ResolveProxyFallbackTarget(a, by, now)
		require.True(t, change)
		require.Equal(t, int64(3), *target)
	})
	t.Run("cycle A->B->A keeps original", func(t *testing.T) {
		b := mkProxy(2, FallbackModeProxy, i64(1), di(-1), now)
		a := mkProxy(1, FallbackModeProxy, i64(2), di(-1), now)
		by := map[int64]Proxy{1: a, 2: b}
		target, change := ResolveProxyFallbackTarget(a, by, now)
		require.False(t, change)
		require.Nil(t, target)
	})
	t.Run("chain tail direct fallback", func(t *testing.T) {
		b := mkProxy(2, FallbackModeDirect, nil, di(-1), now)
		a := mkProxy(1, FallbackModeProxy, i64(2), di(-1), now)
		by := map[int64]Proxy{1: a, 2: b}
		target, change := ResolveProxyFallbackTarget(a, by, now)
		require.True(t, change)
		require.Nil(t, target)
	})
}

func TestResolveFallbackSkipsInactiveBackup(t *testing.T) {
	now := time.Now()
	for _, mode := range []string{FallbackModeNone, FallbackModeProxy, FallbackModeDirect} {
		t.Run(mode, func(t *testing.T) {
			source := mkProxy(1, FallbackModeProxy, i64(2), di(-1), now)
			disabled := mkProxy(2, mode, i64(3), di(30), now)
			disabled.Status = "inactive"
			healthy := mkProxy(3, FallbackModeNone, nil, di(30), now)
			target, change := ResolveProxyFallbackTarget(source, map[int64]Proxy{1: source, 2: disabled, 3: healthy}, now)
			switch mode {
			case FallbackModeNone:
				require.False(t, change)
				require.Nil(t, target)
			case FallbackModeProxy:
				require.True(t, change)
				require.Equal(t, i64(3), target)
			case FallbackModeDirect:
				require.True(t, change)
				require.Nil(t, target)
			}
		})
	}
	t.Run("inactive cycle", func(t *testing.T) {
		source := mkProxy(1, FallbackModeProxy, i64(2), di(-1), now)
		disabled := mkProxy(2, FallbackModeProxy, i64(1), nil, now)
		disabled.Status = "inactive"
		target, change := ResolveProxyFallbackTarget(source, map[int64]Proxy{1: source, 2: disabled}, now)
		require.False(t, change)
		require.Nil(t, target)
	})
}
