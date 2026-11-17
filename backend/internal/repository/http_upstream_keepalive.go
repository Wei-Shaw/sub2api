package repository

import "time"

// TCP keep-alive of upstream connections (newUpstreamDialer). A connection
// whose peer stops answering is aborted upstreamTCPUserTimeout after its last
// received packet: idle 15 s, then 3 probes 5 s apart. With the previous
// KeepAlive of 30 s and Go's default interval 15 s and count 9, the same case
// took 165 s.
//
// TCP_USER_TIMEOUT (Linux only, see upstreamDialControl) is set to
// idle + interval * count, as Cloudflare's "When TCP sockets refuse to die"
// (2019) recommends when both are used: once it is set the kernel aborts on it
// rather than on the probe count, so a larger value would add probes and a
// smaller one would cut them. It also bounds request data the peer never
// acknowledges, where keep-alive does not run. gRPC proposal A18 likewise sets
// it only on Linux.
const (
	upstreamKeepAliveIdle     = 15 * time.Second
	upstreamKeepAliveInterval = 5 * time.Second
	upstreamKeepAliveCount    = 3
	upstreamTCPUserTimeout    = upstreamKeepAliveIdle + upstreamKeepAliveInterval*upstreamKeepAliveCount
)
