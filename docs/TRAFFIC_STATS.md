# HTTP body traffic statistics

## Measurement contract

Traffic statistics count **HTTP entity-body bytes**, not network interface or billing usage. Upstream counts cover the gateway data-plane requests forwarded through the repository `HTTPUpstream` transport, not all outbound HTTP calls in the process. OAuth/CodeAssist credential refresh and background workers are separate control-plane/job traffic and are excluded unless explicitly instrumented. HTTP headers, chunk/frame overhead, TCP/IP and TLS overhead are excluded. No token-to-byte conversion, `Content-Length` estimate, or re-serialized response length is substituted for a measured stream.

| Side and direction | Boundary |
| --- | --- |
| `downstream.request` | Client request body, before gateway normalization or protocol conversion |
| `upstream.request` | Body consumed by the upstream HTTP transport, after gateway conversion |
| `upstream.response` | Upstream response body, before gateway response conversion |
| `downstream.response` | Body accepted by the client-facing response writer, after gateway conversion |

Each body has `actual_bytes`, `original_bytes`, and `compressed`. Actual means the encoded body at that boundary; original means its uncompressed representation at the **same** boundary. The four directions can legitimately differ after protocol or content conversion. Upstream values accumulate all observed HTTP attempts, including retries and failover, rather than just the successful attempt. Request-body compression probes are the exception: a coded attempt that is rejected or discarded while negotiating a `Content-Encoding` is measured in its own collector, logged with its sent bytes, and excluded from the request's usage record, which then describes only the attempt returned to the client.

Compression savings are `(1 - actual_bytes / original_bytes) * 100%`. A negative result means encoding enlarged the body. The rate is unknown when the original size is zero or either value is unavailable. `compressed` records encoding, not whether the encoded body was smaller. This matters for small gzip bodies and retries combining encoded and identity responses.

A missing observation is `null`, never an invented zero. A completely observed empty body can be zero. Historical usage rows are not backfilled. Unknown encodings, failed outgoing writes, unread bodies and paths without instrumentation can leave individual fields unknown. The gateway does not drain otherwise unread request or response bodies just to obtain a statistic.

An upstream response retains the **bytes actually read**, even when an SSE terminal event (`[DONE]`, `response.completed`, etc.) causes the gateway to close it before HTTP EOF, or a later read fails. This is consumed traffic, not the advertised size of the entire intended response. For identity encoding, the same consumed prefix gives both original and actual bytes. For an encoded response, actual bytes remain available, but original bytes and the compression ratio stay unknown unless decoding completes: encoded/decoded read-ahead can otherwise compare different prefixes. Outgoing request readers still require the existing write-confirmation/error rules; reading a request buffer does not prove it was sent.

## Collection and finalization

The request-scoped collector is attached to the request context. Request decoding measures the stream before and after decompression; upstream instrumentation operates at the HTTP round-trip boundary, including internal retries. Client response collection wraps the response compressor so actual bytes include the compressor's final trailer. `compressResponseWriter.Size()` remains a logical uncompressed size used by failover and is **not** used as an actual-byte counter.

The collector finalizes after response-compressor close. Finalization callbacks receive independent immutable snapshots and run outside the collector lock. Usage logging can wait for this snapshot without delaying billing or retaining a pooled Gin context. Asynchronous callers preserve the collector in a detached context and freeze request metadata before leaving the handler. If billing finishes before finalization, the finalized log is dispatched through the same bounded usage worker pool instead of putting normal database latency on the HTTP finalizer. Already-finalized requests keep the original worker path. Unlike optional telemetry, a log for an already charged request is mandatory: a full, stopped or absent pool uses synchronous persistence fallback rather than dropping it. That exceptional fallback retains the existing database timeout/backpressure trade-off.

Upstream decoding is lazy: receiving HTTP headers does not read or wait for the encoded body, and closing an unread response does not initialize a decoder. Supported but malformed encoded bodies fail on `Read` rather than returning raw encoded bytes with a removed `Content-Encoding` header. Unknown or stacked encodings retain their headers/body and an unknown original-byte count.

## Storage and access control

`usage_logs.traffic` is nullable JSONB. Its JSON shape is:

```json
{
  "upstream": {
    "request": {"actual_bytes": 120, "original_bytes": 120, "compressed": false},
    "response": {"actual_bytes": 80, "original_bytes": 400, "compressed": true}
  },
  "downstream": {
    "request": {"actual_bytes": 100, "original_bytes": 100, "compressed": false},
    "response": {"actual_bytes": 90, "original_bytes": 360, "compressed": true}
  }
}
```

The regular-user response DTO has only a downstream member. Upstream fields are not merely hidden in the browser: they cannot be serialized by that DTO. Administrator responses expose both sides. The default user-facing traffic component also projects only downstream data defensively.

Operational totals use a separate `ops_traffic_events` request ledger, not a sum of usage rows or a union of usage and error logs. A request is recorded once independently of whether it generated a billable usage record, an error record, both, or neither. This includes terminal errors and partial streams without double-counting them. Retry bodies belong to the same request event. Event identifiers are server-generated rather than deduplicated using a caller-supplied request ID.

The ledger contains counters and routing dimensions, not request or response content. Request-level platform/group attribution follows the resolved gateway request; it is not a physical per-attempt provider traffic report. Event time is the collector's completion time after compressor close, not the later database-flush time.

Ledger ingestion uses one lazy worker, a 1,024-event queue, batches of up to 64 and a 25 ms batch window. Enqueue is non-blocking: a saturated queue rejects the observation and increments the failure counter rather than delaying gateway responses. Database batches receive two attempts with three-second timeouts and reuse the same event IDs. Shutdown drains for up to five seconds before cancellation. The queue is not a durable spool: a process crash, persistent database failure, or queue saturation can lose observations. Failed writes are logged and the dashboard exposes `ingestion.pending`, `ingestion.failed` and `ingestion.flushed` with `scope: "instance_lifetime"`. These are process-local health counters, not filtered request counts or cluster-wide guarantees; they reset on restart. Measurement coverage is relative to persisted rows and excludes pending/lost rows. Raw ledger retention follows the existing Ops error-log retention setting.

## Dashboard and usage display

The dashboard uses its existing time range, platform, group, refresh, and raw/auto/preaggregated query controls. Its two compact upstream/downstream cards share the overview grid and output-TPS card sizing. Each card shows the request body on the upper half and the response body on the lower half. The headline value is that direction's actual (encoded) size, its compression savings sit beside it rather than in a separate card, and the grey label after the direction name carries the equivalent rate of that same actual traffic. Hovering directly over each byte total, including its unit, reveals that direction's original traffic with its own equivalent rate and coverage, without a separate information icon. Scope, formulas and detailed coverage remain in help tooltips rather than permanent explanatory paragraphs; missing coverage and ingestion problems retain compact visible indicators.

Windows aggregated before the directional split keep side totals but report no split. Such a card falls back to one combined row labelled as the actual total, with the side-level ratio and an explicit no-directional-split badge, instead of inventing a request/response split or hiding the totals.

Original and actual totals have independent sample coverage. The compression rate uses only the same paired sample population; it is not computed by dividing independently covered totals. Side-total coverage is measured against two potential body observations per ledger request. The ledger request count is separate from the dashboard's usage/error-derived request count.

The overview exposes `traffic.upstream.request`, `traffic.upstream.response`, `traffic.downstream.request` and `traffic.downstream.response`. Each available direction contains `original_bytes`, `actual_bytes`, `original_sample_count`, `actual_sample_count` and `compression_percent`. The directional ratio covers only that direction's bodies with both byte values known; it is null when the paired original total is zero, when no body was paired, or when the window includes hourly buckets aggregated before directional pairing was recorded. A window mixing such buckets never derives the ratio from the newer buckets alone. Its coverage denominator is `traffic.request_count`, not twice that count. These are independently aggregated measurements from the same filtered population, never an estimated split of side totals. Missing byte observations remain null and measured empty bodies remain zero.

The usage table column key is `traffic`. Administrators see upstream request/response on the first line and downstream request/response on the second. Users see client upload and client download. Both lines share one column grid, so every direction icon, byte value and compression rate owns its own track: byte values are right-aligned across the lines, and a line without a rate leaves its empty track in place instead of shifting the remaining columns. Labeled direction icons replace the request/response text; input tokens and requests point upward, while output tokens and responses point downward. Colors, labels and bound values are unchanged. Actual bytes and an available compression rate appear side by side. An information icon after the rate reveals original bytes on hover, rather than showing a long inline parenthesis. The rate appears only when that body's `compressed` flag is true and both byte counts permit calculation; an observed actual count with an unknown original remains visible without an invented rate. Missing data renders as `—`; measured zero renders as `0 B`. Units use binary multiples (`KiB`, `MiB`, `GiB`, etc.).

## Compatibility and deployment

Apply the additive database migrations before serving the new backend:

- `246_add_usage_log_traffic.sql`: nullable usage traffic; no default or historical backfill.
- `247_ops_metrics_traffic.sql`: mergeable hourly traffic aggregates.
- `248_add_ops_traffic_events.sql`: independent request traffic ledger.
- `249_add_account_compress_request.sql`: per-account upstream request compression opt-in, disabled by default.

Old hourly buckets do not silently become zero-traffic buckets. Automatic mode falls back to raw observations where preaggregated traffic is unavailable; forced preaggregation reports the existing not-ready result until the required buckets are refreshed. Older data cannot be retroactively measured.

Request/response breakdowns extend the existing hourly JSONB format and require no additional DDL migration. A nonempty old bucket can have valid side totals without directional fields. A window that includes such a bucket retains its totals and paired compression rates, but reports directional breakdowns as null rather than showing only the new head/tail's partial split as the whole window. The existing hourly refresh supplies the new fields when it recomputes a bucket. Raw queries can expose breakdowns for retained ledger observations; missing or expired raw history is not reconstructed by dividing cached totals.

WebSocket frames are outside the HTTP entity-body contract and are not represented as HTTP traffic. For an HTTP client request forwarded over an upstream WebSocket, the client-facing HTTP traffic remains measurable while upstream traffic is unknown. A mixed WebSocket/HTTP fallback also keeps upstream totals unknown rather than presenting only its HTTP attempts as the complete interaction.

Background image jobs do not inherit the submitting request's collector. The submission's HTTP 202 response and the later background execution are different lifetimes; attributing background bytes to that response would depend on scheduling. Submission, polling and client downloads remain measurable when they use the instrumented gateway routes, while background image/batch workers' upstream traffic and usage rows are unknown unless those executions have their own instrumentation.

This is not a process-wide network traffic meter. OAuth/token-refresh/control-plane clients, management APIs, static assets and CORS preflight handled before the gateway middleware are excluded. Some multipart/audio/live/batch/web-search endpoints use their own request-body readers rather than the common decoder: encoded actual bytes can be observed, but original bytes remain unknown unless that representation was decoded at the instrumented boundary. Pre-encoded downstream passthrough, unsupported or stacked encodings and transparently decoded third-party transports can also leave one byte dimension unknown. Panic recovery outside the collector can append an error body after finalization, so that response is marked unknown. These paths do not infer identity encoding or zero traffic.

Operational retention and collection availability also bound the observable history; a partial-coverage badge is not a promise that historical traffic can be reconstructed.
