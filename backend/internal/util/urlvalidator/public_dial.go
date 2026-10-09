package urlvalidator

import (
	"context"
	"errors"
	"net"
)

// DialPublicContext pins the connection to validated IPs so DNS cannot change between validation and dialing.
func DialPublicContext(ctx context.Context, network, address string, dial func(context.Context, string, string) (net.Conn, error)) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	if IsBlockedHost(host) {
		return nil, errors.New("private upstream address is not allowed")
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	if len(ips) == 0 {
		return nil, errors.New("upstream host has no IP addresses")
	}
	for _, ip := range ips {
		if IsBlockedHost(ip.IP.String()) {
			return nil, errors.New("upstream host resolves to a private address")
		}
	}
	for _, ip := range ips {
		var conn net.Conn
		conn, err = dial(ctx, network, net.JoinHostPort(ip.IP.String(), port))
		if err == nil {
			return conn, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}
	return nil, err
}
