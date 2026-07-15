package service

import "time"

const defaultKiroStreamKeepaliveInterval = 25 * time.Second

func streamKeepaliveIntervalForAccount(cfgGatewayStream, cfgKiroStream int, account *Account) time.Duration {
	if account != nil && account.Platform == PlatformKiro {
		if cfgKiroStream > 0 {
			return time.Duration(cfgKiroStream) * time.Second
		}
		return defaultKiroStreamKeepaliveInterval
	}
	if cfgGatewayStream > 0 {
		return time.Duration(cfgGatewayStream) * time.Second
	}
	return 0
}

func (s *GatewayService) streamKeepaliveIntervalForAccount(account *Account) time.Duration {
	if s == nil || s.cfg == nil {
		return streamKeepaliveIntervalForAccount(0, 0, account)
	}
	return streamKeepaliveIntervalForAccount(
		s.cfg.Gateway.StreamKeepaliveInterval,
		0, // Kiro keepalive interval not yet in config (uses default 25s)
		account,
	)
}

func (s *OpenAIGatewayService) streamKeepaliveIntervalForAccount(account *Account) time.Duration {
	if s == nil || s.cfg == nil {
		return streamKeepaliveIntervalForAccount(0, 0, account)
	}
	return streamKeepaliveIntervalForAccount(
		s.cfg.Gateway.StreamKeepaliveInterval,
		0, // Kiro keepalive interval not yet in config (uses default 25s)
		account,
	)
}
