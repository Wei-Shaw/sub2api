# Edge and HTTP Ingress Security

Sub2API supports long-lived SSE and WebSocket requests. Protect request ingress
and individual socket operations without imposing a fixed timeout on the entire
response. Healthy generations can remain open while an upstream is thinking.

## Application defaults

- `server.max_header_bytes: 65536` limits HTTP/1 request headers to 64 KiB;
  Go maps it to the corresponding HTTP/2 header-list limit.
- `server.read_header_timeout: 10` bounds slow-header attacks. It does not
  limit request processing or response streaming.
- `server.max_request_body_size: 268435456` is the absolute 256 MiB safety net.
- `gateway.max_body_size: 268435456` remains available to multimodal, Gemini,
  image, video, and batch-image endpoints.
- `gateway.text_max_body_size: 33554432` limits the known pure-text
  `/embeddings` and `/alpha/search` endpoints to 32 MiB.
- `gateway.request_body_memory_budget_bytes` shares one buffering budget
  across gateway routes and aliases. When it is set in neither the config file
  nor `GATEWAY_REQUEST_BODY_MEMORY_BUDGET_BYTES`, startup sizes it to a quarter
  of the process memory limit (the smallest of physical memory, the cgroup
  limit and `GOMEMLIMIT`), never below 1 GiB, and logs the chosen value. Any
  explicit value is used as-is (`0` disables the budget). Chunk allocations, the
  contiguous body copy, decompressed output, JSON normalization, and a Zstd
  decoder reservation are charged before allocation. Each phase releases the
  intermediate allocations it no longer references: chunks after assembly,
  compressed input and decoder workspace after decoding, and owned input after
  normalization. The final body allocation stays charged until the handler
  returns, including throughout a long SSE response. A preread buffer that may
  still be referenced by middleware stays charged alongside a normalized copy.
  Exhaustion returns 503 with `Retry-After`; a single-request
  size violation remains 413. Set this budget to fit the instance's memory,
  leaving headroom for connections, conversion state, caches and GC. This is
  conservative ingress accounting, not a hard bound on total heap or RSS.
- Compressed request output is capped at 64 MiB, with overflow rejected rather
  than truncated. Zstd additionally has an 8 MiB window cap and one decoder
  worker per request.
- `gateway.request_body_read_timeout_seconds: 120` bounds a stalled body read;
  `gateway.downstream_write_timeout_seconds: 120` bounds each write/flush.
  Successful operations clear their deadline, so upstream thinking does not
  consume a whole-response time budget. Zero disables the corresponding limit.
- `gateway.max_line_size: 524288000` preserves the 500 MiB SSE line default
  required by large base64 4K images, image-generation output and multi-image
  responses. Upgrading does not silently lower this compatibility limit.
  `sse_event_max_bytes: 0` inherits that cap for each complete event and
  `sse_event_max_lines: 4096` also bounds streams of short lines without a blank
  delimiter. `sse_queue_max_bytes: 8388608` bounds queued payload bytes; a line
  larger than 8 MiB can occupy the queue alone, up to the line/event cap.
  The consumer's current line and producer's scanner/read buffer are additional
  bounded storage. The 8 MiB queue budget does not reduce the maximum size of
  one image frame. Lower line/event limits only when the supported models and
  output sizes fit them. Explicit event byte/line settings also apply to Grok
  billing-ping and client-tool prefilters.
- Detached HTTP upstream attempts may continue collecting late billing usage
  after client cancellation, but only for a finite drain window. The window
  follows the stream data interval timeout (180 seconds by default). OpenAI
  image API-key/OAuth forwarding uses the image timeout (900 seconds by
  default); Grok media retains the general drain timeout. Incoming pings do not
  restart the window.
  This replaces the previously unlimited post-disconnect drain: when the
  window expires, the attempt is cancelled and only usage observed before
  cancellation is available for settlement. Final usage arriving later may be
  lost even if the provider continues generating and charging. Disabling the
  stream idle timeout does not disable this drain safety limit (180 seconds;
  OpenAI image API-key/OAuth forwarding retains a 900-second fallback).
- H2C defaults to 50 concurrent streams per connection, a 2 MiB connection
  upload window, and a 512 KiB stream upload window.
- Invalid credential abuse is limited in process by trusted client IP (IPv6
  `/64`): 120 failures per 60 seconds followed by a 60-second block. This is a
  per-instance safety net; multi-instance enforcement still belongs at the
  load balancer, CDN, or WAF.

Do not replace these controls with a single application-wide request semaphore:
an SSE request may legitimately occupy it for many minutes. Apply connection
and unauthenticated request controls at the edge; authenticated user/API-key
concurrency and buffered request bytes remain the application's responsibility.

Compatibility session caches (digest sessions, Responses continuation bindings
and Anthropic digest bindings) each have fixed limits of 16,384 entries and
32 MiB of accounted storage, including 512 bytes of bookkeeping per entry.
These limits are not currently configurable. The least recently used entry is
evicted when either limit is reached, even if its TTL has not expired; a TTL is
therefore not a guarantee that a continuation binding remains available. Active
expiry scans run at most once a minute while the cache is nonempty, and shutdown
stops cleanup. Monitor 503 body-budget rejections, response-too-large errors,
allocation rate, retained heap after GC, goroutine counts, usage-worker queues
and database connection waits when tuning capacity. Notification queues may
skip best-effort messages on overload or shutdown and log a dropped count;
committed-charge persistence is not dropped by that notification policy.
When the platform-quota flusher is disabled, its quota mirror update now runs
inside the usage task with a separate five-second database deadline. Failures
remain counted and logged for reconciliation; this path has no durable automatic
retry. The main charge is committed idempotently before this mirror update.
With the default `gateway.usage_record.overflow_policy: sync`, a full usage
queue executes the task in the submitting handler. Database latency can
therefore delay usage submission and handler completion by up to the separate
five-second quota persistence attempt, in addition to the other usage work.
The configured worker count is not a hard global limit on database concurrency.

## Sizing the request-body budget

Size `request_body_memory_budget_bytes` for the final body allocations retained
by all active requests, plus peak workspace for requests currently being read,
decoded or normalized. Streaming duration increases the number of retained
bodies even after their upload has finished. The budget is not just the number
of simultaneous uploads, and releasing an allocation's charge does not force Go
to reclaim that unreachable memory immediately.

For example, 200 long-lived streams retaining 4 MiB each account for about
800 MiB before new uploads and decoding. Another 32 concurrent Zstd decoders
can reserve 256 MiB of window space alone, exceeding a 1 GiB budget (the
auto-sized value on instances with 4 GiB or less) before their input/output
buffers are counted. Increase the budget for such a workload only with
sufficient instance memory, or reduce concurrency/request sizes. Deployments
that copied the old example config pin `request_body_memory_budget_bytes:
1073741824`; remove that line to use auto-sizing. Leave separate memory headroom for SSE frames (which may be larger than
the queue budget), conversion state, caches, connections and GC. Setting the
budget to `0` disables ingress memory admission rather than limiting RSS.

Legacy Read tool-call completion still validates the growing JSON argument
prefix. Incremental storage removes repeated prefix allocations, but validation
CPU can remain quadratic for large arguments delivered in many small deltas.
Measure this separately from allocation savings; cumulative bytes allocated in
benchmarks are not retained heap or RSS.

## Trusted client IPs

`security.trust_forwarded_ip_for_api_key_acl` is enabled by default for upgrade
compatibility. While enabled, raw forwarding headers take over client-IP
resolution for logs and security-sensitive paths. Custom headers from
`security.forwarded_client_ip_headers` are checked in configured order before
the built-in `CF-Connecting-IP`, `X-Real-IP`, and `X-Forwarded-For` fallback.
Header names are case-insensitive, normalized when loaded, de-duplicated, and
limited to 16 unique valid HTTP field names. Header values must contain IP
literals; comma-separated values are supported, invalid entries are skipped,
and public addresses are preferred over private fallback addresses.

The list can be supplied in YAML or with the comma-separated environment
variable `SECURITY_FORWARDED_CLIENT_IP_HEADERS`; an explicitly empty environment
value clears YAML values. It is also editable from the admin security settings
and updates at runtime without a restart. A request snapshots the switch and
header list together, so one request cannot mix old and new settings. Custom
headers are ignored completely when the switch is disabled. In that mode Gin's
`server.trusted_proxies` chain is authoritative: configure only the exact
CIDR/IP addresses that connect directly to Sub2API. An explicit empty list
trusts no forwarded client IPs.

On the first upgrade to this mode, a legacy `false` value is changed to `true`
only when `server.trusted_proxies` was not explicitly configured; explicit
proxy policies remain in secure mode. New installations persist the configured
custom header list during database initialization. Existing installations
backfill a missing database value from the YAML configuration. A hidden
migration marker prevents later administrator changes from being overwritten.
If settings cannot be read or the persisted custom-header list is malformed,
the process fails closed to trusted-proxy mode with no custom headers. If a
migration write fails, the computed mode remains active for the current process
and startup records a warning.

Compatibility takeover accepts forwarded headers without validating the direct
peer, including any configured custom header. Protect the origin from direct
access while it is enabled. A CDN deployment must firewall the origin so only
the CDN or load balancer can reach it, and that proxy must overwrite every
trusted client-IP header rather than append an untrusted client value.

Example for a proxy on the same host:

```yaml
server:
  trusted_proxies:
    - 127.0.0.1/32
    - ::1/128
```

## Nginx baseline

Define shared zones in the `http` block. Tune rates to measured legitimate
traffic; the values below are conservative starting points, not universal
capacity targets.

```nginx
limit_conn_zone $binary_remote_addr zone=sub2api_conn:20m;
limit_req_zone  $binary_remote_addr zone=sub2api_auth:20m rate=5r/s;
limit_req_zone  $binary_remote_addr zone=sub2api_api:40m rate=30r/s;
map $http_upgrade $connection_upgrade {
    default upgrade;
    ''      close;
}

server {
    listen 443 ssl http2;
    server_name api.example.com;

    client_header_timeout 10s;
    client_max_body_size 256m;
    large_client_header_buffers 4 16k;
    limit_conn sub2api_conn 40;

    location ~ ^/(auth|api/auth)/ {
        limit_req zone=sub2api_auth burst=10 nodelay;
        proxy_pass http://127.0.0.1:8080;
    }

    location ~ ^/(v1/)?(embeddings|alpha/search)$ {
        client_max_body_size 32m;
        limit_req zone=sub2api_api burst=60 nodelay;
        proxy_pass http://127.0.0.1:8080;
    }

    location / {
        limit_req zone=sub2api_api burst=60 nodelay;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $remote_addr;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection $connection_upgrade;
        proxy_buffering off;
        proxy_request_buffering off;
        proxy_read_timeout 1800s;
        proxy_send_timeout 1800s;
        proxy_pass http://127.0.0.1:8080;
    }
}
```

If Nginx gzip is enabled in the `http` block, keep `text/event-stream` out of
`gzip_types` and do not use `gzip_types *` for Sub2API. The
`proxy_buffering off` setting above prevents proxy buffering, but it does not
disable the gzip response filter. Use an explicit list for ordinary responses:

```nginx
gzip on;
gzip_types text/plain text/css application/json application/javascript application/xml image/svg+xml;
```

If a shared global configuration cannot exclude SSE by content type, set
`gzip off;` in the locations serving streaming API routes. This leaves gzip
available for the web UI and static assets.

Do not use an incoming `$http_x_forwarded_for` value unless Nginx real-IP
processing is restricted to explicit trusted proxy CIDRs.

## Caddy and CDN

The bundled `deploy/Caddyfile` sets a 64 KiB header limit, a 10-second header
timeout, a 256 MiB absolute body limit, and overwrites forwarded addresses from
the TCP peer. It is therefore a direct-to-Caddy baseline. Do not use its
`{remote_host}` forwarding lines unchanged behind a CDN: all clients would be
attributed to a CDN egress address, collapsing rejection aggregation and the
invalid-auth limiter onto unrelated users.

The bundled Caddy configuration leaves `flush_interval` unset so Caddy can
automatically flush `text/event-stream` responses while still propagating
client cancellation upstream. Do not set it globally: positive values can add
streaming latency, while Caddy 2.6.2's special `-1` mode also causes
reverse-proxied requests to continue after clients disconnect. The
configuration uses an explicit response content-type list for compression. Do
not replace that list with `text/*` or the shorthand `encode gzip zstd`: both
match `text/event-stream` and can buffer SSE until the response ends. Keep
streaming responses uncompressed while retaining compression for the web UI,
JSON, and static assets.

For a CDN deployment, first firewall the origin so only current CDN egress
CIDRs can connect. Then configure those exact ranges as Caddy trusted proxies
and derive upstream headers from Caddy's parsed `{client_ip}`. For example:

```caddyfile
{
	servers {
		trusted_proxies static 192.0.2.0/24 2001:db8:1234::/48
		trusted_proxies_strict
		client_ip_headers CF-Connecting-IP X-Forwarded-For
	}
}

api.example.com {
	reverse_proxy 127.0.0.1:8080 {
		header_up X-Real-IP {client_ip}
		header_up X-Forwarded-For {client_ip}
	}
}
```

Replace the documentation ranges with the CDN's published, automatically
maintained egress ranges. `CF-Connecting-IP` is safe here only because direct
origin access is blocked and Caddy trusts only those TCP peers. Configure
Sub2API `server.trusted_proxies` with the Caddy address/private subnet so the
application accepts only Caddy's rewritten headers.

Caddy core does not provide a general request-rate limiter; use a trusted
CDN/WAF, a supported rate-limit module, or host firewall controls.

At a CDN/WAF, configure connection limits, header/body limits, bot challenges,
and per-IP/ASN rates before traffic reaches the origin. Allow origin ingress
only from CDN egress CIDRs or a private load balancer. Keep the application port
off the public Internet.

## DDoS boundary

Application checks reduce amplification after a connection reaches Go. They
cannot absorb volumetric attacks, TLS floods, bandwidth saturation, or a large
distributed source set. Those require upstream network capacity, CDN/WAF
filtering, provider firewall rules, and origin isolation. Avoid high-cardinality
metrics or per-request database security logs during rejection storms.
