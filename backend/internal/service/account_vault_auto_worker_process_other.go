//go:build !windows

package service

import "io"

func launchAutoWorker(node, script, origin, ticket string, sink io.Writer) (func() error, string) {
	return launchSameSessionWorker(node, script, origin, ticket, sink)
}
