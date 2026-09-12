# Optional OpenAI API-key availability policy

This policy lets administrators favor another account after a recoverable upstream
failure. It reuses the existing scheduler, account/model transient cooldown state,
failure classification and request-local failover limits.

The default is **disabled**. It applies to OpenAI API-key accounts with pool mode
disabled, through the native and passthrough HTTP Responses paths. OAuth accounts,
pool-mode same-account retries and other platforms keep their existing policies.
This is not a new WebSocket failover implementation.

## Enable or disable

In `config.yaml`:

```yaml
gateway:
  openai_apikey_availability_enabled: true
```

Alternatively, set `GATEWAY_OPENAI_APIKEY_AVAILABILITY_ENABLED=true` in the process
environment. The supplied Compose files pass this variable through from `.env`;
recreate the application container after changing it. Set it to `false` and restart
the application to return to the default policy. Existing persisted account errors,
quota limits and administrator-defined cooldowns still apply.

## Failure handling

| Failure | Additional behavior when enabled |
| --- | --- |
| HTTP 500, 502, 503, 504, 520–524 | Start the existing account/model cooldown on the first failure and skip same-account retries. |
| Recognized transient `error` / `response.failed` SSE event inside HTTP 200 | Apply the same cooldown and propose next-account failover before semantic output. |
| Upstream connection/read failure or incomplete stream | Cool the selected account/model; propose failover only while replay is safe. |
| Existing configured first-output or native stream-idle timeout | Reuse the configured timeout and administrator timeout policy, then apply the availability cooldown. |
| HTTP/semantic 429 | Use existing rate-limit handling; respect `Retry-After` while preserving longer quota reset times, and skip same-account retries. |
| Structured 401 / recognized credential or access-state failure | Reuse existing credential/account handling; passthrough can propose next-account failover. Generic 403 permission errors are not newly classified as credential failures. |

The transient cooldown uses the existing progression: 10 seconds on the first and
second failure, then 45 seconds from the third failure. Existing success/reset and
expiry handling remain in charge of recovery. Cooldown expiry returns the account
to normal scheduling; it does not run an active health probe or a half-open trial.
This account/model transient state is **process-local**, not a cluster-wide circuit
breaker. Persisted credential and quota states continue to use their existing paths.

Recognized request, context-window and policy rejections do not become account
failures under this policy. Local response-size limits and client cancellation do
not trigger the added transport cooldown. An `error` followed by `response.failed`
counts once for the added transient cooldown.

## Replay and latency boundaries

The existing handler decides whether another account may be tried and enforces its
switch limit. Before semantic output, recoverable failures can move to the next
eligible account within the same client request. After actual text or tool output
has been committed, the answer is not replayed: the error is returned and cooldown
protects later requests. If all eligible accounts fail, the client still receives
an error; this option cannot create healthy upstream capacity.

The option adds no fixed delay, active probing, background request, synthetic
"processing" text, or whole-answer buffering. It does not change timeout defaults
or promise a five-second first token. Retry attempts can consume upstream quota
and extend total wait time, so existing timeout and switch limits still matter.

## Verification

The regression suite covers HTTP/SSE transient status codes, credential and quota
errors, transport/read failures, cancellation, response-size limits, native and
passthrough forwarding, model mapping, disabled/OAuth/pool-mode compatibility,
cooldown recovery, and replay prevention after output.

`TestOpenAIAPIKeyAvailabilityFirstTextBeforeCompletion` holds the upstream terminal
event until actual text has been flushed downstream. It verifies that enabling the
policy does not require a complete answer before the first text is delivered.

```sh
cd backend
go test -tags=unit ./...
go test -tags=integration ./...
golangci-lint run ./...
go test ./internal/service -run '^$' \
  -bench 'BenchmarkOpenAIAPIKeyAvailability(FirstText|ConcurrentStream)' \
  -benchmem -benchtime=2000x -count=5
```

The benchmarks compare enabled/disabled parser-to-Flush latency and concurrent
stream processing using an immediately readable synthetic upstream. Their numbers
exclude upstream generation, network, scheduler queueing and client rendering;
they must not be presented as end-to-end client first-token latency.
