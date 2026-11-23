//go:build !linux

package repository

import "syscall"

// upstreamDialControl is nil off Linux: TCP_USER_TIMEOUT is a Linux socket
// option, and the keep-alive configuration alone bounds idle connections.
var upstreamDialControl func(network, address string, c syscall.RawConn) error
