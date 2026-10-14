# TCP relay for a private AstroGate origin

`main.go` forwards port 80 and 443 as raw TCP streams. It does not terminate TLS or store application credentials. Caddy on the private origin handles HTTPS and proxies to the application on loopback.

The default upstreams are `127.0.0.1:18080` for HTTP and `127.0.0.1:18443` for HTTPS. Set `ASTROGATE_HTTP_UPSTREAM` and `ASTROGATE_HTTPS_UPSTREAM` to change them during a migration. `ASTROGATE_HTTP_LISTEN` and `ASTROGATE_HTTPS_LISTEN` set the public listeners; the defaults are `:80` and `:443`.

Build with `go build -trimpath -o astrogate-relay deploy/relay/main.go` and run it under a service manager. If the private origin connects with SSH reverse forwarding, bind each reverse listener to `127.0.0.1` on the relay. Keep TLS certificates and all application data on the private origin.
