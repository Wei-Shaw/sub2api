//go:build !linux

package runtime

import "syscall"

func reusePortControl(network, address string, c syscall.RawConn) error {
	return nil
}
