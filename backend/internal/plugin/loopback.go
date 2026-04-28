package plugin

import (
	"errors"
	"fmt"
	"net"
)

// validateLoopbackAddr rejects plugin handshake addresses that would let any
// other process on the same host (or any LAN peer) connect to the plugin's
// gRPC / HTTP server.
//
// The plugin SDK defaults its listeners to "127.0.0.1:0", so every official
// plugin built with `pluginsdk.Run` automatically passes. The check exists
// to defend against:
//   - third-party plugin binaries that implement the handshake protocol
//     directly and may bind 0.0.0.0
//   - replaced binaries (supply chain compromise) that try to expose the
//     plugin channel to other co-tenants on the same machine
//
// Accepted host forms (split via net.SplitHostPort):
//   - "127.0.0.1"
//   - "::1"
//   - "localhost" (resolved by definition to a loopback)
//
// allowEmpty=true is used for HTTPAddr because plugins that do not register
// any HTTP routes legitimately leave it blank.
func validateLoopbackAddr(addr string, allowEmpty bool) error {
	if addr == "" {
		if allowEmpty {
			return nil
		}
		return errors.New("addr is empty")
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("parse %q: %w", addr, err)
	}
	if host == "localhost" {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf("host %q is not an IP", host)
	}
	if !ip.IsLoopback() {
		return fmt.Errorf("host %q is not loopback", host)
	}
	return nil
}
