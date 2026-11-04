# ChatGPT web image VM runner

The web image client follows `/backend-api/f/conversation`. When Sentinel requires
a VM challenge, configure a local runner using an argv JSON array:

```sh
export SUB2API_CHATGPT_WEB_TURNSTILE_COMMAND='["node","/absolute/path/to/sub2api/backend/scripts/chatgpt-web-turnstile.cjs"]'
```

The included runner requires Node.js and `jsdom`. It resolves `jsdom` from the
frontend workspace or the current working directory. Installing the frontend
development dependencies provides it in the local development environment.
Production Docker images include Node.js, the runner and an isolated, lockfile-pinned
jsdom production dependency tree. The image sets the runner command automatically;
no Compose setting is required. An explicit environment override still takes precedence.
Node runs only when web-image receives a required VM challenge and exits after each
invocation. OAuth and Native-Image do not invoke this runner.

The backend passes the challenge, requirements key, user agent, script URLs and
build identifier through stdin. It never passes account credentials to the
runner. The runner returns a JSON object containing `token` on stdout; the
backend forwards it in Sentinel finalize and the subsequent generation request.
Do not log stdin or stdout. Each invocation has a 15-second timeout.

The runner implements the inspected client's instruction VM in a JSDOM
environment. It does not open or control a logged-in browser. This compatibility
implementation is not a complete browser; upstream VM changes or browser-only
probes may require updating it. Failed or missing proofs produce an explicit
error instead of silently continuing without a required challenge.

Local verification on 2026-10-15 completed Sentinel, conversation prepare,
`/f/conversation`, asynchronous conversation polling and PNG download. This is
separate from the existing Codex Responses image route.

For real-chain diagnostics, `CGPT_WEB_E2E_TRACE=1` reports whether the upstream
requires the VM and whether its payload exists, without logging payloads or
proofs. The Sentinel E2E test also reports whether a proof was produced.
A newly accepted image conversation may briefly return 404 while becoming
readable. The web-image poller waits up to 30 seconds for initial visibility on
the same conversation ID; it never resubmits generation. Authentication errors,
challenges, persistent 404s, and disappearance after a successful read still
fail explicitly.

Successful conversation reads without an image back off to at most one read per
10 seconds. A polling 429 with a usable `Retry-After` is retried on the same
conversation, at most three times and within the existing poll deadline. A 429
without a usable delay, a delay outside the budget, or a challenge is returned
explicitly rather than silently regenerating an image.
