# Native Codex Images HTTP status and proxy timeouts

Non-streaming `/v1/images/generations` and `/v1/images/edits` requests carrying
`X-Codex-Image-Turn-Id` wait for the final response without whitespace heartbeats.
This preserves the actual HTTP error status. Otherwise an early heartbeat commits
HTTP 200, and Codex can mistake a delayed error for an incomplete image response
(for example, a missing `created` field).

Other clients retain the configured `gateway.image_nonstream_keepalive_interval`
behavior. Streaming Images responses are unaffected. The header is a client
capability signal; it does not authenticate a request or grant image permissions.

## Timeout budget

A proxy in front of Sub2API must allow the entire image operation, including queue
waits, account selection and upstream processing, before the first response byte.
The proxy's upstream response-header deadline and idle-read timeout must exceed
that budget. A larger HTTP client total timeout alone does not override a short
proxy deadline. Configure the client, every intervening proxy and the upstream
HTTP transport consistently; use a finite deadline appropriate for the deployment.
For example, an nginx deployment with a 900-second image budget needs
`proxy_read_timeout 900s` or an appropriately larger value for scheduling overhead.
Check any outer CDN/load-balancer timeout as well: changing nginx does not change
that outer limit.

Disabling heartbeats does not shorten image generation. A proxy timeout can occur
while the upstream is still generating and may still incur cost. Diagnose the
original request before retrying; a timeout is not proof that generation never
ran. This status-handling fix does not resolve upstream quota or permission errors.

## Regression coverage

The handler tests register both actual routes, parse JSON generation and multipart
edit requests, select an account and forward over HTTP to a delayed local upstream.
A local reverse proxy with a sufficient header deadline receives the final HTTP
400 error or successful image JSON after the usual heartbeat interval. A proxy
with a shorter header deadline times out for native requests; the same proxy
still receives heartbeats for ordinary clients. These are controlled fixtures,
not successful paid generation claims or a test of a particular deployed CDN.
