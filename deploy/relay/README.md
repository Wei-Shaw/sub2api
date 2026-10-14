# TCP relay for a private AstroGate origin

`main.go` forwards port 80 and 443 as raw TCP streams. It does not terminate TLS or store application credentials. Caddy on the private origin handles HTTPS and proxies to the application on loopback.

The default upstreams are `127.0.0.1:18080` for HTTP and `127.0.0.1:18443` for HTTPS. Set `ASTROGATE_HTTP_UPSTREAM` and `ASTROGATE_HTTPS_UPSTREAM` to change them during a migration. `ASTROGATE_HTTP_LISTEN` and `ASTROGATE_HTTPS_LISTEN` set the public listeners; the defaults are `:80` and `:443`.

Build with `go build -trimpath -o astrogate-relay deploy/relay/main.go` and run it under a service manager. If the private origin connects with SSH reverse forwarding, bind each reverse listener to `127.0.0.1` on the relay. Keep TLS certificates and all application data on the private origin.

## IPv4 relay alongside the existing IPv6 relay

`dicksuck.astrogate.sbs` points to the IPv4 relay at `23.94.184.57`. The original `www.astrogate.sbs` IPv6 relay remains active. On the IPv4 machine, `astrogate-relay-ipv4.service` listens on port 80 and on loopback port 8443. `astrogate-tunnel-ipv4.service` runs on the private origin and forwards loopback ports 18080 and 18443 over SSH. Caddy on the origin serves both names and owns their TLS certificates.

The IPv4 machine also runs Xray. HAProxy owns its public port 443 and routes TLS connections for `dicksuck.astrogate.sbs` to the AstroGate relay; all other TLS traffic goes to Xray on port 1443. The x-ui inbound is stored on port 1443, so newly generated Xray subscription links may advertise 1443. The firewall allows that port, while existing clients using 443 continue through HAProxy. The one-time `switch-xui-to-haproxy.sh` script makes a root-only x-ui database backup before switching and restores it if verification fails. Do not run the switch script again after HAProxy is active.

All four services should be enabled at boot: `astrogate-relay` and `haproxy` on the IPv4 machine, plus `astrogate-tunnel-ipv4` and `caddy` on the origin. Check the public site with `curl --noproxy '*' https://dicksuck.astrogate.sbs/` if the operator machine uses a proxy. A successful TLS connection with HTTP 200 verifies the public path. This relay does not hold application credentials, account records, or TLS private keys.
