package runtime

import (
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/tools/warp-gateway/internal/store"
)

func dialTCP(addr string, d time.Duration) (interface{ Close() error }, error) {
	return net.DialTimeout("tcp", addr, d)
}

// Cloudflare WARP engage anycast endpoints used when local DNS returns
// Clash/mihomo fake-ip (198.18.0.0/15) or resolution fails.
// Only list endpoints verified working for free WARP handshakes on this network.
var warpEngageFallbackIPs = []string{
	"162.159.192.1",
	"162.159.195.1",
}

// buildSingBoxConfig generates a sing-box 1.12+ config:
// SOCKS inbound -> wireguard endpoint (WARP profile).
// DNS is resolved through the tunnel (1.1.1.1) so local fake-ip DNS cannot poison egress.
func buildSingBoxConfig(inst *store.Instance) map[string]any {
	dns := inst.Profile.DNS
	if len(dns) == 0 {
		dns = []string{"1.1.1.1", "1.0.0.1"}
	}
	mtu := inst.Profile.MTU
	if mtu == 0 {
		mtu = 1280
	}

	// Spread multi-instance engage anycast by listen port so concurrent tunnels
	// are less likely to share one path/rate-limit bucket.
	engageIdx := 0
	if inst.ListenPort > 0 {
		engageIdx = inst.ListenPort % len(warpEngageFallbackIPs)
	}

	peers := make([]map[string]any, 0, len(inst.Profile.Peers))
	for _, p := range inst.Profile.Peers {
		allowed := p.AllowedIPs
		if len(allowed) == 0 {
			allowed = []string{"0.0.0.0/0", "::/0"}
		}
		host, port := splitEndpoint(p.Endpoint)
		host = resolveEngageHost(host, engageIdx)
		peers = append(peers, map[string]any{
			"address":                       host,
			"port":                          port,
			"public_key":                    p.PublicKey,
			"allowed_ips":                   allowed,
			"persistent_keepalive_interval": 25,
		})
	}

	localAddr := []string{"172.16.0.2/32"}
	if len(inst.Profile.Address) > 0 {
		localAddr = inst.Profile.Address
	}

	inbound := map[string]any{
		"type":        "socks",
		"tag":         "socks-in",
		"listen":      inst.ListenHost,
		"listen_port": inst.ListenPort,
	}
	if inst.SocksAuthUser != "" {
		inbound["users"] = []map[string]any{{
			"username": inst.SocksAuthUser,
			"password": inst.SocksAuthPass,
		}}
	}

	// DNS over the WG tunnel using an IP server — no chicken-egg with endpoint domain.
	dnsServers := []map[string]any{{
		"type":        "udp",
		"tag":         "dns-through-wg",
		"server":      dns[0],
		"server_port": 53,
	}}

	return map[string]any{
		"log": map[string]any{
			"level":     "info",
			"timestamp": true,
		},
		"dns": map[string]any{
			"servers":  dnsServers,
			"final":    "dns-through-wg",
			"strategy": "prefer_ipv4",
		},
		// sing-box 1.12+: WireGuard is an endpoint, not an outbound.
		"endpoints": []map[string]any{{
			"type":        "wireguard",
			"tag":         "wg-ep",
			"system":      false,
			"mtu":         mtu,
			"address":     localAddr,
			"private_key": inst.Profile.PrivateKey,
			"peers":       peers,
		}},
		"inbounds": []map[string]any{inbound},
		"outbounds": []map[string]any{
			{"type": "direct", "tag": "direct"},
		},
		"route": map[string]any{
			"final":                   "wg-ep",
			"default_domain_resolver": "dns-through-wg",
		},
	}
}

func splitEndpoint(endpoint string) (host string, port int) {
	host = strings.TrimSpace(endpoint)
	port = 2408
	if host == "" {
		return "162.159.192.1", port
	}
	// host:port or [ipv6]:port
	if h, p, err := net.SplitHostPort(host); err == nil {
		host = h
		if n, e := strconv.Atoi(p); e == nil && n > 0 {
			port = n
		}
		return host, port
	}
	// bare host without port
	if strings.Count(host, ":") == 1 {
		// already handled by SplitHostPort for host:port
	}
	return host, port
}

// resolveEngageHost maps Cloudflare WARP engage hostnames to a working anycast IP
// when system DNS is hijacked (Clash fake-ip 198.18.0.0/15) or fails.
// engageIdx selects among fallback anycast IPs for multi-instance spreading.
func resolveEngageHost(host string, engageIdx int) string {
	host = strings.TrimSpace(host)
	fb := warpEngageFallbackIPs[0]
	if len(warpEngageFallbackIPs) > 0 {
		if engageIdx < 0 {
			engageIdx = -engageIdx
		}
		fb = warpEngageFallbackIPs[engageIdx%len(warpEngageFallbackIPs)]
	}
	if host == "" {
		return fb
	}
	// Already an IP
	if ip := net.ParseIP(host); ip != nil {
		if isFakeIP(ip) {
			return fb
		}
		return host
	}
	// Domain: try system lookup, reject fake-ip
	ips, err := net.LookupIP(host)
	if err == nil {
		for _, ip := range ips {
			if ip.To4() != nil && !isFakeIP(ip) {
				return ip.String()
			}
		}
	}
	// Fallback for engage.cloudflareclient.com and unknown domains under fake-ip DNS
	if strings.Contains(strings.ToLower(host), "engage.cloudflareclient.com") ||
		strings.Contains(strings.ToLower(host), "cloudflare") {
		return fb
	}
	return host
}

func isFakeIP(ip net.IP) bool {
	ip4 := ip.To4()
	if ip4 == nil {
		return false
	}
	// 198.18.0.0/15 used by Clash/mihomo fake-ip
	return ip4[0] == 198 && (ip4[1] == 18 || ip4[1] == 19)
}
