//go:build unit

package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func mkProxy(id int64, mode string, backup *int64, expiresInDays *int, now time.Time) Proxy {
	p := Proxy{ID: id, FallbackMode: mode, BackupProxyID: backupREDACTED
	if expiresInDays != nil {
		t := now.AddDate(0, 0, *expiresInDays)
		p.ExpiresAt = &t
REDACTED
	return p
REDACTED
func i64(v int64) *int64 { return &v REDACTED
func di(v int) *int      { return &v REDACTED

func TestResolveFallbackTarget(t *testing.T) {
	now := time.Now()
	t.Run("none keeps original", func(t *testing.T) {
		a := mkProxy(1, FallbackModeNone, nil, di(-1), now)
		by := map[int64]Proxy{1: aREDACTED
		target, change := ResolveProxyFallbackTarget(a, by, now)
		require.False(t, change)
		require.Nil(t, target)
REDACTED)
	t.Run("direct -> nil target, change", func(t *testing.T) {
		a := mkProxy(1, FallbackModeDirect, nil, di(-1), now)
		by := map[int64]Proxy{1: aREDACTED
		target, change := ResolveProxyFallbackTarget(a, by, now)
		require.True(t, change)
		require.Nil(t, target)
REDACTED)
	t.Run("proxy -> healthy backup", func(t *testing.T) {
		b := mkProxy(2, FallbackModeNone, nil, di(30), now)
		a := mkProxy(1, FallbackModeProxy, i64(2), di(-1), now)
		by := map[int64]Proxy{1: a, 2: bREDACTED
		target, change := ResolveProxyFallbackTarget(a, by, now)
		require.True(t, change)
		require.NotNil(t, target)
		require.Equal(t, int64(2), *target)
REDACTED)
	t.Run("chain A->B(expired)->C(healthy)", func(t *testing.T) {
		c := mkProxy(3, FallbackModeNone, nil, di(30), now)
		b := mkProxy(2, FallbackModeProxy, i64(3), di(-1), now)
		a := mkProxy(1, FallbackModeProxy, i64(2), di(-1), now)
		by := map[int64]Proxy{1: a, 2: b, 3: cREDACTED
		target, change := ResolveProxyFallbackTarget(a, by, now)
		require.True(t, change)
		require.Equal(t, int64(3), *target)
REDACTED)
	t.Run("cycle A->B->A keeps original", func(t *testing.T) {
		b := mkProxy(2, FallbackModeProxy, i64(1), di(-1), now)
		a := mkProxy(1, FallbackModeProxy, i64(2), di(-1), now)
		by := map[int64]Proxy{1: a, 2: bREDACTED
		target, change := ResolveProxyFallbackTarget(a, by, now)
		require.False(t, change)
		require.Nil(t, target)
REDACTED)
	t.Run("chain tail direct fallback", func(t *testing.T) {
		b := mkProxy(2, FallbackModeDirect, nil, di(-1), now)
		a := mkProxy(1, FallbackModeProxy, i64(2), di(-1), now)
		by := map[int64]Proxy{1: a, 2: bREDACTED
		target, change := ResolveProxyFallbackTarget(a, by, now)
		require.True(t, change)
		require.Nil(t, target)
REDACTED)
REDACTED
