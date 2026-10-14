//go:build linux

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseAutoWorkerDisplayEnvironmentKeepsOnlyDisplayValues(t *testing.T) {
	got := parseAutoWorkerDisplayEnvironment("DISPLAY=:0\nXAUTHORITY=/run/user/1000/xauth\nWAYLAND_DISPLAY=wayland-0\nXDG_RUNTIME_DIR=/run/user/1000\nDBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/1000/bus\nDATABASE_PASSWORD=secret\n")
	require.Equal(t, ":0", got["DISPLAY"])
	require.Equal(t, "/run/user/1000/xauth", got["XAUTHORITY"])
	require.Equal(t, "wayland-0", got["WAYLAND_DISPLAY"])
	require.Equal(t, "/run/user/1000", got["XDG_RUNTIME_DIR"])
	require.Equal(t, "unix:path=/run/user/1000/bus", got["DBUS_SESSION_BUS_ADDRESS"])
	require.NotContains(t, got, "DATABASE_PASSWORD")
}
